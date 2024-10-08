package kubernetes

import (
	"context"

	"k8s.io/client-go/informers"
	"k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/tools/cache"
	"praas/pkg/logging"
	"praas/pkg/util"
)

type pInformerFactoryKey struct{}
type podInformerKey struct{}

func WithPraaSInformerFactory(ctx context.Context) context.Context {
	factory := NewInformerFactory(ctx, util.Namespace())
	return context.WithValue(ctx, pInformerFactoryKey{}, factory)
}

func GetPraaSInformerFactory(ctx context.Context) informers.SharedInformerFactory {
	untyped := ctx.Value(pInformerFactoryKey{})
	if untyped == nil {
		logging.FromContext(ctx).Fatalw("Could not get informer factory from context")
	}
	return untyped.(informers.SharedInformerFactory)
}

func WithPodInformer(ctx context.Context) context.Context {
	kube := Get(ctx)
	factory := informers.NewSharedInformerFactory(kube, 0)
	informer := factory.Core().V1().Pods()
	go factory.Start(ctx.Done())
	if !cache.WaitForCacheSync(ctx.Done(), informer.Informer().HasSynced) {
		logging.FromContext(ctx).Fatalw("Could not sync cache for pod informer")
	}

	return context.WithValue(ctx, podInformerKey{}, informer)
}

func GetPodInformer(ctx context.Context) v1.PodInformer {
	untyped := ctx.Value(podInformerKey{})
	if untyped == nil {
		logging.FromContext(ctx).Fatalw("Could not get informer factory from context")
	}
	return untyped.(v1.PodInformer)
}

func InformerAddEventHandler(
	ctx context.Context,
	factory informers.SharedInformerFactory,
	informer cache.SharedIndexInformer,
	handler cache.FilteringResourceEventHandler,
) {
	informer.AddEventHandler(handler)
	factory.Start(ctx.Done())
}

func NewInformerFactory(ctx context.Context, namespace string) informers.SharedInformerFactory {
	kubeClient := Get(ctx)
	return informers.NewSharedInformerFactoryWithOptions(
		kubeClient,
		0,
		informers.WithNamespace(namespace),
	)
}
