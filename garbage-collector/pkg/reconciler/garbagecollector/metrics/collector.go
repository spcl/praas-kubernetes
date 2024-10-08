package metrics

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/clock"
	corev1listers "k8s.io/client-go/listers/core/v1"
	filteredpodinformer "knative.dev/pkg/client/injection/kube/informers/core/v1/pod/filtered"
	"knative.dev/pkg/logging"
	"knative.dev/pkg/logging/logkey"
	"knative.dev/serving/pkg/apis/autoscaling"
	autoscalingv1alpha1 "knative.dev/serving/pkg/apis/autoscaling/v1alpha1"
	"knative.dev/serving/pkg/apis/serving"
	"praas/pkg/resources"
)

const (
	// scrapeTickInterval is the interval of time between triggering statsScraper.Scrape()
	// to get metrics across all pods of a revision.
	scrapeTickInterval = time.Second
)

var (
	// errNotCollecting denotes that the collector is not collecting metrics for the given resource.
	errNotCollecting = errors.New("no metrics are being collected for the requested resource")
)

type collectorKey struct{}

// WithCollector returns a context object that has a MetricCollector attached
func WithCollector(ctx context.Context) context.Context {
	logger := logging.FromContext(ctx)
	podLister := filteredpodinformer.Get(ctx, serving.RevisionUID).Lister()
	praasCfg := resources.GetPraasConfig(ctx)
	collector := newMetricCollector(statsScraperFactoryFunc(podLister, praasCfg), logger)

	return context.WithValue(ctx, collectorKey{}, collector)
}

func statsScraperFactoryFunc(podLister corev1listers.PodLister, cfg *resources.PraasConfig) statsScraperFactory {
	return func(metric *autoscalingv1alpha1.Metric, logger *zap.SugaredLogger) (statsScraper, error) {
		revisionName := metric.Labels[serving.RevisionLabelKey]
		if revisionName == "" {
			logger.Error("Could not find revision for ", metric.Name)
			return nil, fmt.Errorf("label %q not found or empty in Metric %s", serving.RevisionLabelKey, metric.Name)
		}

		podAccessor := resources.NewPodLister(podLister, metric.Namespace, revisionName)
		return newStatsScraper(podAccessor, logger, cfg), nil
	}
}

// GetCollector returns the MetricCollector attached to the given context
func GetCollector(ctx context.Context) *MetricCollector {
	untyped := ctx.Value(collectorKey{})
	if untyped == nil {
		logging.FromContext(ctx).Panic("Unable to fetch metrics from context.")
	}
	return untyped.(*MetricCollector)
}

// statsScraperFactory creates a statsScraper for a given Metric.
type statsScraperFactory func(*autoscalingv1alpha1.Metric, *zap.SugaredLogger) (statsScraper, error)

// FullCollector starts and stops metric collection for a given entity.
type FullCollector interface {
	// CreateOrUpdate either creates a collection for the given metric or update it, should
	// it already exist.
	CreateOrUpdate(metric *autoscalingv1alpha1.Metric) error
	// Record allows stats to be captured that came from outside the Collector.
	Record(key types.NamespacedName, now time.Time, stat []Stat)
	// Delete deletes a Metric and halts collection.
	Delete(string, string)
	// Watch registers a singleton function to call when a specific collector's status changes.
	// The passed name is the namespace/name of the metric owned by the respective collector.
	Watch(func(types.NamespacedName))
	// Alert registers a singleton function, that is called when the given filter function returns true.
	Alert(handler func(key types.NamespacedName), filter func([]Stat) bool)
}

type metricClient interface {
	LatestStats(key types.NamespacedName) ([]Stat, error)
}

// MetricCollector manages collection of metrics for many entities.
type MetricCollector struct {
	logger *zap.SugaredLogger

	statsScraperFactory statsScraperFactory
	clock               clock.Clock

	collectionsMutex sync.RWMutex
	collections      map[types.NamespacedName]*collection

	watcherMutex sync.RWMutex
	watcher      func(types.NamespacedName)

	alertMutex  sync.Mutex
	alert       func(name types.NamespacedName)
	alertFilter func(stat []Stat) bool
}

var _ FullCollector = (*MetricCollector)(nil)
var _ metricClient = (*MetricCollector)(nil)

// newMetricCollector creates a new metric collector that returns Stats for all pods.
func newMetricCollector(statsScraperFactory statsScraperFactory, logger *zap.SugaredLogger) *MetricCollector {
	return &MetricCollector{
		logger:              logger,
		collections:         make(map[types.NamespacedName]*collection),
		statsScraperFactory: statsScraperFactory,
		clock:               clock.RealClock{},
	}
}

