package scaling

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	v12 "k8s.io/client-go/informers/core/v1"
	v1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/cache"
	"knative.dev/pkg/apis/duck"
	kubeclient "knative.dev/pkg/client/injection/kube/client"
	filteredpodinformer "knative.dev/pkg/client/injection/kube/informers/core/v1/pod/filtered"
	"knative.dev/pkg/injection/clients/dynamicclient"
	"knative.dev/pkg/logging"
	autoscalingv1alpha1 "knative.dev/serving/pkg/apis/autoscaling/v1alpha1"
	"knative.dev/serving/pkg/apis/serving"
	"knative.dev/serving/pkg/client/injection/ducks/autoscaling/v1alpha1/podscalable"
	"praas/pkg/networking"
	"praas/pkg/reconciler/garbagecollector/metrics"
	"praas/pkg/resources"
	"praas/pkg/store"
)

const (
	DeletionCostAnnotation = "controller.kubernetes.io/pod-deletion-cost"
	minDeletionCost        = "-2147483647"
	MaxDeletionCost        = "2147483647"
)

var (
	noReusablePodError = errors.New("reuse: no suitable pods were found")
	newPodNotFound     = errors.New("could not find new pod after scaling up")
)

// Scaler scales the target of a kpa-class PA up or down including scaling to zero.
type Scaler struct {
	listerFactory  func(gvr schema.GroupVersionResource) (cache.GenericLister, error)
	dynamicClient  dynamic.Interface
	store          store.StateStore
	podInformer    v12.PodInformer
	collector      *metrics.MetricCollector
	httpClient     *networking.HTTPClient
	cfg            *resources.PraasConfig
	swappedOutPods sync.Map
	kubeClient     v1.CoreV1Interface
}

type scalerKey struct{}

// WithScaler returns a context derived from the parent context containing a scaler instance
func WithScaler(ctx context.Context) context.Context {
	psInformerFactory := podscalable.Get(ctx)
	podInformer := filteredpodinformer.Get(ctx, serving.RevisionUID)
	collector := metrics.GetCollector(ctx)
	httpClient := networking.GetHTTPClient(ctx)
	cfg := resources.GetPraasConfig(ctx)
	scaler := newScaler(ctx, psInformerFactory, podInformer, collector, httpClient, cfg)

	return context.WithValue(ctx, scalerKey{}, scaler)
}

// GetScaler returns the scalre instance stored in the context
func GetScaler(ctx context.Context) *Scaler {
	untyped := ctx.Value(scalerKey{})
	if untyped == nil {
		logging.FromContext(ctx).Panic("Unable to fetch scaler from context.")
	}
	return untyped.(*Scaler)
}

// newScaler creates a Scaler.
func newScaler(
	ctx context.Context,
	psInformerFactory duck.InformerFactory,
	podInformer v12.PodInformer,
	collector *metrics.MetricCollector,
	client *networking.HTTPClient,
	cfg *resources.PraasConfig,
) *Scaler {
	ks := &Scaler{
		dynamicClient: dynamicclient.Get(ctx),
		kubeClient:    kubeclient.Get(ctx).CoreV1(),
		store:         store.GetStateStore(ctx),
		podInformer:   podInformer,
		collector:     collector,
		httpClient:    client,
		cfg:           cfg,

		// We wrap the PodScalable Informer Factory here so Get() uses the outer context.
		// As the returned Informer is shared across reconciles, passing the context from
		// an individual reconcile to Get() could lead to problems.
		listerFactory: func(gvr schema.GroupVersionResource) (cache.GenericLister, error) {
			_, l, err := psInformerFactory.Get(ctx, gvr)
			return l, err
		},
	}
	return ks
}

