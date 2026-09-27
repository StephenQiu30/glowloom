package workspace_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	auditapp "github.com/StephenQiu30/lanverse/backend/internal/audit/application"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	identitydomain "github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
	inbox "github.com/StephenQiu30/lanverse/backend/internal/infra/inbox/application"
	workspaceapp "github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workspace/domain"
)

type updateProjectCommandStore struct {
	project domain.Project
	finds   int
	writes  int
	before  domain.Project
	after   domain.Project
	events  []identityapp.OutboxEvent
	err     error
}

func (s *updateProjectCommandStore) FindProject(_ context.Context, _ identityapp.Principal, _ uuid.UUID) (domain.Project, error) {
	s.finds++
	return s.project, nil
}

func (s *updateProjectCommandStore) UpdateProjectWithEvents(_ context.Context, _ identityapp.Principal, before, after domain.Project, events []identityapp.OutboxEvent) (domain.Project, error) {
	s.writes++
	s.before, s.after, s.events = before, after, events
	if s.err != nil {
		return domain.Project{}, s.err
	}
	after.CreateTime = s.project.CreateTime
	after.UpdateTime = s.project.UpdateTime.Add(time.Second)
	return after, nil
}

func updateProjectFixture(actor identityapp.Principal) domain.Project {
	project := validProject()
	project.OrgID = actor.OrgID
	project.Description = "旧说明"
	project.StylePresetID = uuid.New()
	project.CreateTime = time.Date(2026, 9, 27, 9, 0, 0, 0, time.UTC)
	project.UpdateTime = project.CreateTime
	return project
}

func updateProjectInput(projectID uuid.UUID) workspaceapp.UpdateProjectInput {
	name := "  新名称  "
	description := "新说明只存数据库"
	presetID := uuid.New()
	allowOverseas := true
	return workspaceapp.UpdateProjectInput{
		ProjectID: projectID, ExpectedRevision: 1,
		Name: &name, Description: &description, StylePresetID: &presetID,
		AllowOverseasModels: &allowOverseas, RequestID: uuid.NewString(),
	}
}

