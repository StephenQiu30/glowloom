package canvas_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	mediawire "github.com/StephenQiu30/glowloom-server/backend/internal/app/media"
	pgcanvas "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/adapter/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

type saveBeforeReadAuthorization struct {
	application.ProjectAccess
	save func() error
}

func (a saveBeforeReadAuthorization) AuthorizeProject(ctx context.Context, actor identityapp.Principal, project uuid.UUID, write bool) error {
	if err := a.save(); err != nil {
		return err
	}
	return a.ProjectAccess.AuthorizeProject(ctx, actor, project, write)
}

func TestDAGGetRetriesWholeSnapshotAfterConcurrentCommitAndBoundsRetries(t *testing.T) {
	for _, continuous := range []bool{false, true} {
		t.Run(map[bool]string{false: "one concurrent commit", true: "bounded contention"}[continuous], func(t *testing.T) {
			db := canvasDB(t, "GLOWLOOM_TEST_CANVAS_DB_DSN")
			actor, project := seedCanvasActor(t, db)
			writer := application.NewService(canvasStore(db))
			doc, err := writer.Create(t.Context(), actor, project, uuid.NewString(), application.CreateInput{Name: "一致性读取"})
			if err != nil {
				t.Fatal(err)
			}
			node := resourceNode("text")
			if _, err := writer.Execute(t.Context(), actor, doc.ID, uuid.NewString(), application.CommandsInput{ExpectedRevision: 1, Commands: []domain.Command{{Type: "AddNodes", Nodes: []domain.Node{node}}}}); err != nil {
				t.Fatal(err)
			}
			calls := 0
			revision := int64(2)
			reader := pgcanvas.NewScopedStore(db, nil, func(tx *gorm.DB) application.ProjectAccess {
				return saveBeforeReadAuthorization{ProjectAccess: mediawire.NewCanvasProjectAccess(tx), save: func() error {
					calls++
					if continuous || calls == 1 {
						_, err := writer.Execute(t.Context(), actor, doc.ID, uuid.NewString(), application.CommandsInput{ExpectedRevision: revision, Commands: []domain.Command{{Type: "MoveNodes", Moves: []domain.Move{{ID: node.ID, X: 35}}}}})
						if err != nil {
							return err
						}
						revision++
					}
					return nil
				}}
			})
			got, err := reader.Get(t.Context(), actor, doc.ID)
			if continuous {
				var serialization *pgconn.PgError
				if !errors.As(err, &serialization) || serialization.Code != "40001" || calls != 3 {
					t.Fatalf("retry not bounded: calls=%d err=%v", calls, err)
				}
			} else if err != nil || got.Revision != 3 || len(got.Nodes) != 1 || got.Nodes[0].X != 35 || calls != 2 {
				t.Fatalf("concurrent save broke consistent GET: calls=%d doc=%+v err=%v", calls, got, err)
			}
		})
	}
}

func TestDAGReadOnlyGraphCannotAdmitNewExecutionInputs(t *testing.T) {
	for _, kind := range []string{"unclassified_relation", "opaque_timeline", "opaque_audio"} {
		t.Run(kind, func(t *testing.T) {
			db := canvasDB(t, "GLOWLOOM_TEST_CANVAS_DB_DSN")
			actor, project := seedCanvasActor(t, db)
			store := canvasStore(db)
			service := application.NewService(store)
			doc, err := service.Create(t.Context(), actor, project, uuid.NewString(), application.CreateInput{Name: "只读图任务门禁"})
			if err != nil {
				t.Fatal(err)
			}
			timeline, audio := timelineNode(), resourceNode("audio")
			audio.RefID = ptr(seedCanvasMedia(t, db, project, "audio"))
			if _, err := service.Execute(t.Context(), actor, doc.ID, uuid.NewString(), application.CommandsInput{ExpectedRevision: 1, Commands: []domain.Command{{Type: "AddNodes", Nodes: []domain.Node{timeline, audio}}}}); err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "unclassified_relation":
				err = db.Exec(`INSERT INTO canvas.edge(id,document_id,edge_type,source_node_id,target_node_id) VALUES(?,?,'input',?,?)`, uuid.New(), doc.ID, audio.ID, timeline.ID).Error
			case "opaque_timeline":
				err = db.Exec(`UPDATE canvas.node SET config=jsonb_set(config,'{timeline,future_export_field}','true'::jsonb) WHERE id=?`, timeline.ID).Error
			case "opaque_audio":
				err = db.Exec(`UPDATE canvas.node SET config=config || '{"future_source_field":true}'::jsonb WHERE id=?`, audio.ID).Error
			}
			if err != nil {
				t.Fatal(err)
			}
			loaded, err := service.Get(t.Context(), actor, doc.ID)
			if err != nil || loaded.ReadOnlyReason == "" {
				t.Fatal("graph not readable as protected", err)
			}
			if _, err := application.NewTimelineReader(store).FreezeTimeline(t.Context(), actor, project, doc.ID, timeline.ID, 2); !errors.Is(err, domain.ErrGraphReadOnly) || !errors.Is(err, domain.ErrInvalidCommand) {
				t.Fatal("export admitted from protected graph", err)
			}
			if _, err := application.NewTranscriptionSourceReader(store).FreezeTranscriptionSource(t.Context(), actor, project, doc.ID, audio.ID, 2); !errors.Is(err, domain.ErrGraphReadOnly) || !errors.Is(err, domain.ErrInvalidCommand) {
				t.Fatal("source admitted from protected graph", err)
			}
			var revision, logs, receipts, operations int64
			for query, dest := range map[string]*int64{`SELECT revision FROM canvas.document WHERE id=?`: &revision, `SELECT count(*) FROM canvas.command_log WHERE document_id=?`: &logs, `SELECT count(*) FROM canvas.save_receipt WHERE document_id=?`: &receipts} {
				if err := db.Raw(query, doc.ID).Scan(dest).Error; err != nil {
					t.Fatal(err)
				}
			}
			if err := db.Raw(`SELECT count(*) FROM operation.operation WHERE project_id=?`, project).Scan(&operations).Error; err != nil {
				t.Fatal(err)
			}
			if revision != 2 || logs != 1 || receipts != 1 || operations != 0 {
				t.Fatal("admission changed graph or created work", revision, logs, receipts, operations)
			}
		})
	}
}