func (ks *Scaler) applyScale(
	ctx context.Context,
	pa *autoscalingv1alpha1.PodAutoscaler,
	ps *autoscalingv1alpha1.PodScalable,
	currentScale, desiredScale int32,
) error {

	// We don't scale below 0 and don't do anything if we are already at the desired scale
	if desiredScale < 0 || currentScale == desiredScale {
		return nil
	}

	logger := logging.FromContext(ctx)
	logger.Infof("Scaling from %d to %d", currentScale, desiredScale)

	gvr, name, err := scaleResourceArguments(pa.Spec.ScaleTargetRef)
	if err != nil {
		return err
	}

	psNew := ps.DeepCopy()
	psNew.Spec.Replicas = &desiredScale
	patch, err := duck.CreatePatch(ps, psNew)
	if err != nil {
		return err
	}
	patchBytes, err := patch.MarshalJSON()
	if err != nil {
		return err
	}

	_, err = ks.dynamicClient.Resource(*gvr).Namespace(pa.Namespace).Patch(
		ctx, ps.Name, types.JSONPatchType, patchBytes, metav1.PatchOptions{},
	)
	if err != nil {
		return fmt.Errorf("failed to apply scale %d to scale target %s: %w", desiredScale, name, err)
	}

	logger.Debug("Successfully scaled to ", desiredScale)
	return nil
}

func (ks *Scaler) ScaleDownPods(
	ctx context.Context,
	client v1.CoreV1Interface,
	pa *autoscalingv1alpha1.PodAutoscaler,
	app *store.PraasApp,
	podNames []string,
) (int32, error) {
	logger := logging.FromContext(ctx)
	if len(podNames) <= 0 {
		return -1, nil
	}

	// Get the lock for this app
	hasLock, _ := ks.store.LockApp(ctx, app)
	if !hasLock {
		// Couldn't get the lock we will retry later
		return -1, fmt.Errorf("scaler could not get the lock for app %v", app.ServiceName)
	}
	defer ks.store.UnLockApp(ctx, app)

	// Make sure pods that are not running are not deleted
	podsClient := client.Pods(pa.Namespace)
	podList, err := podsClient.List(
		ctx,
		metav1.ListOptions{
			// FieldSelector: fmt.Sprintf("status.phase!=%s", corev1.PodRunning),
			LabelSelector: fmt.Sprintf("%s=%d", resources.AppIDLabelKey, app.ID),
		},
	)
	if err != nil {
		return -1, err
	}
	canDelete := true
	// Filter out pods that would take precedent over deletion cost (e.g. pending pods)
	for _, pod := range podList.Items {
		if pod.DeletionTimestamp == nil { // We don't care about deleted pods
			if pod.Status.Phase != corev1.PodRunning {
				canDelete = false
				break
			}

			for _, status := range pod.Status.ContainerStatuses {
				if status.State.Running == nil {
					canDelete = false
					break
				}
			}
		}
	}
	if !canDelete {
		logger.Warn("There are pending pods, delete will be retried")
		return -1, nil
	}

	// Check with the state store, which of the given pods we can delete
	processes, notFound, err := ks.store.GetDeletablePods(ctx, app, podNames)
	if err != nil {
		return -1, err
	}
	if len(notFound) > 0 {
		logger.Warnf("The following pods were selected to be deleted, but are not deletable: %v", notFound)
	}
	logger.Info("Deletable pods: ", processes)

	// Instruct pods to swap out and set deletion costs
	var deletedPods []string
	for _, podName := range processes {
		pod, err := podsClient.Get(ctx, podName, metav1.GetOptions{})
		if err != nil {
			logger.Errorw("Could not get pod "+podName, zap.Error(err))
			continue
		}
		if ks.isSwappedOut(pod.Status.PodIP) {
			wasNotAlreadySet, err := setDeletionCost(ctx, pod, minDeletionCost, client)
			if err != nil {
				logger.Errorw("Could not set deletion cost for pod "+podName, zap.Error(err))
				continue
			} else if !wasNotAlreadySet {
				logger.Info("Delete pod: ", podName)
				deletedPods = append(deletedPods, podName)
				ks.swappedOutPods.Delete(pod.Status.PodIP)
			}
		}
	}
	if len(deletedPods) <= 0 {
		return -1, nil
	}
	logger.Infof("Delete %d pods", len(deletedPods))

	// Save the list of pods we currently have for later
	// lister := resources.NewPodLister(ks.podInformer.Lister(), pa.Namespace, pa.Name)
	// currPods, err := lister.Pods()
	// currPodsMap := make(map[string]*corev1.Pod)
	// for _, p := range currPods {
	// 	currPodsMap[p.Name] = p
	// }
	// if err != nil {
	// 	logger.Fatalw("Failed to get current pods", zap.Error(err))
	// 	return -1, err
	// }

	ps, err := getScaleResource(pa.Namespace, pa.Spec.ScaleTargetRef, ks.listerFactory)
	if err != nil {
		return 0, err
	}
	currentScale := ks.store.GetCurrentScale(ctx, app)
	desiredScale := currentScale - int32(len(deletedPods))
	err = ks.applyScale(ctx, pa, ps, currentScale, desiredScale)
	if err != nil {
		logger.Errorw("Failed to apply scale", zap.Error(err))
		return currentScale, err
	}

	// var newPods []*corev1.Pod
	// for len(newPods) != int(desiredScale) {
	// 	newPods, err = lister.Pods()
	// 	if err != nil {
	// 		logger.Fatalw("Failed to get current pods", zap.Error(err))
	// 		return -1, err
	// 	}
	// 	if len(newPods) != int(desiredScale) {
	// 		time.Sleep(500 * time.Millisecond)
	// 	}
	// }
	// deletedPodNames := make([]string, 0)
	// for _, p := range newPods {
	// 	delete(currPodsMap, p.Name)
	// }
	// for pName, pod := range currPodsMap {
	// 	deletedPodNames = append(deletedPodNames, pName)
	// 	if pod.Annotations[DeletionCostAnnotation] != minDeletionCost {
	// 		logger.Errorw("Deleted pod with annotation cost not minimum", zap.Any("pod", pod))
	// 	}
	// }
	// logger.Info("Pods deleted: ", deletedPodNames)

	// Now we can actually delete from the state
	for _, podName := range deletedPods {
		err = ks.store.DeleteProcess(ctx, app, podName)
		if err != nil {
			logger.Errorw("Failed to delete pod from state", zap.Error(err), zap.Any("pod", podName))
		}
	}

	// Wait for deleted pods to have a deletion timestamp
	lister := resources.NewPodLister(ks.podInformer.Lister(), pa.Namespace, pa.Name)
	count := 0
	for count != int(desiredScale) {
		count, err = lister.PodCount()
		if err != nil {
			logger.Errorw("Failed to get pod count", zap.Error(err))
		}
	}
	return desiredScale, nil
}

