package control_plane

import (
	"context"
	"errors"
	"fmt"
	"net/url"

	"go.uber.org/zap"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"praas/pkg/config"
	"praas/pkg/control_plane/store"
	"praas/pkg/types"
)

var (
	failedToCleanup       = errors.New("failed to delete process from state")
	failedToDeletePods    = errors.New("failed to delete pods belonging to process")
	unexpectedFieldsError = errors.New("unexpected fields in request")
)

var _ K8APIObjectHandler = (*appHandler)(nil)

func NewApplicationHandler(ctx context.Context) K8APIObjectHandler {
	return &appHandler{
		buildBase(ctx),
	}
}

type appHandler struct {
	objectHandlerBase
}

func (p *appHandler) Post(ctx context.Context, _ map[string]any) (any, error) {
	pid, err := p.store.CreateApp(ctx)
	if err != nil {
		return nil, err
	}

	return map[string]any{appIDKey: pid}, nil
}

func (p *appHandler) Delete(ctx context.Context, input map[string]any) (any, error) {
	var appID types.IDType
	err := parseData(input, asID(appIDKey, &appID))
	if err != nil {
		return nil, err
	}
	if len(input) != 1 {
		return nil, unexpectedFieldsError
	}

	err = p.store.DeleteApp(ctx, appID)
	if err != nil {
		return nil, failedToCleanup
	}

	err = p.deleteAppPods(ctx, appID)
	if err != nil {
		if errors.Is(err, store.AppDoesNotExist) {
			return nil, err
		}

		p.logger.Errorw("Could not delete pods belonging to app", zap.Error(err))
		return nil, failedToDeletePods
	}

	return nil, nil
}

func (p *appHandler) deleteAppPods(ctx context.Context, pid types.IDType) error {
	selector := v1.ListOptions{LabelSelector: fmt.Sprintf("%s=%d", config.AppIDLabelKey, pid)}
	err := p.kubeClient.CoreV1().Pods(p.cfg.ContainerNamespace).DeleteCollection(ctx, v1.DeleteOptions{}, selector)
	return err
}

func (p *appHandler) Get(ctx context.Context, query url.Values) (any, error) {
	appIDStr := query.Get("app-id")
	if appIDStr == "" {
		return nil, fmt.Errorf("missing app-id field")
	}
	appID := types.IDFromString(appIDStr)
	if appID == types.InvalidAppID {
		return nil, fmt.Errorf("invalid app-id")
	}

	processes, activeProcesses, err := p.store.GetProcesses(ctx, appID)
	if err != nil {
		return nil, fmt.Errorf("could not read data for " + appIDStr)
	}
	return map[string]any{"processes": processes, "active": activeProcesses}, nil
}
