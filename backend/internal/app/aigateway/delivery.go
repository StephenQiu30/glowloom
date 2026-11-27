package aigateway

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	mediawire "github.com/StephenQiu30/glowloom-server/backend/internal/app/media"
	scopewire "github.com/StephenQiu30/glowloom-server/backend/internal/app/scope"
	subject "github.com/StephenQiu30/glowloom-server/backend/internal/creation/subject/application"
	catalog "github.com/StephenQiu30/glowloom-server/backend/internal/execution/catalog/domain"
	pggateway "github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/adapter/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/domain"
	operation "github.com/StephenQiu30/glowloom-server/backend/internal/execution/operation/application"
	mediaobjects "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/objectstorage"
	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	mediaflow "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/workflow"
	media "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	mediadomain "github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
	objectplatform "github.com/StephenQiu30/glowloom-server/backend/internal/platform/objectstorage"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
)

// GatewayObjectReads opens only a previously admitted, exact physical object.
// Recovery rehashes the actual bytes rather than trusting stored metadata.
type GatewayObjectReads interface {
	Get(context.Context, objectplatform.Ref) (io.ReadCloser, error)
}

// GatewayArchiver receives only saved bytes and source metadata, never paid send authority.
type GatewayArchiver interface {
	ArchiveGatewaySource(context.Context, media.SourceOperation, uuid.UUID, *media.Downloaded) (mediadomain.MediaAsset, []mediadomain.Rendition, error)
}

// VoiceMediaPreparer is the existing personal Media byte pipeline.
type VoiceMediaPreparer interface {
	Prepare(context.Context, media.PersonalGeneratedAudioRequest, *media.Downloaded) (media.PreparedPersonalGeneratedAudio, error)
}

// OutputSummary contains no provider credentials, physical routes or private URLs.
type OutputSummary struct {
	RequestID uuid.UUID               `json:"request_id"`
	AssetID   uuid.UUID               `json:"asset_id"`
	Kind      string                  `json:"kind"`
	Status    string                  `json:"status"`
	SHA256    string                  `json:"sha256"`
	Voice     *subject.VoiceJobResult `json:"voice,omitempty"`
}

type deliverySource struct {
	Call       domain.Call
	Receipt    domain.Receipt
	ReceiptSHA string
	Route      domain.Route
	Created    time.Time
}
type outputManifest struct {
	RequestID   uuid.UUID `json:"request_id"`
	AssetID     uuid.UUID `json:"asset_id"`
	ReceiptSHA  string    `json:"receipt_sha256"`
	SHA256      string    `json:"sha256"`
	MIMEType    string    `json:"mime_type"`
	ByteSize    int64     `json:"byte_size"`
	Kind        string    `json:"kind"`
	Policy      string    `json:"policy"`
	AspectRatio string    `json:"aspect_ratio"`
}

func outputAssetID(id uuid.UUID) uuid.UUID {
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte("ai-output/"+id.String()+"/1"))
}

// PinMediaOutput is called before Queue in the very same admission transaction.
// Resolving the exact physical profile is local and performs no supplier request.
func (r *Runtime) PinMediaOutput(ctx context.Context, tx *gorm.DB, call domain.Call) error {
	if call.Model.Protocol != "media" {
		return nil
	}
	if r == nil || r.Objects == nil || tx == nil || call.ID == uuid.Nil || call.Project == uuid.Nil {
		return domain.ErrUnavailable
	}
	owner := media.StorageWorkOwner{Kind: media.StorageWorkKindOperation, OwnerID: call.Project, WorkID: call.ID}
	intent, err := pgmedia.NewStorageRoutes(tx).PinStorageWork(ctx, owner)
	if err != nil {
		return err
	}
	if intent.State != media.StorageWorkOpen {
		return domain.ErrConflict
	}
	key := "projects/" + call.Project.String() + "/" + strings.TrimSuffix(call.Model.Capability, ".generate") + "/admission/" + outputAssetID(call.ID).String()
	return r.Objects.CheckWrite(ctx, objectplatform.Ref{ProfileID: intent.ProfileID, Bucket: intent.Bucket, Key: key})
}

