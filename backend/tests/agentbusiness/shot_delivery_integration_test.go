package agentbusiness_test

import (
	"encoding/json"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain"
	agentwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/agent"
	"github.com/StephenQiu30/glowloom-server/backend/internal/app/storageoperator"
	canvas "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	canvasdomain "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	media "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	operationdomain "github.com/StephenQiu30/glowloom-server/backend/internal/operation/domain"
	"github.com/StephenQiu30/glowloom-server/backend/tests/storagefixture"
)

// Metadata fixtures isolate adoption, atomic Yjs/export admission and replay.
// They do not prove supplier execution or real video encoding; those require the
// separate original-generation/FFmpeg acceptance test.
func shotCandidate(t *testing.T, db *gorm.DB, g domain.Grant, canvasID, nodeID uuid.UUID, revision int64, asset uuid.UUID) (uuid.UUID, uuid.UUID) {
	t.Helper()
	provider, model, version, op, output := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	require.NoError(t, db.Exec(`INSERT INTO catalog.provider(id,key,name,adapter_key,region) VALUES(?,?,'shot fixture','mock','domestic')`, provider, "shot-"+provider.String()).Error)
	require.NoError(t, db.Exec(`INSERT INTO catalog.model_profile(id,model_key,provider_id,capability,display_name) VALUES(?,?,?,'video.generate','shot fixture')`, model, "shot-"+model.String(), provider).Error)
	require.NoError(t, db.Exec(`INSERT INTO catalog.model_profile_version(id,model_profile_id,version_no,provider_model_id,modes,limits,param_schema,supports_query,supports_cancel,supports_callback,expected_max_ms) VALUES(?,?,1,'shot',ARRAY['text2video'],'{}','[]',true,false,false,1000)`, version, model).Error)
	source, _ := json.Marshal(map[string]any{"canvas_id": canvasID, "node_id": nodeID, "revision": revision})
	require.NoError(t, db.Exec(`INSERT INTO operation.operation(id,project_id,target_type,capability,mode,model_profile_version_id,params,input_hash,origin,status,region,source_context,settled_micros) VALUES(?,?,'free','video.generate','text2video',?,'{}',?,'canvas','completed','domestic',?::jsonb,0)`, op, g.Scope.ProjectID, version, domain.Hash([]byte("shot")), string(source)).Error)
	require.NoError(t, db.Exec(`INSERT INTO operation.operation_output(id,project_id,operation_id,seq_no,kind,media_asset_id,moderation_status) VALUES(?,?,?,1,'media',?,'passed')`, output, g.Scope.ProjectID, op, asset).Error)
	require.NoError(t, db.Exec(`UPDATE media.media_asset SET origin='generated',source_operation_id=?,provider_key='fixture',model_key='fixture',region='domestic' WHERE id=?`, op, asset).Error)
	require.NoError(t, db.Exec(`INSERT INTO operation.operation_input(id,operation_id,seq_no,role,ref_type,text_value) VALUES(?,?,0,'prompt','text','one shot source')`, uuid.New(), op).Error)
	return op, output
}

func TestShotDeliveryAtomicAdoptionTimelineExportsAndReplay(t *testing.T) {
	for _, planned := range []bool{true, false} {
		name := "unplanned"
		if planned {
			name = "source_frozen"
		}
		t.Run(name, func(t *testing.T) { testShotDeliveryGoalLifecycle(t, planned) })
	}
}

