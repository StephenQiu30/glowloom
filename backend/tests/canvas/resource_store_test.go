package canvas_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	pgcanvas "github.com/StephenQiu30/lanverse/backend/internal/canvas/adapter/postgres"
	"github.com/StephenQiu30/lanverse/backend/internal/canvas/application"
	"github.com/StephenQiu30/lanverse/backend/internal/canvas/domain"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	pgmedia "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
)

func seedCanvasMedia(t *testing.T, db *gorm.DB, project uuid.UUID, kind string) uuid.UUID {
	t.Helper()
	id := uuid.New()
	key := "projects/" + project.String() + "/" + kind + "/2026/09/" + id.String() + ".bin"
	if err := db.Exec(`INSERT INTO media.media_asset(id,project_id,kind,origin,status,object_key,file_name,mime_type,byte_size,moderation_status) VALUES(?,?,?,'upload','ready',?,'fixture','application/octet-stream',1,'passed')`, id, project, kind, key).Error; err != nil {
		t.Fatal(err)
	}
	return id
}

type pausedMediaReference struct {
	reader application.MediaReader
	read   chan<- struct{}
	resume <-chan struct{}
}

func (p pausedMediaReference) Reference(ctx context.Context, actor identityapp.Principal, project, asset uuid.UUID) (mediaapp.AssetSummary, error) {
	summary, err := p.reader.Reference(ctx, actor, project, asset)
	if err != nil {
		return mediaapp.AssetSummary{}, err
	}
	p.read <- struct{}{}
	select {
	case <-p.resume:
		return summary, nil
	case <-ctx.Done():
		return mediaapp.AssetSummary{}, ctx.Err()
	}
}

