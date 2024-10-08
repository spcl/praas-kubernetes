package metrics

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"go.uber.org/atomic"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
	"praas/pkg/resources"
)

const (
	httpClientTimeout = 3 * time.Second
)

var (
	// ErrFailedGetEndpoints specifies the error returned by scraper when it fails to
	// get endpoints.
	ErrFailedGetEndpoints = errors.New("failed to get endpoints")

	// ErrDidNotReceiveStat specifies the error returned by scraper when it does not receive
	// stat from an unscraped pod
	ErrDidNotReceiveStat = errors.New("did not receive stat from an unscraped pod")

	// Sentinel error to return from pod scraping routine, when all pods fail
	// with a 503 error code, indicating (most likely), that mesh is enabled.
	errDirectScrapingNotAvailable = errors.New("all pod scrapes returned 503 error")
	errPodsExhausted              = errors.New("pods exhausted")
)

type statsScraper interface {
	// Scrape scrapes the Revision endpoint. The duration is used
	// to cutoff young pods, whose stats might skew lower.
	Scrape(time.Duration) ([]Stat, error)
}

type scraper struct {
	directClient *httpScrapeClient
	podLister    resources.PodLister
	logger       *zap.SugaredLogger
	endpoint     string
}

// keepAliveTransport is a http.Transport with the default settings, but with
// keepAlive upped to allow 1000 connections.
var keepAliveTransport = func() *http.Transport {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.DisableKeepAlives = false // default, but for clarity.
	t.MaxIdleConns = 1000
	return t
}()

// client is a normal http client with HTTP Keep-Alive enabled.
// This client is used in the direct pod scraping where we want
// to take advantage of HTTP Keep-Alive to avoid connection creation overhead
// between scrapes of the same pod.
var client = &http.Client{
	Timeout:   httpClientTimeout,
	Transport: keepAliveTransport,
}

// newStatsScraper creates a new statsScraper for the Revision which
// the given Metric is responsible for.
func newStatsScraper(
	podLister resources.PodLister,
	logger *zap.SugaredLogger,
	cfg *resources.PraasConfig,
) statsScraper {
	directClient := newHTTPScrapeClient(client)
	endpoint := cfg.ContainerMetricsEndpoint
	path := ":" + cfg.ContainerPort
	if endpoint[0] != '/' {
		path += "/"
	}
	path += endpoint

	return &scraper{
		directClient: directClient,
		endpoint:     path,
		podLister:    podLister,
		logger:       logger,
	}
}

// Scrape calls the destination service then sends it
// to the given stats channel.
func (s *scraper) Scrape(_ time.Duration) (stat []Stat, err error) {
	return s.scrapePods()
}

func (s *scraper) scrapePods() ([]Stat, error) {
	pods, youngPods, err := s.podLister.PodsSplitByReady()
	if err != nil {
		s.logger.Infow("Error querying pods by age", zap.Error(err))
		return nil, err
	}
	lp := len(pods)
	lyp := len(youngPods)
	// s.logger.Debugf("|OldPods| = %d, |YoungPods| = %d", lp, lyp)
	total := lp + lyp
	if total == 0 {
		return nil, nil
	}

	if lp == 0 && lyp > 0 {
		s.logger.Warnf("Did not scrape, waiting for %d young pods", lyp)
	}

	resultArr := make([]Stat, lp)

	grp, egCtx := errgroup.WithContext(context.Background())
	idx := atomic.NewInt32(-1)
	succesCount := atomic.NewInt32(0)
	// Start |lp| threads to scan in parallel.
	for i := 0; i < lp; i++ {
		grp.Go(
			func() error {
				// If a given pod failed to scrape, we want to continue
				// scanning pods down the line.
				for {
					// Acquire next pod.
					myIdx := int(idx.Inc())
					// All out?
					if myIdx >= lp {
						return errPodsExhausted
					}
					pod := pods[myIdx]

					// Scrape!
					ip := pod.Status.PodIP
					target := "http://" + ip + s.endpoint
					req, err := http.NewRequestWithContext(egCtx, http.MethodGet, target, nil)
					if err != nil {
						return err
					}

					statEntry, err := s.directClient.Do(req)
					if err == nil {
						resultArr[myIdx] = Stat{
							Entries: []StatEntry{statEntry},
							PodName: pod.Name,
							PodIP:   ip,
						}
						succesCount.Inc()
						return nil
					}

					s.logger.Infow("Failed scraping pod "+pod.Name, zap.Error(err))
				}
			},
		)
	}

	err = grp.Wait()

	// We only get here if one of the scrapers failed to scrape
	// at least one pod.
	if err != nil {
		// Got some (but not enough) successful pods.
		if succesCount.Load() > 0 {
			s.logger.Warnf("Too many pods failed scraping for meaningful interpolation error: %v", err)
			return nil, errPodsExhausted
		}
		// No pods, and we only saw mesh-related errors, so infer that mesh must be
		// enabled and fall back to service scraping.
		s.logger.Warn("0 pods were successfully scraped out of ", strconv.Itoa(len(pods)))
		return nil, errDirectScrapingNotAvailable
	}

	return resultArr, nil
}
