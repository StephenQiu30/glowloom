// @title Agent business API
// @version 1.0
// @description Service-authenticated Agent business boundary.
// @securityDefinitions.apikey ServiceIdentity
// @in header
// @name Authorization
package http

import (
	"encoding/json"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain"
)

type businessAccepted struct {
	Accepted bool `json:"accepted"`
}
type businessValid struct {
	Valid bool `json:"valid"`
}
type businessInputSources struct {
	Grant domain.Grant    `json:"grant"`
	Draft json.RawMessage `json:"draft" swaggertype:"object"`
}
type businessSkillDiscover struct {
	Grant domain.Grant `json:"grant"`
}
type businessSkillLoad struct {
	Grant        domain.Grant `json:"grant"`
	Version      string       `json:"version"`
	ResourceHash string       `json:"resource_hash"`
	ResourcePath string       `json:"resource_path,omitempty"`
}

// businessProposal matches the response exposed by the proposal endpoint.
type businessProposal struct {
	ProposalID            uuid.UUID       `json:"proposal_id"`
	RootRunID             uuid.UUID       `json:"root_run_id"`
	CandidateHash         string          `json:"candidate_hash"`
	AuthorizationRevision int64           `json:"authorization_revision"`
	Candidate             json.RawMessage `json:"candidate" swaggertype:"object"`
	CanvasRevision        int64           `json:"canvas_revision"`
	PriceVersion          string          `json:"price_version"`
	BudgetMicros          int64           `json:"budget_micros"`
	Control               struct {
		Terminate bool `json:"terminate"`
	} `json:"control"`
}
