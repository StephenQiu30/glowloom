package aigateway

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path"
	"strings"

	"github.com/google/uuid"

	subject "github.com/StephenQiu30/glowloom-server/backend/internal/creation/subject/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/domain"
	operation "github.com/StephenQiu30/glowloom-server/backend/internal/execution/operation/application"
	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	media "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	mediadomain "github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
	platform "github.com/StephenQiu30/glowloom-server/backend/internal/platform/objectstorage"
)

// deliveryInput prefers our exact original after responsibility was frozen. A
// supplier URL is needed only while no complete owned original can be recovered.
func (r *Runtime) deliveryInput(ctx context.Context, s deliverySource) (*media.Downloaded, outputManifest, error) {
	var manifest outputManifest
	var row struct {
		Manifest                                    []byte
		ManifestSHA256, SHA256, ReceiptSHA256, Kind string
		AssetID                                     uuid.UUID
	}
	q := r.DB.WithContext(ctx).Raw("SELECT manifest,manifest_sha256,sha256,receipt_sha256,kind,asset_id FROM ai.output WHERE request_id=? AND state='preparing'", s.Call.ID).Scan(&row)
	if q.Error != nil {
		return nil, manifest, q.Error
	}
	if q.RowsAffected == 1 {
		if json.Unmarshal(row.Manifest, &manifest) != nil || domain.Digest(canonical(manifest)) != row.ManifestSHA256 || manifest.RequestID != s.Call.ID || manifest.AssetID != outputAssetID(s.Call.ID) || manifest.AssetID != row.AssetID || manifest.ReceiptSHA != s.ReceiptSHA || manifest.ReceiptSHA != row.ReceiptSHA256 || manifest.SHA256 != row.SHA256 || manifest.Kind != row.Kind || manifest.Kind != strings.TrimSuffix(s.Call.Model.Capability, ".generate") || manifest.Policy != s.Call.Model.OutputPolicy || manifest.ByteSize < 1 || manifest.ByteSize > 512<<20 || len(manifest.SHA256) != 64 || mediaExtension(manifest.MIMEType) == ".bin" {
			return nil, manifest, domain.ErrConflict
		}
		var result operation.GenerationResult
		if json.Unmarshal(s.Receipt.Body, &result) != nil || result.Outcome != operation.ProviderOutcomeCompleted || result.State != operation.ProviderTaskSucceeded || len(result.Images)+len(result.Audio)+len(result.URLs) != 1 {
			return nil, manifest, domain.ErrInvalid
		}
		if len(result.URLs) == 1 {
			file, found, err := r.recoverOriginal(ctx, s, manifest)
			if err != nil || found {
				return file, manifest, err
			}
		}
	}
	file, err := r.deliveryBytes(ctx, s)
	if err != nil {
		return nil, manifest, err
	}
	if q.RowsAffected == 1 {
		if file.SHA256 != manifest.SHA256 || file.Size != manifest.ByteSize || file.MIMEType != manifest.MIMEType {
			_ = file.Close()
			return nil, manifest, media.ErrObjectMismatch
		}
		return file, manifest, nil
	}
	aspect, err := r.deliveryAspect(ctx, s.Call.ID, s.Call.Project)
	if err != nil {
		_ = file.Close()
		return nil, manifest, err
	}
	manifest = outputManifest{RequestID: s.Call.ID, AssetID: outputAssetID(s.Call.ID), ReceiptSHA: s.ReceiptSHA, SHA256: file.SHA256, MIMEType: file.MIMEType, ByteSize: file.Size, Kind: strings.TrimSuffix(s.Call.Model.Capability, ".generate"), Policy: s.Call.Model.OutputPolicy, AspectRatio: aspect}
	return file, manifest, nil
}

func (r *Runtime) recoverOriginal(ctx context.Context, s deliverySource, m outputManifest) (*media.Downloaded, bool, error) {
	reader := r.OutputReads
	if reader == nil && r.Objects != nil {
		reader = r.Objects
	}
	if reader == nil {
		return nil, false, nil
	}
	owner := media.StorageWorkOwner{Kind: media.StorageWorkKindOperation, OwnerID: s.Call.Project, WorkID: s.Call.ID}
	key := path.Join("projects", s.Call.Project.String(), m.Kind, s.Created.UTC().Format("2006/01"), m.AssetID.String()+mediaExtension(m.MIMEType))
	if s.Call.Project == uuid.Nil {
		var voice voiceIntentRow
		q := r.DB.WithContext(ctx).Raw("SELECT * FROM ai.voice_intent WHERE request_id=?", s.Call.ID).Scan(&voice)
		if q.Error != nil {
			return nil, false, q.Error
		}
		var personal subject.VoiceOwner
		if q.RowsAffected != 1 || json.Unmarshal(voice.Owner, &personal) != nil || voice.ActorID != s.Call.Actor || voice.ScopeID != s.Call.Scope || personal.ScopeID != s.Call.Scope || personal.PersonalOrgID == uuid.Nil {
			return nil, false, domain.ErrConflict
		}
		owner = media.StorageWorkOwner{Kind: media.StorageWorkKindSubjectVoice, OwnerID: s.Call.Scope, WorkID: s.Call.ID}
		key = path.Join("personal", personal.PersonalOrgID.String(), s.Call.Actor.String(), "audio", s.Created.UTC().Format("2006/01"), m.AssetID.String()+mediaExtension(m.MIMEType))
	}
	intent, err := pgmedia.NewStorageRoutes(r.DB).ReadStorageWork(ctx, owner)
	if err != nil {
		return nil, false, err
	}
	location, found, err := pgmedia.NewStore(r.DB).FindGatewayOriginal(ctx, intent, key)
	if err != nil || !found {
		return nil, false, err
	}
	if location.State == mediadomain.ObjectLocationPresent && (location.SHA256 == nil || *location.SHA256 != m.SHA256 || location.ByteSize == nil || *location.ByteSize != m.ByteSize || location.MIMEType == nil || *location.MIMEType != m.MIMEType) {
		return nil, false, media.ErrObjectMismatch
	}
	stream, err := reader.Get(ctx, platform.Ref{ProfileID: intent.ProfileID, Bucket: intent.Bucket, Key: key})
	if err != nil {
		return nil, false, nil
	}
	file, err := copyRecoveredOriginal(ctx, stream, m)
	if err != nil {
		return nil, false, err
	}
	return file, true, nil
}

// copyRecoveredOriginal owns its read handle and independently consumes/re-hashes
// the complete original. A partial/extra/mismatched read never becomes proof of
// completion and never mutates storage debt or the original frozen manifest.
func copyRecoveredOriginal(ctx context.Context, stream io.ReadCloser, m outputManifest) (*media.Downloaded, error) {
	if stream == nil {
		return nil, media.ErrUnavailable
	}
	file, err := os.CreateTemp("", "glowloom-ai-recover-*")
	if err != nil {
		_ = stream.Close()
		return nil, err
	}
	result := &media.Downloaded{File: file, MIMEType: m.MIMEType}
	keep := false
	defer func() {
		if !keep {
			_ = result.Close()
		}
	}()
	hash := sha256.New()
	size, readErr := io.Copy(io.MultiWriter(file, hash), io.LimitReader(stream, m.ByteSize+1))
	closeErr := stream.Close()
	if err := errors.Join(readErr, closeErr, ctx.Err()); err != nil {
		return nil, err
	}
	digest := hex.EncodeToString(hash.Sum(nil))
	if size != m.ByteSize || digest != m.SHA256 {
		return nil, media.ErrObjectMismatch
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	result.Size = size
	result.SHA256 = digest
	keep = true
	return result, nil
}