func (r *Runtime) deliverySource(ctx context.Context, id uuid.UUID) (deliverySource, error) {
	var out deliverySource
	if r == nil || r.DB == nil || id == uuid.Nil {
		return out, domain.ErrInvalid
	}
	var row struct {
		Contract, Receipt, Route, Ciphertext []byte
		State, Digest, KeyID                 string
		CreatedAt                            time.Time
	}
	q := r.DB.WithContext(ctx).Raw(`SELECT q.contract,r.state,r.created_at,a.receipt,a.digest,a.contract AS route,k.key_id,k.ciphertext FROM ai.request r JOIN ai.quote q ON q.id=r.id JOIN ai.attempt a ON a.request_id=r.id AND a.receipt IS NOT NULL JOIN ai.credential k ON k.id=a.credential_id WHERE r.id=? ORDER BY a.number DESC LIMIT 1`, id).Scan(&row)
	if q.Error != nil {
		return out, q.Error
	}
	if q.RowsAffected != 1 {
		return out, domain.ErrUnavailable
	}
	if json.Unmarshal(row.Contract, &out.Call) != nil || out.Call.ID != id || out.Call.Model.Protocol != "media" || json.Unmarshal(row.Receipt, &out.Receipt) != nil || out.Receipt.Pending || json.Unmarshal(row.Route, &out.Route) != nil {
		return out, domain.ErrInvalid
	}
	if row.State != "settled" && (!out.Call.Model.UsesVideoDeliveryMeter() || row.State != "received" && row.State != "unknown") {
		return out, domain.ErrUnavailable
	}
	// Hash the canonical saved receipt; the original transport digest remains in ai.attempt.
	raw, err := domain.CanonicalJSON(row.Receipt)
	if err != nil || len(row.Digest) != 64 {
		return out, domain.ErrConflict
	}
	out.ReceiptSHA = domain.Digest(raw)
	out.Created = row.CreatedAt
	out.Route.KeyID = row.KeyID
	out.Route.Ciphertext = row.Ciphertext
	return out, nil
}

