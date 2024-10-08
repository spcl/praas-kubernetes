package tracking

import (
	"context"
	"encoding/json"
	"time"

	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
	"praas/pkg/config"
	"praas/pkg/kubernetes"
	"praas/pkg/logging"
	"praas/pkg/networking"
	"praas/pkg/types"
	"praas/pkg/util"
)

func SetupProcessTracking(ctx context.Context) {
	logger := logging.FromContext(ctx)
	cfg := config.GetPraasConfig(ctx)
	httpClient := networking.GetHTTPClient(ctx)

	activeProcs := make(map[string]util.Set[string])

	factory := kubernetes.NewInformerFactory(ctx, cfg.ContainerNamespace)
	kubernetes.InformerAddEventHandler(
		ctx,
		factory,
		factory.Core().V1().Pods().Informer(),
		cache.FilteringResourceEventHandler{
			FilterFunc: func(obj interface{}) bool {
				// Filter pods that are not part of PraaS
				if mObj, ok := obj.(metav1.Object); ok {
					_, exists := mObj.GetLabels()[config.AppIDLabelKey]
					return exists
				}
				return false
			},
			Handler: cache.ResourceEventHandlerFuncs{
				AddFunc: nil,
				DeleteFunc: func(obj interface{}) {
					if pod, ok := obj.(*corev1.Pod); ok {
						appID := pod.Labels[config.AppIDLabelKey]
						activeProcs[appID].Delete(pod.Status.PodIP)
					}
				},
				UpdateFunc: func(oldObj, newObj interface{}) {
					oldPod, oldOk := oldObj.(*corev1.Pod)
					newPod, newOk := newObj.(*corev1.Pod)
					if oldOk && newOk {
						newPodIP := newPod.Status.PodIP
						if oldPod.Status.PodIP == "" && newPodIP != "" {
							logger.Infof("New active pod: %s", newPodIP)
							appID := newPod.Labels[config.AppIDLabelKey]
							set, exists := activeProcs[appID]
							if !exists {
								set = make(util.Set[string])
								activeProcs[appID] = set
							}
							activeSoFar := set.Elems()
							set.Add(newPodIP)

							// Send msg to new pod to create connection with other pods
							go func() {
								processID := ""
								msg := types.ProcessCoordinationMsg{Peers: activeSoFar, ProcessID: processID}
								logger.Infow("Message new pod", zap.Any("peers", msg))
								for idx, _ := range msg.Peers {
									// We need to add the ports as well
									msg.Peers[idx] += ":" + cfg.ContainerPort
								}
								msgBytes, err := json.Marshal(msg)
								if err != nil {
									logger.Errorw("Failed to marshal coordination msg", zap.Error(err))
									return
								}

								path := networking.CreateURI(newPodIP, cfg.ContainerPort, cfg.ContainerCoordEndpoint)
								success := false
								retryCount := 5
								for !success && retryCount >= 0 {
									err = httpClient.DoPost(path, msgBytes, nil)
									if err != nil {
										logger.Errorw("Failed to send coordination msg", zap.Error(err))
										time.Sleep(time.Second)
									} else {
										logger.Infof("Successfully messaged %s", newPodIP)
										success = true
									}
									retryCount--
								}
							}()
						}
					}
				},
			},
		},
	)
}
