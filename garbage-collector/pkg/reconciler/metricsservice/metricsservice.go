package metricsservice

import (
	"context"
	"errors"
	"time"

	"k8s.io/apimachinery/pkg/types"
	"knative.dev/pkg/logging"
	pkgreconciler "knative.dev/pkg/reconciler"
	autoscalingv1alpha1 "knative.dev/serving/pkg/apis/autoscaling/v1alpha1"
	metricreconciler "knative.dev/serving/pkg/client/injection/reconciler/autoscaling/v1alpha1/metric"
	"praas/pkg/reconciler/garbagecollector/metrics"
)

// reconciler implements controller.Reconciler for Metric resources.
type reconciler struct {
	collector metrics.FullCollector
}

// Check that our reconciler implements the necessary interfaces.
var (
	_ metricreconciler.Interface        = (*reconciler)(nil)
	_ pkgreconciler.OnDeletionInterface = (*reconciler)(nil)
)

// ReconcileKind implements Interface.ReconcileKind.
func (r *reconciler) ReconcileKind(ctx context.Context, metric *autoscalingv1alpha1.Metric) pkgreconciler.Event {
	logger := logging.FromContext(ctx)
	logger.Infof(
		"metric ReconcileKind: Received metric: %v %v %v",
		metric.Name,
		metric.Spec.ScrapeTarget,
		metric.Spec.StableWindow,
	)
	metric.Spec.StableWindow = 30 * time.Second
	if err := r.collector.CreateOrUpdate(metric); err != nil {
		logger.Errorw("Failed to create or update metric", metric, err)
		switch {
		case errors.Is(err, metrics.ErrFailedGetEndpoints):
			metric.Status.MarkMetricNotReady("NoEndpoints", err.Error())
		case errors.Is(err, metrics.ErrDidNotReceiveStat):
			metric.Status.MarkMetricFailed("DidNotReceiveStat", err.Error())
		default:
			metric.Status.MarkMetricFailed(
				"CollectionFailed",
				"Failed to reconcile metric collection: "+err.Error(),
			)
		}

		// We don't return an error because retrying is of no use. We'll be poked by collector on a change.
		return nil
	}

	metric.Status.MarkMetricReady()
	return nil
}

func (r *reconciler) ObserveDeletion(ctx context.Context, key types.NamespacedName) error {
	r.collector.Delete(key.Namespace, key.Name)
	return nil
}
