package scraping

import (
	"context"
	"errors"
	"sync"
	"time"

	"go.uber.org/atomic"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	listers "k8s.io/client-go/listers/core/v1"
	"k8s.io/utils/clock"
	"praas/pkg/config"
	"praas/pkg/networking"
	"praas/pkg/types"
)

type Scraper interface {
	StartScraping(clock clock.WithTicker)
	StopScraping()
	SetHandler(handler FilteredHandler)
}

const (
	tickerInterval = 2 * time.Second
)

var (
	PodNotReadyError   = errors.New("pod is not ready yet")
	NoSuchProcessError = errors.New("process does not exist")
)

type scraper struct {
	stopChan  chan struct{}
	waitChan  chan bool
	handler   FilteredHandler
	cfg       *config.PraasConfig
	client    *networking.HTTPClient
	logger    *zap.SugaredLogger
	appID     types.IDType
	selector  labels.Selector
	entries   sync.Map
	podLister listers.PodNamespaceLister
}

var _ Scraper = (*scraper)(nil)

func NewScraper(
	cfg *config.PraasConfig,
	logger *zap.SugaredLogger,
	client *networking.HTTPClient,
	lister listers.PodLister,
	appID types.IDType,
) Scraper {
	return &scraper{
		stopChan:  make(chan struct{}),
		waitChan:  make(chan bool),
		appID:     appID,
		cfg:       cfg,
		client:    client,
		logger:    logger,
		podLister: lister.Pods(cfg.ContainerNamespace),
		selector: labels.SelectorFromSet(
			labels.Set{
				config.AppIDLabelKey: appID.String(),
			},
		),
	}
}

func (s *scraper) StartScraping(clock clock.WithTicker) {
	go func() {
		defer s.finished()

		ticker := clock.NewTicker(tickerInterval)
		defer ticker.Stop()
		for {
			select {
			case <-s.stopChan:
				s.logger.Info("Stop scraping")
				return
			case <-ticker.C():
				s.doScrape()
			}
		}
	}()
}

func (s *scraper) StopScraping() {
	close(s.stopChan)
	<-s.waitChan
}

func (s *scraper) SetHandler(handler FilteredHandler) {
	s.handler = handler
}

func (s *scraper) doScrape() {
	scrapingStart := time.Now()
	if s.handler == nil {
		return
	}

	// Get all pods for this process
	ctx, cancel := context.WithTimeout(context.Background(), 2*tickerInterval)
	defer cancel()
	listingStart := time.Now()
	pods, err := s.podLister.List(s.selector)
	if err != nil {
		s.logger.Errorw("Failed scraping: could not get pods", zap.Error(err))
		return
	}

	// Filter pods that are not ready
	var readyPods []*v1.Pod
	for _, pod := range pods {
		if podReady(pod) && pod.Status.PodIP != "" {
			readyPods = append(readyPods, pod)
		}
	}
	listingStop := time.Now()
	s.logger.Debugf("Pod listing time: %d ms", listingStop.Sub(listingStart).Milliseconds())

	readyPodsLen := len(readyPods)
	if readyPodsLen == 0 {
		// Nothing to do
		return
	}

	// Get the set of pods we have records for
	var member struct{}
	podsSeen := make(map[string]struct{})
	s.entries.Range(
		func(key, _ any) bool {
			PID := key.(string)
			podsSeen[PID] = member
			return true
		},
	)

	// Set all pods as not scraped (yet)
	podsScraped := make([]bool, readyPodsLen)
	for i := 0; i < readyPodsLen; i++ {
		podsScraped[i] = false
	}

	reqStart := time.Now()

	grp, egCtx := errgroup.WithContext(ctx)
	idx := atomic.NewInt32(-1)
	successCount := atomic.NewInt32(0)
	for range readyPods {
		grp.Go(
			func() error {
				myIdx := int(idx.Inc())
				pod := readyPods[myIdx]
				ip := pod.Status.PodIP
				if ip == "" {
					return nil
				}

				process := types.PodToProcessRef(pod)
				if process.AppID == types.InvalidAppID {
					s.logger.Errorw("Failed to turn pod into process ref", zap.Any("podName", pod.Name))
					return nil
				}

				path := networking.CreateURI(ip, s.cfg.ContainerPort, s.cfg.ContainerMetricsEndpoint)

				var stat StatEntry
				err := s.client.DoGetWithCtx(egCtx, path, &stat)
				if err != nil {
					s.handler.HandleError(process, err)
					return nil
				}
				s.handleNewEntry(stat, process)
				successCount.Inc()
				podsScraped[myIdx] = true

				s.logger.Debugf("Stat from %s: %v", process, stat)
				value, _ := s.entries.Load(process.PID)
				entries := value.([]StatEntry)
				// If we have a handler registered, handle these stats
				if s.handler != nil && s.handler.Filter(entries) {
					// Run handler in a different routine, with a new context
					go func() {
						handleCtx, handleCancel := context.WithTimeout(context.Background(), time.Minute)
						defer handleCancel()
						err = s.handler.Handle(handleCtx, pod)
						if err != nil {
							s.handler.HandleError(process, err)
						}
					}()
				}
				return nil
			},
		)
	}

	_ = grp.Wait()
	reqStop := time.Now()
	s.logger.Debugf("Scraping requests time: %d ms", reqStop.Sub(reqStart).Milliseconds())

	success := int(successCount.Load())
	if success != readyPodsLen {
		s.logger.Warnf("Not all processes were scraped. Scraped: %d, total: %d", success, readyPodsLen)
	} else {
		s.logger.Debugf("Scraped %d processes", success)
	}

	// Get the pods we have seen before, but have not scraped now
	for i := 0; i < readyPodsLen; i++ {
		if podsScraped[i] {
			delete(podsSeen, types.PodToProcessRef(readyPods[i]).PID)
		}
	}
	for sessionID := range podsSeen {
		s.entries.Delete(sessionID)
	}

	scrapingStop := time.Now()
	s.logger.Debugf("Scraping time %d ms", scrapingStop.Sub(scrapingStart).Milliseconds())
}

func (s *scraper) finished() {
	s.waitChan <- true
}

func (s *scraper) handleNewEntry(stat StatEntry, process types.ProcessRef) {
	e, exists := s.entries.Load(process.PID)
	// If this is the first entry for this pod record the entry and return
	if !exists {
		s.entries.Store(process.PID, []StatEntry{stat})
		return
	}

	currentInterval := 0
	currentEntries := e.([]StatEntry)
	for _, entry := range currentEntries {
		currentInterval += entry.Interval
	}

	// Look for entries we could remove (so we only store metrics for the interval we need)
	idx := 0
	removed := true
	newNonAdjustedInterval := currentInterval + stat.Interval
	for removed && idx < len(currentEntries) {
		removed = false
		if newNonAdjustedInterval-currentEntries[idx].Interval >= s.cfg.MetricTime {
			newNonAdjustedInterval -= currentEntries[idx].Interval
			removed = true
			idx++
		}
	}
	s.entries.Store(process.PID, append(currentEntries[idx:], stat))
}

// podReady checks whether pod's Ready status is True.
func podReady(p *v1.Pod) bool {
	if p.DeletionTimestamp != nil {
		return false
	}

	for _, cond := range p.Status.Conditions {
		if cond.Type == v1.PodReady {
			return cond.Status == v1.ConditionTrue
		}
	}
	// No ready status, probably not even running.
	return false
}
