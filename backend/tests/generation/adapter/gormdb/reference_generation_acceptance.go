package gormdb

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/StephenQiu30/lanverse/backend/internal/access/authentication"
	generationgorm "github.com/StephenQiu30/lanverse/backend/internal/generation/adapter/gormdb"
	generationhttp "github.com/StephenQiu30/lanverse/backend/internal/generation/adapter/httpapi"
	"github.com/StephenQiu30/lanverse/backend/internal/generation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// AssertReferenceGenerationProgressStorage reuses the real, committed Target and
// Execution published by the script journey. Every injected fault is restored.
func AssertReferenceGenerationProgressStorage(t *testing.T, ctx context.Context, database *gorm.DB, actor application.Actor, execution domain.ReferenceExecution) {
	t.Helper()
	query := application.NewReferenceGenerationQuery(generationgorm.New(database))
	targetID, projectID := execution.ReadSet.TargetRef.ID, execution.ProjectID
	factCounts := func() [8]int64 {
		t.Helper()
		var counts [8]int64
		for index, table := range []any{&model.CommandReceipt{}, &model.GenerationTarget{}, &model.GenerationReferenceTargetHead{}, &model.GenerationReferenceExecution{}, &model.GenerationReferenceExecutionHead{}, &model.GenerationReferenceProviderJob{}, &model.GenerationReferenceProviderCall{}, &model.GenerationReferenceStagedMedia{}} {
			if err := database.Model(table).Where("workspace_id = ?", execution.WorkspaceID).Count(&counts[index]).Error; err != nil {
				t.Fatal(err)
			}
		}
		return counts
	}
	countsBefore := factCounts()
	read := func() application.ReferenceGenerationProgress {
		t.Helper()
		value, err := query.Get(ctx, actor, projectID, targetID)
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
	before := read()
	progress, err := application.NewReferenceExecutionQuery(generationgorm.New(database)).Get(ctx, actor, projectID, execution.ID)
	if err != nil || before.GenerationTargetRef != execution.ReadSet.TargetRef || before.Execution == nil || !reflect.DeepEqual(*before.Execution, progress) {
		t.Fatalf("Target did not recover complete execution: %v", err)
	}
	if again := read(); !reflect.DeepEqual(before, again) {
		t.Fatal("read changed identity")
	}
	for _, fault := range []string{"actor", "token", "project", "target", "outer_transaction"} {
		who, project, target, reader := actor, projectID, targetID, query
		switch fault {
		case "actor":
			who.UserID = uuid.NewString()
		case "token":
			who.TokenVersion++
		case "project":
			project = uuid.NewString()
		case "target":
			target = uuid.NewString()
		case "outer_transaction":
			tx := database.Begin()
			if tx.Error != nil {
				t.Fatal(tx.Error)
			}
			defer tx.Rollback()
			reader = application.NewReferenceGenerationQuery(generationgorm.New(tx))
		}
		if value, err := reader.Get(ctx, who, project, target); err == nil || !reflect.DeepEqual(value, application.ReferenceGenerationProgress{}) {
			t.Fatalf("accepted %s", fault)
		}
	}
	var member model.Membership
	if err := database.Where("workspace_id = ? AND user_id = ?", execution.WorkspaceID, actor.UserID).First(&member).Error; err != nil {
		t.Fatal(err)
	}
	setRole := func(role string) error {
		return database.Model(&model.Membership{}).Where("id = ?", member.ID).UpdateColumn("role", role).Error
	}
	defer func() {
		if err := setRole(member.Role); err != nil {
			t.Error(err)
		}
	}()
	if err := setRole("viewer"); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, read()) {
		t.Fatal("viewer cannot read frozen execution")
	}
	if err := setRole(member.Role); err != nil {
		t.Fatal(err)
	}

	// The public handler uses the production JWT verifier, without provider or
	// private media calls. The returned projection is identical to the Owner read.
	secret := strings.Repeat("generation-query-test-only-", 3)
	issuer := authentication.NewIssuer(secret, "generation-test", "generation-test", time.Hour, time.Now, uuid.NewString)
	token, err := issuer.Issue(actor.UserID, actor.TokenVersion)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	generationhttp.NewReferenceGenerationHandler(query, authentication.NewVerifier(secret, "generation-test", "generation-test", time.Now)).Register(mux)
	request := httptest.NewRequest(http.MethodGet, "/api/projects/"+projectID+"/reference-generation-targets/"+targetID, nil).WithContext(ctx)
	request.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	var envelope struct {
		Data application.ReferenceGenerationProgress `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil || response.Code != 200 || response.Header().Get("Cache-Control") != "no-store" || !reflect.DeepEqual(before, envelope.Data) {
		t.Fatalf("public Target query: code=%d err=%v", response.Code, err)
	}

	var head model.GenerationReferenceExecutionHead
	if err := database.Where("workspace_id = ? AND project_id = ? AND target_id = ?", execution.WorkspaceID, projectID, targetID).First(&head).Error; err != nil {
		t.Fatal(err)
	}
	setHash := func(hash string) error {
		return database.Model(&model.GenerationReferenceExecutionHead{}).Where("workspace_id = ? AND project_id = ? AND target_id = ?", head.WorkspaceID, head.ProjectID, head.TargetID).UpdateColumn("current_execution_hash", hash).Error
	}
	defer func() {
		if err := setHash(head.CurrentExecutionHash); err != nil {
			t.Error(err)
		}
	}()
	callback := "test_reference_generation_snapshot_" + uuid.NewString()
	var fired atomic.Bool
	var concurrentErr error
	if err := database.Callback().Query().After("gorm:query").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Schema == nil || tx.Statement.Schema.Name != "GenerationTarget" || !fired.CompareAndSwap(false, true) {
			return
		}
		concurrentErr = setHash(strings.Repeat("f", 64))
	}); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = database.Callback().Query().Remove(callback) }()
	if during := read(); !fired.Load() || concurrentErr != nil || !reflect.DeepEqual(before, during) {
		t.Fatalf("mixed Target/Head snapshots: %v", concurrentErr)
	}
	if value, err := query.Get(ctx, actor, projectID, targetID); err == nil || !reflect.DeepEqual(value, application.ReferenceGenerationProgress{}) {
		t.Fatal("next snapshot ignored Head drift")
	}
	if err := setHash(head.CurrentExecutionHash); err != nil {
		t.Fatal(err)
	}

	// Remove only this fixture's exact Head, not the execution or its Calls.
	// Missing published Head must never masquerade as unprepared or be repaired.
	if err := database.Where("workspace_id = ? AND project_id = ? AND target_id = ?", head.WorkspaceID, head.ProjectID, head.TargetID).Delete(&model.GenerationReferenceExecutionHead{}).Error; err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := database.Omit(clause.Associations).Clauses(clause.OnConflict{DoNothing: true}).Create(&head).Error; err != nil {
			t.Error(err)
		}
	}()
	if value, err := query.Get(ctx, actor, projectID, targetID); err == nil || !reflect.DeepEqual(value, application.ReferenceGenerationProgress{}) {
		t.Fatal("orphan execution reported as unprepared")
	}
	var count int64
	if err := database.Model(&model.GenerationReferenceExecutionHead{}).Where("workspace_id = ? AND project_id = ? AND target_id = ?", head.WorkspaceID, head.ProjectID, head.TargetID).Count(&count).Error; err != nil || count != 0 {
		t.Fatal("query repaired missing Head")
	}
	if err := database.Omit(clause.Associations).Create(&head).Error; err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, read()) {
		t.Fatal("restored facts changed progress")
	}
	if factCounts() != countsBefore {
		t.Fatal("read queries changed published fact counts")
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := query.Get(cancelled, actor, projectID, targetID); !errors.Is(err, context.Canceled) {
		t.Fatalf("lost cancellation: %v", err)
	}
}
