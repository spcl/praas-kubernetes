package store

import (
	"context"
	"errors"

	"praas/pkg/types"
)

var (
	AppDoesNotExist      = errors.New("there is no app with the given pid")
	ProcessAlreadyActive = errors.New("process already active")
	InvalidPID           = errors.New("invalid AppID")
)

func GetStateStore(ctx context.Context) StateStore {
	return getRedisStore(ctx)
}

// StateStore is the interface all praas control plane state stores will have to obey.
type StateStore interface {
	CreateApp(ctx context.Context) (types.IDType, error)
	DeleteApp(context.Context, types.IDType) error

	NewProcess(context.Context, types.ProcessRef, bool) ([]string, error)
	DeleteProcess(context.Context, types.ProcessRef) error
	ProcessExistsAndActive(ctx context.Context, session types.ProcessRef) bool
	GetProcesses(ctx context.Context, appID types.IDType) ([]string, []string, error)
	GetProcessForFunc(ctx context.Context, appID types.IDType, funcLimit int) (string, []string, error)
	AllocFunc(ctx context.Context, appID types.IDType, pid string) error
	DeAllocFunc(ctx context.Context, appID types.IDType, pid string) error
}
