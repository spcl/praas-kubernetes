package metricsservice

import (
	"context"

	"knative.dev/pkg/configmap"
	"knative.dev/pkg/controller"
	"knative.dev/pkg/logging"
	metricinformer "knative.dev/serving/pkg/client/injection/informers/autoscaling/v1alpha1/metric"
	metricreconciler "knative.dev/serving/pkg/client/injection/reconciler/autoscaling/v1alpha1/metric"
	"praas/pkg/reconciler/garbagecollector/metrics"
)

func NewMetricController(
	ctx context.Context,
	_ configmap.Watcher,
) *controller.Impl {
	logger := logging.FromContext(ctx)
	metricInformer := metricinformer.Get(ctx)
	collector := metrics.GetCollector(ctx)

	logger.Info("New metric controller")

	c := &reconciler{
		collector: collector,
	}
	impl := metricreconciler.NewImpl(ctx, c)

	// Watch all the Metric objects.
	metricInformer.Informer().AddEventHandler(controller.HandleAll(impl.Enqueue))

	collector.Watch(impl.EnqueueKey)

	return impl
}
