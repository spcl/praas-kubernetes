package resources

import (
	"context"

	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeclient "knative.dev/pkg/client/injection/kube/client"
	cm "knative.dev/pkg/configmap"
	cminformer "knative.dev/pkg/configmap/informer"
	"knative.dev/pkg/logging"
	"knative.dev/pkg/system"
)

const (
	configName = "config-praas"
)

// PraasConfig holds all configuration items relevant to praas
type PraasConfig struct {
	ContainerImage           string
	ContainerNamespace       string
	ContainerMetricsEndpoint string
	ContainerRestartEndpoint string
	ContainerSwapEndpoint    string
	ContainerPort            string
	RedisAddress             string
	ContainerRestartPort     string

	DeleteGracePeriodSeconds int64
	MetricThreshold          float64
	MetricTime               int
}

type cfgKey struct{}

// WithPraasConfigFromCM returns a context derived from the parent context containing a PraasConfig object
func WithPraasConfigFromCM(ctx context.Context) context.Context {
	cfg := newPraasConfigFromCM(ctx)
	return context.WithValue(ctx, cfgKey{}, cfg)
}

func GetPraasConfig(ctx context.Context) *PraasConfig {
	untyped := ctx.Value(cfgKey{})
	if untyped == nil {
		logging.FromContext(ctx).Panic("Unable to fetch praas config from context.")
	}
	return untyped.(*PraasConfig)
}

func newPraasConfigFromCM(ctx context.Context) *PraasConfig {
	logger := logging.FromContext(ctx)
	cfg := newDefault()

	kubeClient := kubeclient.Get(ctx)
	configMap, err := kubeClient.CoreV1().ConfigMaps(system.Namespace()).Get(ctx, configName, metav1.GetOptions{})
	if err != nil {
		logger.Warnw("Could not get config map for PraaS, using default values", zap.Error(err))
		return cfg
	}
	err = updateFromCM(cfg, configMap)
	if err != nil {
		logger.Warnw("Failed to parse config map for PraaS, using default values", zap.Error(err))
		return cfg
	}

	// Setup watcher so that the config is updated on configmap changes
	// req, err := cminformer.FilterConfigByLabelExists()
	cmWatcher := cminformer.NewInformedWatcher(kubeClient, system.Namespace())
	cmWatcher.Watch(
		configName, func(configMap *corev1.ConfigMap) {
			logger.Info("Update praas config map")
			err = updateFromCM(cfg, configMap)
			if err != nil {
				logger.Warnw("Failed to update praas config", zap.Error(err))
			}
		},
	)
	logger.Info("Starting praas configuration manager...")
	if err = cmWatcher.Start(ctx.Done()); err != nil {
		logger.Fatalw("Failed to start configuration manager", zap.Error(err))
	}
	logger.Infof("Created PraaS config %v", cfg)
	return cfg
}

func newDefault() *PraasConfig {
	return &PraasConfig{
		ContainerImage:           "",
		ContainerNamespace:       "default",
		ContainerMetricsEndpoint: "/custom-metrics",
		ContainerRestartEndpoint: "/restart",
		ContainerSwapEndpoint:    "/swap",
		ContainerPort:            "8080",
		ContainerRestartPort:     "8080",

		RedisAddress: "",

		DeleteGracePeriodSeconds: 30,
		MetricThreshold:          0,
		MetricTime:               30,
	}
}

func updateFromCM(cfg *PraasConfig, configMap *corev1.ConfigMap) error {
	if configMap == nil || cfg == nil {
		return nil
	}

	return cm.Parse(
		configMap.Data,
		cm.AsString("container-image", &cfg.ContainerImage),
		cm.AsString("container-namespace", &cfg.ContainerNamespace),
		cm.AsString("container-metrics-endpoint", &cfg.ContainerMetricsEndpoint),
		cm.AsString("container-restart-endpoint", &cfg.ContainerRestartEndpoint),
		cm.AsString("container-swap-endpoint", &cfg.ContainerSwapEndpoint),
		cm.AsString("container-port", &cfg.ContainerPort),
		cm.AsString("container-restart-port", &cfg.ContainerRestartPort),

		cm.AsString("redis-address", &cfg.RedisAddress),

		cm.AsInt64("delete-grace-period-sec", &cfg.DeleteGracePeriodSeconds),
		cm.AsFloat64("metric-threshold", &cfg.MetricThreshold),
		cm.AsInt("metric-time", &cfg.MetricTime),
	)
}