func TestMediaReferenceRemainsLockedUntilCanvasCommit(t *testing.T) {
	database := canvasDB(t, "LV_TEST_CANVAS_DB_DSN")
	actor, project := seedCanvasActor(t, database)
	asset := seedCanvasMedia(t, database, project, "image")
	read, resume := make(chan struct{}, 1), make(chan struct{})
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	var workers sync.WaitGroup
	defer func() { cancel(); workers.Wait() }()
	store := pgcanvas.NewStore(database, func(tx *gorm.DB) application.MediaReader {
		return pausedMediaReference{reader: mediaapp.NewAssetQuery(pgmedia.NewStore(tx), nil), read: read, resume: resume}
	})
	service := application.NewService(store)
	doc, err := service.Create(ctx, actor, project, uuid.NewString(), application.CreateInput{Name: "绑定提交事务"})
	if err != nil {
		t.Fatal(err)
	}
	node := resourceNode("image")
	node.RefID = &asset
	committed := make(chan error, 1)
	workers.Go(func() {
		_, err := service.Execute(ctx, actor, doc.ID, uuid.NewString(), application.CommandsInput{ExpectedRevision: 1, Commands: []domain.Command{{Type: "AddNodes", Nodes: []domain.Node{node}}}})
		committed <- err
	})
	select {
	case <-read:
	case err := <-committed:
		t.Fatalf("reference never reached transaction pause: %v", err)
	case <-ctx.Done():
		t.Fatal("reference transaction did not start")
	}
	mutated := make(chan error, 1)
	workers.Go(func() {
		mutated <- database.WithContext(ctx).Exec(`UPDATE media.media_asset SET status='processing',moderation_status='pending' WHERE id=?`, asset).Error
	})
	blocked := false
	select {
	case <-time.After(200 * time.Millisecond):
		blocked = true
	case err := <-mutated:
		if err != nil {
			t.Errorf("concurrent media change failed before locking check: %v", err)
		}
	}
	close(resume)
	if err := <-committed; err != nil {
		t.Fatalf("canvas reference commit: %v", err)
	}
	if blocked {
		if err := <-mutated; err != nil {
			t.Fatalf("media change did not resume after commit: %v", err)
		}
	} else {
		t.Fatal("media became unavailable between reference check and canvas commit")
	}
}
func TestResourcePersistenceScopesMediaAndSoftDeleteReplay(t *testing.T) {
	database := canvasDB(t, "LV_TEST_CANVAS_DB_DSN")
	actor, project := seedCanvasActor(t, database)
	service := application.NewService(canvasStore(database))
	doc, err := service.Create(t.Context(), actor, project, uuid.NewString(), application.CreateInput{Name: "正式画布"})
	if err != nil {
		t.Fatal(err)
	}
	image := resourceNode("image")
	asset := seedCanvasMedia(t, database, project, "image")
	image.RefID = &asset
	group := resourceNode("group")
	text := resourceNode("text")
	input := application.CommandsInput{ExpectedRevision: 1, Commands: []domain.Command{{Type: "AddNodes", Nodes: []domain.Node{group, text, image}}, {Type: "SetNodeParents", Parents: []domain.Parent{{ID: text.ID, ParentID: &group.ID}, {ID: image.ID, ParentID: &group.ID}}}, {Type: "ResizeNodes", Sizes: []domain.Size{{ID: group.ID, Width: 5000, Height: 8000}}}}}
	result, err := service.Execute(t.Context(), actor, doc.ID, uuid.NewString(), input)
	if err != nil || result.Revision != 2 {
		t.Fatalf("resource commit %v", err)
	}
	current, err := service.Get(t.Context(), actor, doc.ID)
	if err != nil || len(current.Nodes) != 3 {
		t.Fatalf("resource read %v %+v", err, current)
	}
	_, foreignProject := seedCanvasActor(t, database)
	foreign := seedCanvasMedia(t, database, foreignProject, "image")
	bad := resourceNode("image")
	bad.RefID = &foreign
	_, err = service.Execute(t.Context(), actor, doc.ID, uuid.NewString(), application.CommandsInput{ExpectedRevision: 2, Commands: []domain.Command{{Type: "MoveNodes", Moves: []domain.Move{{ID: text.ID, X: 99}}}, {Type: "AddNodes", Nodes: []domain.Node{bad}}}})
	var indexed *domain.CommandError
	if !errors.Is(err, mediaapp.ErrNotFound) || !errors.As(err, &indexed) || indexed.Index != 1 {
		t.Fatalf("foreign media %v", err)
	}
	current, _ = service.Get(t.Context(), actor, doc.ID)
	if current.Revision != 2 {
		t.Fatal("foreign media partially committed")
	}
	key := uuid.NewString()
	renamed, err := service.Rename(t.Context(), actor, doc.ID, key, application.RenameInput{ExpectedRevision: 2, Name: "改名"})
	if err != nil || renamed.Revision != 3 {
		t.Fatalf("rename %v", err)
	}
	replayed, err := service.Rename(t.Context(), actor, doc.ID, key, application.RenameInput{ExpectedRevision: 2, Name: "改名"})
	if err != nil || replayed.Revision != 3 {
		t.Fatalf("rename replay %v", err)
	}
	key = uuid.NewString()
	deleted, err := service.Delete(t.Context(), actor, doc.ID, key, application.DeleteInput{ExpectedRevision: 3})
	if err != nil || !deleted.Deleted || deleted.Revision != 4 {
		t.Fatalf("delete %v", err)
	}
	deleted, err = application.NewService(canvasStore(database)).Delete(t.Context(), actor, doc.ID, key, application.DeleteInput{ExpectedRevision: 3})
	if err != nil || deleted.Revision != 4 {
		t.Fatalf("durable delete replay %v", err)
	}
	if _, err := service.Get(t.Context(), actor, doc.ID); !errors.Is(err, application.ErrNotFound) {
		t.Fatalf("deleted readable %v", err)
	}
	if _, err := pgmedia.NewStore(database).FindAsset(t.Context(), actor, project, asset); err != nil {
		t.Fatalf("source media deleted %v", err)
	}
	query := mediaapp.NewAssetQuery(pgmedia.NewStore(database), nil)
	page, err := query.List(t.Context(), actor, project, "image", "", 1)
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != asset {
		t.Fatalf("media list %+v %v", page, err)
	}
	if _, err := query.Preview(t.Context(), actor, project, asset); !errors.Is(err, mediaapp.ErrUnavailable) {
		t.Fatalf("nil preview signer %v", err)
	}
}

