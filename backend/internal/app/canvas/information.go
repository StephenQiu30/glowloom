package canvas

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	mediawire "github.com/StephenQiu30/glowloom-server/backend/internal/app/media"
	billingpg "github.com/StephenQiu30/glowloom-server/backend/internal/billing/adapter/postgres"
	billingapp "github.com/StephenQiu30/glowloom-server/backend/internal/billing/application"
	canvasapp "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

type informationUsage struct{ tx *gorm.DB }
type informationBillingAccess struct{ tx *gorm.DB }

func (a informationBillingAccess) AuthorizeBilling(ctx context.Context, p identity.Principal, id uuid.UUID, write bool) error {
	return mediawire.NewCanvasProjectAccess(a.tx).AuthorizeProject(ctx, p, id, write)
}

// NewInformationUsage maps the billing-owned read model to the canvas query contract.
func NewInformationUsage(tx *gorm.DB) canvasapp.CanvasUsageReader { return informationUsage{tx: tx} }
func (r informationUsage) ReadCanvasUsage(ctx context.Context, a identity.Principal, p, c uuid.UUID, page int) (canvasapp.CanvasUsage, error) {
	facts, err := billingpg.NewScopedStore(r.tx, func(tx *gorm.DB) billingapp.ProjectAccess { return informationBillingAccess{tx: tx} }).ReadCanvasUsage(ctx, a, p, c, page)
	if err != nil {
		return canvasapp.CanvasUsage{}, err
	}
	result := canvasapp.CanvasUsage{Currency: "CNY", SettledMicros: facts.SettledMicros, TaskCount: facts.TaskCount, NextPage: facts.NextPage, Entries: []canvasapp.CanvasCharge{}}
	for _, e := range facts.Entries {
		result.Entries = append(result.Entries, canvasapp.CanvasCharge{ID: e.ID, OperationID: e.OperationID, AmountMicros: e.AmountMicros, CreatedAt: e.CreatedAt, ModelKey: e.ModelKey})
	}
	return result, nil
}