func TestUpdateProjectCommandChangesOnlyMutableSettingsAndEmitsSafeEvents(t *testing.T) {
	actor := projectCommandActor(identitydomain.RoleProducer)
	project := updateProjectFixture(actor)
	store := &updateProjectCommandStore{project: project}
	input := updateProjectInput(project.ID)
	now := time.Date(2026, 9, 27, 10, 0, 0, 0, time.UTC)
	updated, err := workspaceapp.NewUpdateProjectCommand(store, func() time.Time { return now }).Execute(t.Context(), actor, input)
	if err != nil || store.finds != 1 || store.writes != 1 {
		t.Fatalf("update project: result=%+v finds=%d writes=%d err=%v", updated, store.finds, store.writes, err)
	}
	if store.before != project || store.after.ID != project.ID || store.after.OrgID != actor.OrgID ||
		store.after.Name != "新名称" || store.after.Description != *input.Description ||
		store.after.StylePresetID != *input.StylePresetID || !store.after.AllowOverseasModels ||
		store.after.Revision != 2 || store.after.AspectRatio != project.AspectRatio ||
		store.after.StyleType != project.StyleType || store.after.StyleSubtype != project.StyleSubtype ||
		store.after.Resolution != project.Resolution || store.after.Status != project.Status {
		t.Fatalf("unexpected project update: before=%+v after=%+v", store.before, store.after)
	}
	if updated.ID != project.ID || updated.Name != "新名称" || updated.Description != *input.Description ||
		updated.StylePresetID == nil || *updated.StylePresetID != *input.StylePresetID ||
		!updated.AllowOverseasModels || updated.Revision != 2 || updated.AspectRatio != project.AspectRatio ||
		updated.StyleType != project.StyleType || updated.Status != "active" || updated.UpdateTime.IsZero() {
		t.Fatalf("unexpected update result: %+v", updated)
	}
	if len(store.events) != 2 || store.events[0].ID == store.events[1].ID {
		t.Fatalf("project update events = %+v, want distinct change and audit", store.events)
	}
	events := make(map[string]identityapp.OutboxEvent, 2)
	for _, event := range store.events {
		if event.ID == uuid.Nil || event.PartitionKey != project.ID.String() ||
			bytes.Contains(event.Payload, []byte("新名称")) ||
			bytes.Contains(event.Payload, []byte(*input.Description)) {
			t.Fatalf("project event contains free-form text or wrong routing: %+v", event)
		}
		events[event.Topic] = event
	}
	changed, ok := events["lanverse.workspace.project_changed.v1"]
	if !ok {
		t.Fatal("missing project change event")
	}
	var change struct {
		EventID    uuid.UUID `json:"event_id"`
		EventType  string    `json:"event_type"`
		OccurredAt time.Time `json:"occurred_at"`
		OrgID      uuid.UUID `json:"org_id"`
		ProjectID  uuid.UUID `json:"project_id"`
		Aggregate  struct {
			Type     string    `json:"type"`
			ID       uuid.UUID `json:"id"`
			Revision int64     `json:"revision"`
		} `json:"aggregate"`
		Data struct {
			Change string `json:"change"`
		} `json:"data"`
	}
	if err := json.Unmarshal(changed.Payload, &change); err != nil ||
		change.EventID != changed.ID || change.EventType != changed.Topic ||
		!change.OccurredAt.Equal(now) || change.OrgID != actor.OrgID ||
		change.ProjectID != project.ID || change.Aggregate.Type != "project" ||
		change.Aggregate.ID != project.ID || change.Aggregate.Revision != 2 ||
		change.Data.Change != "updated" {
		t.Fatalf("invalid project change event: %+v err=%v", change, err)
	}
	auditEvent, ok := events["lanverse.audit.recorded.v1"]
	if !ok {
		t.Fatal("missing project update audit")
	}
	audit, err := auditapp.NewRecordedActionParser().Parse(inbox.Record{
		Topic: auditEvent.Topic, Key: []byte(auditEvent.PartitionKey), Value: auditEvent.Payload,
	})
	if err != nil || audit.Action != "project.updated" || audit.OrgID != actor.OrgID ||
		audit.ProjectID == nil || *audit.ProjectID != project.ID ||
		audit.ObjectType != "project" || audit.ObjectID != project.ID.String() ||
		audit.RequestID != input.RequestID {
		t.Fatalf("invalid project update audit: %+v err=%v", audit, err)
	}
	var before, after map[string]any
	if err := json.Unmarshal(audit.Before, &before); err != nil {
		t.Fatalf("decode audit before: %v", err)
	}
	if err := json.Unmarshal(audit.After, &after); err != nil {
		t.Fatalf("decode audit after: %v", err)
	}
	for label, summary := range map[string]map[string]any{"before": before, "after": after} {
		if summary["name"] != nil || summary["description"] != nil ||
			summary["prompt_fragment"] != nil || summary["reference_asset_ids"] != nil {
			t.Fatalf("%s audit contains free-form or preset content: %+v", label, summary)
		}
	}
	if len(before) != 3 || len(after) != 5 ||
		before["revision"] != float64(1) || after["revision"] != float64(2) ||
		before["allow_overseas_models"] != false || after["allow_overseas_models"] != true ||
		before["style_preset_id"] != project.StylePresetID.String() ||
		after["style_preset_id"] != input.StylePresetID.String() ||
		after["name_changed"] != true || after["description_changed"] != true {
		t.Fatalf("audit summary does not describe committed safe settings: before=%+v after=%+v", before, after)
	}
}

func TestUpdateProjectCommandNoChangeDoesNotWrite(t *testing.T) {
	actor := projectCommandActor(identitydomain.RoleProducer)
	project := updateProjectFixture(actor)
	store := &updateProjectCommandStore{project: project}
	name := "  逆光  "
	description := project.Description
	presetID := project.StylePresetID
	allowOverseas := false
	updated, err := workspaceapp.NewUpdateProjectCommand(store, time.Now).Execute(t.Context(), actor, workspaceapp.UpdateProjectInput{
		ProjectID: project.ID, ExpectedRevision: 1,
		Name: &name, Description: &description, StylePresetID: &presetID,
		AllowOverseasModels: &allowOverseas, RequestID: uuid.NewString(),
	})
	if err != nil || store.finds != 1 || store.writes != 0 ||
		updated.ID != project.ID || updated.Revision != project.Revision {
		t.Fatalf("no-change update: result=%+v finds=%d writes=%d err=%v", updated, store.finds, store.writes, err)
	}
}