func (c *MetricCollector) LatestStats(key types.NamespacedName) ([]Stat, error) {
	c.collectionsMutex.RLock()
	defer c.collectionsMutex.RUnlock()

	collection, exists := c.collections[key]
	if !exists {
		return nil, errNotCollecting
	}

	stats := make([]Stat, 0, len(collection.stat))
	for _, s := range collection.stat {
		stats = append(stats, s)
	}
	return stats, nil
}

// CreateOrUpdate either creates a collection for the given metric or updates it, should
// it already exist.
func (c *MetricCollector) CreateOrUpdate(metric *autoscalingv1alpha1.Metric) error {
	key := types.NamespacedName{Namespace: metric.Namespace, Name: metric.Name}
	logger := c.logger.With(zap.String(logkey.Key, key.String()))

	annotations := metric.Annotations
	class, ok := annotations[autoscaling.ClassAnnotationKey]
	if !ok || class != resources.PraasAutoScalerName {
		logger.Warn("Ignoring metric for non-praas controlled ksvc:", key)
		return nil
	}

	c.collectionsMutex.Lock()
	defer c.collectionsMutex.Unlock()

	_, exists := c.collections[key]
	if !exists {
		scraperInstance, err := c.statsScraperFactory(metric, logger)
		if err != nil {
			return err
		}
		if scraperInstance == nil {
			logger.Warn("Will not collect metrics for:", key)
			return nil
		}
		c.collections[key] = newCollection(metric, scraperInstance, c.clock, c.Inform, c.alertCallback, logger)
	}
	return nil
}

// Delete deletes a Metric and halts collection.
func (c *MetricCollector) Delete(namespace, name string) {
	c.collectionsMutex.Lock()
	defer c.collectionsMutex.Unlock()

	key := types.NamespacedName{Namespace: namespace, Name: name}
	if collection, ok := c.collections[key]; ok {
		collection.close()
		delete(c.collections, key)
	}
}

// Record records a stat that's been generated outside of the metric collector.
func (c *MetricCollector) Record(key types.NamespacedName, now time.Time, stat []Stat) {
	c.collectionsMutex.RLock()
	defer c.collectionsMutex.RUnlock()

	if collection, exists := c.collections[key]; exists {
		c.logger.Infof("collector: Record metric with key: %v", key)
		collection.record(stat, nil)
	}
}

// Watch registers a singleton function to call when collector status changes.
func (c *MetricCollector) Watch(fn func(types.NamespacedName)) {
	c.watcherMutex.Lock()
	defer c.watcherMutex.Unlock()

	if c.watcher != nil {
		c.logger.Panic("Multiple calls to Watch() not supported")
	}
	c.watcher = fn
}

// Inform sends an update to the registered watcher function, if it is set.
func (c *MetricCollector) Inform(event types.NamespacedName) {
	c.watcherMutex.RLock()
	defer c.watcherMutex.RUnlock()

	if c.watcher != nil {
		c.watcher(event)
	}
}

func (c *MetricCollector) alertCallback(stats []Stat, event types.NamespacedName) {
	c.alertMutex.Lock()
	defer c.alertMutex.Unlock()

	// Check if there is an alert registered
	if c.alert == nil || c.alertFilter == nil {
		return
	}

	// If the filter is true call alert
	if c.alertFilter(stats) {
		c.alert(event)
	}
}

func (c *MetricCollector) Alert(handler func(key types.NamespacedName), filter func([]Stat) bool) {
	c.alertMutex.Lock()
	defer c.alertMutex.Unlock()

	if c.alert != nil {
		c.logger.Error("Tried to overwrite existing alert. FullCollector supports only a single alert")
		return
	}

	c.alert = handler
	c.alertFilter = filter
}

type collection struct {
	// mux guards access to all of the collection's state.
	mux sync.RWMutex

	metric *autoscalingv1alpha1.Metric

	// Fields relevant to metric collection in general.
	stat map[string]Stat

	// Fields relevant for metric scraping specifically.
	scraper  statsScraper
	lastErr  error
	grp      sync.WaitGroup
	stopCh   chan struct{}
	callback func([]Stat, types.NamespacedName)
}

func (c *collection) updateScraper(ss statsScraper) {
	c.mux.Lock()
	defer c.mux.Unlock()
	c.scraper = ss
}

func (c *collection) getScraper() statsScraper {
	c.mux.RLock()
	defer c.mux.RUnlock()
	return c.scraper
}

// currentMetric safely returns the current metric stored in the collection.
func (c *collection) currentMetric() *autoscalingv1alpha1.Metric {
	c.mux.RLock()
	defer c.mux.RUnlock()

	return c.metric
}