// Deliver archives the original successful receipt without dispatching a model.
// Billable video archives wake settlement once the asset is ready.
func (r *Runtime) Deliver(ctx context.Context, id uuid.UUID) error {
	source, err := r.deliverySource(ctx, id)
	if err != nil {
		return err
	}
	var existing struct{ Summary []byte }
	q := r.DB.WithContext(ctx).Raw("SELECT summary FROM ai.output WHERE request_id=? AND state IN ('pending','ready')", id).Scan(&existing)
	if q.Error != nil {
		return q.Error
	}
	if q.RowsAffected == 1 {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	file, manifest, err := r.deliveryInput(ctx, source)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	kind := manifest.Kind
	token, err := r.claimDelivery(ctx, manifest)
	if err != nil {
		return err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_ = r.DB.WithContext(cleanup).Exec("UPDATE ai.output SET lease_id=NULL,lease_until=NULL WHERE request_id=? AND lease_id=? AND state='preparing'", id, token).Error
	}()
	var voice voiceIntentRow
	query := r.DB.WithContext(ctx).Raw("SELECT * FROM ai.voice_intent WHERE request_id=?", id).Scan(&voice)
	if query.Error != nil {
		return query.Error
	}
	if query.RowsAffected == 1 {
		return r.deliverVoice(ctx, source, voice, manifest, token, file)
	}
	if source.Call.Project == uuid.Nil {
		return domain.ErrInvalid
	}
	if err := r.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error { _, err := deliveryProjectActor(ctx, tx, source.Call); return err }); err != nil {
		return err
	}
	archive := r.MediaArchive
	if archive == nil {
		if r.Objects == nil {
			return domain.ErrUnavailable
		}
		routes := pgmedia.NewStorageRoutes(r.DB)
		archive = media.NewGeneratedOutputArchiver(routes, mediaobjects.NewGeneratedObjects(r.Objects, routes), mediaflow.FFProber{}, mediaflow.FFRenderer{})
	}
	var config catalog.ProviderConfig
	if json.Unmarshal(source.Route.DriverConfig, &config) != nil || config.Validate() != nil {
		return domain.ErrInvalid
	}
	region := string(config.Region)
	facts := media.SourceOperation{ID: id, ProjectID: source.Call.Project, ProviderKey: source.Route.Supplier.String(), ModelKey: source.Call.Model.Name, Region: region, AspectRatio: manifest.AspectRatio, OutputKind: mediadomain.Kind(kind), CreateTime: source.Created}
	asset, rends, err := archive.ArchiveGatewaySource(ctx, facts, manifest.AssetID, file)
	if err != nil {
		return err
	}
	if asset.SourceAIRequestID == nil || *asset.SourceAIRequestID != id || asset.SHA256 == nil || *asset.SHA256 != manifest.SHA256 || asset.ByteSize != manifest.ByteSize || asset.MimeType != manifest.MIMEType {
		return media.ErrObjectMismatch
	}
	state := "pending"
	if manifest.Policy == "provider_success_v1" {
		asset.ModerationStatus = mediadomain.ModerationPassed
		asset.Status = mediadomain.StatusReady
		asset.ModerationDetail = canonical(map[string]any{"method": "ai_gateway_provider_success_v1", "request_id": id, "receipt_sha256": manifest.ReceiptSHA, "output_sha256": manifest.SHA256})
		state = "ready"
	}
	summary := OutputSummary{RequestID: id, AssetID: asset.ID, Kind: kind, Status: string(asset.Status), SHA256: manifest.SHA256}
	return r.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := r.verifyDeliveryClaim(tx, id, token, manifest); err != nil {
			return err
		}
		actor, err := deliveryProjectActor(ctx, tx, source.Call)
		if err != nil {
			return err
		}
		if err := pgmedia.NewScopedStore(tx, mediawire.NewMediaWorkerScopeAccess).AcceptGatewayOutput(ctx, actor, asset, rends); err != nil {
			return err
		}
		if err := finishDelivery(tx, id, token, state, summary); err != nil {
			return err
		}
		if source.Call.Model.UsesVideoDeliveryMeter() && state == "ready" {
			return taskruntime.Enqueue(ctx, tx, pggateway.SettlementKind, id.String(), time.Now())
		}
		return nil
	})
}

func (r *Runtime) claimDelivery(ctx context.Context, m outputManifest) (uuid.UUID, error) {
	token := uuid.New()
	raw := canonical(m)
	hash := domain.Digest(raw)
	err := r.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("INSERT INTO ai.output(request_id,asset_id,receipt_sha256,sha256,manifest_sha256,manifest,kind) VALUES(?,?,?,?,?,?::jsonb,?) ON CONFLICT DO NOTHING", m.RequestID, m.AssetID, m.ReceiptSHA, m.SHA256, hash, string(raw), m.Kind).Error; err != nil {
			return err
		}
		var row struct {
			ManifestSHA256, State string
			LeaseUntil            *time.Time
		}
		q := tx.Raw("SELECT manifest_sha256,state,lease_until FROM ai.output WHERE request_id=? FOR UPDATE", m.RequestID).Scan(&row)
		if q.Error != nil {
			return q.Error
		}
		if row.ManifestSHA256 != hash {
			return domain.ErrConflict
		}
		if row.State != "preparing" || row.LeaseUntil != nil && row.LeaseUntil.After(time.Now()) {
			return domain.ErrUnavailable
		}
		return tx.Exec("UPDATE ai.output SET lease_id=?,lease_until=clock_timestamp()+interval '11 minutes' WHERE request_id=?", token, m.RequestID).Error
	})
	return token, err
}
func (r *Runtime) verifyDeliveryClaim(tx *gorm.DB, id, token uuid.UUID, m outputManifest) error {
	var n int64
	if err := tx.Raw("SELECT count(*) FROM ai.output WHERE request_id=? AND lease_id=? AND lease_until>clock_timestamp() AND state='preparing' AND manifest_sha256=?", id, token, domain.Digest(canonical(m))).Scan(&n).Error; err != nil {
		return err
	}
	if n != 1 {
		return domain.ErrConflict
	}
	return nil
}
func finishDelivery(tx *gorm.DB, id, token uuid.UUID, state string, summary OutputSummary) error {
	q := tx.Exec("UPDATE ai.output SET state=?,summary=?::jsonb,lease_id=NULL,lease_until=NULL,updated_at=clock_timestamp() WHERE request_id=? AND lease_id=? AND state='preparing'", state, string(canonical(summary)), id, token)
	if q.Error != nil {
		return q.Error
	}
	if q.RowsAffected != 1 {
		return domain.ErrConflict
	}
	return nil
}

