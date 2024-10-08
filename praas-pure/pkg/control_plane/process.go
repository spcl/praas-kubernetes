package control_plane

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"reflect"
	"strconv"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	k8sresource "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	listers "k8s.io/client-go/listers/core/v1"
	"praas/pkg/config"
	"praas/pkg/control_plane/scraping"
	"praas/pkg/control_plane/store"
	"praas/pkg/kubernetes"
	"praas/pkg/networking"
	"praas/pkg/types"
)

var (
	pHandler processHandler

	failedToAllocateError    = errors.New("failed to allocate session")
	failedToAllocatePodError = errors.New("failed to allocate pod for session")
)

var _ K8APIObjectHandler = (*processHandler)(nil)
var _ scraping.FilteredHandler = (*processHandler)(nil)

func NewProcessHandler(ctx context.Context) K8APIObjectHandler {
	pHandler = processHandler{
		objectHandlerBase: buildBase(ctx),
		networkClient:     networking.GetHTTPClient(ctx),
		podLister:         kubernetes.GetPodInformer(ctx).Lister(),
	}
	pHandler.collector.AddFilteredHandler(&pHandler)
	return &pHandler
}

type processHandler struct {
	objectHandlerBase
	networkClient *networking.HTTPClient
	podLister     listers.PodLister
}

func (s *processHandler) Filter(stats []scraping.StatEntry) bool {
	canDelete := true
	totalTime := 0
	for _, entry := range stats {
		canDelete = canDelete && entry.Value <= s.cfg.MetricThreshold
		totalTime += entry.Interval
	}
	return canDelete && totalTime >= s.cfg.MetricTime
}

func (s *processHandler) Handle(ctx context.Context, pod *corev1.Pod) error {
	// Notify the pod that it needs to swap out
	path := networking.CreateURI(pod.Status.PodIP, s.cfg.ContainerPort, s.cfg.ContainerSwapEndpoint)
	err := s.networkClient.DoGetWithCtx(ctx, path, nil)
	if err != nil {
		return err
	}

	ref := types.PodToProcessRef(pod)
	return s.deleteProcess(ctx, ref)
}

func (s *processHandler) HandleError(ref types.ProcessRef, err error) {
	// s.logger.Errorw("HandleError", zap.Error(err), zap.Any("session", ref))
	if errors.Is(err, scraping.PodNotReadyError) {
		s.logger.Debugw("Scraper sent pod not ready error", zap.Any("pod", ref))
	} else if errors.Is(err, context.Canceled) {
		s.logger.Debugw("Context was cancelled during scraping", zap.Any("pod", ref))
	} else if errors.Is(err, store.NoSuchProcess) || k8serrors.IsNotFound(err) {
		s.logger.Debugw("Tried to delete process that was already deleted", zap.Any("process", ref))
	} else if urlErr, ok := err.(*url.Error); ok {
		if urlErr.Timeout() {
			// Check if this pod should have returned a response or just got cleaned up late
			if s.store.ProcessExistsAndActive(context.Background(), ref) {
				s.logger.Errorw(
					"Pod is not answering scraping request, but exists in the state",
					zap.Error(urlErr),
					zap.Any("pod", ref),
				)
			} else {
				s.logger.Debugf("Pod %v is not answering scraping request and does not exist in the state", ref)
			}
		}
	} else {
		s.logger.Errorw(
			"There was an error during scraping",
			zap.Error(err),
			zap.Any("pod", ref),
			zap.Any("errType", reflect.TypeOf(err)),
		)
	}
}