// updateMetric safely updates the metric stored in the collection.
func (c *collection) updateMetric(metric *autoscalingv1alpha1.Metric) {
	c.mux.Lock()
	defer c.mux.Unlock()

	c.metric = metric
}

// close stops collecting metrics, stops the scraper.
func (c *collection) close() {
	close(c.stopCh)
	c.grp.Wait()
}

// newCollection creates a new collection, which uses the given scraper to
// collect stats every scrapeTickInterval.
func newCollection(
	metric *autoscalingv1alpha1.Metric,
	scraper statsScraper,
	clock clock.Clock,
	callback func(types.NamespacedName),
	recordCallback func(stats []Stat, event types.NamespacedName),
	logger *zap.SugaredLogger,
) *collection {
	c := &collection{
		metric:   metric,
		scraper:  scraper,
		stat:     make(map[string]Stat),
		stopCh:   make(chan struct{}),
		callback: recordCallback,
	}

	key := types.NamespacedName{Namespace: metric.Namespace, Name: metric.Name}
	logger = logger.Named("collector").With(zap.String(logkey.Key, key.String()))
	logger.Infof("collector: newCollection with key %v", key)

	c.grp.Add(1)
	go func() {
		defer c.grp.Done()

		scrapeTicker := clock.NewTicker(scrapeTickInterval)
		defer scrapeTicker.Stop()
		for {
			select {
			case <-c.stopCh:
				return
			case <-scrapeTicker.C():
				scraper := c.getScraper()
				if scraper == nil {
					// Don't scrape empty target service.
					if c.updateLastError(nil) {
						callback(key)
					}
					continue
				}

				stat, err := scraper.Scrape(c.currentMetric().Spec.StableWindow)
				if err != nil {
					logger.Errorw("Failed to scrape metrics", zap.Error(err))
				}
				if c.updateLastError(err) {
					callback(key)
				}
				if stat != nil {
					c.record(stat, logger)
				}
			}
		}
	}()

	return c
}

// updateLastError updates the last error returned from the scraper
// and returns true if the error or error state changed.
func (c *collection) updateLastError(err error) bool {
	c.mux.Lock()
	defer c.mux.Unlock()

	if errors.Is(err, c.lastErr) {
		return false
	}
	c.lastErr = err
	return true
}

func (c *collection) lastError() error {
	c.mux.RLock()
	defer c.mux.RUnlock()

	return c.lastErr
}

// record adds a stat to the current collection.
func (c *collection) record(stat []Stat, logger *zap.SugaredLogger) {
	stats := make([]Stat, 0, len(stat))

	// First save all the pods we know about
	var hasRecord struct{}
	podsOnRecord := make(map[string]struct{})
	for knownPod := range c.stat {
		podsOnRecord[knownPod] = hasRecord
	}

	// Go through the stats we just received
	for _, s := range stat {
		statEntry, exists := c.stat[s.PodIP]
		var newEntry Stat
		if !exists {
			newEntry = s
		} else {
			newEntryDuration := 0
			for _, entry := range s.Entries {
				newEntryDuration += entry.Interval
			}

			currentDuration := 0
			for _, entry := range statEntry.Entries {
				currentDuration += entry.Interval
			}

			// Remove the oldest entries if we are still in the window after they are removed and the new entry added
			didRemoveEntry := true
			entryIdx := 0
			newDuration := currentDuration + newEntryDuration
			minDuration := int(c.metric.Spec.StableWindow.Seconds())
			// logger.Infof("Min duration: %d (%v)", minDuration, c.metric)
			for didRemoveEntry && entryIdx < len(statEntry.Entries) {
				didRemoveEntry = false
				if newDuration-statEntry.Entries[entryIdx].Interval >= minDuration {
					newDuration -= statEntry.Entries[entryIdx].Interval
					didRemoveEntry = true
					entryIdx += 1
				}
			}
			statEntry.Entries = statEntry.Entries[entryIdx:]

			// Add new entries
			statEntry.Entries = append(statEntry.Entries, s.Entries...)
			newEntry = statEntry
		}
		c.stat[s.PodIP] = newEntry
		delete(podsOnRecord, s.PodIP)
		stats = append(stats, newEntry)
	}

	// Now in podsOnRecord has all the pod IPs we have received records from previously, but not in this round
	// For now we just delete those
	for podIp := range podsOnRecord {
		delete(c.stat, podIp)
	}

	c.callback(stats, types.NamespacedName{Namespace: c.metric.Namespace, Name: c.metric.Name})
}
