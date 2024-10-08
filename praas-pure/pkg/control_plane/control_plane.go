package control_plane

import (
	"context"
	"net/url"

	"go.uber.org/zap"
	k8sclient "k8s.io/client-go/kubernetes"
	"praas/pkg/config"
	"praas/pkg/control_plane/scraping"
	"praas/pkg/control_plane/store"
	"praas/pkg/kubernetes"
	"praas/pkg/logging"
)

const (
	appIDKey     = "app-id"
	processesKey = "processes"
	PIDKey       = "pid"
)

type K8APIObjectHandler interface {
	Post(context.Context, map[string]any) (any, error)
	Delete(context.Context, map[string]any) (any, error)
	Get(ctx context.Context, query url.Values) (any, error)
}

type objectHandlerBase struct {
	logger     *zap.SugaredLogger
	store      store.StateStore
	kubeClient k8sclient.Interface
	collector  scraping.ScraperCollection
	cfg        *config.PraasConfig
}

func buildBase(ctx context.Context) objectHandlerBase {
	return objectHandlerBase{
		logger:     logging.FromContext(ctx),
		store:      store.GetStateStore(ctx),
		kubeClient: kubernetes.Get(ctx),
		collector:  scraping.Get(ctx),
		cfg:        config.GetPraasConfig(ctx),
	}
}