func (s *processHandler) Post(ctx context.Context, input map[string]any) (any, error) {
	startTime := time.Now()
	if len(input) != 2 {
		return nil, unexpectedFieldsError
	}

	var appID types.IDType
	var processes []types.ProcessData
	err := parseData(
		input,
		asID(appIDKey, &appID),
		asDataArray(processesKey, &processes),
	)
	if err != nil {
		return nil, err
	}

	var processRefs []types.ProcessRef
	for _, process := range processes {
		processRef, err := s.CreateProcess(ctx, appID, process)
		if err == nil {
			processRefs = append(processRefs, processRef)
		}
	}
	var createdProcesses []types.ProcessCreationData
	for _, processRef := range processRefs {
		ip, err := s.GetPodIP(processRef.String())
		if err == nil {
			createdProcesses = append(
				createdProcesses,
				types.ProcessCreationData{PodName: processRef.String(), PID: processRef.PID, IP: ip},
			)
		}
	}

	if len(createdProcesses) == 0 {
		return nil, err
	}

	s.logger.Infof("Process allocation time: %d", time.Now().Sub(startTime).Milliseconds())
	return map[string]any{"processes": createdProcesses}, nil
}

func (s *processHandler) Delete(ctx context.Context, input map[string]any) (any, error) {
	var appID types.IDType
	var pid string
	err := parseData(input, asID(appIDKey, &appID), asString(PIDKey, &pid))
	if err != nil {
		return nil, err
	}

	return nil, s.deleteProcess(ctx, types.ProcessRef{AppID: appID, PID: pid})
}

func (s *processHandler) createPod(ctx context.Context, process types.ProcessRef, namespace string) error {
	resourceLimits := make(corev1.ResourceList)
	if s.cfg.CPULimit != "" {
		q, err := k8sresource.ParseQuantity(s.cfg.CPULimit)
		if err == nil {
			resourceLimits[corev1.ResourceCPU] = q
		} else {
			s.logger.Warnw("Could not parse CPU limit, it will not be set", zap.Error(err))
		}
	}
	if s.cfg.MEMLimit != "" {
		q, err := k8sresource.ParseQuantity(s.cfg.MEMLimit)
		if err == nil {
			resourceLimits[corev1.ResourceMemory] = q
		} else {
			s.logger.Warnw("Could not parse RAM limit, it will not be set", zap.Error(err))
		}
	}

	newPod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      process.String(),
			Namespace: namespace,
			Labels: map[string]string{
				config.AppIDLabelKey: fmt.Sprint(process.AppID),
				config.PIDLabelKey:   process.PID,
			},
		},
		Spec: corev1.PodSpec{
			Affinity: &corev1.Affinity{PodAntiAffinity: &corev1.PodAntiAffinity{
				PreferredDuringSchedulingIgnoredDuringExecution: []corev1.WeightedPodAffinityTerm{{
					Weight: 100,
					PodAffinityTerm: corev1.PodAffinityTerm{
						LabelSelector: &metav1.LabelSelector{MatchLabels: map[string]string{config.AppIDLabelKey: process.AppID.String()}},
						TopologyKey:   "kubernetes.io/hostname",
					},
				}},
			}},
			Containers: []corev1.Container{{
				Name:            config.ProcessContainerName,
				Image:           s.cfg.ContainerImage,
				ImagePullPolicy: corev1.PullIfNotPresent,
				Ports:           []corev1.ContainerPort{{ContainerPort: 8080}},
				Resources:       corev1.ResourceRequirements{Limits: resourceLimits},
				Env: []corev1.EnvVar{
					{Name: "APP_ID", ValueFrom: &corev1.EnvVarSource{FieldRef: &corev1.ObjectFieldSelector{FieldPath: "metadata.labels['" + config.AppIDLabelKey + "']"}}},
					{Name: "PROCESS_ID", ValueFrom: &corev1.EnvVarSource{FieldRef: &corev1.ObjectFieldSelector{FieldPath: "metadata.labels['" + config.PIDLabelKey + "']"}}},
					{Name: "CPU_LIMIT", ValueFrom: &corev1.EnvVarSource{ResourceFieldRef: &corev1.ResourceFieldSelector{Resource: "limits.cpu", ContainerName: config.ProcessContainerName}}},
					{Name: "RAM_LIMIT", ValueFrom: &corev1.EnvVarSource{ResourceFieldRef: &corev1.ResourceFieldSelector{Resource: "limits.memory", ContainerName: config.ProcessContainerName}}},
				},
			}},
		},
	}
	_, err := s.kubeClient.CoreV1().Pods(s.cfg.ContainerNamespace).Create(ctx, &newPod, metav1.CreateOptions{})
	return err
}

