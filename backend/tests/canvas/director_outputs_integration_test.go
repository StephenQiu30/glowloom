package canvas_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	pgcanvas "github.com/StephenQiu30/lanverse/backend/internal/canvas/adapter/postgres"
	"github.com/StephenQiu30/lanverse/backend/internal/canvas/application"
	"github.com/StephenQiu30/lanverse/backend/internal/canvas/domain"
	pgmedia "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
)

func TestDirectorOutputsPersistAndRejectUnavailableImages(t *testing.T) {
	database := canvasDB(t, "LV_TEST_CANVAS_DB_DSN")
	actor, project := seedCanvasActor(t, database)
	service := application.NewService(canvasStore(database))
	doc, err := service.Create(t.Context(), actor, project, uuid.NewString(), application.CreateInput{Name: "导演台图库"})
	if err != nil {
		t.Fatal(err)
	}
	galleryAsset := seedCanvasMedia(t, database, project, "image")
	coverAsset := seedCanvasMedia(t, database, project, "image")
	node := directorNode()
	screenshot := directorScreenshot()
	screenshot.AssetID = galleryAsset
	node.Config.Director.Shots[0].Screenshots = []domain.DirectorScreenshot{screenshot}
	node.Config.Director.Cover = &domain.DirectorCover{AssetID: coverAsset, ShotID: node.Config.Director.Shots[0].ID}
	input := application.CommandsInput{ExpectedRevision: 1, Commands: []domain.Command{{Type: "AddNodes", Nodes: []domain.Node{node}}}}
	key := uuid.NewString()
	result, err := service.Execute(t.Context(), actor, doc.ID, key, input)
	if err != nil || result.Revision != 2 {
		t.Fatalf("persist ready images: %+v %v", result, err)
	}
	freshService := application.NewService(canvasStore(database))
	current, err := freshService.Get(t.Context(), actor, doc.ID)
	if err != nil || len(current.Nodes) != 1 || current.Nodes[0].Config.Director.Cover.AssetID != coverAsset || current.Nodes[0].Config.Director.Shots[0].Screenshots[0] != screenshot {
		t.Fatalf("refresh lost output identities: %+v %v", current, err)
	}
	replayed, err := freshService.Execute(t.Context(), actor, doc.ID, key, input)
	if err != nil || replayed.Revision != 2 {
		t.Fatalf("persistent screenshot replay: %+v %v", replayed, err)
	}
	otherProject := uuid.New()
	if err := database.Exec(`INSERT INTO workspace.project(id,org_id,name,aspect_ratio,style_type) VALUES(?,?,'其他项目','16:9','realistic')`, otherProject, actor.OrgID).Error; err != nil {
		t.Fatal(err)
	}
	_, foreignProject := seedCanvasActor(t, database)
	foreignSameOrg := seedCanvasMedia(t, database, otherProject, "image")
	foreignOrg := seedCanvasMedia(t, database, foreignProject, "image")
	for _, tc := range []struct {
		name       string
		asset      uuid.UUID
		status     string
		moderation string
		deleted    bool
		kind       string
		invalid    error
	}{
		{name: "other project", asset: foreignSameOrg, invalid: mediaapp.ErrNotFound},
		{name: "other organization", asset: foreignOrg, invalid: mediaapp.ErrNotFound},
		{name: "missing asset", asset: uuid.New(), invalid: mediaapp.ErrNotFound},
		{name: "uploading", status: "uploading", moderation: "pending", invalid: mediaapp.ErrNotFound},
		{name: "processing", status: "processing", invalid: mediaapp.ErrNotFound},
		{name: "failed", status: "failed", invalid: mediaapp.ErrNotFound},
		{name: "pending moderation", status: "processing", moderation: "pending", invalid: mediaapp.ErrNotFound},
		{name: "rejected moderation", status: "rejected", moderation: "rejected", invalid: mediaapp.ErrNotFound},
		{name: "deleted", deleted: true, invalid: mediaapp.ErrNotFound},
		{name: "video", kind: "video", invalid: domain.ErrInvalidCommand},
		{name: "audio", kind: "audio", invalid: domain.ErrInvalidCommand},
		{name: "model", kind: "model", invalid: domain.ErrInvalidCommand},
	} {
		t.Run(tc.name, func(t *testing.T) {
			asset := tc.asset
			if asset == uuid.Nil {
				kind := tc.kind
				if kind == "" {
					kind = "image"
				}
				if kind == "model" {
					asset = uuid.New()
					if err := database.Exec(`INSERT INTO media.media_asset(id,project_id,kind,origin,status,object_key,file_name,mime_type,byte_size,codec,moderation_status) VALUES(?,?,'model','upload','ready',?,'fixture.glb','model/gltf-binary',128,'glb2','passed')`, asset, project, "projects/"+project.String()+"/model/2026/10/"+asset.String()+".glb").Error; err != nil {
						t.Fatal(err)
					}
				} else {
					asset = seedCanvasMedia(t, database, project, kind)
				}
				status, moderation := tc.status, tc.moderation
				if status == "" {
					status = "ready"
				}
				if moderation == "" {
					moderation = "passed"
				}
				var failure any
				if status == "failed" {
					failure = "synthetic ingest failure"
				}
				if err := database.Exec(`UPDATE media.media_asset SET status=?,moderation_status=?,is_delete=?,failure_reason=? WHERE id=?`, status, moderation, tc.deleted, failure, asset).Error; err != nil {
					t.Fatal(err)
				}
			}
			for _, target := range []string{"gallery", "cover"} {
				t.Run(target, func(t *testing.T) {
					config := *node.Config.Director
					config.Shots = append([]domain.DirectorShot(nil), config.Shots...)
					config.Shots[0].Screenshots = []domain.DirectorScreenshot{screenshot}
					config.Cover = &domain.DirectorCover{AssetID: coverAsset, ShotID: config.Shots[0].ID}
					if target == "gallery" {
						config.Shots[0].Screenshots[0].AssetID = asset
					} else {
						config.Cover.AssetID = asset
					}
					failedKey := uuid.NewString()
					_, err := freshService.Execute(t.Context(), actor, doc.ID, failedKey, application.CommandsInput{ExpectedRevision: 2, Commands: []domain.Command{
						{Type: "MoveNodes", Moves: []domain.Move{{ID: node.ID, X: 100, Y: 100}}},
						{Type: "UpdateNodeConfig", ID: node.ID, Config: &domain.NodeConfig{Director: &config}},
					}})
					var indexed *domain.CommandError
					if !errors.Is(err, tc.invalid) || !errors.As(err, &indexed) || indexed.Index != 1 {
						t.Fatalf("unavailable %s image accepted or wrong command index: %v", target, err)
					}
					after, err := freshService.Get(t.Context(), actor, doc.ID)
					if err != nil {
						t.Fatal(err)
					}
					beforeJSON, err := json.Marshal(current)
					if err != nil {
						t.Fatal(err)
					}
					afterJSON, err := json.Marshal(after)
					if err != nil || string(afterJSON) != string(beforeJSON) {
						t.Fatalf("rejected output changed graph: %s %v", afterJSON, err)
					}
					var facts struct{ Events, Logs, Receipts int64 }
					if err := database.Raw(`SELECT (SELECT count(*) FROM infra.outbox WHERE partition_key=?) AS events,(SELECT count(*) FROM canvas.command_log WHERE document_id=?) AS logs,(SELECT count(*) FROM infra.idempotency_record WHERE actor_id=? AND idem_key=?) AS receipts`, project.String(), doc.ID, actor.ID, failedKey).Scan(&facts).Error; err != nil {
						t.Fatal(err)
					}
					if facts.Events != 2 || facts.Logs != 1 || facts.Receipts != 0 {
						t.Fatalf("rejected output persisted side effects: %+v", facts)
					}
				})
			}
		})
	}
}

