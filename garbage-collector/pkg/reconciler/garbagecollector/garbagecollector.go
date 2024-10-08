package garbagecollector

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	v1 "k8s.io/client-go/kubernetes/typed/core/v1"
	corev1listers "k8s.io/client-go/listers/core/v1"
	nv1alpha1 "knative.dev/networking/pkg/apis/networking/v1alpha1"
	netclientset "knative.dev/networking/pkg/client/clientset/versioned"
	nlisters "knative.dev/networking/pkg/client/listers/networking/v1alpha1"
	"knative.dev/pkg/logging"
	"knative.dev/pkg/ptr"
	pkgreconciler "knative.dev/pkg/reconciler"
	autoscalingv1alpha1 "knative.dev/serving/pkg/apis/autoscaling/v1alpha1"
	"knative.dev/serving/pkg/apis/serving"
	clientset "knative.dev/serving/pkg/client/clientset/versioned"
	_ "knative.dev/serving/pkg/client/clientset/versioned/typed/autoscaling/v1alpha1"
	pareconciler "knative.dev/serving/pkg/client/injection/reconciler/autoscaling/v1alpha1/podautoscaler"
	listers "knative.dev/serving/pkg/client/listers/autoscaling/v1alpha1"
	"praas/pkg/reconciler/garbagecollector/metrics"
	"praas/pkg/reconciler/garbagecollector/resources"
	resourceutil "praas/pkg/resources"
	"praas/pkg/scaling"
	"praas/pkg/store"
)

// reconciler implements the control loop for the PPA resources.
type reconciler struct {
	Client           clientset.Interface
	NetworkingClient netclientset.Interface
	SKSLister        nlisters.ServerlessServiceLister
	MetricLister     listers.MetricLister

	podsLister corev1listers.PodLister
	scaler     *scaling.Scaler
	collector  *metrics.MetricCollector
	kubeClient v1.CoreV1Interface
	store      store.StateStore
	cfg        *resourceutil.PraasConfig
}

// Check that our reconciler implements pareconciler.Interface
var _ pareconciler.Interface = (*reconciler)(nil)

// ReconcileKind implements Interface.ReconcileKind.
func (c *reconciler) ReconcileKind(ctx context.Context, pa *autoscalingv1alpha1.PodAutoscaler) pkgreconciler.Event {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	logger := logging.FromContext(ctx)

	// Compare the desired and observed resources to determine our situation.
	podCounter := resourceutil.NewPodAccessor(c.podsLister, pa.Namespace, pa.Labels[serving.RevisionLabelKey])
	ready, notReady, pending, terminating, err := podCounter.PodCountsByState()
	if err != nil {
		return fmt.Errorf("error counting pods: %w", err)
	}

	if !pa.Status.IsScaleTargetInitialized() {
		pa.Status.MarkScaleTargetInitialized()
	}

	sksName := pa.Name
	sks, err := c.SKSLister.ServerlessServices(pa.Namespace).Get(sksName)
	if err != nil && !errors.IsNotFound(err) {
		logger.Warnw("Error retrieving SKS for Scaler", zap.Error(err))
	}

	metricKey := types.NamespacedName{Namespace: pa.Namespace, Name: pa.Name}
	latestStats, err := c.collector.LatestStats(metricKey)
	if err == nil && latestStats != nil {
		podNames := findDeletablePods(latestStats, c.cfg)
		// logger.Infof("Deletable pod num: %d, pods: %v", len(podNames), podNames)
		// Only scale down if we are sure there are pods ready to be deleted
		if len(podNames) > 0 {
			// TODO(gr): Is there a better way to get the service name?
			serviceKey := types.NamespacedName{Namespace: pa.Namespace, Name: sks.Labels["serving.knative.dev/service"]}
			app, err := c.store.GetAppByNamespacedName(ctx, serviceKey)
			if err != nil {
				return err
			}
			desiredScale, err := c.scaler.ScaleDownPods(ctx, c.kubeClient, pa, app, podNames)
			if err != nil {
				return fmt.Errorf("error deleting pods: %v (%v)", podNames, err)
			}
			if desiredScale != -1 {
				pa.Status.DesiredScale = ptr.Int32(desiredScale)
			}
		}
	}

	logger.Debugf(
		"PPA: %d is ready, %d is not ready, %d is pending, %d is terminating",
		ready,
		notReady,
		pending,
		terminating,
	)

	mode := nv1alpha1.SKSOperationModeServe // This could also be proxy (if scale is 0)
	numActivators := int32(1)               // I guess?
	sks, err = c.reconcileSKS(ctx, pa, mode, numActivators)
	if err != nil {
		return fmt.Errorf("error reconciling SKS: %w", err)
	}
	pa.Status.MetricsServiceName = sks.Status.PrivateServiceName

	// Propagate service name.
	pa.Status.ServiceName = sks.Status.ServiceName
	if err = c.reconcileMetric(ctx, pa, pa.Status.MetricsServiceName); err != nil {
		return fmt.Errorf("error reconciling Metric: %w", err)
	}

	// If SKS is not ready — ensure we're not becoming ready.
	if sks.IsReady() {
		logger.Debug("SKS is ready, marking SKS status ready")
		pa.Status.MarkSKSReady()
	} else {
		logger.Debug("SKS is not ready, marking SKS status not ready")
		pa.Status.MarkSKSNotReady(sks.Status.GetCondition(nv1alpha1.ServerlessServiceConditionReady).GetMessage())
	}

	// Update status
	pa.Status.ActualScale = ptr.Int32(int32(ready))
	pa.Status.MarkActive()
	return nil
}

