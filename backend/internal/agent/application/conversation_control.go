package application

import (
	"context"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

// ControlStatus exposes business decisions, never a copy of Agent execution state.
type ControlStatus struct {
	AuthorizationRevision int64                  `json:"authorization_revision"`
	CurrentDecision       *domain.Decision       `json:"current_decision,omitempty"`
	DeliveryStatus        string                 `json:"delivery_status,omitempty"`
	ApplicationReceipt    *domain.AppliedReceipt `json:"application_receipt,omitempty"`
	RequestDecision       *domain.Decision       `json:"request_decision,omitempty"`
	RequestDeliveryStatus string                 `json:"request_delivery_status,omitempty"`
	RequestReceipt        *domain.AppliedReceipt `json:"request_receipt,omitempty"`
}

// ControlStatusReader reads authorized control status and unresolved delivery facts.
type ControlStatusReader interface {
	ReadControlStatus(context.Context, identity.Principal, uuid.UUID, uuid.UUID, uuid.UUID, *uuid.UUID) (ControlStatus, error)
}
