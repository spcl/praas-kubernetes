package control_plane

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"go.uber.org/zap"
	k8corev1 "k8s.io/api/core/v1"
	k8v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/client/pkg/serving"
	"knative.dev/pkg/injection"
	"knative.dev/pkg/logging"
	"knative.dev/serving/pkg/apis/autoscaling"
	servingv1 "knative.dev/serving/pkg/apis/serving/v1"
	knv1 "knative.dev/serving/pkg/client/clientset/versioned/typed/serving/v1"
	servingv1client "knative.dev/serving/pkg/client/clientset/versioned/typed/serving/v1"
	"praas/pkg/networking"
	"praas/pkg/resources"
	"praas/pkg/scaling"
	"praas/pkg/store"
)

type appHandler struct {
	logger    *zap.SugaredLogger
	client    knv1.ServingV1Interface
	store     store.StateStore
	cfg       *resources.PraasConfig
	parentCtx context.Context
}

func NewApplicationHandler(ctx context.Context) http.Handler {
	client, err := servingv1client.NewForConfig(injection.GetConfig(ctx))
	if err != nil {
		panic("Could not get serving client")
	}
	return &appHandler{
		logger:    logging.FromContext(ctx),
		client:    client,
		store:     store.GetStateStore(ctx),
		cfg:       resources.GetPraasConfig(ctx),
		parentCtx: ctx,
	}
}

type appRequest struct {
	AppID int64 `json:"app-id"`
}

type appCreationResponse struct {
	networking.ResponseBase
	Data struct {
		AppID int64 `json:"app-id"`
	} `json:"data"`
}

func (p *appHandler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	// Set up a new context for this request
	ctx, cancel := context.WithTimeout(p.parentCtx, requestTimeout)
	defer cancel()

	switch request.Method {
	case http.MethodPost:
		p.handleCreation(ctx, writer, request)
	case http.MethodDelete:
		p.handleDeletion(ctx, writer, request)
	default:
		handleBadRequest(writer, unknownMethod(request))
	}
}

func (p *appHandler) handleCreation(ctx context.Context, writer http.ResponseWriter, request *http.Request) {
	app, err := p.createApp(ctx)
	if err != nil {
		p.logger.Errorw("Failed to start new app", zap.Error(err))
		handleServerError(writer, err)
		return
	}
	response, err := json.Marshal(
		&appCreationResponse{
			ResponseBase: networking.ResponseBase{Success: true},
			Data: struct {
				AppID int64 `json:"app-id"`
			}{AppID: app.ID},
		},
	)
	if err != nil {
		p.logger.Errorw("Failed to marshal response", zap.Error(err))
		handleServerError(writer, err)
		return
	}
	writer.WriteHeader(200)
	_, _ = writer.Write(response)
}

func (p *appHandler) handleDeletion(ctx context.Context, writer http.ResponseWriter, request *http.Request) {
	var req appRequest
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	err := decoder.Decode(&req)
	if err != nil {
		handleServerError(writer, err)
		return
	}

	app, err := p.store.GetApp(ctx, req.AppID)
	if err != nil {
		handleServerError(writer, err)
		return
	}
	err = p.deleteApp(ctx, app)
	if err != nil {
		handleServerError(writer, err)
		return
	}
	response, err := json.Marshal(&networking.ResponseBase{Success: true})
	if err != nil {
		p.logger.Errorw("Failed to marshal response", zap.Error(err))
		handleServerError(writer, err)
		return
	}
	writer.WriteHeader(200)
	_, _ = writer.Write(response)
}

func (p *appHandler) createApp(ctx context.Context) (*store.PraasApp, error) {
	// Post a new service object
	appID := p.store.GetNextAppID(ctx)
	namespace := p.cfg.ContainerNamespace
	image := p.cfg.ContainerImage
	service := &servingv1.Service{
		ObjectMeta: k8v1.ObjectMeta{
			Name:      fmt.Sprint("app-", appID),
			Namespace: namespace,
		},
	}
	service.Spec.Template = servingv1.RevisionTemplateSpec{
		Spec: servingv1.RevisionSpec{},
		ObjectMeta: k8v1.ObjectMeta{
			Annotations: map[string]string{
				serving.UserImageAnnotationKey:        image,
				autoscaling.ClassAnnotationKey:        resources.PraasAutoScalerName,
				autoscaling.InitialScaleAnnotationKey: "0",
				scaling.DeletionCostAnnotation:        scaling.MaxDeletionCost,
			},
			Labels: map[string]string{
				resources.AppIDLabelKey: fmt.Sprint(appID),
				resources.PIDLabelKey:   "",
			},
			// DeletionGracePeriodSeconds: &p.cfg.DeleteGracePeriodSeconds,
		},
	}
	service.Spec.Template.Spec.Containers = []k8corev1.Container{{
		Image:           image,
		ImagePullPolicy: k8corev1.PullIfNotPresent,
		Env: []k8corev1.EnvVar{
			{Name: "PROCESS_ID", ValueFrom: &k8corev1.EnvVarSource{FieldRef: &k8corev1.ObjectFieldSelector{FieldPath: "metadata.labels['" + resources.PIDLabelKey + "']"}}},
		},
	}}

	// Submit the service object to knative
	servicesGetter := p.client.Services(namespace)
	if servicesGetter == nil {
		p.logger.Warn("Service getter is nil")
		return nil, nil
	}
	service, err := servicesGetter.Create(ctx, service, k8v1.CreateOptions{})
	if err != nil {
		p.logger.Errorw("Failed to create_op service", zap.Error(err))
		return nil, err
	}
	p.logger.Infof("Created service with appID: %d", appID)

	// Post our representation of the process and store it in the state store
	app := &store.PraasApp{
		ID:               appID,
		ServiceNamespace: namespace,
		ServiceName:      service.GetName(),
	}

	err = p.store.StoreApp(ctx, app)
	if err != nil {
		// Should we roll back the process creation?
		return nil, err
	}

	return app, nil
}

func (p *appHandler) deleteApp(ctx context.Context, app *store.PraasApp) error {
	err := p.store.DeleteApp(ctx, app)
	if err != nil {
		return err
	}
	return p.client.Services(app.ServiceNamespace).Delete(ctx, app.ServiceName, k8v1.DeleteOptions{})
}
