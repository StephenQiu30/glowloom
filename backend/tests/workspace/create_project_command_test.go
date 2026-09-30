package workspace_test

import (
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

type createProjectCommandStore struct {
	called  int
	actor   identityapp.Principal
	project domain.Project
	events  []identityapp.OutboxEvent
	err     error
}

func (s *createProjectCommandStore) CreateProjectWithEvents(_ context.Context, actor identityapp.Principal, project domain.Project, events []identityapp.OutboxEvent) (domain.Project, error) {
	s.called++
	s.actor = actor
	s.project = project
	s.events = events
	if s.err != nil {
		return domain.Project{}, s.err
	}
	project.CreateTime = time.Date(2026, 9, 27, 9, 0, 0, 0, time.UTC)
	project.UpdateTime = project.CreateTime
	return project, nil
}

func (s *createProjectCommandStore) CreateProjectIdempotently(ctx context.Context, actor identityapp.Principal, project domain.Project, events []identityapp.OutboxEvent, _ workspaceapp.ProjectCreationRequest) (domain.Project, error) {
	return s.CreateProjectWithEvents(ctx, actor, project, events)
}

func createProjectCommandInput() workspaceapp.CreateProjectInput {
	return workspaceapp.CreateProjectInput{
		Name: " 逆光 ", Description: "短剧项目", AspectRatio: "16:9",
		StyleType: "stylized", StyleSubtype: "guofeng_xianxia",
		StylePresetID: uuid.New(), RequestID: uuid.NewString(),
	}
}

func projectCommandActor(role identitydomain.Role) identityapp.Principal {
	return identityapp.Principal{ID: uuid.New(), OrgID: uuid.New(), Role: role}
}

func TestCreateProjectCommandCreatesSafeProjectAndEvents(t *testing.T) {
	for _, role := range []identitydomain.Role{identitydomain.RoleProducer, identitydomain.RoleAdmin} {
		t.Run(string(role), func(t *testing.T) {
			actor := projectCommandActor(role)
			input := createProjectCommandInput()
			store := &createProjectCommandStore{}
			occurredAt := time.Date(2026, 9, 27, 10, 0, 0, 0, time.UTC)
			created, err := workspaceapp.NewCreateProjectCommand(store, func() time.Time { return occurredAt }).Execute(t.Context(), actor, input)
			if err != nil || store.called != 1 {
				t.Fatalf("create project: result=%+v writes=%d err=%v", created, store.called, err)
			}
			if store.actor != actor || store.project.ID == uuid.Nil || store.project.OrgID != actor.OrgID ||
				store.project.Name != "逆光" || store.project.Description != input.Description ||
				store.project.AspectRatio != input.AspectRatio || store.project.StyleType != input.StyleType ||
				store.project.StyleSubtype != input.StyleSubtype || store.project.StylePresetID != input.StylePresetID ||
				store.project.Resolution != "1080p" || store.project.AllowOverseasModels ||
				store.project.Status != "active" || store.project.Revision != 1 || store.project.IsDelete {
				t.Fatalf("unexpected project specification: %+v", store.project)
			}
			if created.ID != store.project.ID || created.OrgID != actor.OrgID || created.Name != "逆光" ||
				created.Resolution != "1080p" || created.AllowOverseasModels || created.Status != "active" ||
				created.Revision != 1 || created.CreateTime.IsZero() || created.UpdateTime.IsZero() {
				t.Fatalf("unsafe or incomplete create result: %+v", created)
			}
			if len(store.events) != 2 {
				t.Fatalf("durable events = %d, want change and audit", len(store.events))
			}
			events := make(map[string]identityapp.OutboxEvent, len(store.events))
			for _, event := range store.events {
				if event.ID == uuid.Nil || event.PartitionKey != created.ID.String() || len(event.Payload) == 0 {
					t.Fatalf("invalid project event envelope: %+v", event)
				}
				if _, duplicated := events[event.Topic]; duplicated {
					t.Fatalf("duplicate event topic: %s", event.Topic)
				}
				events[event.Topic] = event
			}
			changed, ok := events["lanverse.workspace.project_changed.v1"]
			if !ok {
				t.Fatal("missing project change event")
			}
			var changedBody struct {
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
			}
			if err := json.Unmarshal(changed.Payload, &changedBody); err != nil ||
				changedBody.EventID != changed.ID || changedBody.EventType != changed.Topic ||
				!changedBody.OccurredAt.Equal(occurredAt) || changedBody.OrgID != actor.OrgID ||
				changedBody.ProjectID != created.ID || changedBody.Aggregate.Type != "project" ||
				changedBody.Aggregate.ID != created.ID || changedBody.Aggregate.Revision != 1 {
				t.Fatalf("invalid project change event: %+v err=%v", changedBody, err)
			}
			auditEvent, ok := events["lanverse.audit.recorded.v1"]
			if !ok {
				t.Fatal("missing project create audit")
			}
			audit, err := auditapp.NewRecordedActionParser().Parse(inbox.Record{
				Topic: auditEvent.Topic, Key: []byte(auditEvent.PartitionKey), Value: auditEvent.Payload,
			})
			if err != nil || audit.Action != "project.created" || audit.OrgID != actor.OrgID ||
				audit.ProjectID == nil || *audit.ProjectID != created.ID ||
				audit.ObjectType != "project" || audit.ObjectID != created.ID.String() ||
				audit.RequestID != input.RequestID {
				t.Fatalf("invalid project create audit: %+v err=%v", audit, err)
			}
			var after map[string]any
			if err := json.Unmarshal(audit.After, &after); err != nil ||
				after["aspect_ratio"] != "16:9" || after["style_type"] != "stylized" ||
				after["style_subtype"] != "guofeng_xianxia" ||
				after["style_preset_id"] != input.StylePresetID.String() ||
				after["status"] != "active" || after["revision"] != float64(1) ||
				after["name"] != nil || after["description"] != nil {
				t.Fatalf("invalid project audit summary: %+v err=%v", after, err)
			}
		})
	}
}

func TestCreateProjectCommandRejectsInvalidCallerAndInputBeforeStorage(t *testing.T) {
	actor := projectCommandActor(identitydomain.RoleProducer)
	valid := createProjectCommandInput()
	for _, tc := range []struct {
		name   string
		actor  identityapp.Principal
		change func(*workspaceapp.CreateProjectInput)
		want   error
	}{
		{"missing actor", identityapp.Principal{OrgID: actor.OrgID, Role: identitydomain.RoleProducer}, nil, identityapp.ErrForbidden},
		{"missing organization", identityapp.Principal{ID: actor.ID, Role: identitydomain.RoleProducer}, nil, identityapp.ErrForbidden},
		{"unknown role", identityapp.Principal{ID: actor.ID, OrgID: actor.OrgID}, nil, identityapp.ErrForbidden},
		{"must change password", identityapp.Principal{ID: actor.ID, OrgID: actor.OrgID, Role: actor.Role, MustChangePassword: true}, nil, identityapp.ErrForbidden},
		{"blank name", actor, func(v *workspaceapp.CreateProjectInput) { v.Name = " \t" }, workspaceapp.ErrInvalidCreateProject},
		{"long name", actor, func(v *workspaceapp.CreateProjectInput) { v.Name = strings.Repeat("剧", 51) }, workspaceapp.ErrInvalidCreateProject},
		{"invalid aspect", actor, func(v *workspaceapp.CreateProjectInput) { v.AspectRatio = "1:1" }, workspaceapp.ErrInvalidCreateProject},
		{"stylized without subtype", actor, func(v *workspaceapp.CreateProjectInput) { v.StyleSubtype = "" }, workspaceapp.ErrInvalidCreateProject},
		{"realistic with subtype", actor, func(v *workspaceapp.CreateProjectInput) { v.StyleType = "realistic" }, workspaceapp.ErrInvalidCreateProject},
		{"invalid subtype", actor, func(v *workspaceapp.CreateProjectInput) { v.StyleSubtype = "other" }, workspaceapp.ErrInvalidCreateProject},
		{"missing request ID", actor, func(v *workspaceapp.CreateProjectInput) { v.RequestID = "" }, workspaceapp.ErrInvalidCreateProject},
		{"invalid request ID", actor, func(v *workspaceapp.CreateProjectInput) { v.RequestID = "not-a-uuid" }, workspaceapp.ErrInvalidCreateProject},
		{"nil request ID", actor, func(v *workspaceapp.CreateProjectInput) { v.RequestID = uuid.Nil.String() }, workspaceapp.ErrInvalidCreateProject},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &createProjectCommandStore{}
			input := valid
			if tc.change != nil {
				tc.change(&input)
			}
			_, err := workspaceapp.NewCreateProjectCommand(store, time.Now).Execute(t.Context(), tc.actor, input)
			if !errors.Is(err, tc.want) || store.called != 0 {
				t.Fatalf("invalid create reached store: err=%v writes=%d", err, store.called)
			}
		})
	}
	store := &createProjectCommandStore{}
	_, err := workspaceapp.NewCreateProjectCommand(store, func() time.Time { return time.Time{} }).Execute(t.Context(), actor, valid)
	if !errors.Is(err, workspaceapp.ErrInvalidCreateProject) || store.called != 0 {
		t.Fatalf("zero clock reached store: err=%v writes=%d", err, store.called)
	}
}

func TestCreateProjectCommandPreservesStoreFailure(t *testing.T) {
	sentinel := errors.New("store unavailable")
	store := &createProjectCommandStore{err: sentinel}
	actor := projectCommandActor(identitydomain.RoleProducer)
	_, err := workspaceapp.NewCreateProjectCommand(store, time.Now).Execute(t.Context(), actor, createProjectCommandInput())
	if !errors.Is(err, sentinel) || store.called != 1 {
		t.Fatalf("store error chain lost: err=%v writes=%d", err, store.called)
	}
}