func TestResourceGroupRestoreAndParentValidationOnPostgres(t *testing.T) {
	database := canvasDB(t, "LV_TEST_CANVAS_DB_DSN")
	actor, project := seedCanvasActor(t, database)
	service := application.NewService(canvasStore(database))
	doc, err := service.Create(t.Context(), actor, project, uuid.NewString(), application.CreateInput{Name: "分组恢复"})
	if err != nil {
		t.Fatal(err)
	}
	group, nested, child := resourceNode("group"), resourceNode("group"), resourceNode("text")
	child.X, child.Y = 120, 240
	parents := []domain.Parent{{ID: child.ID, ParentID: &group.ID}, {ID: nested.ID, ParentID: &group.ID}}
	key := uuid.NewString()
	input := application.CommandsInput{ExpectedRevision: 1, Commands: []domain.Command{{Type: "AddNodes", Nodes: []domain.Node{group, nested, child}}, {Type: "SetNodeParents", Parents: parents}}}
	if _, err := service.Execute(t.Context(), actor, doc.ID, key, input); err != nil {
		t.Fatal(err)
	}
	for _, invalidParents := range [][]domain.Parent{
		{{ID: child.ID, ParentID: ptr(uuid.New())}},
		{{ID: group.ID, ParentID: &nested.ID}},
		{{ID: child.ID, ParentID: &child.ID}},
	} {
		_, err := service.Execute(t.Context(), actor, doc.ID, uuid.NewString(), application.CommandsInput{ExpectedRevision: 2, Commands: []domain.Command{{Type: "MoveNodes", Moves: []domain.Move{{ID: child.ID, X: 999, Y: 999}}}, {Type: "SetNodeParents", Parents: invalidParents}}})
		var indexed *domain.CommandError
		if !errors.Is(err, domain.ErrInvalidCommand) || !errors.As(err, &indexed) || indexed.Index != 1 {
			t.Fatalf("invalid parent batch accepted: %v", err)
		}
	}
	result, err := service.Execute(t.Context(), actor, doc.ID, uuid.NewString(), application.CommandsInput{ExpectedRevision: 2, Commands: []domain.Command{{Type: "DeleteNodes", IDs: []uuid.UUID{group.ID}}}})
	if err != nil || result.Revision != 3 || len(result.Nodes) != 2 {
		t.Fatalf("group removal: %+v %v", result, err)
	}
	for _, n := range result.Nodes {
		if n.ParentID != nil || n.ID == child.ID && (n.X != 120 || n.Y != 240) {
			t.Fatalf("group removal changed child world coordinates: %+v", n)
		}
	}
	result, err = service.Execute(t.Context(), actor, doc.ID, uuid.NewString(), application.CommandsInput{ExpectedRevision: 3, Commands: []domain.Command{{Type: "AddNodes", Nodes: []domain.Node{group}}, {Type: "SetNodeParents", Parents: parents}}})
	if err != nil || result.Revision != 4 {
		t.Fatalf("restore group and parents atomically: %v", err)
	}
	current, err := application.NewService(canvasStore(database)).Get(t.Context(), actor, doc.ID)
	if err != nil || len(current.Nodes) != 3 || current.Revision != 4 {
		t.Fatalf("durable group restore: %+v %v", current, err)
	}
	for _, n := range current.Nodes {
		if n.ID != group.ID && (n.ParentID == nil || *n.ParentID != group.ID) {
			t.Fatal("restored group membership was not persisted")
		}
	}
	if err := database.Exec(`UPDATE workspace.project SET status='archived' WHERE id=?`, project).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := service.Get(t.Context(), actor, doc.ID); err != nil {
		t.Fatalf("archived project should remain readable: %v", err)
	}
	if _, err := service.Execute(t.Context(), actor, doc.ID, key, input); !errors.Is(err, application.ErrNotFound) {
		t.Fatalf("archived project replay bypassed current write access: %v", err)
	}
}