func setDeletionCost(ctx context.Context, pod *corev1.Pod, newCost string, kube v1.CoreV1Interface) (bool, error) {
	// Please note that this function always returns false now.

	logger := logging.FromContext(ctx)
	logger.Infof("Set Deletion Cost for pod %s to %s", pod.Name, newCost)
	annotations := pod.GetAnnotations()
	if annotations[DeletionCostAnnotation] != newCost {
		newPod := pod.DeepCopy()
		newPod.Annotations[DeletionCostAnnotation] = newCost
		patch, err := duck.CreatePatch(pod, newPod)
		if err != nil {
			return false, err
		}
		patchBytes, err := patch.MarshalJSON()
		if err != nil {
			return false, err
		}
		newPod, err = kube.Pods(pod.Namespace).Patch(
			ctx,
			pod.Name,
			types.JSONPatchType,
			patchBytes,
			metav1.PatchOptions{},
		)
		if err != nil {
			return false, err
		}

		return false, nil
	}
	return false, nil
}

func (ks *Scaler) NewProcesses(
	ctx context.Context,
	pa *autoscalingv1alpha1.PodAutoscaler,
	app *store.PraasApp,
	processes []store.ProcessData,
) ([]string, []string, error) {
	logger := logging.FromContext(ctx)
	// Get the lock for this app
	var err error
	hasLock := false
	for !hasLock {
		hasLock, err = ks.store.LockApp(ctx, app)
		if err != nil {
			return nil, nil, err
		}
	}
	defer ks.store.UnLockApp(ctx, app)

	// Check if there are pods we could reuse
	key := types.NamespacedName{Namespace: pa.Namespace, Name: pa.Name}
	PIDs, pods, err := ks.findReusablePods(ctx, key, app, logger, processes)
	if err == nil {
		if len(PIDs) == len(processes) {
			return PIDs, pods, nil
		}
		// Get rid of processes we already mapped
		processes = processes[len(PIDs):]
	}

	// Create new pod
	createdPIDs, createdPods, err := ks.newPods(ctx, pa, app, logger, processes)
	if err != nil {
		return nil, nil, err
	}

	return append(PIDs, createdPIDs...), append(pods, createdPods...), nil
}