func TestDirectorGalleryLinksSurviveResourceNodeDeletion(t *testing.T) {
	database := canvasDB(t, "LV_TEST_CANVAS_DB_DSN")
	actor, project := seedCanvasActor(t, database)
	service := application.NewService(canvasStore(database))
	doc, err := service.Create(t.Context(), actor, project, uuid.NewString(), application.CreateInput{Name: "稳定图库引用"})
	if err != nil {
		t.Fatal(err)
	}
	asset := seedCanvasMedia(t, database, project, "image")
	image, node := resourceNode("image"), directorNode()
	image.RefID = &asset
	screenshot := directorScreenshot()
	screenshot.AssetID = asset
	node.Config.Director.Shots[0].Screenshots = []domain.DirectorScreenshot{screenshot}
	node.Config.Director.Cover = &domain.DirectorCover{AssetID: asset, ShotID: node.Config.Director.Shots[0].ID}
	if _, err := service.Execute(t.Context(), actor, doc.ID, uuid.NewString(), application.CommandsInput{ExpectedRevision: 1, Commands: []domain.Command{{Type: "AddNodes", Nodes: []domain.Node{image, node}}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Execute(t.Context(), actor, doc.ID, uuid.NewString(), application.CommandsInput{ExpectedRevision: 2, Commands: []domain.Command{{Type: "DeleteNodes", IDs: []uuid.UUID{image.ID}}}}); err != nil {
		t.Fatal(err)
	}
	current, err := application.NewService(canvasStore(database)).Get(t.Context(), actor, doc.ID)
	if err != nil || len(current.Nodes) != 1 || current.Nodes[0].Config.Director.Cover.AssetID != asset || current.Nodes[0].Config.Director.Shots[0].Screenshots[0].AssetID != asset {
		t.Fatalf("source node deletion lost gallery links: %+v %v", current, err)
	}
	config := *current.Nodes[0].Config.Director
	config.Shots = append([]domain.DirectorShot(nil), config.Shots...)
	config.Shots[0].Screenshots = nil
	config.Cover = nil
	if _, err := service.Execute(t.Context(), actor, doc.ID, uuid.NewString(), application.CommandsInput{ExpectedRevision: 3, Commands: []domain.Command{{Type: "UpdateNodeConfig", ID: node.ID, Config: &domain.NodeConfig{Director: &config}}}}); err != nil {
		t.Fatal(err)
	}
	var removed bool
	if err := database.Raw(`SELECT is_delete FROM media.media_asset WHERE id=?`, asset).Scan(&removed).Error; err != nil || removed {
		t.Fatalf("unlinking gallery deleted original asset: removed=%v %v", removed, err)
	}
	stale := config
	stale.Cover = node.Config.Director.Cover
	_, err = service.Execute(t.Context(), actor, doc.ID, uuid.NewString(), application.CommandsInput{ExpectedRevision: 3, Commands: []domain.Command{{Type: "UpdateNodeConfig", ID: node.ID, Config: &domain.NodeConfig{Director: &stale}}}})
	var conflict *application.RevisionConflict
	if !errors.As(err, &conflict) || conflict.CurrentRevision != 4 {
		t.Fatalf("stale capture overwrote current cover: %v", err)
	}
}

func TestDirectorOutputsHoldMediaEligibilityUntilCommit(t *testing.T) {
	for _, target := range []string{"gallery", "cover"} {
		t.Run(target, func(t *testing.T) {
			database := canvasDB(t, "LV_TEST_CANVAS_DB_DSN")
			actor, project := seedCanvasActor(t, database)
			asset := seedCanvasMedia(t, database, project, "image")
			read, resume := make(chan struct{}, 1), make(chan struct{})
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			var workers sync.WaitGroup
			var release sync.Once
			defer func() { release.Do(func() { close(resume) }); cancel(); workers.Wait() }()
			service := application.NewService(pgcanvas.NewStore(database, func(tx *gorm.DB) application.MediaReader {
				return pausedMediaReference{reader: mediaapp.NewAssetQuery(pgmedia.NewStore(tx), nil), read: read, resume: resume}
			}))
			doc, err := service.Create(ctx, actor, project, uuid.NewString(), application.CreateInput{Name: "截图授权事务"})
			if err != nil {
				t.Fatal(err)
			}
			node := directorNode()
			if target == "gallery" {
				screenshot := directorScreenshot()
				screenshot.AssetID = asset
				node.Config.Director.Shots[0].Screenshots = []domain.DirectorScreenshot{screenshot}
			} else {
				node.Config.Director.Cover = &domain.DirectorCover{AssetID: asset, ShotID: node.Config.Director.Shots[0].ID}
			}
			committed := make(chan error, 1)
			workers.Go(func() {
				_, err := service.Execute(ctx, actor, doc.ID, uuid.NewString(), application.CommandsInput{ExpectedRevision: 1, Commands: []domain.Command{{Type: "AddNodes", Nodes: []domain.Node{node}}}})
				committed <- err
			})
			select {
			case <-read:
			case err := <-committed:
				t.Fatalf("image eligibility was not checked: %v", err)
			case <-ctx.Done():
				t.Fatal("image eligibility gate timed out")
			}
			mutated := make(chan error, 1)
			pidReady := make(chan int, 1)
			workers.Go(func() {
				err := database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
					var pid int
					if err := tx.Raw(`SELECT pg_backend_pid()`).Scan(&pid).Error; err != nil {
						return err
					}
					pidReady <- pid
					return tx.Exec(`UPDATE media.media_asset SET status='processing',moderation_status='pending' WHERE id=?`, asset).Error
				})
				mutated <- err
			})
			var pid int
			select {
			case pid = <-pidReady:
			case err := <-mutated:
				t.Fatalf("media updater failed before lock check: %v", err)
			case <-ctx.Done():
				t.Fatal("media updater did not start")
			}
			ticker := time.NewTicker(10 * time.Millisecond)
			defer ticker.Stop()
			for {
				var blocked bool
				if err := database.WithContext(ctx).Raw(`SELECT cardinality(pg_blocking_pids(?)) > 0`, pid).Scan(&blocked).Error; err != nil {
					t.Fatal(err)
				}
				if blocked {
					break
				}
				select {
				case err := <-mutated:
					t.Fatalf("media eligibility was mutable before canvas commit: %v", err)
				case <-ticker.C:
				case <-ctx.Done():
					t.Fatal("PostgreSQL did not observe the reference lock")
				}
			}
			release.Do(func() { close(resume) })
			if err := <-committed; err != nil {
				t.Fatalf("canvas commit failed: %v", err)
			}
			if err := <-mutated; err != nil {
				t.Fatalf("media updater did not resume: %v", err)
			}
			if _, err := mediaapp.NewAssetQuery(pgmedia.NewStore(database), nil).Reference(ctx, actor, project, asset); !errors.Is(err, mediaapp.ErrNotFound) {
				t.Fatalf("later media state bypassed current authorization: %v", err)
			}
		})
	}
}
