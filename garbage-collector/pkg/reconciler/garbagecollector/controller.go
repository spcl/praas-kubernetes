package garbagecollector

import (
	"context"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/cache"
	networkingclient "knative.dev/networking/pkg/client/injection/client"
	sksinformer "knative.dev/networking/pkg/client/injection/informers/networking/v1alpha1/serverlessservice"
	kubeclient "knative.dev/pkg/client/injection/kube/client"
	filteredpodinformer "knative.dev/pkg/client/injection/kube/informers/core/v1/pod/filtered"
	"knative.dev/pkg/configmap"
	"knative.dev/pkg/controller"
	"knative.dev/pkg/logging"
	pkgreconciler "knative.dev/pkg/reconciler"
	"knative.dev/serving/pkg/apis/autoscaling"
	autoscalingv1alpha1 "knative.dev/serving/pkg/apis/autoscaling/v1alpha1"
	"knative.dev/serving/pkg/apis/serving"
	servingclient "knative.dev/serving/pkg/client/injection/client"
	metricinformer "knative.dev/serving/pkg/client/injection/informers/autoscaling/v1alpha1/metric"
	painformer "knative.dev/serving/pkg/client/injection/informers/autoscaling/v1alpha1/podautoscaler"
	pareconciler "knative.dev/serving/pkg/client/injection/reconciler/autoscaling/v1alpha1/podautoscaler"
	"praas/pkg/reconciler/garbagecollector/metrics"
	"praas/pkg/resources"
	"praas/pkg/scaling"
	"praas/pkg/store"
)

// NewController creates a new garbage collector controller instance
func NewController(
	ctx context.Context,
	_ configmap.Watcher,
) *controller.Impl {
	logger := logging.FromContext(ctx)
	logger.Info("Post new PPA controller")

	paInformer := painformer.Get(ctx)
	sksInformer := sksinformer.Get(ctx)
	kubeClient := kubeclient.Get(ctx).CoreV1()
	metricInformer := metricinformer.Get(ctx)

	podsInformer := filteredpodinformer.Get(ctx, serving.RevisionUID)
	podLister := podsInformer.Lister()
	collector := metrics.GetCollector(ctx)
	stateStore := store.GetStateStore(ctx)

	c := &reconciler{
		Client:           servingclient.Get(ctx),
		NetworkingClient: networkingclient.Get(ctx),
		SKSLister:        sksInformer.Lister(),
		MetricLister:     metricInformer.Lister(),
		podsLister:       podLister,
		collector:        collector,
		kubeClient:       kubeClient,
		store:            stateStore,
		cfg:              resources.GetPraasConfig(ctx),
	}

	onlyPPAClass := pkgreconciler.AnnotationFilterFunc(
		autoscaling.ClassAnnotationKey,
		resources.PraasAutoScalerName,
		false,
	)
	impl := pareconciler.NewImpl(ctx, c, resources.PraasAutoScalerName)
	c.scaler = scaling.GetScaler(ctx)

	// And here we should set up some event handlers
	logger.Info("Setting up PPA Class event handlers")

	paInformer.Informer().AddEventHandler(
		cache.FilteringResourceEventHandler{
			FilterFunc: onlyPPAClass,
			// Handler:    controller.HandleAll(impl.Enqueue),
			Handler: controller.HandleAll(
				func(obj interface{}) {
					// logger.Infof("pa Informer called the handler")
					impl.Enqueue(obj)
				},
			),
		},
	)

	onlyPAControlled := controller.FilterController(&autoscalingv1alpha1.PodAutoscaler{})
	handleMatchingControllers := cache.FilteringResourceEventHandler{
		FilterFunc: pkgreconciler.ChainFilterFuncs(onlyPPAClass, onlyPAControlled),
		Handler: controller.HandleAll(
			func(obj interface{}) {
				// logger.Info("SKS event")
				impl.EnqueueControllerOf(obj)
			},
		),
	}
	sksInformer.Informer().AddEventHandler(handleMatchingControllers)

	// Watch the knative pods.
	handlerFunc := impl.EnqueueLabelOfNamespaceScopedResource("", serving.RevisionLabelKey)
	podsInformer.Informer().AddEventHandler(
		cache.FilteringResourceEventHandler{
			FilterFunc: pkgreconciler.LabelExistsFilterFunc(serving.RevisionLabelKey),
			Handler: controller.HandleAll(
				func(obj interface{}) {
					queueLen := impl.WorkQueue().Len()
					// Only handle this if there are no other work items
					if queueLen == 0 {
						// logger.Infof("podsInformer event")
						handlerFunc(obj)
					} else {
						logger.Debug("There are items in the work queue podsInformer was ignored")
					}
				},
			),
		},
	)

	// Enqueue PA if at least one of the pods reported that they can be deleted
	cfg := resources.GetPraasConfig(ctx)
	collector.Alert(
		func(key types.NamespacedName) {
			logger.Debug("Alert enqueued autoscaler")
			impl.EnqueueKey(key)
		}, func(stats []metrics.Stat) bool {
			// logger.Info("Check stats: ", stats)
			return len(findDeletablePods(stats, cfg)) > 0
		},
	)

	return impl
}