func (ks *Scaler) findReusablePods(
	ctx context.Context,
	key types.NamespacedName,
	app *store.PraasApp,
	logger *zap.SugaredLogger,
	processes []store.ProcessData,
) ([]string, []string, error) {
	// Get the current state
	stats, err := ks.collector.LatestStats(key)
	if err != nil {
		return nil, nil, err
	}
	logger.Debugf("We could reuse %d pods", len(stats))
	count := len(processes)
	sessionIdx := 0
	var PIDs []string
	var reusedPods []string
	for _, stat := range stats {
		// Check and reuse if possible
		canDelete := true
		interval := 0
		config := resources.GetPraasConfig(ctx)
		for _, entry := range stat.Entries {
			interval += entry.Interval
			canDelete = canDelete && entry.Value <= config.MetricThreshold
		}
		canDelete = canDelete && interval >= config.MetricTime
		if canDelete {
			pod, err := ks.podInformer.Lister().Pods(ks.cfg.ContainerNamespace).Get(stat.PodName)
			if err != nil || pod.GetAnnotations()[DeletionCostAnnotation] == minDeletionCost {
				continue
			}
			PID, err := ks.reusePod(
				ctx,
				app,
				logger,
				stat.PodIP,
				stat.PodName,
				processes[sessionIdx].PID,
			)
			if err == nil {
				err = ks.updatePIDLabel(ctx, pod, processes[sessionIdx].PID)
				if err != nil {
					logger.Errorw("Failed to update pod PID Label", zap.Error(err))
				}
				logger.Info("Reused pod: ", stat.PodName)
				PIDs = append(PIDs, PID)
				reusedPods = append(reusedPods, stat.PodName)
				sessionIdx++
				if sessionIdx == count {
					// We reused enough pods
					return PIDs, reusedPods, nil
				}
			} else if !errors.Is(err, store.NotReusableAnymoreError) {
				logger.Warnw("Could not reuse pod: "+stat.PodName, zap.Error(err))
			}
		}
	}

	return PIDs, reusedPods, nil
}

func (ks *Scaler) reusePod(
	ctx context.Context,
	app *store.PraasApp,
	logger *zap.SugaredLogger,
	podIP, podName, PID string,
) (string, error) {
	logger.Infow("Reuse pod "+podName, zap.Any("AppID", PID))

	swap := true
	if PID == "" {
		PID = uuid.NewString()
		swap = false
	}
	err := ks.store.MarkAsReused(ctx, app, podName, PID, swap)
	if err != nil {
		return "", err
	}

	path := networking.CreateURI(podIP, ks.cfg.ContainerRestartPort, ks.cfg.ContainerRestartEndpoint)
	var resp networking.ResponseBase
	return PID, ks.httpClient.Do(ctx, path, &resp)
}

