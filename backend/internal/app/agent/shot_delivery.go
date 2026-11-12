package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain"
	mediawire "github.com/StephenQiu30/glowloom-server/backend/internal/app/media"
	canvas "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	canvasdomain "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	media "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	mediadomain "github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
	mediatool "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/application"
	tooldomain "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/domain"
	operation "github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	operationdomain "github.com/StephenQiu30/glowloom-server/backend/internal/operation/domain"
)

const (
	shotDeliveryProposeTool   = "shot_delivery_propose"
	shotDeliveryCreateTool    = "shot_delivery_create"
	shotDeliveryGetTool       = "shot_delivery_get"
	shotDeliveryCandidateKind = "shot_delivery"
	shotDeliveryPriceVersion  = "local-shot-delivery-v1"
)

// ShotDeliveryInput selects an original candidate, never a URL or a paid retry.
type ShotDeliveryInput struct {
	CanvasID         uuid.UUID             `json:"canvas_id"`
	NodeID           uuid.UUID             `json:"node_id"`
	OperationID      uuid.UUID             `json:"operation_id"`
	OutputID         uuid.UUID             `json:"output_id"`
	ExpectedRevision int64                 `json:"expected_revision"`
	DurationMS       int64                 `json:"duration_ms"`
	SourceStartMS    int64                 `json:"source_start_ms"`
	AspectRatio      string                `json:"aspect_ratio"`
	FPS              int                   `json:"fps"`
	ProjectFormat    tooldomain.OutputKind `json:"project_format"`
}

type shotDeliveryCandidate struct {
	Kind          string                       `json:"kind"`
	Input         ShotDeliveryInput            `json:"input"`
	Source        operationdomain.CanvasSource `json:"source"`
	AssetID       uuid.UUID                    `json:"asset_id"`
	AssetRevision int64                        `json:"asset_revision"`
	AssetSHA256   string                       `json:"asset_sha256"`
}

func (c shotDeliveryCandidate) matches(task operation.TaskDetail, asset mediadomain.MediaAsset) bool {
	return c.Kind == shotDeliveryCandidateKind && task.Source != nil && domain.ContentHash(c.Source) == domain.ContentHash(*task.Source) && c.AssetID == asset.ID && c.AssetRevision == asset.Revision && asset.SHA256 != nil && c.AssetSHA256 == *asset.SHA256
}

type shotDelivery struct {
	DeliveryID      uuid.UUID                    `json:"delivery_id"`
	Input           ShotDeliveryInput            `json:"input"`
	Source          operationdomain.CanvasSource `json:"source"`
	AssetID         uuid.UUID                    `json:"asset_id"`
	AssetRevision   int64                        `json:"asset_revision"`
	AssetSHA256     string                       `json:"asset_sha256"`
	AdoptedNodeID   uuid.UUID                    `json:"adopted_node_id"`
	TimelineNodeID  uuid.UUID                    `json:"timeline_node_id"`
	CanvasRevision  canvas.Revision              `json:"canvas_revision"`
	VideoExportID   uuid.UUID                    `json:"video_export_id"`
	ProjectExportID uuid.UUID                    `json:"project_export_id"`
	Status          domain.DeliveryStatus        `json:"status"`
	Jobs            []tooldomain.ExportJob       `json:"jobs"`
}

func (in ShotDeliveryInput) valid() bool {
	return in.CanvasID != uuid.Nil && in.NodeID != uuid.Nil && in.OperationID != uuid.Nil && in.OutputID != uuid.Nil && in.ExpectedRevision > 0 && deliverySpec(in).Valid()
}

