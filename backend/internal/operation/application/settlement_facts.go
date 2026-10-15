package application

import (
	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/domain"
)

// SettlementFacts contains no prompt, media, output or private event data.
type SettlementFacts struct {
	Status     domain.Status
	ProjectID  uuid.UUID
	Capability string
	Region     *string
	ModelKey   string
	PriceUnit  string
}