func testShotDeliveryGoalLifecycle(t *testing.T, planned bool) {
	if dsn := os.Getenv("GLOWLOOM_TEST_AGENT_SHOT_DB_DSN"); dsn != "" {
		t.Setenv("GLOWLOOM_TEST_AGENT_CANVAS_DB_DSN", dsn)
	} else {
		t.Skip("dedicated isolated shot database required")
	}
	db, b, router, actor, g := realCanvasBoundary(t)
	status, created, msg := businessToolRequest(t, router, g, "canvas_create", "shot-canvas", map[string]any{"name": "one-shot", "kind": "short_drama"})
	require.Equal(t, 200, status, msg)
	var doc canvasdomain.Document
	require.NoError(t, json.Unmarshal(created.Result, &doc))
	text := "one shot source"
	node := canvasdomain.Node{ID: uuid.New(), Title: "shot", NodeType: "text", NodeAction: "resource", Config: canvasdomain.NodeConfig{Text: &text}}
	status, prepared, msg := businessToolRequest(t, router, g, "canvas_commands", "shot-source", agentwire.CanvasCommandsInput{CanvasID: doc.ID, ExpectedRevision: doc.Revision, Operations: []canvasdomain.Operation{{Type: "create_node", Node: &node}}})
	require.Equal(t, 200, status, msg)
	var changed struct {
		Revision canvas.Revision `json:"revision"`
	}
	require.NoError(t, json.Unmarshal(prepared.Result, &changed))
	asset := uuid.New()
	loc := uuid.New()
	key := "projects/" + g.Scope.ProjectID.String() + "/video/2026/11/" + asset.String() + ".mp4"
	require.NoError(t, pgmedia.NewStorageRoutes(db).RegisterProfiles(t.Context(), []media.StorageProfile{{ID: "test-main", Provider: "cos", Endpoint: "http://127.0.0.1:19000", SigningEndpoint: "http://127.0.0.1:19000", Region: "us-east-1", Bucket: "shot-delivery-1250000000"}}))
	require.NoError(t, db.Exec(`INSERT INTO media.object_location(id,profile_id,bucket,object_key,state,byte_size,sha256,mime_type)VALUES(?,'test-main','shot-delivery-1250000000',?,'accepted',16,repeat('a',64),'video/mp4')`, loc, key).Error)
	require.NoError(t, db.Exec(`INSERT INTO media.media_asset(id,project_id,kind,origin,status,object_key,object_location_id,file_name,mime_type,byte_size,sha256,moderation_status)VALUES(?,?,'video','upload','ready',?,?,'shot.mp4','video/mp4',16,repeat('a',64),'passed')`, asset, g.Scope.ProjectID, key, loc).Error)
	require.NoError(t, db.Exec(`UPDATE media.media_asset SET duration_ms=1000,width=64,height=64,fps=25 WHERE id=?`, asset).Error)
	op, output := shotCandidate(t, db, g, doc.ID, node.ID, int64(changed.Revision), asset)
	cfg := storagefixture.Configuration(t, "http://127.0.0.1:19000", "shot-delivery-1250000000")
	environment, err := storageoperator.Configuration(cfg)
	require.NoError(t, err)
	environment.Revision = strconv.FormatInt(time.Now().UnixNano(), 10)
	routes := pgmedia.NewStorageRoutes(db)
	require.NoError(t, routes.RegisterProfiles(t.Context(), []media.StorageProfile{environment.Profiles[0].StorageProfile}))
	var profiles []media.StorageProfile
	require.NoError(t, db.Table("media.storage_profile").Select("id,provider,endpoint,signing_endpoint,region,bucket,addressing_style").Order("id").Scan(&profiles).Error)
	environment.Profiles = nil
	for _, profile := range profiles {
		environment.Profiles = append(environment.Profiles, media.StorageDeploymentProfile{StorageProfile: profile, Mode: media.StorageProfileModeReadWrite})
	}
	environment.ConfigSHA256 = domain.ContentHash(environment)
	_, err = routes.ApplyEnvironment(t.Context(), environment)
	require.NoError(t, err)
	input := map[string]any{"canvas_id": doc.ID, "node_id": node.ID, "operation_id": op, "output_id": output, "expected_revision": int64(changed.Revision), "duration_ms": 500, "source_start_ms": 0, "aspect_ratio": "1:1", "fps": 25, "project_format": "fcpxml"}
	// Admit the business goal before any proposal, using the real restricted
	// store. This does not ask a provider to generate or encode media.
	admitted, e := b.Store.CurrentAuthority(t.Context(), g.RootRunID)
	require.NoError(t, e)
	admission := admitted.Admission
	admission.RootRunID, admission.SessionID, admission.SubmissionID, admission.RequestID = uuid.New(), uuid.New(), uuid.New(), uuid.New()
	admission.GoalContract = domain.GoalContract{Kind: domain.GoalProduction, Source: &operationdomain.CanvasSource{CanvasID: doc.ID, NodeID: node.ID, Revision: int64(changed.Revision)}, Delivery: &domain.GoalDeliverySpec{DurationMS: 500, AspectRatio: "1:1", FPS: 25, ProjectFormat: "fcpxml"}}
	wrong := admission
	wrongCanvas := uuid.New()
	wrong.Scope.CanvasID = &wrongCanvas
	require.ErrorIs(t, b.Store.AdmitRun(t.Context(), wrong), domain.ErrConflict)
	wrong = admission
	badSource := *admission.GoalContract.Source
	badSource.CanvasID = uuid.New()
	wrong.GoalContract.Source = &badSource
	require.ErrorIs(t, b.Store.AdmitRun(t.Context(), wrong), domain.ErrNotFound)
	if !planned {
		admission.GoalContract.Source = nil
	}
	require.NoError(t, b.Store.AdmitRun(t.Context(), admission))
	g, e = b.Store.RegisterGrant(t.Context(), domain.GrantRequest{RootRunID: admission.RootRunID, SubmissionID: admission.SubmissionID, OwnerEpoch: 1, AuthorizationRevision: 1, LeaseUntil: time.Now().Add(time.Minute)})
	require.NoError(t, e)
	goal := shotGoalCompletion(t, b, g)
	require.Equal(t, domain.DeliveryIncomplete, goal.Status)
	require.Contains(t, goal.Missing, "required_delivery_missing")
	if !planned {
		require.Contains(t, goal.Missing, "goal_source_unplanned")
	}
	initialHash := goal.ContractHash
	for _, invalid := range []struct {
		field  string
		value  any
		status int
	}{
		{"output_id", uuid.New(), 404},
		{"expected_revision", int64(changed.Revision) - 1, 409},
		{"duration_ms", 2000, 409},
		{"node_id", uuid.New(), 409},
		{"fps", 0, 422},
		{"aspect_ratio", "unsupported", 422},
	} {
		bad := make(map[string]any, len(input))
		for k, v := range input {
			bad[k] = v
		}
		bad[invalid.field] = invalid.value
		status, _, msg = businessToolRequest(t, router, g, "shot_delivery_propose", "invalid-"+invalid.field, bad)
		require.Equal(t, invalid.status, status, msg)
	}
	status, _, msg = businessToolRequest(t, router, g, "shot_delivery_create", "unapproved-delivery", input)
	require.Equal(t, 409, status, msg)
	extra := uuid.New()
	require.NoError(t, db.Exec(`INSERT INTO operation.operation_input(id,operation_id,seq_no,role,ref_type,media_asset_id) VALUES(?,?,1,'reference_image','media_asset',?)`, extra, op, asset).Error)
	status, _, msg = businessToolRequest(t, router, g, "shot_delivery_propose", "different-original-references", input)
	require.Equal(t, 409, status, msg)
	require.NoError(t, db.Exec(`DELETE FROM operation.operation_input WHERE id=?`, extra).Error)
	require.NoError(t, db.Exec(`UPDATE media.media_asset SET fps=NULL WHERE id=?`, asset).Error)
	status, _, msg = businessToolRequest(t, router, g, "shot_delivery_propose", "missing-source-frame-rate", input)
	require.Equal(t, 409, status, msg)
	require.NoError(t, db.Exec(`UPDATE media.media_asset SET fps=25 WHERE id=?`, asset).Error)
	status, proposed, msg := businessToolRequest(t, router, g, "shot_delivery_propose", "propose-delivery", input)
	require.Equal(t, 200, status, msg)
	var proposal domain.Proposal
	require.NoError(t, json.Unmarshal(proposed.Result, &proposal))
	approved, e := b.Store.Decide(t.Context(), actor, g.Scope.ProjectID, g.RootRunID, domain.DecisionInput{RequestID: uuid.New(), Action: "approve", ProposalID: proposal.ProposalID, CandidateHash: proposal.CandidateHash, ExpectedAuthorizationRevision: proposal.AuthorizationRevision})
	require.NoError(t, e)
	g, e = b.Store.Quiesce(t.Context(), g.RootRunID, domain.QuiescedReceipt{ReceiptID: uuid.New(), DecisionID: approved.DecisionID, AuthorizationRevision: approved.AuthorizationRevision, OwnerEpoch: 1, ContinuationID: domain.ContinuationID(g.RootRunID, approved.DecisionID), LeaseUntil: time.Now().Add(time.Minute)})
	require.NoError(t, e)
	view, e := b.Capabilities.RunSnapshot(t.Context(), g)
	require.NoError(t, e)
	required := view.(map[string]any)["shot_deliveries"].([]map[string]any)
	require.Len(t, required, 1, "approved production obligation exists before successful create")
	require.Equal(t, domain.DeliveryIncomplete, required[0]["status"])
	goal = shotGoalCompletion(t, b, g)
	require.Equal(t, domain.DeliveryIncomplete, goal.Status, "approved does not mean created")
	// Inject and restore a revision-only race in this metadata fixture. The
	// approved contract must reject it before any canvas/export mutation.
	require.NoError(t, db.Exec(`UPDATE media.media_asset SET revision=revision+1 WHERE id=?`, asset).Error)
	status, _, msg = businessToolRequest(t, router, g, "shot_delivery_create", "stale-approved-asset", input)
	require.Equal(t, 409, status, msg)
	require.NoError(t, db.Exec(`UPDATE media.media_asset SET revision=revision-1 WHERE id=?`, asset).Error)
	status, delivery, msg := businessToolRequest(t, router, g, "shot_delivery_create", "deliver", input)
	require.Equal(t, 200, status, msg)
	var result struct {
		DeliveryID      uuid.UUID `json:"delivery_id"`
		TimelineNodeID  uuid.UUID `json:"timeline_node_id"`
		VideoExportID   uuid.UUID `json:"video_export_id"`
		ProjectExportID uuid.UUID `json:"project_export_id"`
		Status          string    `json:"status"`
	}
	require.NoError(t, json.Unmarshal(delivery.Result, &result))
	require.NotEqual(t, uuid.Nil, result.DeliveryID)
	snapshotView, snapshotErr := b.Capabilities.RunSnapshot(t.Context(), g)
	require.NoError(t, snapshotErr)
	shotFacts := snapshotView.(map[string]any)["shot_deliveries"].([]map[string]any)
	require.Len(t, shotFacts, 1)
	require.Equal(t, result.VideoExportID, shotFacts[0]["video_export_id"])
	require.Equal(t, result.ProjectExportID, shotFacts[0]["project_export_id"])
	require.Equal(t, "pending", result.Status)
	var nodes, jobs int64
	require.NoError(t, db.Raw(`SELECT count(*) FROM canvas.node WHERE document_id=? AND NOT is_delete`, doc.ID).Scan(&nodes).Error)
	require.EqualValues(t, 3, nodes)
	require.NoError(t, db.Raw(`SELECT count(*) FROM mediatool.export_job WHERE canvas_id=?`, doc.ID).Scan(&jobs).Error)
	require.EqualValues(t, 2, jobs)
	for _, call := range []string{"deliver", "another-tool-turn"} {
		status, replay, message := businessToolRequest(t, router, g, "shot_delivery_create", call, input)
		require.Equal(t, 200, status, message)
		var again map[string]any
		require.NoError(t, json.Unmarshal(replay.Result, &again))
		require.Equal(t, result.DeliveryID.String(), again["delivery_id"])
	}
	require.NoError(t, db.Raw(`SELECT count(*) FROM mediatool.export_job WHERE canvas_id=?`, doc.ID).Scan(&jobs).Error)
	require.EqualValues(t, 2, jobs)
	status, live, msg := businessToolRequest(t, router, g, "shot_delivery_get", "read-delivery", map[string]any{"delivery_id": result.DeliveryID})
	require.Equal(t, 200, status, msg)
	var snapshot map[string]any
	require.NoError(t, json.Unmarshal(live.Result, &snapshot))
	require.Equal(t, "pending", snapshot["status"])
	goal = shotGoalCompletion(t, b, g)
	require.Equal(t, domain.DeliveryPending, goal.Status)
	// Metadata fixtures prove completion follows current original job, review,
	// source and artifact records; actual worker/bytes are covered separately.
	finalAsset, finalLocation, artifact, artifactLocation := uuid.New(), uuid.New(), uuid.NewSHA1(result.ProjectExportID, []byte("export-artifact/1")), uuid.New()
	finalKey := "projects/" + g.Scope.ProjectID.String() + "/video/2026/11/" + finalAsset.String() + ".mp4"
	var exportCreatedAt time.Time
	require.NoError(t, db.Raw(`SELECT created_at FROM mediatool.export_job WHERE id=?`, result.ProjectExportID).Scan(&exportCreatedAt).Error)
	artifactKey := "projects/" + g.Scope.ProjectID.String() + "/exports/" + exportCreatedAt.UTC().Format("2006/01") + "/" + artifact.String() + ".zip"
	require.NoError(t, db.Exec(`INSERT INTO media.object_location(id,profile_id,bucket,object_key,state,byte_size,sha256,mime_type)VALUES(?,'test-main','shot-delivery-1250000000',?,'accepted',16,repeat('b',64),'video/mp4'),(?,'test-main','shot-delivery-1250000000',?,'accepted',16,repeat('c',64),'application/zip')`, finalLocation, finalKey, artifactLocation, artifactKey).Error)
	require.NoError(t, db.Exec(`INSERT INTO media.media_asset(id,project_id,kind,origin,status,object_key,object_location_id,file_name,mime_type,byte_size,sha256,moderation_status) VALUES(?,?,'video','upload','ready',?,?,'final.mp4','video/mp4',16,repeat('b',64),'passed')`, finalAsset, g.Scope.ProjectID, finalKey, finalLocation).Error)
	require.NoError(t, db.Exec(`UPDATE media.media_asset SET duration_ms=500,width=64,height=64,fps=25 WHERE id=?`, finalAsset).Error)
	require.NoError(t, db.Exec(`INSERT INTO mediatool.export_artifact(id,job_id,project_id,attempt,format,format_version,file_name,mime_type,byte_size,sha256,object_key,object_location_id,created_at) VALUES(?,?,?,1,'fcpxml','1.10',?,'application/zip',16,repeat('c',64),?,?,now())`, artifact, result.ProjectExportID, g.Scope.ProjectID, "timeline-"+result.ProjectExportID.String()+".zip", artifactKey, artifactLocation).Error)
	require.NoError(t, db.Exec(`UPDATE mediatool.export_job SET status='review_required',asset_id=?,sha256=repeat('b',64),progress=100 WHERE id=?`, finalAsset, result.VideoExportID).Error)
	require.NoError(t, db.Exec(`UPDATE mediatool.export_job SET status='review_required',artifact_id=?,sha256=repeat('c',64),progress=100 WHERE id=?`, artifact, result.ProjectExportID).Error)
	require.Equal(t, domain.DeliveryAwaitingReview, shotGoalCompletion(t, b, g).Status)
	require.NoError(t, db.Exec(`UPDATE mediatool.export_job SET status='succeeded' WHERE id IN (?,?)`, result.VideoExportID, result.ProjectExportID).Error)
	goal = shotGoalCompletion(t, b, g)
	require.Equal(t, domain.DeliveryCompleted, goal.Status)
	require.Equal(t, 1, goal.CompletedDeliveries, "replayed tool receipts count one stable delivery")
	require.Empty(t, goal.Missing)
	require.NotEmpty(t, goal.ContractHash)
	require.Equal(t, initialHash, goal.ContractHash, "planning through an approved candidate does not change the frozen goal")
	other := admission
	other.RootRunID, other.SessionID, other.SubmissionID, other.RequestID = uuid.New(), uuid.New(), uuid.New(), uuid.New()
	other.GoalContract.Source = nil
	require.NoError(t, b.Store.AdmitRun(t.Context(), other))
	otherGrant, err := b.Store.RegisterGrant(t.Context(), domain.GrantRequest{RootRunID: other.RootRunID, SubmissionID: other.SubmissionID, OwnerEpoch: 1, AuthorizationRevision: 1, LeaseUntil: time.Now().Add(time.Minute)})
	require.NoError(t, err)
	require.Equal(t, domain.DeliveryIncomplete, shotGoalCompletion(t, b, otherGrant).Status, "another root cannot borrow completed work")
	reused, _, message := businessToolRequest(t, router, otherGrant, "shot_delivery_create", "reused-unapproved", input)
	require.Equal(t, 200, reused, message)
	require.Equal(t, domain.DeliveryIncomplete, shotGoalCompletion(t, b, otherGrant).Status, "a reused original receipt without this root's approved plan cannot fulfill its production goal")
	require.NoError(t, db.Exec(`UPDATE agent_business.run_authorization SET goal_contract=goal_contract-'source' WHERE root_run_id=?`, g.RootRunID).Error)
	require.Equal(t, domain.DeliveryCompleted, shotGoalCompletion(t, b, g).Status, "same-root approved frozen candidate forms a valid single-shot plan")
	frozenGoal, err := json.Marshal(admission.GoalContract)
	require.NoError(t, err)
	require.NoError(t, db.Exec(`UPDATE agent_business.run_authorization SET goal_contract=?::jsonb WHERE root_run_id=?`, string(frozenGoal), g.RootRunID).Error)
	// A different admitted source revision/spec cannot borrow this success.
	changes := []string{`jsonb_set(goal_contract,'{delivery,duration_ms}',to_jsonb(600::bigint))`, `jsonb_set(goal_contract,'{required_deliveries}',to_jsonb(2::int))`}
	if planned {
		changes = append(changes, `jsonb_set(goal_contract,'{source,revision}',to_jsonb(999::bigint))`, `jsonb_set(goal_contract,'{source,node_id}',to_jsonb('00000000-0000-0000-0000-000000000001'::text))`)
	}
	for _, change := range changes {
		require.NoError(t, db.Exec(`UPDATE agent_business.run_authorization SET goal_contract=`+change+` WHERE root_run_id=?`, g.RootRunID).Error)
		require.Equal(t, domain.DeliveryIncomplete, shotGoalCompletion(t, b, g).Status)
		frozen, err := json.Marshal(admission.GoalContract)
		require.NoError(t, err)
		require.NoError(t, db.Exec(`UPDATE agent_business.run_authorization SET goal_contract=?::jsonb WHERE root_run_id=?`, string(frozen), g.RootRunID).Error)
	}
	require.NoError(t, db.Exec(`UPDATE media.media_asset SET is_delete=true WHERE id=?`, finalAsset).Error)
	require.Equal(t, domain.DeliveryIncomplete, shotGoalCompletion(t, b, g).Status, "success rows do not replace current usable output")
}