func (s *processHandler) deleteProcess(ctx context.Context, ref types.ProcessRef) error {
	kubePods := s.kubeClient.CoreV1().Pods(s.cfg.ContainerNamespace)

	// Delete from state
	err := s.store.DeleteProcess(ctx, ref)
	if err != nil {
		if errors.Is(err, store.NoSuchProcess) {
			// Make sure there is no such session in kubernetes
			pod, err := kubePods.Get(ctx, ref.String(), metav1.GetOptions{})
			if err != nil && !k8serrors.IsNotFound(err) {
				s.logger.Errorw("Error getting pod: "+ref.String(), zap.Error(err))
				return failedToDeletePods
			}
			if pod == nil {
				return store.NoSuchProcess
			} else {
				return kubePods.Delete(ctx, ref.String(), metav1.DeleteOptions{})
			}
		}
		return err
	}

	// Delete kubernetes pod
	return kubePods.Delete(ctx, ref.String(), metav1.DeleteOptions{})
}

func (s *processHandler) CreateProcess(
	ctx context.Context,
	appID types.IDType,
	processData types.ProcessData,
) (types.ProcessRef, error) {
	pid := processData.PID
	swap := true

	s.logger.Debugw("Create process", zap.Any("appID", appID), zap.Any("pid", pid))

	// Generate pid if needed
	if pid == "" {
		pid = uuid.NewString()
		swap = false
	}

	// Add to the state
	process := types.ProcessRef{AppID: appID, PID: pid}
	_, err := s.store.NewProcess(ctx, process, swap)
	if err != nil {
		if errors.Is(err, store.AppDoesNotExist) || errors.Is(err, store.ProcessAlreadyActive) {
			return types.InvalidProcess(), err
		}

		s.logger.Errorw("Failed to create new process", zap.Error(err))
		return types.InvalidProcess(), failedToAllocateError
	}

	// Post pod
	err = s.createPod(ctx, process, s.cfg.ContainerNamespace)
	if err != nil {
		s.logger.Errorw("Failed to start up new pod for process", zap.Error(err))

		// Rollback state
		err := s.store.DeleteProcess(context.Background(), process) // New context so this will execute even if timeout
		if err != nil {
			s.logger.Errorw("Failed to roll back state after error", zap.Error(err))
		}

		return types.InvalidProcess(), failedToAllocatePodError
	}

	return process, nil
}

func (s *processHandler) GetPodIP(podName string) (string, error) {
	isCurrReady := false
	ip := ""
	lister := s.podLister.Pods(s.cfg.ContainerNamespace)
	for !isCurrReady {
		pod, err := lister.Get(podName)
		if err != nil {
			s.logger.Warnw("Failed to get pod: "+podName, zap.Error(err))
			time.Sleep(time.Millisecond)
		} else {
			ip = getPodIPIfReady(pod)
			isCurrReady = ip != ""
		}
	}
	return ip, nil
}

func (s *processHandler) Get(ctx context.Context, query url.Values) (any, error) {
	pid := query.Get("pid")
	if pid == "" {
		return nil, fmt.Errorf("missing pid field")
	}
	appID, err := strconv.Atoi(query.Get("app"))
	if err != nil {
		return nil, fmt.Errorf("missing or incorrect app field")
	}
	process := types.ProcessRef{AppID: types.IDType(appID), PID: pid}

	ip, err := s.GetPodIP(process.String())
	return map[string]string{"ip": ip}, nil
}
