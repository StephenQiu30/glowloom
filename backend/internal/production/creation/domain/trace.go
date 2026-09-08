package domain

import "time"

type StageDescriptor struct {
	StepKey        string `json:"step_key"`
	Title          string `json:"title"`
	Scope          string `json:"scope"`
	ReviewRequired bool   `json:"review_required"`
	ReviewKey      string `json:"review_key"`
	InstanceCount  *int   `json:"instance_count"`
}
type PlanTemplate struct {
	Version  int               `json:"version"`
	FlowType string            `json:"flow_type"`
	Stages   []StageDescriptor `json:"stages"`
}
type Manifest struct {
	Version           int          `json:"version"`
	CommandID         string       `json:"command_id"`
	RunID             string       `json:"run_id"`
	PayloadHash       string       `json:"payload_hash"`
	SourceRevisionID  string       `json:"source_revision_id"`
	SourceContentHash string       `json:"source_content_hash"`
	ReleaseHash       string       `json:"release_hash"`
	CallLimit         int          `json:"call_limit"`
	Template          PlanTemplate `json:"template"`
	TemplateHash      string       `json:"template_hash"`
}
type ManifestSnapshot struct {
	Schema       string    `json:"schema"`
	CommandID    string    `json:"command_id"`
	RunID        string    `json:"run_id"`
	Availability string    `json:"availability"`
	Manifest     *Manifest `json:"manifest"`
	ManifestHash *string   `json:"manifest_hash"`
}
type Attempt struct {
	AttemptID         string     `json:"attempt_id"`
	AttemptNo         int        `json:"attempt_no"`
	Fence             int64      `json:"fence"`
	InputHash         string     `json:"input_hash"`
	State             string     `json:"state"`
	StartedAt         time.Time  `json:"started_at"`
	ExecutionDeadline time.Time  `json:"execution_deadline"`
	LeaseExpiresAt    time.Time  `json:"lease_expires_at"`
	FinishedAt        *time.Time `json:"finished_at"`
	ResultHash        *string    `json:"result_hash"`
	LastError         *string    `json:"last_error"`
	UsageStatus       string     `json:"usage_status"`
	LeaseExpired      bool       `json:"lease_expired"`
}
type AttemptHistory struct {
	Schema           string    `json:"schema"`
	CommandID        string    `json:"command_id"`
	RunID            string    `json:"run_id"`
	StepID           string    `json:"step_id"`
	StepKey          string    `json:"step_key"`
	CurrentAttemptID *string   `json:"current_attempt_id"`
	HistoryOrigin    string    `json:"history_origin"`
	Attempts         []Attempt `json:"attempts"`
}
