package scraping

import (
	"context"
	"errors"
	"fmt"
	"sync"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/clock"
	"praas/pkg/logging"
	"praas/pkg/types"
)

var (
	AppExists = errors.New("app with the given id already exists")
)

type ScraperFactoryFunc func(appID types.IDType) Scraper

type FilteredHandler interface {
	Filter([]StatEntry) bool
	Handle(context.Context, *corev1.Pod) error
	HandleError(types.ProcessRef, error)
}

type ScraperCollection interface {
	CreateCollection(appID types.IDType) error
	DeleteCollection(appID types.IDType) error
	AddFilteredHandler(handler FilteredHandler)
}

type Collector struct {
	collections   map[types.IDType]Scraper
	mux           sync.Mutex
	scrapeFactory ScraperFactoryFunc
	handler       FilteredHandler
	clock         clock.WithTicker
}

var _ ScraperCollection = (*Collector)(nil)

type scrapingKey struct{}

func Get(ctx context.Context) ScraperCollection {
	untyped := ctx.Value(scrapingKey{})
	if untyped == nil {
		logging.FromContext(ctx).Panic("Could not get scraper from context")
	}
	return untyped.(*Collector)
}

func WithCollector(ctx context.Context, factory ScraperFactoryFunc) context.Context {
	collector := newCollector(factory)
	return context.WithValue(ctx, scrapingKey{}, collector)
}

func newCollector(scrapeFactory ScraperFactoryFunc) *Collector {
	return &Collector{
		collections:   make(map[types.IDType]Scraper),
		scrapeFactory: scrapeFactory,
		clock:         clock.RealClock{},
	}
}

func (c *Collector) CreateCollection(appID types.IDType) error {
	c.mux.Lock()
	defer c.mux.Unlock()

	// Make sure process does not exist yet
	_, exists := c.collections[appID]
	if exists {
		return AppExists
	}

	scraper := c.scrapeFactory(appID)
	scraper.SetHandler(c.handler)
	scraper.StartScraping(c.clock)

	c.collections[appID] = scraper
	return nil
}

func (c *Collector) DeleteCollection(appID types.IDType) error {
	c.mux.Lock()
	defer c.mux.Unlock()

	// Make sure process exists
	scraper, exists := c.collections[appID]
	if !exists {
		return fmt.Errorf("cannot delete collection, app with id %d does not exist", appID)
	}

	scraper.StopScraping()
	delete(c.collections, appID)

	return nil
}

func (c *Collector) AddFilteredHandler(handler FilteredHandler) {
	c.mux.Lock()
	defer c.mux.Unlock()

	c.handler = handler

	// Update existing scrapers
	for _, scraper := range c.collections {
		scraper.SetHandler(handler)
	}
}