func shotGoalCompletion(t *testing.T, b *agentwire.Boundary, g domain.Grant) domain.GoalCompletion {
	t.Helper()
	view, err := b.Capabilities.RunSnapshot(t.Context(), g)
	require.NoError(t, err)
	raw, err := json.Marshal(view)
	require.NoError(t, err)
	var result struct {
		Completion domain.GoalCompletion `json:"goal_completion"`
	}
	require.NoError(t, json.Unmarshal(raw, &result))
	return result.Completion
}

func TestShotDeliveryRejectsUnrelatedOrUnreadySource(t *testing.T) {
	_, _, router, _, g := realCanvasBoundary(t)
	status, _, msg := businessToolRequest(t, router, g, "shot_delivery_create", "invalid-source", map[string]any{"canvas_id": uuid.New(), "node_id": uuid.New(), "operation_id": uuid.New(), "output_id": uuid.New(), "expected_revision": 1, "duration_ms": 1000, "source_start_ms": 0, "aspect_ratio": "1:1", "fps": 25, "project_format": "fcpxml"})
	require.Contains(t, []int{404, 409}, status, msg)
	// The descriptor must exist; a missing tool isn't an authorization check.
	status, _, msg = businessToolRequest(t, router, g, "shot_delivery_create", "malformed-source", map[string]any{})
	require.Equal(t, 422, status, msg)
}
