package server

type Request map[string]any

type Response struct {
	Success bool   `json:"success"`
	Reason  string `json:"reason,omitempty"`
	Data    any    `json:"data,omitempty"`
}