func shotDeliveryTools() []toolBinding {
	propose := tool(shotDeliveryProposeTool, "Propose adoption of one original video output and a frozen timeline/video/project delivery. Stops the whole Agent tree for explicit approval; does not generate or render.", "write", commandSchema(ShotDeliveryInput{}), nil)
	create := tool(shotDeliveryCreateTool, "After approval of the exact shot_delivery_propose candidate, atomically adopt the original video, save a new one-shot timeline and admit original video/project export jobs. A queued job is not a completed film.", "write", commandSchema(ShotDeliveryInput{}), nil)
	propose.descriptor.Roles = []string{"producer", "edit"}
	create.descriptor.Roles = []string{"producer", "edit"}
	get := tool(shotDeliveryGetTool, "Read the original shot delivery and real export/review states. Never create another generation or render job.", "read", `{"type":"object","properties":{"delivery_id":{"type":"string","format":"uuid"}},"required":["delivery_id"],"additionalProperties":false}`, func(ctx context.Context, tx *gorm.DB, s domain.Scope, _ uuid.UUID, raw json.RawMessage) (any, error) {
		var in struct {
			DeliveryID uuid.UUID `json:"delivery_id"`
		}
		if decode(raw, &in) != nil || in.DeliveryID == uuid.Nil {
			return nil, domain.ErrInvalid
		}
		return readShotDelivery(ctx, tx, s, in.DeliveryID)
	})
	return []toolBinding{propose, create, get}
}

func deliveryIdentity(s domain.Scope, in ShotDeliveryInput) uuid.UUID {
	return uuid.NewSHA1(s.ProjectID, []byte("shot-delivery:v1:"+s.ActorID.String()+":"+s.ScopeID.String()+":"+domain.ContentHash(in)))
}

// loadShotCandidate rechecks owning application facts before both approval and
// acceptance. Merely marking an output succeeded is insufficient for adoption.
func loadShotCandidate(ctx context.Context, tx *gorm.DB, s domain.Scope, in ShotDeliveryInput) (operation.TaskDetail, mediadomain.MediaAsset, error) {
	if !in.valid() {
		return operation.TaskDetail{}, mediadomain.MediaAsset{}, domain.ErrInvalid
	}
	if s.CanvasID != nil && *s.CanvasID != in.CanvasID {
		return operation.TaskDetail{}, mediadomain.MediaAsset{}, domain.ErrNotFound
	}
	task, err := operation.NewPublicQuery(operationStore(tx)).Task(ctx, s.Principal(), s.ProjectID, in.OperationID)
	if err != nil {
		return task, mediadomain.MediaAsset{}, businessError(err)
	}
	if task.Status != operationdomain.StatusCompleted || task.Capability != operationdomain.CapabilityVideoGenerate || task.Source == nil || task.Source.CanvasID != in.CanvasID || task.Source.NodeID != in.NodeID || task.Source.RowID != nil {
		return task, mediadomain.MediaAsset{}, domain.ErrBlocked
	}
	var assetID uuid.UUID
	for _, output := range task.Outputs {
		if output.ID == in.OutputID && output.Kind == string(mediadomain.KindVideo) && output.ModerationStatus == string(mediadomain.ModerationPassed) && output.MediaAssetID != nil {
			assetID = *output.MediaAssetID
		}
	}
	if assetID == uuid.Nil {
		return task, mediadomain.MediaAsset{}, domain.ErrNotFound
	}
	repo := pgmedia.NewScopedStore(tx, mediawire.NewMediaWorkerScopeAccess)
	if _, err = media.NewAssetQuery(repo).Reference(ctx, s.Principal(), s.ProjectID, assetID); err != nil {
		return task, mediadomain.MediaAsset{}, businessError(err)
	}
	asset, err := repo.FindAsset(ctx, s.Principal(), s.ProjectID, assetID)
	if err != nil {
		return task, asset, businessError(err)
	}
	if asset.Kind != mediadomain.KindVideo || asset.SourceOperationID == nil || *asset.SourceOperationID != in.OperationID || asset.DurationMS == nil || asset.SHA256 == nil || in.SourceStartMS > int64(*asset.DurationMS) || in.DurationMS > int64(*asset.DurationMS)-in.SourceStartMS {
		return task, asset, domain.ErrConflict
	}
	if asset.FPS == nil || int64(*asset.DurationMS) > canvasdomain.MaxTimelineDurationMS {
		return task, asset, domain.ErrBlocked
	}
	if _, _, ok := mediatool.ProjectSourceFrameRate(*asset.FPS); !ok {
		return task, asset, domain.ErrBlocked
	}
	doc, err := canvasService(tx).Get(ctx, s.Principal(), in.CanvasID)
	if err != nil {
		return task, asset, businessError(err)
	}
	if doc.ProjectID != s.ProjectID || doc.Revision != in.ExpectedRevision {
		return task, asset, domain.ErrConflict
	}
	var node *canvasdomain.Node
	for i := range doc.Nodes {
		if doc.Nodes[i].ID == in.NodeID {
			node = &doc.Nodes[i]
			break
		}
	}
	if node == nil {
		return task, asset, domain.ErrNotFound
	}
	if err = checkShotSource(tx, *node, task); err != nil {
		return task, asset, err
	}
	return task, asset, nil
}

