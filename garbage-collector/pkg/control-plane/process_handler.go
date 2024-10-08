package control_plane

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"go.uber.org/zap"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/pkg/injection"
	"knative.dev/pkg/logging"
	v1alpha12 "knative.dev/serving/pkg/apis/autoscaling/v1alpha1"
	servingv1 "knative.dev/serving/pkg/apis/serving/v1"
	knv1 "knative.dev/serving/pkg/client/clientset/versioned/typed/serving/v1"
	servingv1client "knative.dev/serving/pkg/client/clientset/versioned/typed/serving/v1"
	"knative.dev/serving/pkg/client/injection/informers/autoscaling/v1alpha1/podautoscaler"
	"knative.dev/serving/pkg/client/listers/autoscaling/v1alpha1"
	"praas/pkg/networking"
	"praas/pkg/reconciler/garbagecollector/metrics"
	"praas/pkg/scaling"
	"praas/pkg/store"
)

var (
	missingPayloadError = errors.New("missing field: payload")
	missingPIDError     = errors.New("missing field: pid")
	noSuccess           = errors.New("no requested operation succeeded")
)

type processArray struct {
	Processes []createdProcessData `json:"processes"`
}

type createdProcessData struct {
	Pod string `json:"pod_name"`
	PID string `json:"pid"`
}

type processCreationResponse struct {
	networking.ResponseBase
	Data processArray `json:"data"`
}

type processRequest struct {
	AppID     int64 `json:"app-id"`
	Processes []store.ProcessData
}

type sessionHandler struct {
	logger    *zap.SugaredLogger
	scaler    *scaling.Scaler
	paLister  v1alpha1.PodAutoscalerLister
	store     store.StateStore
	client    knv1.ServingV1Interface
	parentCtx context.Context
	collector *metrics.MetricCollector
}

// NewProcessHandler creates an object that can handle http requests against the /session API
func NewProcessHandler(ctx context.Context) http.Handler {
	client, err := servingv1client.NewForConfig(injection.GetConfig(ctx))
	if err != nil {
		panic("Could not get serving client")
	}
	return &sessionHandler{
		logger:    logging.FromContext(ctx),
		scaler:    scaling.GetScaler(ctx),
		paLister:  podautoscaler.Get(ctx).Lister(),
		store:     store.GetStateStore(ctx),
		collector: metrics.GetCollector(ctx),
		client:    client,
		parentCtx: ctx,
	}
}

func (s *sessionHandler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	// Set up a new context with a timeout for this request
	ctx, cancel := context.WithTimeout(s.parentCtx, requestTimeout)
	defer cancel()

	switch request.Method {
	case http.MethodPost:
		s.handleCreation(ctx, writer, request)
	default:
		handleBadRequest(writer, unknownMethod(request))
	}
}

func (s *sessionHandler) handleCreation(ctx context.Context, writer http.ResponseWriter, request *http.Request) {
	startTime := time.Now()

	// Decode request
	var req processRequest
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	err := decoder.Decode(&req)
	if err != nil {
		handleServerError(writer, err)
		return
	}

	if len(req.Processes) == 0 {
		handleBadRequest(writer, "no sessions were given")
		return
	}

	// Create new context with timeout based on the number of sessions
	ctx, cancel := context.WithTimeout(s.parentCtx, time.Duration(len(req.Processes))*time.Minute)
	defer cancel()

	// Get the PraasApp and the PodAutoscaler
	app, err := s.store.GetApp(ctx, req.AppID)
	if err != nil {
		handleServerError(writer, err)
		return
	}
	pa, err := s.getPA(ctx, app)
	if err != nil {
		handleServerError(writer, err)
		return
	}

	// Increase the scale
	ids, pods, err := s.scaler.NewProcesses(ctx, pa, app, req.Processes)
	if err != nil {
		handleServerError(writer, err)
		return
	}
	if len(pods) == 0 {
		handleServerError(writer, noSuccess)
		return
	}

	// Try to start collecting sooner
	_ = s.collector.CreateOrUpdate(
		&v1alpha12.Metric{
			TypeMeta:   pa.TypeMeta,
			ObjectMeta: pa.ObjectMeta,
		},
	)

	data := make([]createdProcessData, len(ids))
	for idx, id := range ids {
		data[idx].PID = id
		data[idx].Pod = pods[idx]
	}

	// Send response
	response, err := json.Marshal(
		&processCreationResponse{
			ResponseBase: networking.ResponseBase{Success: true},
			Data:         processArray{Processes: data},
		},
	)
	if err != nil {
		handleServerError(writer, err)
		return
	}
	writer.WriteHeader(200)
	_, _ = writer.Write(response)
	endTime := time.Now()
	s.logger.Infof("Process allocation time: %d ms", endTime.Sub(startTime).Milliseconds())
}

func (s *sessionHandler) getPA(
	ctx context.Context,
	app *store.PraasApp,
) (*v1alpha12.PodAutoscaler, error) {
	// Get service (and make sure it's ready)
	hasRevision := false
	var service *servingv1.Service
	var err error
	for !hasRevision {
		service, err = s.client.Services(app.ServiceNamespace).Get(ctx, app.ServiceName, v1.GetOptions{})
		if err != nil {
			return nil, err
		}
		if service.Status.LatestReadyRevisionName != "" {
			hasRevision = true
		}
	}

	// Get the name of the revision and the PA
	latestRevisionName := service.Status.LatestReadyRevisionName
	pa, err := s.paLister.PodAutoscalers(app.ServiceNamespace).Get(latestRevisionName)
	if err != nil {
		s.logger.Errorw("Failed to get PA when creating session", zap.Error(err), zap.Any("service", service))
		return nil, err
	}

	return pa, err
}
