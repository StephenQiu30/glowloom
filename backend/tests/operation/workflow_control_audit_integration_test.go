package operation_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	auditevent "github.com/StephenQiu30/glowloom-server/backend/internal/audit/adapter/event"
	auditapp "github.com/StephenQiu30/glowloom-server/backend/internal/audit/application"
	pginbox "github.com/StephenQiu30/glowloom-server/backend/internal/infra/inbox/adapter/postgres"
	inbox "github.com/StephenQiu30/glowloom-server/backend/internal/infra/inbox/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
)

func TestWorkflowControlProducedAuditsConsumeAndReplayWithoutEventChanges(t *testing.T) {
	for _, scenario := range []struct{ kind, action, status string }{{"operation", "recover", "manual"}, {"operation", "cancel", "confirmed"}, {"batch", "cancel", "confirmed"}, {"batch", "resume", "running"}} {
		t.Run(scenario.kind+"_"+scenario.action, func(t *testing.T) {
			db := operationStoreDB(t)
			actor, pid := operationStoreProject(t, db)
			batch, items := seedProjectBatch(t, db, actor, pid, 1)
			target := batch
			if scenario.kind == "operation" {
				target = items[0].Operation.ID
				require.NoError(t, db.Exec(`UPDATE operation.operation SET batch_id=NULL,status=? WHERE id=?`, scenario.status, target).Error)
				if scenario.action == "recover" {
					require.NoError(t, db.Exec(`UPDATE catalog.provider SET adapter_key='mock' WHERE id=(SELECT m.provider_id FROM catalog.model_profile m JOIN catalog.model_profile_version v ON v.model_profile_id=m.id WHERE v.id=?)`, items[0].Operation.ModelProfileVersionID).Error)
					require.NoError(t, db.Exec(`UPDATE catalog.model_profile_version SET supports_query=true,queue='agent.mock' WHERE id=?`, items[0].Operation.ModelProfileVersionID).Error)
					require.NoError(t, db.Exec(`INSERT INTO operation.provider_call(id,project_id,operation_id,provider_key,action,attempt,region,outcome,provider_task_id,response_summary,request_summary) VALUES(?,?,?,'mock','submit',1,'domestic','ok','original-task','{"state":"accepted"}','{}')`, uuid.New(), pid, target).Error)
				}
			} else {
				require.NoError(t, db.Exec(`UPDATE operation.batch SET status=?,paused_reason='budget_guard' WHERE id=?`, scenario.status, target).Error)
			}
			key := uuid.NewString()
			input := application.WorkflowControlInput{ProjectID: pid, TargetID: target, TargetType: scenario.kind, Action: scenario.action, RequestID: key}
			command := application.NewWorkflowControlCommand(operationStore(db))
			accepted, err := command.Execute(t.Context(), actor, input)
			require.NoError(t, err)
			require.True(t, accepted.Accepted)
			replay, err := command.Execute(t.Context(), actor, input)
			require.NoError(t, err)
			require.Equal(t, accepted, replay)
			var rows []struct {
				ID                  uuid.UUID
				Topic, PartitionKey string
				Payload             []byte
			}
			require.NoError(t, db.Raw(`SELECT id,topic,partition_key,payload FROM infra.outbox WHERE topic='glowloom.audit.recorded.v1' AND payload->'data'->>'request_id'=?`, key).Scan(&rows).Error)
			require.Len(t, rows, 1)
			r := rows[0]
			record := inbox.Record{Topic: r.Topic, Key: []byte(r.PartitionKey), Value: r.Payload}
			parser := auditapp.NewRecordedActionParser()
			parsed, err := parser.Parse(record)
			require.NoError(t, err)
			require.Equal(t, scenario.kind+"."+scenario.action+"_requested", parsed.Action)
			require.Equal(t, target.String(), parsed.ObjectID)
			require.Equal(t, key, parsed.RequestID)
			handler := auditevent.NewHandler(pginbox.NewStore(db), parser)
			require.NoError(t, handler.Handle(t.Context(), record))
			require.NoError(t, handler.Handle(t.Context(), record))
			var audits, markers int64
			require.NoError(t, db.Raw(`SELECT count(*) FROM audit.audit_log WHERE id=?`, r.ID).Scan(&audits).Error)
			require.NoError(t, db.Raw(`SELECT count(*) FROM infra.processed_event WHERE consumer='audit' AND event_id=?`, r.ID).Scan(&markers).Error)
			require.EqualValues(t, 1, audits)
			require.EqualValues(t, 1, markers)
		})
	}
}
