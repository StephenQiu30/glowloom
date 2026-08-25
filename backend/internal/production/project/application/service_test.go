package application

import (
	"context"
	"testing"
	"time"

	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
	"github.com/StephenQiu30/lanverse/backend/internal/production/project/domain"
)

func TestCreateProjectAuthorizesAndPersistsDefaults(t *testing.T) {
	store := newMemoryStore()
	service := NewService(store, func() time.Time { return time.Unix(10, 0).UTC() }, func() string { return "project-1" })

	project, err := service.Create(context.Background(), Actor{UserID: "user-1", TokenVersion: 3}, CreateCommand{
		WorkspaceID:    "workspace-1",
		Name:           "  Harbor  ",
		IdempotencyKey: "create-project-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if project.ID != "project-1" || project.Name != "Harbor" || project.Revision != 1 {
		t.Fatalf("project = %#v", project)
	}
	if project.AspectRatio != "9:16" || project.Language != "zh-CN" || project.Currency != "CNY" {
		t.Fatalf("defaults = %#v", project)
	}
	if store.authorizedCapability != ContentWrite {
		t.Fatalf("capability = %q", store.authorizedCapability)
	}
	if len(store.audit) != 1 || store.audit[0].Action != "project.created" {
		t.Fatalf("audit = %#v", store.audit)
	}
}

func TestCreateProjectReplaysReceiptAndRejectsKeyReuseWithDifferentInput(t *testing.T) {
	store := newMemoryStore()
	ids := []string{"project-1", "receipt-1", "unused"}
	service := NewService(store, func() time.Time { return time.Unix(10, 0).UTC() }, func() string {
		id := ids[0]
		ids = ids[1:]
		return id
	})
	command := CreateCommand{WorkspaceID: "workspace-1", Name: "Harbor", IdempotencyKey: "create-project-1"}

	first, err := service.Create(context.Background(), Actor{UserID: "user-1", TokenVersion: 1}, command)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Create(context.Background(), Actor{UserID: "user-1", TokenVersion: 1}, command)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID || len(store.projects) != 1 || len(store.audit) != 1 || len(store.receipts) != 1 {
		t.Fatalf("replay first=%#v second=%#v projects=%d audit=%d receipts=%d", first, second, len(store.projects), len(store.audit), len(store.receipts))
	}
	command.Name = "Different"
	if _, err = service.Create(context.Background(), Actor{UserID: "user-1", TokenVersion: 1}, command); !IsCode(err, CodeIdempotencyConflict) {
		t.Fatalf("different input error = %v", err)
	}
}

func TestUpdateRejectsStaleRevisionWithoutMutation(t *testing.T) {
	store := newMemoryStore()
	store.projects["project-1"] = domain.Project{ID: "project-1", WorkspaceID: "workspace-1", Name: "Before", Status: domain.StatusActive, Revision: 2}
	service := NewService(store, time.Now, func() string { return "unused" })
	name := "After"

	_, err := service.Update(context.Background(), Actor{UserID: "user-1", TokenVersion: 1}, "project-1", UpdateCommand{Name: &name, ExpectedRevision: 1, IdempotencyKey: "update-project-1"})
	if !IsCode(err, CodeVersionConflict) {
		t.Fatalf("error = %v", err)
	}
	if store.projects["project-1"].Name != "Before" {
		t.Fatalf("stale update mutated project: %#v", store.projects["project-1"])
	}
}

func TestDeleteIsBlockedByDependentFacts(t *testing.T) {
	store := newMemoryStore()
	store.projects["project-1"] = domain.Project{ID: "project-1", WorkspaceID: "workspace-1", Name: "Harbor", Status: domain.StatusActive, Revision: 1}
	store.dependencies = DependencySummary{Episodes: 2, Assets: 1}
	service := NewService(store, time.Now, func() string { return "unused" })

	preflight, err := service.DeletePreflight(context.Background(), Actor{UserID: "user-1", TokenVersion: 1}, "project-1")
	if err != nil {
		t.Fatal(err)
	}
	if preflight.Allowed || len(preflight.Blockers) != 2 {
		t.Fatalf("preflight = %#v", preflight)
	}
	err = service.Delete(context.Background(), Actor{UserID: "user-1", TokenVersion: 1}, "project-1", 1, "delete-project-1")
	if !IsCode(err, CodeStateConflict) {
		t.Fatalf("error = %v", err)
	}
	if _, exists := store.projects["project-1"]; !exists {
		t.Fatal("blocked project was deleted")
	}
}

type memoryStore struct {
	projects             map[string]domain.Project
	audit                []AuditEvent
	dependencies         DependencySummary
	authorizedCapability Capability
	receipts             []platformcommand.Receipt
}

func newMemoryStore() *memoryStore {
	return &memoryStore{projects: map[string]domain.Project{}}
}

func (store *memoryStore) Authorize(_ context.Context, actor Actor, workspaceID string, capability Capability) error {
	store.authorizedCapability = capability
	return nil
}

func (store *memoryStore) WithinTransaction(_ context.Context, operation func(Repository) error) error {
	copyProjects := make(map[string]domain.Project, len(store.projects))
	for key, value := range store.projects {
		copyProjects[key] = value
	}
	copyAudit := append([]AuditEvent(nil), store.audit...)
	copyReceipts := append([]platformcommand.Receipt(nil), store.receipts...)
	if err := operation(store); err != nil {
		store.projects = copyProjects
		store.audit = copyAudit
		store.receipts = copyReceipts
		return err
	}
	return nil
}

func (store *memoryStore) Create(_ context.Context, project domain.Project) error {
	store.projects[project.ID] = project
	return nil
}
func (store *memoryStore) Get(_ context.Context, id string, _ bool) (domain.Project, error) {
	project, ok := store.projects[id]
	if !ok {
		return domain.Project{}, ErrNotFound
	}
	return project, nil
}
func (store *memoryStore) Save(_ context.Context, project domain.Project) error {
	store.projects[project.ID] = project
	return nil
}
func (store *memoryStore) Delete(_ context.Context, id string) error {
	delete(store.projects, id)
	return nil
}
func (store *memoryStore) List(_ context.Context, query ListQuery) ([]domain.Project, int, error) {
	items := make([]domain.Project, 0, len(store.projects))
	for _, project := range store.projects {
		items = append(items, project)
	}
	return items, len(items), nil
}
func (store *memoryStore) Dependencies(_ context.Context, _ string) (DependencySummary, error) {
	return store.dependencies, nil
}
func (store *memoryStore) AppendAudit(_ context.Context, event AuditEvent) error {
	store.audit = append(store.audit, event)
	return nil
}
func (store *memoryStore) FindReceipt(_ context.Context, workspaceID, operation, key string) (platformcommand.Receipt, error) {
	for _, receipt := range store.receipts {
		if receipt.WorkspaceID == workspaceID && receipt.Operation == operation && receipt.IdempotencyKey == key {
			return receipt, nil
		}
	}
	return platformcommand.Receipt{}, platformcommand.ErrReceiptNotFound
}
func (store *memoryStore) FindReceiptByResource(_ context.Context, resourceID, actorID, operation, key string) (platformcommand.Receipt, error) {
	for _, receipt := range store.receipts {
		if receipt.ResourceID == resourceID && receipt.CreatedBy == actorID && receipt.Operation == operation && receipt.IdempotencyKey == key {
			return receipt, nil
		}
	}
	return platformcommand.Receipt{}, platformcommand.ErrReceiptNotFound
}
func (store *memoryStore) CreateReceipt(_ context.Context, receipt platformcommand.Receipt) error {
	store.receipts = append(store.receipts, receipt)
	return nil
}
