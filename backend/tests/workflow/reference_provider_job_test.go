package workflow_test

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	agentapp "github.com/StephenQiu30/lanverse/backend/internal/agent/application"
	generationgorm "github.com/StephenQiu30/lanverse/backend/internal/generation/adapter/gormdb"
	openaiadapter "github.com/StephenQiu30/lanverse/backend/internal/generation/adapter/openai"
	generationapp "github.com/StephenQiu30/lanverse/backend/internal/generation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	generationtestgorm "github.com/StephenQiu30/lanverse/backend/tests/generation/adapter/gormdb"
	"github.com/google/uuid"
)

func assertReferenceProviderJobPersistence(t *testing.T, ctx context.Context, database *generationtestgorm.Database, actor generationapp.Actor, fixture referencePreparationFixture, target generationapp.ReferenceGenerationTarget, brief agentapp.AcceptedReferenceBrief, profile domain.ProviderModelProfileVersion) {
	t.Helper()
	compiled, err := openaiadapter.NewFactory(nil, nil, nil).CompileReferenceImages(target, brief, profile)
	if err != nil {
		t.Fatal(err)
	}
	inputs := make([]domain.ReferenceProviderCallInput, len(compiled.Requests))
	for i, request := range compiled.Requests {
		inputs[i] = domain.ReferenceProviderCallInput{BundleIndex: request.BundleIndex, SlotKey: request.SlotKey, CompiledRequestHash: request.ContentHash}
	}
	execution := fixture.execution
	executionRef := domain.GenerationRevisionRef{ID: execution.ID, Revision: execution.Revision, ContentHash: execution.ContentHash}
	expectedJob, expectedCalls, err := domain.BuildReferenceProviderJob(executionRef, inputs)
	if err != nil {
		t.Fatal(err)
	}
	store := generationgorm.New(database)
	err = store.WithinReferenceExecution(ctx, func(repo generationapp.ReferenceExecutionRepository) error {
		job, calls, err := repo.FindReferenceProviderJob(ctx, execution.WorkspaceID, execution.ProjectID, execution.ID)
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(job, expectedJob) || !reflect.DeepEqual(calls, expectedCalls) {
			t.Fatal("persisted Provider call set differs from exact compiled requests")
		}
		for _, scope := range [][2]string{{uuid.NewString(), execution.ProjectID}, {execution.WorkspaceID, uuid.NewString()}} {
			if _, _, err := repo.FindReferenceProviderJob(ctx, scope[0], scope[1], execution.ID); err == nil {
				t.Fatal("cross-scope job read accepted")
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	var rows []model.GenerationReferenceProviderCall
	if err := database.Where("execution_id = ?", execution.ID).Order("bundle_index, slot_key").Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) != len(expectedCalls) {
		t.Fatal("incomplete persisted call set")
	}
	for _, row := range rows {
		if row.Status != domain.ProviderCallPending || row.Revision != 1 {
			t.Fatal("preparation granted a send right")
		}
		for _, forbidden := range []string{"prompt", "api_key", "ciphertext", "https://", "base64"} {
			if strings.Contains(string(row.Content), forbidden) {
				t.Fatal("call identity leaked invocation material")
			}
		}
	}
	for _, fault := range []string{"job_hash", "job_content", "call_request", "call_content", "call_scope", "missing_call", "extra_call"} {
		if err := database.SavePoint("reference_call_set_fault").Error; err != nil {
			t.Fatal(err)
		}
		call := rows[0]
		switch fault {
		case "job_hash":
			err = database.Model(&model.GenerationReferenceProviderJob{}).Where("execution_id = ?", execution.ID).UpdateColumns(map[string]any{"content_hash": strings.Repeat("f", 64)}).Error
		case "job_content":
			bad := expectedJob
			bad.CallKeys = bad.CallKeys[:len(bad.CallKeys)-1]
			raw, _ := json.Marshal(bad)
			err = database.Model(&model.GenerationReferenceProviderJob{}).Where("execution_id = ?", execution.ID).UpdateColumns(map[string]any{"content": string(raw)}).Error
		case "call_request":
			err = database.Model(&call).Where("call_key = ?", call.CallKey).UpdateColumns(map[string]any{"compiled_request_hash": strings.Repeat("f", 64)}).Error
		case "call_content":
			bad := expectedCalls[0]
			bad.SlotKey = "unrelated"
			raw, _ := json.Marshal(bad)
			err = database.Model(&call).Where("call_key = ?", call.CallKey).UpdateColumns(map[string]any{"content": string(raw)}).Error
		case "call_scope":
			err = database.Model(&call).Where("call_key = ?", call.CallKey).UpdateColumns(map[string]any{"workspace_id": uuid.New()}).Error
		case "missing_call":
			err = generationtestgorm.DeleteWithoutHooks(database, &model.GenerationReferenceProviderCall{}, "call_key = ?", call.CallKey)
		case "extra_call":
			_, extra, buildErr := domain.BuildReferenceProviderJob(executionRef, []domain.ReferenceProviderCallInput{{SlotKey: "unrelated", CompiledRequestHash: strings.Repeat("a", 64)}})
			if buildErr != nil {
				t.Fatal(buildErr)
			}
			call.CallKey, call.SlotKey, call.CompiledRequestHash = extra[0].CallKey, extra[0].SlotKey, extra[0].CompiledRequestHash
			call.BundleIndex = 0
			call.Content, err = json.Marshal(extra[0])
			if err == nil {
				err = database.Omit("Job").Create(&call).Error
			}
		}
		if err != nil {
			t.Fatalf("inject %s: %v", fault, err)
		}
		assertReferencePreparationRejected(t, ctx, actor, fixture, false)
		assertReferencePreparationRejected(t, ctx, actor, fixture, true)
		if err := database.RollbackTo("reference_call_set_fault").Error; err != nil {
			t.Fatal(err)
		}
	}
	// A changed request hash must not create another invocation for one slot.
	if err := database.SavePoint("reference_call_slot_unique").Error; err != nil {
		t.Fatal(err)
	}
	duplicate := rows[0]
	duplicate.CallKey = strings.Repeat("f", 64)
	duplicate.CompiledRequestHash = strings.Repeat("e", 64)
	if err := database.Omit("Job").Create(&duplicate).Error; err == nil {
		t.Fatal("same execution/slot accepted a second call")
	}
	if err := database.RollbackTo("reference_call_slot_unique").Error; err != nil {
		t.Fatal(err)
	}
	if replayed, err := fixture.service.PrepareInitial(ctx, actor, fixture.command); err != nil || !reflect.DeepEqual(replayed, fixture.execution) {
		t.Fatalf("valid replay after rollback: %v", err)
	}
}