func TestResourceWithOperationBindingRejectsEditsWithoutFalseSavedRevision(t *testing.T) {
	database := canvasDB(t, "LV_TEST_CANVAS_DB_DSN")
	actor, project := seedCanvasActor(t, database)
	service := application.NewService(canvasStore(database))
	doc, err := service.Create(t.Context(), actor, project, uuid.NewString(), application.CreateInput{Name: "保护已绑定节点"})
	if err != nil {
		t.Fatal(err)
	}
	node, group := resourceNode("text"), resourceNode("group")
	node.ParentID = &group.ID
	if _, err := service.Execute(t.Context(), actor, doc.ID, uuid.NewString(), application.CommandsInput{ExpectedRevision: 1, Commands: []domain.Command{{Type: "AddNodes", Nodes: []domain.Node{group, node}}}}); err != nil {
		t.Fatal(err)
	}
	// This is legal under the published schema and represents a pre-existing
	// protected projection, never a public AddNodes payload.
	if err := database.Exec(`UPDATE canvas.node SET last_operation_id=? WHERE id=?`, uuid.New(), node.ID).Error; err != nil {
		t.Fatal(err)
	}
	for _, command := range []domain.Command{
		{Type: "MoveNodes", Moves: []domain.Move{{ID: node.ID, X: 90, Y: 80}}},
		{Type: "RenameNodes", Names: []domain.Name{{ID: node.ID, Title: "不能修改"}}},
		{Type: "ResizeNodes", Sizes: []domain.Size{{ID: node.ID, Width: 180, Height: 120}}},
		{Type: "UpdateNodeConfig", ID: node.ID, Config: &domain.NodeConfig{Text: ptr("不能修改")}},
		{Type: "SetNodeParents", Parents: []domain.Parent{{ID: node.ID}}},
		{Type: "SetNodeZIndex", ZIndices: []domain.ZIndex{{ID: node.ID, ZIndex: 2}}},
		{Type: "DeleteNodes", IDs: []uuid.UUID{node.ID}},
		{Type: "DeleteNodes", IDs: []uuid.UUID{group.ID}},
	} {
		t.Run(command.Type, func(t *testing.T) {
			_, err := service.Execute(t.Context(), actor, doc.ID, uuid.NewString(), application.CommandsInput{ExpectedRevision: 2, Commands: []domain.Command{command}})
			if !errors.Is(err, domain.ErrUnsupportedCommand) {
				t.Fatalf("protected command reported saved: %v", err)
			}
			current, err := service.Get(t.Context(), actor, doc.ID)
			if err != nil || current.Revision != 2 || len(current.Nodes) != 2 {
				t.Fatalf("protected command advanced persisted facts: %+v %v", current, err)
			}
			for _, n := range current.Nodes {
				if n.ID == node.ID && (n.X != 0 || n.Title != node.Title || n.ParentID == nil || *n.ParentID != group.ID) {
					t.Fatalf("protected node changed: %+v", n)
				}
			}
		})
	}
	result, err := service.Execute(t.Context(), actor, doc.ID, uuid.NewString(), application.CommandsInput{ExpectedRevision: 2, Commands: []domain.Command{{Type: "SetViewport", Viewport: &domain.Viewport{Zoom: 0.5}}}})
	if err != nil || result.Revision != 3 {
		t.Fatalf("viewport should remain editable around a protected node: %v", err)
	}
}