func (r *Runtime) deliveryBytes(ctx context.Context, s deliverySource) (*media.Downloaded, error) {
	var result operation.GenerationResult
	if json.Unmarshal(s.Receipt.Body, &result) != nil || result.State != operation.ProviderTaskSucceeded || result.Outcome != operation.ProviderOutcomeCompleted || len(result.Images)+len(result.Audio)+len(result.URLs) != 1 {
		return nil, domain.ErrInvalid
	}
	var data []byte
	var mime string
	if len(result.Images) == 1 {
		data = result.Images[0].Bytes
		mime = result.Images[0].MediaType
	}
	if len(result.Audio) == 1 {
		data = result.Audio[0].Bytes
		mime = result.Audio[0].MediaType
	}
	if len(data) > 0 {
		file, err := media.ReadUpload(ctx, bytes.NewReader(data), "output"+mediaExtension(mime))
		if err != nil {
			return nil, err
		}
		if file.MIMEType != mime {
			_ = file.Close()
			return nil, media.ErrObjectMismatch
		}
		return file, nil
	}
	if len(result.URLs) != 1 {
		return nil, domain.ErrInvalid
	}
	return r.downloadResult(ctx, s, result, result.URLs[0])
}
func mediaExtension(mime string) string {
	switch mime {
	case "image/png":
		return ".png"
	case "image/jpeg":
		return ".jpg"
	case "image/webp":
		return ".webp"
	case "image/gif":
		return ".gif"
	case "audio/mpeg":
		return ".mp3"
	case "audio/wave", "audio/x-wav":
		return ".wav"
	case "audio/ogg":
		return ".ogg"
	case "video/mp4":
		return ".mp4"
	default:
		return ".bin"
	}
}

func deliveryProjectActor(ctx context.Context, tx *gorm.DB, call domain.Call) (media.Actor, error) {
	scope, err := scopewire.ProvideScopeAccess(tx).Resolve(ctx, call.Actor, call.Scope, true)
	if err != nil {
		return media.Actor{}, err
	}
	actor := media.Actor{ID: call.Actor, ScopeID: call.Scope, OrgID: scope.PhysicalScopeID}
	if err := mediawire.NewMediaWorkerScopeAccess(tx).AuthorizeProject(ctx, actor, call.Project, true); err != nil {
		return actor, err
	}
	return actor, nil
}

func (r *Runtime) deliveryAspect(ctx context.Context, id, project uuid.UUID) (string, error) {
	var row struct{ Aspect string }
	q := r.DB.WithContext(ctx).Raw("SELECT manifest->>'aspect_ratio' AS aspect FROM ai.output WHERE request_id=?", id).Scan(&row)
	if q.Error != nil {
		return "", q.Error
	}
	if q.RowsAffected == 1 && row.Aspect != "" {
		return row.Aspect, nil
	}
	if project == uuid.Nil {
		return "16:9", nil
	}
	var aspect string
	if err := r.DB.WithContext(ctx).Raw("SELECT aspect_ratio FROM workspace.project WHERE id=? AND NOT is_delete", project).Row().Scan(&aspect); err != nil {
		return "", err
	}
	return aspect, nil
}
