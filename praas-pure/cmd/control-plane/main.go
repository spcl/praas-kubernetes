package main

import (
	"context"
	"errors"
	"net/http"

	"go.uber.org/zap"
	"praas/pkg/config"
	"praas/pkg/control_plane/scraping"
	"praas/pkg/control_plane/store"
	"praas/pkg/kubernetes"
	"praas/pkg/logging"
	"praas/pkg/networking"
	"praas/pkg/server"
	"praas/pkg/types"
	"praas/pkg/util"
)

func main() {
	ctx := util.NewContext()

	// Setup dependencies
	ctx = logging.WithLogger(ctx)
	ctx = networking.WithHTTPClient(ctx)
	ctx = kubernetes.WithK8Client(ctx)
	ctx = kubernetes.WithPraaSInformerFactory(ctx)
	ctx = kubernetes.WithPodInformer(ctx)
	ctx = config.WithPraasConfigFromCM(ctx)
	ctx = scraping.WithCollector(ctx, scraperFactory(ctx))
	ctx = store.WithRedisStore(ctx)

	// tracking.SetupProcessTracking(ctx)

	// Setup HTTP server
	ctrlPlane := server.NewControlPlaneServer(ctx)
	err := ctrlPlane.ListenAndServe()
	if err != nil && errors.Is(err, http.ErrServerClosed) {
		logging.FromContext(ctx).Errorw("Error while running control plane server", zap.Error(err))
	} else {
		err := ctrlPlane.Shutdown(context.Background())
		if err != nil {
			logging.FromContext(ctx).Errorw("Error while shutting down control plane server", zap.Error(err))
		}
	}
}

func scraperFactory(ctx context.Context) scraping.ScraperFactoryFunc {
	logger := logging.FromContext(ctx)
	cfg := config.GetPraasConfig(ctx)
	client := networking.GetHTTPClient(ctx)
	lister := kubernetes.GetPodInformer(ctx).Lister()
	return func(pid types.IDType) scraping.Scraper {
		return scraping.NewScraper(cfg, logger, client, lister, pid)
	}
}