func TestUpdateProjectCommandRejectsInvalidInputBeforeStorage(t *testing.T) {
	actor := projectCommandActor(identitydomain.RoleProducer)
	project := updateProjectFixture(actor)
	valid := updateProjectInput(project.ID)
	for _, tc := range []struct {
		name   string
		actor  identityapp.Principal
		change func(*workspaceapp.UpdateProjectInput)
		want   error
	}{
		{"missing actor", identityapp.Principal{OrgID: actor.OrgID, Role: actor.Role}, nil, identityapp.ErrForbidden},
		{"missing organization", identityapp.Principal{ID: actor.ID, Role: actor.Role}, nil, identityapp.ErrForbidden},
		{"unknown role", identityapp.Principal{ID: actor.ID, OrgID: actor.OrgID}, nil, identityapp.ErrForbidden},
		{"must change password", identityapp.Principal{ID: actor.ID, OrgID: actor.OrgID, Role: actor.Role, MustChangePassword: true}, nil, identityapp.ErrForbidden},
		{"missing project", actor, func(v *workspaceapp.UpdateProjectInput) { v.ProjectID = uuid.Nil }, workspaceapp.ErrInvalidUpdateProject},
		{"missing revision", actor, func(v *workspaceapp.UpdateProjectInput) { v.ExpectedRevision = 0 }, workspaceapp.ErrInvalidUpdateProject},
		{"empty patch", actor, func(v *workspaceapp.UpdateProjectInput) {
			v.Name, v.Description, v.StylePresetID, v.AllowOverseasModels = nil, nil, nil, nil
		}, workspaceapp.ErrInvalidUpdateProject},
		{"blank name", actor, func(v *workspaceapp.UpdateProjectInput) { blank := " \t"; v.Name = &blank }, workspaceapp.ErrInvalidUpdateProject},
		{"long name", actor, func(v *workspaceapp.UpdateProjectInput) { name := strings.Repeat("剧", 51); v.Name = &name }, workspaceapp.ErrInvalidUpdateProject},
		{"invalid request ID", actor, func(v *workspaceapp.UpdateProjectInput) { v.RequestID = "bad" }, workspaceapp.ErrInvalidUpdateProject},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := valid
			if tc.change != nil {
				tc.change(&input)
			}
			store := &updateProjectCommandStore{project: project}
			_, err := workspaceapp.NewUpdateProjectCommand(store, time.Now).Execute(t.Context(), tc.actor, input)
			if !errors.Is(err, tc.want) || store.finds != 0 || store.writes != 0 {
				t.Fatalf("invalid update reached store: err=%v finds=%d writes=%d", err, store.finds, store.writes)
			}
		})
	}
}

func TestUpdateProjectCommandRejectsStaleOrArchivedProject(t *testing.T) {
	actor := projectCommandActor(identitydomain.RoleProducer)
	project := updateProjectFixture(actor)
	for _, tc := range []struct {
		name    string
		prepare func(*domain.Project)
		input   workspaceapp.UpdateProjectInput
		want    error
	}{
		{"stale revision", nil, func() workspaceapp.UpdateProjectInput {
			input := updateProjectInput(project.ID)
			input.ExpectedRevision = 2
			return input
		}(), domain.ErrProjectRevisionConflict},
		{"archived", func(p *domain.Project) { p.Status = "archived" }, updateProjectInput(project.ID), domain.ErrProjectStateConflict},
		{"recycled", func(p *domain.Project) { p.IsDelete = true }, updateProjectInput(project.ID), domain.ErrProjectStateConflict},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stored := project
			if tc.prepare != nil {
				tc.prepare(&stored)
			}
			store := &updateProjectCommandStore{project: stored}
			_, err := workspaceapp.NewUpdateProjectCommand(store, time.Now).Execute(t.Context(), actor, tc.input)
			if !errors.Is(err, tc.want) || store.writes != 0 {
				t.Fatalf("invalid state reached writer: err=%v writes=%d", err, store.writes)
			}
		})
	}
}

func TestUpdateProjectCommandPreservesStoreFailure(t *testing.T) {
	actor := projectCommandActor(identitydomain.RoleProducer)
	project := updateProjectFixture(actor)
	sentinel := errors.New("project write failed")
	store := &updateProjectCommandStore{project: project, err: sentinel}
	_, err := workspaceapp.NewUpdateProjectCommand(store, time.Now).Execute(t.Context(), actor, updateProjectInput(project.ID))
	if !errors.Is(err, sentinel) || store.writes != 1 {
		t.Fatalf("store error chain lost: err=%v writes=%d", err, store.writes)
	}
}
