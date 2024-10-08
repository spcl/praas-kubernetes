package config

import (
	"context"
	"strconv"

	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
	"praas/pkg/kubernetes"
	"praas/pkg/logging"
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
	ContainerCoordEndpoint   string
	ContainerSwapEndpoint    string
	ContainerPort            string
	RedisAddress             string
	ServePort                string
	MetricTime               int
	MetricThreshold          float64

	CPULimit  string
	MEMLimit  string
	FuncLimit int
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
		logging.FromContext(ctx).Panic("Could not get praas config from context")
	}
	return untyped.(*PraasConfig)
}

func newPraasConfigFromCM(ctx context.Context) *PraasConfig {
	logger := logging.FromContext(ctx)
	cfg := newDefault()
	var err error

	// Set up watcher to update config
	factory := kubernetes.GetPraaSInformerFactory(ctx)
	kubernetes.InformerAddEventHandler(
		ctx,
		factory,
		factory.Core().V1().ConfigMaps().Informer(),
		cache.FilteringResourceEventHandler{
			FilterFunc: func(obj interface{}) bool {
				if mObj, ok := obj.(metav1.Object); ok {
					return mObj.GetName() == configName
				}
				return false
			},
			Handler: cache.ResourceEventHandlerFuncs{
				DeleteFunc: nil,
				AddFunc: func(obj interface{}) {
					if cm, ok := obj.(*corev1.ConfigMap); ok {
						if err = updateFromCM(cfg, cm); err != nil {
							logger.Errorw("Failed to update praas config", zap.Error(err))
						}
					}
				},
				UpdateFunc: func(oldObj, newObj interface{}) {
					if cm, ok := newObj.(*corev1.ConfigMap); ok {
						if err = updateFromCM(cfg, cm); err != nil {
							logger.Errorw("Failed to update praas config", zap.Error(err))
						}
					}
				},
			},
		},
	)

	logger.Infof("Created PraaS config %v", cfg)
	return cfg
}

func newDefault() *PraasConfig {
	cfg := &PraasConfig{}
	setDefault(cfg)
	return cfg
}

func setDefault(cfg *PraasConfig) {
	cfg.ServePort = ":8080"

	cfg.ContainerImage = ""
	cfg.ContainerNamespace = "default"
	cfg.ContainerMetricsEndpoint = "/custom-metrics"
	cfg.ContainerCoordEndpoint = "/coordinate/peers"
	cfg.ContainerRestartEndpoint = "/restart"
	cfg.ContainerSwapEndpoint = "/swap"
	cfg.ContainerPort = "8080"

	cfg.RedisAddress = "redis-cluster.praas:6379"
	cfg.MetricTime = 30
	cfg.MetricThreshold = 0

	cfg.CPULimit = ""
	cfg.MEMLimit = ""
	cfg.FuncLimit = 1
}

func updateFromCM(cfg *PraasConfig, configMap *corev1.ConfigMap) error {
	if configMap == nil || cfg == nil {
		return nil
	}

	// First reset to default values (if some keys were deleted)
	setDefault(cfg)

	return parseCM(
		configMap.Data,
		parseString("serve-port", &cfg.ServePort),

		parseString("container-image", &cfg.ContainerImage),
		parseString("container-namespace", &cfg.ContainerNamespace),
		parseString("container-metrics-endpoint", &cfg.ContainerMetricsEndpoint),
		parseString("container-restart-endpoint", &cfg.ContainerRestartEndpoint),
		parseString("container-coord-endpoint", &cfg.ContainerCoordEndpoint),
		parseString("container-swap-endpoint", &cfg.ContainerSwapEndpoint),
		parseString("container-port", &cfg.ContainerPort),

		parseString("redis-address", &cfg.RedisAddress),

		parseInt("metric-time", &cfg.MetricTime),
		parseFloat64("metric-threshold", &cfg.MetricThreshold),

		parseString("cpu-limit", &cfg.CPULimit),
		parseString("mem-limit", &cfg.MEMLimit),

		parseInt("max-funcs-per-process", &cfg.FuncLimit),
	)
}

func parseCM(data map[string]string, parsers ...func(map[string]string) error) error {
	for _, parser := range parsers {
		if err := parser(data); err != nil {
			return err
		}
	}
	return nil
}

func parseString(key string, target *string) func(map[string]string) error {
	return func(data map[string]string) error {
		value, exists := data[key]
		if exists {
			*target = value
		}
		return nil
	}
}

func parseInt(key string, target *int) func(map[string]string) error {
	return func(data map[string]string) error {
		value, exists := data[key]
		if exists {
			parsedValue, err := strconv.Atoi(value)
			if err != nil {
				return err
			}
			*target = parsedValue
		}
		return nil
	}
}

func parseFloat64(key string, target *float64) func(map[string]string) error {
	return func(data map[string]string) error {
		value, exists := data[key]
		if exists {
			parsedValue, err := strconv.ParseFloat(value, 64)
			if err != nil {
				return err
			}
			*target = parsedValue
		}
		return nil
	}
}
