package kubernetes

import (
	"context"

	"go.uber.org/zap"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"praas/pkg/logging"
)

type k8Key struct{}

func Get(ctx context.Context) kubernetes.Interface {
	untyped := ctx.Value(k8Key{})
	if untyped == nil {
		logging.FromContext(ctx).Panic("Could not get kubernetes client from context")
	}
	return untyped.(kubernetes.Interface)
}

func WithK8Client(ctx context.Context) context.Context {
	logger := logging.FromContext(ctx)
	config, err := rest.InClusterConfig()
	if err != nil {
		logger.Fatalw("Could not get kubernetes InClusterConfig", zap.Error(err))
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		logger.Fatalw("Could not get kubernetes clientset from config", zap.Error(err))
	}

	return context.WithValue(ctx, k8Key{}, clientset)
}
