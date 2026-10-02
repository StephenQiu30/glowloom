package media_test

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.temporal.io/api/enums/v1"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"

	inboxpg "github.com/StephenQiu30/lanverse/backend/internal/infra/inbox/adapter/postgres"
	inboxapp "github.com/StephenQiu30/lanverse/backend/internal/infra/inbox/application"
	mediaevent "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/event"
	pgmedia "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/postgres"
	mediaflow "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/workflow"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
)

type isolatedPurgeClient struct {
	client.Client
	queue string
}

func (c isolatedPurgeClient) ExecuteWorkflow(ctx context.Context, options client.StartWorkflowOptions, function any, args ...any) (client.WorkflowRun, error) {
	options.TaskQueue = c.queue
	args[1] = mediaflow.PurgeWorkflowConfig{ActivityTaskQueue: c.queue}
	return c.Client.ExecuteWorkflow(ctx, options, function, args...)
}

func TestMediaPurgeActualTemporalPrivateObjectsAndUnknownRecovery(t *testing.T) {
	address, namespace := os.Getenv("LV_TEST_TEMPORAL_ADDR"), os.Getenv("LV_TEST_TEMPORAL_NAMESPACE")
	if address == "" || namespace == "" {
		t.Skip("set authorized Temporal address/namespace with an isolated owning queue")
	}
	c, err := client.Dial(client.Options{HostPort: address, Namespace: namespace})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	for _, unknownOutcome := range []bool{false, true} {
		t.Run(map[bool]string{false: "actual all-object removal", true: "actual unknown removal then exact reconciliation"}[unknownOutcome], func(t *testing.T) {
			db := libraryRuntimeDB(t)
			actor, _, in, keys, objects := purgeBinaryFixture(t, db)
			repo := pgmedia.NewPurgeStore(db, libraryTestAccess, transferTestGuards, purgeTestReferenceFactory, time.Now)
			job, err := repo.CreatePurge(t.Context(), actor, in)
			if err != nil {
				t.Fatal(err)
			}
			queue := "mediaPurge-test-" + job.ID.String()
			private := &purgeUnknownRemove{PurgeObjects: objects, once: unknownOutcome}
			activities := mediaflow.NewPurgeActivities(mediaapp.NewPurgeWorker(repo, private), repo)
			w := worker.New(c, queue, worker.Options{})
			mediaflow.RegisterPurgeWorkflow(w)
			mediaflow.RegisterPurgeActivities(w, activities)
			if err := w.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(w.Stop)
			handler := mediaevent.NewPurgeHandler(inboxpg.NewStore(db), repo, mediaflow.NewPurgeStarter(isolatedPurgeClient{Client: c, queue: queue}))
			deliver := func(key uuid.UUID) (string, error) {
				var row struct {
					Payload      []byte
					PartitionKey string
				}
				if err := db.Raw(`SELECT o.payload,o.partition_key FROM media.purge_command q JOIN infra.outbox o ON o.id=q.event_id WHERE q.actor_id=? AND q.idem_key=?`, actor.ID, key).Scan(&row).Error; err != nil {
					return "", err
				}
				var event struct {
					Data mediaapp.PurgeDelivery `json:"data"`
				}
				if err := json.Unmarshal(row.Payload, &event); err != nil {
					return "", err
				}
				err := handler.Handle(t.Context(), inboxapp.Record{Topic: mediaevent.PurgeTopic, Key: []byte(row.PartitionKey), Value: row.Payload})
				id := "media-purge/" + job.ID.String() + "/1"
				if event.Data.Action == "reconcile" {
					id = "media-purge/" + job.ID.String() + "/2/reconcile/" + key.String()
				}
				return id, err
			}
			name, err := deliver(in.Key)
			if err != nil {
				t.Fatal(err)
			}
			ctx, stop := context.WithTimeout(t.Context(), 45*time.Second)
			defer stop()
			var current domain.PurgeJob
			if err := c.GetWorkflow(ctx, name, "").Get(ctx, &current); err != nil {
				t.Fatal(err)
			}
			if unknownOutcome {
				if current.Status != "needs_reconciliation" || private.removeCount != 1 {
					t.Fatal("unknown actual deletion fabricated a terminal result", current)
				}
				key := uuid.New()
				if _, err := repo.ControlPurge(ctx, actor, job.ID, key, current.Revision, "reconcile"); err != nil {
					t.Fatal(err)
				}
				name, err = deliver(key)
				if err != nil {
					t.Fatal(err)
				}
				if err := c.GetWorkflow(ctx, name, "").Get(ctx, &current); err != nil {
					t.Fatal(err)
				}
			}
			if current.Status != "succeeded" || private.removeCount != len(keys) {
				t.Fatal("actual private-object workflow did not settle exactly", current, private.removeCount)
			}
			for _, key := range keys {
				exists, err := objects.Exists(ctx, key)
				if err != nil || exists {
					t.Fatal("actual key still exists after successful workflow", err)
				}
			}
			iterator := c.GetWorkflowHistory(ctx, name, "", false, enums.HISTORY_EVENT_FILTER_TYPE_ALL_EVENT)
			scheduled := 0
			for iterator.HasNext() {
				event, err := iterator.Next()
				if err != nil {
					t.Fatal(err)
				}
				if event.EventType == enums.EVENT_TYPE_ACTIVITY_TASK_SCHEDULED {
					scheduled++
				}
			}
			if scheduled != 1 {
				t.Fatal("automatic destructive activity retry or fake history", scheduled)
			}
		})
	}
}
