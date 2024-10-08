package store

import (
	"context"
	"errors"

	"k8s.io/apimachinery/pkg/types"
)

var (
	NotReusableAnymoreError   = errors.New("pod is not reusable anymore, either it's been deleted or reused")
	sessionAlreadyActiveError = errors.New("session already active")
	ProcessNotFound           = errors.New("could not find given process")
	sessionNotActiveError     = errors.New("session is not active")
	CoolDownError             = errors.New("operation waiting for cool down")
)

// PraasApp represents a process in the system.
type PraasApp struct {
	ID               int64
	ServiceNamespace string
	ServiceName      string
}

type ProcessData struct {
	PID string `json:"pid,omitempty"`
}

func GetStateStore(ctx context.Context) StateStore {
	return getRedisStore(ctx)
}

// StateStore is the interface all praas control plane state stores will have to obey.
type StateStore interface {
	StoreApp(context.Context, *PraasApp) error
	DeleteApp(context.Context, *PraasApp) error
	GetApp(context.Context, int64) (*PraasApp, error)
	GetAppByNamespacedName(context.Context, types.NamespacedName) (*PraasApp, error)
	GetDeletablePods(context.Context, *PraasApp, []string) (found, notFound []string, err error)
	CreateProcess(context.Context, *PraasApp, string, bool) error
	DeleteProcess(context.Context, *PraasApp, string) error
	GetNextAppID(ctx context.Context) int64
	LockApp(context.Context, *PraasApp) (isLocked bool, err error)
	UnLockApp(context.Context, *PraasApp)
	GetCurrentScale(ctx context.Context, process *PraasApp) int32
	MarkAsReused(context.Context, *PraasApp, string, string, bool) error
	MapPodToProcess(context.Context, *PraasApp, string, string) error
}
