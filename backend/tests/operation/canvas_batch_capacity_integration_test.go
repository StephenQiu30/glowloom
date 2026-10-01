package operation_test

import (
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	pgoperation "github.com/StephenQiu30/lanverse/backend/internal/operation/adapter/postgres"
	operationapp "github.com/StephenQiu30/lanverse/backend/internal/operation/application"
)

func TestCanvasSplitBatchesShareSavedConcurrencyAndReleaseOnlySettledSlots(t *testing.T) {
	database := operationStoreDB(t)
	actor, project := operationStoreProject(t, database)
	quoted, model, _ := seedQuoteCatalog(t, database, project, time.Now().UTC())
	setPublicQuoteLimits(t, database, quoted.ModelProfileVersionID)
	if err := database.Exec(`UPDATE catalog.provider SET concurrency_limit=32 WHERE id=(SELECT provider_id FROM catalog.model_profile WHERE id=?)`, model).Error; err != nil {
		t.Fatal(err)
	}
	canvas, sourceA, sourceB := uuid.New(), uuid.New(), uuid.New()
	rows := []uuid.UUID{uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()}
	prompts := []string{"A", "B", "C", "D", "E", "F"}
	if err := database.Exec(`INSERT INTO canvas.document(id,project_id,name,revision) VALUES(?,?,'分组统一并发',3)`, canvas, project).Error; err != nil {
		t.Fatal(err)
	}
	for _, source := range []uuid.UUID{sourceA, sourceB} {
		configured := make([]map[string]any, len(rows))
		for i, row := range rows {
			configured[i] = map[string]any{"id": row, "enabled": true, "prompt": prompts[i], "input_node_ids": []any{nil}}
		}
		body, err := json.Marshal(configured)
		if err != nil {
			t.Fatal(err)
		}
		config := fmt.Sprintf(`{"batch_table":{"version":1,"operation":"creative","mode":"text_to_image","output_count":1,"global_prompt":"","concurrency":1,"model_profile_id":"%s","params":{},"reference_columns":[{"id":"%s","label":"图1"}],"rows":%s}}`, model, uuid.New(), body)
		if err := database.Exec(`INSERT INTO canvas.node(id,document_id,node_type,node_action,config,x,y) VALUES(?,?,'batch_table','tool',?::jsonb,0,0)`, source, canvas, config).Error; err != nil {
			t.Fatal(err)
		}
	}
	router := publicOperationRouter(actor, database)
	var sourceRevision = 3
	create := func(source uuid.UUID, row int) operationapp.CreateBatchFreeQuoteResult {
		t.Helper()
		item := func(i int) string {
			return fmt.Sprintf(`{"model_key":"quote-model-%s","capability":"%s","mode":"text_to_image","prompt":"%s","params":{},"output_count":1,"source":{"canvas_id":"%s","node_id":"%s","row_id":"%s","revision":%d}}`, model, quoted.Capability, prompts[i], canvas, source, rows[i], sourceRevision)
		}
		response := publicOperationRequest(t, router, "POST", "/api/projects/"+project.String()+"/quotes", `{"items":[`+item(row)+`,`+item(row+1)+`]}`, uuid.NewString())
		var batch operationapp.CreateBatchFreeQuoteResult
		if response.Code != 201 || json.Unmarshal(response.Body.Bytes(), &batch) != nil || batch.BatchID == nil || batch.Items[0].OperationID == nil {
			t.Fatalf("split quote: %d %s", response.Code, response.Body)
		}
		response = publicOperationRequest(t, router, "POST", "/api/batches/"+batch.BatchID.String()+"/confirm", `{"project_id":"`+project.String()+`"}`, uuid.NewString())
		if response.Code != 200 {
			t.Fatalf("split confirm: %d %s", response.Code, response.Body)
		}
		if err := database.Exec(`UPDATE operation.batch SET status='running' WHERE id=?`, batch.BatchID).Error; err != nil {
			t.Fatal(err)
		}
		return batch
	}
	groups := []operationapp.CreateBatchFreeQuoteResult{create(sourceA, 0), create(sourceA, 2)}
	store := pgoperation.NewStore(database)
	var workers sync.WaitGroup
	admitted := make([]bool, 2)
	errors := make([]error, 2)
	for index := range groups {
		workers.Go(func() {
			admitted[index], errors[index] = store.AcquireBatchLaunch(t.Context(), *groups[index].BatchID, *groups[index].Items[0].OperationID)
		})
	}
	workers.Wait()
	if errors[0] != nil || errors[1] != nil || admitted[0] == admitted[1] {
		t.Fatalf("split groups exceeded saved concurrency: admitted=%v errors=%v", admitted, errors)
	}
	independent := create(sourceB, 4)
	if ok, err := store.AcquireBatchLaunch(t.Context(), *independent.BatchID, *independent.Items[0].OperationID); err != nil || !ok {
		t.Fatalf("another source node was incorrectly serialized: %t %v", ok, err)
	}
	if err := database.Exec(`UPDATE canvas.node SET config=jsonb_set(config,'{batch_table,concurrency}','2'::jsonb) WHERE id=?`, sourceA).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Exec(`UPDATE canvas.document SET revision=4 WHERE id=?`, canvas).Error; err != nil {
		t.Fatal(err)
	}
	sourceRevision = 4
	wider := create(sourceA, 4)
	if ok, err := store.AcquireBatchLaunch(t.Context(), *wider.BatchID, *wider.Items[0].OperationID); err != nil || ok {
		t.Fatalf("new source revision bypassed older active limit: %t %v", ok, err)
	}
	active := 0
	if admitted[1] {
		active = 1
	}
	operation := *groups[active].Items[0].OperationID
	if err := database.Transaction(func(tx *gorm.DB) error { return store.ReleaseBatchLaunchInTransaction(t.Context(), tx, operation) }); err == nil {
		t.Fatal("unsettled active child released capacity")
	}
	if err := database.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`UPDATE operation.operation SET status='completed',settled_micros=0 WHERE id=?`, operation).Error; err != nil {
			return err
		}
		return store.ReleaseBatchLaunchInTransaction(t.Context(), tx, operation)
	}); err != nil {
		t.Fatal(err)
	}
	waiting := groups[1-active]
	if ok, err := store.AcquireBatchLaunch(t.Context(), *waiting.BatchID, *waiting.Items[0].OperationID); err != nil || !ok {
		t.Fatalf("settled slot did not unblock next group: %t %v", ok, err)
	}
}
