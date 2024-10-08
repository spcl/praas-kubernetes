/*
Copyright 2019 The Knative Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"context"
	"errors"
	"log"
	"net/http"

	"go.uber.org/zap"
	filteredinformerfactory "knative.dev/pkg/client/injection/kube/informers/factory/filtered"
	"knative.dev/pkg/injection"
	"knative.dev/pkg/injection/sharedmain"
	"knative.dev/pkg/logging"
	"knative.dev/pkg/metrics"
	"knative.dev/pkg/signals"
	"knative.dev/serving/pkg/apis/serving"
	controlplane "praas/pkg/control-plane"
	"praas/pkg/networking"
	"praas/pkg/reconciler/garbagecollector"
	praasmetrics "praas/pkg/reconciler/garbagecollector/metrics"
	"praas/pkg/reconciler/metricsservice"
	"praas/pkg/resources"
	"praas/pkg/scaling"
	"praas/pkg/store"
)

const (
	component = "praas-controller"
)

var ctors = []injection.ControllerConstructor{
	garbagecollector.NewController,
	metricsservice.NewMetricController,
}

func main() {
	ctx := signals.NewContext()
	ctx = filteredinformerfactory.WithSelectors(ctx, serving.RevisionUID)

	ctx, startInformers := injection.EnableInjectionOrDie(ctx, nil)

	// Set up our logger.
	loggingConfig, err := sharedmain.GetLoggingConfig(ctx)
	if err != nil {
		log.Fatal("Error loading/parsing logging configuration: ", err)
	}
	logger, _ := logging.NewLoggerFromConfig(loggingConfig, component)
	defer flush(logger)

	ctx = logging.WithLogger(ctx, logger)
	ctx = networking.WithHTTPClient(ctx)
	ctx = resources.WithPraasConfigFromCM(ctx)
	ctx = store.WithRedisStore(ctx)
	ctx = praasmetrics.WithCollector(ctx)
	ctx = scaling.WithScaler(ctx)

	startInformers()

	go sharedmain.MainWithConfig(ctx, component, injection.GetConfig(ctx), ctors...)

	controlPlaneSrv := buildControlPlane(ctx)
	if err = controlPlaneSrv.ListenAndServe(); err != nil && errors.Is(err, http.ErrServerClosed) {
		logger.Errorw("Error while running control plane server", zap.Error(err))
	} else {
		err = controlPlaneSrv.Shutdown(context.Background())
		if err != nil {
			logger.Errorw("Could not close control plane server.", zap.Error(err))
		}
	}
}

func buildControlPlane(ctx context.Context) *http.Server {
	serveMux := http.NewServeMux()
	serveMux.Handle("/application", controlplane.NewApplicationHandler(ctx))
	serveMux.Handle("/process", controlplane.NewProcessHandler(ctx))

	return &http.Server{
		Addr:    ":8080",
		Handler: serveMux,
	}
}

func flush(logger *zap.SugaredLogger) {
	logger.Sync()
	metrics.FlushExporter()
}