func checkShotSource(tx *gorm.DB, node canvasdomain.Node, task operation.TaskDetail) error {
	var prompt string
	references := []canvasdomain.GenerationReference{}
	for _, input := range task.Inputs {
		if input.Role == "prompt" && input.Text != nil {
			prompt = *input.Text
		}
		if input.MediaAssetID != nil {
			references = append(references, canvasdomain.GenerationReference{Role: input.Role, MediaAssetID: *input.MediaAssetID})
		}
	}
	if node.NodeType == canvasdomain.NodeTypeText {
		input, err := node.TextInput()
		if err == nil && len(references) == 0 && len(input.MediaPins) == 0 && len(input.Draft.References) == 0 && strings.TrimSpace(input.PlainText) == prompt {
			return nil
		}
	}
	gen := node.Config.Generation
	if node.NodeType != canvasdomain.NodeTypeGeneration || gen == nil || gen.ModelProfileID == nil || strings.TrimSpace(gen.Prompt) != prompt || gen.Capability != task.Capability || gen.Mode != task.Mode || gen.OutputCount != int(task.OutputCount) || domain.ContentHash(gen.Inputs) != domain.ContentHash(references) {
		return domain.ErrConflict
	}
	var model struct {
		ID     uuid.UUID
		Schema []byte
	}
	if err := tx.Raw(`SELECT model_profile_id AS id,param_schema AS schema FROM catalog.model_profile_version WHERE id=(SELECT model_profile_version_id FROM operation.operation WHERE id=?)`, task.ID).Scan(&model).Error; err != nil {
		return err
	}
	params, err := operation.NormalizeQuoteParams(gen.Params, model.Schema, task.Mode)
	if err != nil || model.ID != *gen.ModelProfileID || domain.ContentHash(params) != domain.Hash(task.Params) {
		return domain.ErrConflict
	}
	return nil
}

func (b *Boundary) proposeShotDelivery(ctx context.Context, tx *gorm.DB, s domain.Scope, key uuid.UUID, in domain.CallInput) (domain.CallResult, error) {
	var requested ShotDeliveryInput
	if decode(in.Input, &requested) != nil {
		return domain.CallResult{}, domain.ErrInvalid
	}
	task, asset, err := loadShotCandidate(ctx, tx, s, requested)
	if err != nil {
		return domain.CallResult{}, err
	}
	candidate, err := json.Marshal(shotDeliveryCandidate{Kind: shotDeliveryCandidateKind, Input: requested, Source: *task.Source, AssetID: asset.ID, AssetRevision: asset.Revision, AssetSHA256: *asset.SHA256})
	if err != nil {
		return domain.CallResult{}, err
	}
	p, err := b.Store.ProposeInTransaction(tx, domain.ProposalInput{Grant: in.Grant, RequestID: uuid.NewSHA1(key, []byte("proposal")), CandidateHash: domain.Hash(candidate), CanvasRevision: requested.ExpectedRevision, PriceVersion: shotDeliveryPriceVersion, BudgetMicros: 0, Candidate: candidate})
	if err != nil {
		return domain.CallResult{}, err
	}
	raw, err := json.Marshal(map[string]any{"proposal_id": p.ProposalID, "candidate_hash": p.CandidateHash, "authorization_revision": p.AuthorizationRevision, "candidate": p.Candidate, "control": map[string]bool{"terminate": true}})
	return domain.CallResult{Status: "completed", Result: raw}, err
}

