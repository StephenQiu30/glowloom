package workflow_test

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	generationgorm "github.com/StephenQiu30/lanverse/backend/internal/generation/adapter/gormdb"
	genapp "github.com/StephenQiu30/lanverse/backend/internal/generation/application"
	gen "github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	generationtestgorm "github.com/StephenQiu30/lanverse/backend/tests/generation/adapter/gormdb"
	"github.com/google/uuid"
)

func assertCompiledBaseVisionReviewInput(t *testing.T, ctx context.Context, database *generationtestgorm.Database, actor genapp.Actor, execution gen.ReferenceExecution, bundles gen.ReferenceBundleInputCollection) {
	t.Helper()
	store := generationgorm.New(database)
	// No Vision Release or Invocation is claimed by this input-preparation test.
	stageHash := strings.Repeat("e", 64)
	readyIndex := -1
	var accepted contract.VisionReviewInput
	for index, bundle := range bundles.Bundles {
		input, err := store.CompileBaseVisionReviewInput(ctx, actor, execution.ProjectID, execution.ID, index, stageHash)
		if !bundle.Admission.InternalReviewReady {
			if err == nil || !reflect.DeepEqual(input, contract.VisionReviewInput{}) {
				t.Fatal("failed Bundle prepared review input")
			}
			continue
		}
		if err != nil || input.Validate() != nil || input.Subject.BundleInputRef.ContentHash != bundle.Input.ContentHash || input.BriefContentHash == "" || input.VisualContext.VisualGrammar.Medium == "" || input.Admission.SelectionReady || input.Admission.PublicationReady {
			t.Fatalf("compile persisted complete Vision input: %v", err)
		}
		repeated, err := store.CompileBaseVisionReviewInput(ctx, actor, execution.ProjectID, execution.ID, index, stageHash)
		if err != nil || !reflect.DeepEqual(repeated, input) {
			t.Fatalf("persisted Vision input replay: %v", err)
		}
		readyIndex, accepted = index, input
	}
	if readyIndex < 0 {
		t.Fatal("no complete Bundle exercised Vision input preparation")
	}
	for _, fault := range []string{"actor", "token", "project", "execution", "bundle", "release", "outer_transaction"} {
		reader, who, project, id, index, release := store, actor, execution.ProjectID, execution.ID, readyIndex, stageHash
		switch fault {
		case "actor":
			who.UserID = uuid.NewString()
		case "token":
			who.TokenVersion++
		case "project":
			project = uuid.NewString()
		case "execution":
			id = uuid.NewString()
		case "bundle":
			index = -1
		case "release":
			release = "latest"
		case "outer_transaction":
			tx := database.Begin()
			if tx.Error != nil {
				t.Fatal(tx.Error)
			}
			defer tx.Rollback()
			reader = generationgorm.New(tx)
		}
		if value, err := reader.CompileBaseVisionReviewInput(ctx, who, project, id, index, release); err == nil || !reflect.DeepEqual(value, contract.VisionReviewInput{}) {
			t.Fatalf("Vision input accepted %s", fault)
		}
	}
	var policy model.EffectivePolicySnapshot
	if err := database.WithContext(ctx).First(&policy, "id = ? AND project_id = ?", accepted.BriefInput.EffectivePolicySnapshotRef.OwnerVersionID, execution.ProjectID).Error; err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal(policy.Content, &body); err != nil {
		t.Fatal(err)
	}
	body["fidelity_invariants"] = []any{}
	changed, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := database.Model(&model.EffectivePolicySnapshot{}).Where("id = ? AND project_id = ?", policy.ID, execution.ProjectID).UpdateColumn("content", policy.Content).Error; err != nil {
			t.Error(err)
		}
	}()
	if err := database.Model(&model.EffectivePolicySnapshot{}).Where("id = ? AND project_id = ?", policy.ID, execution.ProjectID).UpdateColumn("content", json.RawMessage(changed)).Error; err != nil {
		t.Fatalf("inject owned Policy content drift: %v", err)
	}
	if _, err := store.CompileBaseVisionReviewInput(ctx, actor, execution.ProjectID, execution.ID, readyIndex, stageHash); err == nil {
		t.Fatal("Vision input ignored mutated effective Policy contents")
	}
	if err := database.Model(&model.EffectivePolicySnapshot{}).Where("id = ? AND project_id = ?", policy.ID, execution.ProjectID).UpdateColumn("content", policy.Content).Error; err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := database.Model(&model.GenerationReferenceExecutionHead{}).Where("project_id = ? AND target_id = ?", execution.ProjectID, execution.ReadSet.TargetRef.ID).UpdateColumn("current_execution_hash", execution.ContentHash).Error; err != nil {
			t.Error(err)
		}
	}()
	if err := database.Model(&model.GenerationReferenceExecutionHead{}).Where("project_id = ? AND target_id = ?", execution.ProjectID, execution.ReadSet.TargetRef.ID).UpdateColumn("current_execution_hash", strings.Repeat("f", 64)).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := store.CompileBaseVisionReviewInput(ctx, actor, execution.ProjectID, execution.ID, readyIndex, stageHash); err == nil {
		t.Fatal("Vision input ignored the current Execution Head")
	}
}