func (ks *Scaler) newPods(
	ctx context.Context,
	pa *autoscalingv1alpha1.PodAutoscaler,
	app *store.PraasApp,
	logger *zap.SugaredLogger,
	processes []store.ProcessData,
) ([]string, []string, error) {
	// logger.Infow("Create new pod", zap.Any("AppID", AppID), zap.Any("payload", payload))

	var PIDs []string
	successCount := 0
	for _, process := range processes {
		swap := true
		PID := process.PID
		if PID == "" {
			PID = uuid.NewString()
			swap = false
		}
		err := ks.store.CreateProcess(ctx, app, PID, swap)
		if err == nil {
			successCount++
			PIDs = append(PIDs, PID)
		}
	}

	// Save the list of pods we currently have for later
	lister := resources.NewPodLister(ks.podInformer.Lister(), pa.Namespace, pa.Name)
	currPods, err := lister.Pods()
	if err != nil {
		return nil, nil, err
	}

	// Apply new scale
	currentScale := ks.store.GetCurrentScale(ctx, app)
	desiredScale := currentScale + int32(successCount)
	ps, err := getScaleResource(pa.Namespace, pa.Spec.ScaleTargetRef, ks.listerFactory)
	if err != nil {
		return nil, nil, err
	}
	err = ks.applyScale(ctx, pa, ps, currentScale, desiredScale)
	if err != nil {
		return nil, nil, err
	}

	// Wait for new pods to be visible
	logger.Info("Get new pods")
	createdPods, err := getNewPods(lister, currPods, successCount)
	// TODO(gr): rollback pod creation if error?
	if err != nil {
		logger.Errorw("Failed to get the name of the new pods after scaling", zap.Error(err))
		return nil, nil, err
	}

	var successPIDs []string
	var successPods []string
	for idx, PID := range PIDs {
		pod := createdPods[idx]
		err = ks.updatePIDLabel(ctx, pod, PID)
		if err != nil {
			logger.Errorw("Failed to update pod PID", zap.Error(err))
		}
		podName := pod.Name
		err = ks.store.MapPodToProcess(ctx, app, podName, PID)
		if err == nil {
			successPIDs = append(successPIDs, PID)
			successPods = append(successPods, podName)
		}
	}
	return successPIDs, successPods, nil
}

func (ks *Scaler) updatePIDLabel(ctx context.Context, pod *corev1.Pod, PID string) error {
	newPod := pod.DeepCopy()
	newPod.Labels[resources.PIDLabelKey] = PID
	patch, err := duck.CreatePatch(pod, newPod)
	if err != nil {
		return err
	}
	patchBytes, err := patch.MarshalJSON()
	if err != nil {
		return err
	}

	_, err = ks.kubeClient.Pods(pod.Namespace).Patch(
		ctx,
		pod.Name,
		types.JSONPatchType,
		patchBytes,
		metav1.PatchOptions{},
	)

	return err
}

func (ks *Scaler) isSwappedOut(ip string) bool {
	_, isSwappedOut := ks.swappedOutPods.Load(ip)
	if isSwappedOut {
		return true
	}

	// Launch a background function that will swap it out
	go func() {
		// Make the request
		path := networking.CreateURI(ip, ks.cfg.ContainerRestartPort, ks.cfg.ContainerSwapEndpoint)
		requestCtx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		err := ks.httpClient.Do(requestCtx, path, nil)
		if err != nil {
			return
		}

		var member struct{}
		ks.swappedOutPods.Store(ip, member)
	}()
	return false
}

func getNewPods(
	lister resources.PodLister,
	currPods []*corev1.Pod,
	count int,
) ([]*corev1.Pod, error) {
	visible := false
	var pods []*corev1.Pod
	var err error
	// Wait for new pods to become visible
	for !visible {
		pods, err = lister.Pods()
		if err != nil {
			return nil, err
		}
		if len(currPods) == len(pods)-count {
			visible = true
		}
	}

	// Now we need to find the elements that are different in pods than currPods
	for _, currP := range currPods {
		// Find and remove pod from new pods
		for idx, newP := range pods {
			if newP.Name == currP.Name {
				end := len(pods) - 1
				pods[idx] = pods[end]
				pods = pods[:end]
				break
			}
		}
	}

	if len(pods) == count {
		return pods, nil
	}

	return nil, newPodNotFound
}
