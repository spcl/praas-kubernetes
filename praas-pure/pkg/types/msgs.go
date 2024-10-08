package types

type ProcessCoordinationMsg struct {
	Peers     []string `json:"peers"`
	ProcessID string   `json:"process_id"`
}