func findDeletablePods(stats []metrics.Stat, config *resourceutil.PraasConfig) []string {
	deletable := make([]string, 0)
	for _, stat := range stats {
		canDelete := true
		interval := 0
		for _, entry := range stat.Entries {
			interval += entry.Interval
			canDelete = canDelete && entry.Value <= config.MetricThreshold
		}
		if canDelete && interval >= config.MetricTime {
			deletable = append(deletable, stat.PodName)
		}
	}
	return deletable
}

// reconcileSKS reconciles a ServerlessService based on the given PodAutoscaler.
func (c *reconciler) reconcileSKS(
	ctx context.Context, pa *autoscalingv1alpha1.PodAutoscaler,
	mode nv1alpha1.ServerlessServiceOperationMode, numActivators int32,
) (*nv1alpha1.ServerlessService, error) {
	logger := logging.FromContext(ctx)

	sksName := pa.Name
	sks, err := c.SKSLister.ServerlessServices(pa.Namespace).Get(sksName)
	if errors.IsNotFound(err) {
		logger.Info("SKS does not exist; creating.")
		sks = resources.MakeSKS(pa, mode, numActivators)
		if _, err = c.NetworkingClient.NetworkingV1alpha1().ServerlessServices(sks.Namespace).Create(
			ctx,
			sks,
			metav1.CreateOptions{},
		); err != nil {
			return nil, fmt.Errorf("error creating SKS %s: %w", sksName, err)
		}
		logger.Info("Created SKS")
	} else if err != nil {
		return nil, fmt.Errorf("error getting SKS %s: %w", sksName, err)
	} else if !metav1.IsControlledBy(sks, pa) {
		pa.Status.MarkResourceNotOwned("ServerlessService", sksName)
		return nil, fmt.Errorf("PA: %s does not own SKS: %s", pa.Name, sksName)
	} else {
		tmpl := resources.MakeSKS(pa, mode, numActivators)
		if !equality.Semantic.DeepEqual(tmpl.Spec, sks.Spec) {
			want := sks.DeepCopy()
			want.Spec = tmpl.Spec
			logger.Infof("SKS %s changed; reconciling, want mode: %v", sksName, want.Spec.Mode)
			if sks, err = c.NetworkingClient.NetworkingV1alpha1().ServerlessServices(sks.Namespace).Update(
				ctx,
				want,
				metav1.UpdateOptions{},
			); err != nil {
				return nil, fmt.Errorf("error updating SKS %s: %w", sksName, err)
			}
		}
	}
	logger.Debug("Done reconciling SKS ", sksName)
	return sks, nil
}

// reconcileMetric reconciles a metric instance out of the given PodAutoscaler to control metric collection.
func (c *reconciler) reconcileMetric(
	ctx context.Context,
	pa *autoscalingv1alpha1.PodAutoscaler,
	metricSN string,
) error {
	desiredMetric := resources.MakeMetric(pa, metricSN)
	metric, err := c.MetricLister.Metrics(desiredMetric.Namespace).Get(desiredMetric.Name)
	if errors.IsNotFound(err) {
		_, err = c.Client.AutoscalingV1alpha1().Metrics(desiredMetric.Namespace).Create(
			ctx,
			desiredMetric,
			metav1.CreateOptions{},
		)
		if err != nil {
			return fmt.Errorf("error creating metric: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("error fetching metric: %w", err)
	} else if !metav1.IsControlledBy(metric, pa) {
		pa.Status.MarkResourceNotOwned("Metric", desiredMetric.Name)
		return fmt.Errorf("PA: %s does not own Metric: %s", pa.Name, desiredMetric.Name)
	} else if !equality.Semantic.DeepEqual(desiredMetric.Spec, metric.Spec) {
		want := metric.DeepCopy()
		want.Spec = desiredMetric.Spec
		if _, err = c.Client.AutoscalingV1alpha1().Metrics(desiredMetric.Namespace).Update(
			ctx,
			want,
			metav1.UpdateOptions{},
		); err != nil {
			return fmt.Errorf("error updating metric: %w", err)
		}
	}

	return nil
}