func (b *Boundary) executeShotDelivery(ctx context.Context, in domain.CallInput) (domain.CallReceipt, error) {
	if b.canvasRuntime == nil {
		return domain.CallReceipt{}, domain.ErrUnavailable
	}
	var requested ShotDeliveryInput
	if decode(in.Input, &requested) != nil || !requested.valid() || in.Call.ContractVersion != "v1" || domain.Hash(in.Input) != in.Call.InputHash {
		return domain.CallReceipt{}, domain.ErrInvalid
	}
	if old, err := b.Store.ReadCall(ctx, in.Grant.RootRunID, in.Grant.SubmissionID, in.Call); err == nil {
		if old.Name != shotDeliveryCreateTool || old.Kind != "tool" || old.MaxMicros != in.MaxMicros {
			return domain.CallReceipt{}, domain.ErrConflict
		}
		return old, nil
	} else if !errors.Is(err, domain.ErrNotFound) {
		return domain.CallReceipt{}, err
	}
	g, err := b.Store.ValidateGrant(ctx, in.Grant)
	if err != nil {
		return domain.CallReceipt{}, err
	}
	id := deliveryIdentity(g.Scope, requested)
	if old, err := readShotReceipt(b.db.WithContext(ctx), g.Scope, id); err == nil {
		return b.Store.ExecuteAtomicTool(ctx, shotDeliveryCreateTool, in, func(_ *gorm.DB, _ domain.Scope, _ uuid.UUID) (domain.CallResult, error) {
			raw, e := json.Marshal(old)
			return domain.CallResult{Status: "completed", Result: raw}, e
		})
	} else if !errors.Is(err, domain.ErrNotFound) {
		return domain.CallReceipt{}, err
	}
	var frozen shotDeliveryCandidate
	if err = b.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		proposal, e := b.Store.ApprovedProposalInTransaction(tx, in.Grant)
		if errors.Is(e, domain.ErrNotFound) {
			return domain.ErrBlocked
		}
		if e != nil {
			return e
		}
		if json.Unmarshal(proposal.Candidate, &frozen) != nil || frozen.Kind != shotDeliveryCandidateKind || domain.ContentHash(frozen.Input) != domain.ContentHash(requested) {
			return domain.ErrBlocked
		}
		return nil
	}); err != nil {
		return domain.CallReceipt{}, err
	}
	task, asset, err := loadShotCandidate(ctx, b.db.WithContext(ctx), g.Scope, requested)
	if err != nil {
		return domain.CallReceipt{}, err
	}
	if !frozen.matches(task, asset) {
		return domain.CallReceipt{}, domain.ErrConflict
	}
	record := shotDelivery{DeliveryID: id, Input: requested, Source: *task.Source, AssetID: asset.ID, AssetRevision: asset.Revision, AssetSHA256: *asset.SHA256, AdoptedNodeID: uuid.NewSHA1(id, []byte("adopted")), TimelineNodeID: uuid.NewSHA1(id, []byte("timeline")), Status: domain.DeliveryPending}
	track := uuid.NewSHA1(id, []byte("track"))
	duration := int64(*asset.DurationMS)
	timeline := deliverySpec(requested).TimelineConfig()
	timeline.Tracks = []canvasdomain.TimelineTrack{{ID: track, Kind: canvasdomain.NodeTypeVideo, Label: "Shot", Visible: true}}
	timeline.Clips = []canvasdomain.TimelineClip{{ID: uuid.NewSHA1(id, []byte("clip")), TrackID: track, Kind: canvasdomain.NodeTypeVideo, NodeID: &record.AdoptedNodeID, AssetID: &record.AssetID, Title: "Shot", DurationMS: requested.DurationMS, SourceStartMS: requested.SourceStartMS, SourceDurationMS: &duration, Volume: 1}}
	if err = timeline.Validate(); err != nil {
		return domain.CallReceipt{}, domain.ErrInvalid
	}
	commands := []canvasdomain.Operation{{Type: canvasdomain.OperationCreateNode, Node: &canvasdomain.Node{ID: record.AdoptedNodeID, Title: "Adopted shot", NodeType: canvasdomain.NodeTypeVideo, NodeAction: canvasdomain.NodeActionResource, RefType: canvasdomain.RefTypeMediaAsset, RefID: &record.AssetID}}, {Type: canvasdomain.OperationCreateNode, Node: &canvasdomain.Node{ID: record.TimelineNodeID, Title: "Shot timeline", NodeType: canvasdomain.NodeTypeTimeline, NodeAction: canvasdomain.NodeActionTool, Config: canvasdomain.NodeConfig{Timeline: &timeline}, X: 400}}}
	prepared, err := canvas.NewYjsService(canvasStore(b.db), b.canvasRuntime).PrepareCommands(ctx, g.Scope.Principal(), requested.CanvasID, canvas.YjsCommandInput{CommandID: uuid.NewSHA1(id, []byte("adopt-timeline")), SessionID: g.SubmissionID, ExpectedRevision: requested.ExpectedRevision, Operations: commands})
	if err != nil {
		return domain.CallReceipt{}, businessError(err)
	}
	return b.Store.ExecuteAtomicTool(ctx, shotDeliveryCreateTool, in, func(tx *gorm.DB, s domain.Scope, _ uuid.UUID) (domain.CallResult, error) {
		if e := tx.Exec(`SELECT pg_advisory_xact_lock(hashtextextended(?,0))`, "shot-delivery:"+id.String()).Error; e != nil {
			return domain.CallResult{}, e
		}
		if old, e := readShotReceipt(tx, s, id); e == nil {
			raw, e := json.Marshal(old)
			return domain.CallResult{Status: "completed", Result: raw}, e
		} else if !errors.Is(e, domain.ErrNotFound) {
			return domain.CallResult{}, e
		}
		proposal, e := b.Store.ApprovedProposalInTransaction(tx, in.Grant)
		if e != nil {
			return domain.CallResult{}, e
		}
		var approved shotDeliveryCandidate
		if json.Unmarshal(proposal.Candidate, &approved) != nil || approved.Kind != shotDeliveryCandidateKind || domain.ContentHash(approved.Input) != domain.ContentHash(requested) {
			return domain.CallResult{}, domain.ErrBlocked
		}
		currentTask, current, e := loadShotCandidate(ctx, tx, s, requested)
		if e != nil {
			return domain.CallResult{}, e
		}
		if !approved.matches(currentTask, current) || current.Revision != record.AssetRevision || current.SHA256 == nil || *current.SHA256 != record.AssetSHA256 {
			return domain.CallResult{}, domain.ErrConflict
		}
		committed, e := canvas.NewYjsService(canvasStore(tx), b.canvasRuntime).CommitCommands(ctx, s.Principal(), prepared)
		if e != nil {
			return domain.CallResult{}, businessError(e)
		}
		record.CanvasRevision = committed.ContentRevision
		for _, kind := range []tooldomain.OutputKind{tooldomain.OutputVideo, requested.ProjectFormat} {
			job, e := exportStore(tx).Create(ctx, s.Principal(), s.ProjectID, uuid.NewSHA1(id, []byte("export:"+string(kind))), mediatool.CreateInput{CanvasID: requested.CanvasID, NodeID: record.TimelineNodeID, Revision: int64(record.CanvasRevision), OutputKind: kind})
			if e != nil {
				return domain.CallResult{}, businessError(e)
			}
			if kind == tooldomain.OutputVideo {
				record.VideoExportID = job.ID
			} else {
				record.ProjectExportID = job.ID
			}
			record.Jobs = append(record.Jobs, job)
		}
		raw, e := json.Marshal(record)
		return domain.CallResult{Status: "completed", Result: raw}, e
	})
}
