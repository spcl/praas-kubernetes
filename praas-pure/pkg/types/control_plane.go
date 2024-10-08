package types

import (
	"fmt"
	"strconv"

	v1 "k8s.io/api/core/v1"
	"praas/pkg/config"
)

type IDType int

func IDFromString(s string) IDType {
	val, err := strconv.Atoi(s)
	if err != nil {
		return InvalidAppID
	} else {
		return IDType(val)
	}
}

func (p IDType) String() string {
	return strconv.Itoa(int(p))
}

type ProcessRef struct {
	AppID IDType
	PID   string
}

func InvalidProcess() ProcessRef {
	return ProcessRef{AppID: InvalidAppID}
}

const (
	InvalidAppID IDType = -1
)

func (p IDType) MarshalBinary() ([]byte, error) {
	return []byte(fmt.Sprint(p)), nil
}

func (s ProcessRef) String() string {
	return fmt.Sprintf("app-%d-%s", s.AppID, s.PID)
}

func PodToProcessRef(pod *v1.Pod) ProcessRef {
	pid := IDFromString(pod.Labels[config.AppIDLabelKey])
	if pid == InvalidAppID {
		return ProcessRef{AppID: InvalidAppID}
	}
	processID := pod.Labels[config.PIDLabelKey]
	return ProcessRef{AppID: pid, PID: processID}
}

type ProcessData struct {
	PID string `json:"pid,omitempty"`
}

type ProcessCreationData struct {
	PodName string `json:"pod_name"`
	PID     string `json:"pid"`
	IP      string `json:"ip"`
}
