package gormdb

import (
	"context"
	"time"

	biblegorm "github.com/StephenQiu30/lanverse/backend/internal/production/bible/adapter/gormdb"
	bibleapp "github.com/StephenQiu30/lanverse/backend/internal/production/bible/application"
	bibledomain "github.com/StephenQiu30/lanverse/backend/internal/production/bible/domain"
	app "github.com/StephenQiu30/lanverse/backend/internal/production/creation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/production/creation/domain"
	storyboardgorm "github.com/StephenQiu30/lanverse/backend/internal/production/storyboard/adapter/gormdb"
	storyboardapp "github.com/StephenQiu30/lanverse/backend/internal/production/storyboard/application"
	storyboarddomain "github.com/StephenQiu30/lanverse/backend/internal/production/storyboard/domain"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func applyWorldOrIntent(ctx context.Context, tx *gorm.DB, run domain.Run, p domain.Proposal, actor app.Actor, input app.AdoptCommand, mapping map[string]string, result *domain.AdoptionReceipt, now time.Time) (string, string, error) {
	id := uuid.NewString()
	if p.Stage == "build_world" {
		risks := make([]bibledomain.TextWorldRiskResolution, 0, len(input.RiskResolutions))
		for _, risk := range input.RiskResolutions {
			risks = append(risks, bibledomain.TextWorldRiskResolution{Code: risk.Code, Scope: risk.Scope, Reason: risk.Reason})
		}
		value, err := biblegorm.AcceptTextWorld(ctx, tx, bibleapp.TextWorldInput{Scope: bibleapp.TextWorldScope{ID: id, RunID: run.Command.RunID, ProposalID: p.ID, DecisionID: input.DecisionID, WorkspaceID: run.Command.WorkspaceID, ProjectID: run.Command.ProjectID, SourceRevisionID: p.SourceRevisionID, SourceHash: p.SourceHash, CreatedBy: actor.UserID, CreatedAt: now}, Candidate: p.Candidate, Task: p.Draft.Task, IDMapping: mapping, RiskResolutions: risks})
		if err != nil {
			return "", "", err
		}
		result.IDMapping = value.IDMapping
		result.FormalRefs = append(result.FormalRefs, domain.ResourceRef{Owner: "production/bible", Type: "text_world_version", ID: value.ID, Revision: value.Revision, ContentHash: value.ContentHash})
		return "production/bible", "creation.text.build_world", nil
	}
	if p.Stage == "direct_scene" {
		risks := make([]storyboarddomain.TextIntentRiskResolution, 0, len(input.RiskResolutions))
		for _, risk := range input.RiskResolutions {
			risks = append(risks, storyboarddomain.TextIntentRiskResolution{Code: risk.Code, Scope: risk.Scope, Reason: risk.Reason})
		}
		value, err := storyboardgorm.AcceptTextIntent(ctx, tx, storyboardapp.TextIntentInput{Scope: storyboardapp.TextIntentScope{ID: id, RunID: run.Command.RunID, ProposalID: p.ID, DecisionID: input.DecisionID, WorkspaceID: run.Command.WorkspaceID, ProjectID: run.Command.ProjectID, SourceRevisionID: p.SourceRevisionID, SourceHash: p.SourceHash, CreatedBy: actor.UserID, CreatedAt: now}, Candidate: p.Candidate, Task: p.Draft.Task, IDMapping: mapping, RiskResolutions: risks})
		if err != nil {
			return "", "", err
		}
		result.IDMapping = value.IDMapping
		result.FormalRefs = append(result.FormalRefs, domain.ResourceRef{Owner: "production/storyboard", Type: "text_intent_version", ID: value.ID, Revision: value.Revision, ContentHash: value.ContentHash})
		return "production/storyboard", "creation.text.direct_scene", nil
	}
	return "", "", app.Problem("unsupported_creation_stage", 422)
}
