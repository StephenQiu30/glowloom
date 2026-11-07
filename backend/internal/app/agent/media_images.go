package agent

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"image"

	// Register decoders used to validate the original image before model access.
	_ "image/jpeg"
	// Register PNG decoding for the same original-image validation.
	_ "image/png"
	"io"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	app "github.com/StephenQiu30/glowloom-server/backend/internal/agent/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain"
	mediawire "github.com/StephenQiu30/glowloom-server/backend/internal/app/media"
	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	media "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	mediadomain "github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

const imageInspectLimit = 2 * 1024 * 1024

type imageContent struct {
	Type     string `json:"type"`
	Data     string `json:"data"`
	MIMEType string `json:"mimeType"`
}
type inspectedImage struct {
	AssetID  uuid.UUID      `json:"asset_id"`
	Revision int64          `json:"revision"`
	SHA256   string         `json:"sha256"`
	Width    int            `json:"width"`
	Height   int            `json:"height"`
	Content  []imageContent `json:"content"`
}

// WithMediaObjects installs only the routed physical reader; no signed URL or
// storage credential is passed to the Agent process.
func (b *Boundary) WithMediaObjects(objects media.ObjectReader) *Boundary {
	if objects == nil {
		return b
	}
	b.mediaObjects = objects
	b.registerMediaReferences()
	binding := tool("media_image_inspect", "读取项目正式图片原始字节以进行视觉检查，返回Pi图像内容。仅支持2MiB以内PNG/JPEG，最多4百万像素；需真实已归档物理位置，不能凭文件名描述图片。", "read", `{"type":"object","properties":{"asset_id":{"type":"string","format":"uuid"},"revision":{"type":"integer","minimum":1}},"required":["asset_id"],"additionalProperties":false}`, nil)
	b.bindings = append(b.bindings, binding)
	b.Capabilities.Tools = append(b.Capabilities.Tools, app.Tool{Descriptor: binding.descriptor})
	return b
}
func imageFact(ctx context.Context, db *gorm.DB, s domain.Scope, id uuid.UUID) (mediadomain.MediaAsset, error) {
	a, err := pgmedia.NewScopedStore(db, mediawire.NewMediaWorkerScopeAccess).FindAsset(ctx, s.Principal(), s.ProjectID, id)
	if err != nil {
		return a, businessError(err)
	}
	if a.Kind != mediadomain.KindImage || a.Status != mediadomain.StatusReady || a.ModerationStatus != mediadomain.ModerationPassed || a.IsDelete || a.ContainsRealPerson || a.ConsentRecordID != nil {
		return a, domain.ErrNotFound
	}
	if a.ByteSize < 1 || a.ByteSize > imageInspectLimit || a.SHA256 == nil || len(*a.SHA256) != 64 || !a.ObjectLocation.Readable(a.ObjectKey) || (a.MimeType != "image/png" && a.MimeType != "image/jpeg") {
		return a, domain.ErrUnavailable
	}
	return a, nil
}
func imageFactHash(a mediadomain.MediaAsset) string {
	return domain.ContentHash(struct {
		ID             uuid.UUID
		Revision       int64
		Key, MIME, SHA string
		Size           int64
		Location       *mediadomain.ObjectLocation
	}{a.ID, a.Revision, a.ObjectKey, a.MimeType, *a.SHA256, a.ByteSize, a.ObjectLocation})
}
func (b *Boundary) executeMediaImage(ctx context.Context, in domain.CallInput) (domain.CallReceipt, error) {
	if b.mediaObjects == nil {
		return domain.CallReceipt{}, domain.ErrUnavailable
	}
	if in.MaxMicros < 0 || in.Call.ContractVersion != "v1" || in.Call.InputHash != domain.Hash(in.Input) {
		return domain.CallReceipt{}, domain.ErrInvalid
	}
	var requested struct {
		AssetID  uuid.UUID `json:"asset_id"`
		Revision *int64    `json:"revision,omitempty"`
	}
	if err := decode(in.Input, &requested); err != nil || requested.AssetID == uuid.Nil {
		return domain.CallReceipt{}, domain.ErrInvalid
	}
	old, err := b.Store.ReadCall(ctx, in.Grant.RootRunID, in.Grant.SubmissionID, in.Call)
	if err == nil {
		if old.Kind != "tool" || old.Name != "media_image_inspect" || old.Call.InputHash != in.Call.InputHash || old.MaxMicros != in.MaxMicros {
			return old, domain.ErrConflict
		}
		// An old receipt is still a private image response. Reauthorize its exact
		// asset before replay, including catalog visibility and actual current scope.
		var prior inspectedImage
		if json.Unmarshal(old.Result, &prior) != nil {
			return old, domain.ErrConflict
		}
		authority, readErr := b.Store.CurrentAuthority(ctx, in.Grant.RootRunID)
		if readErr != nil {
			return old, readErr
		}
		fact, readErr := imageFact(ctx, b.db, authority.Scope, requested.AssetID)
		if readErr != nil {
			return domain.CallReceipt{}, readErr
		}
		if prior.AssetID != fact.ID || prior.Revision != fact.Revision || prior.SHA256 != *fact.SHA256 || requested.Revision != nil && *requested.Revision != fact.Revision {
			return domain.CallReceipt{}, domain.ErrConflict
		}
		return old, nil
	}
	if !errors.Is(err, domain.ErrNotFound) {
		return domain.CallReceipt{}, err
	}
	grant, err := b.Store.ValidateGrant(ctx, in.Grant)
	if err != nil {
		return domain.CallReceipt{}, err
	}
	fact, err := imageFact(ctx, b.db, grant.Scope, requested.AssetID)
	if err != nil {
		return domain.CallReceipt{}, err
	}
	if requested.Revision != nil && *requested.Revision != fact.Revision {
		return domain.CallReceipt{}, domain.ErrConflict
	}
	readCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	// All remote object I/O and decoding occurs after the fact-reader transaction
	// has closed, before the final authorization + receipt transaction.
	body, err := b.mediaObjects.Get(readCtx, fact.ObjectLocation.Ref)
	if err != nil {
		return domain.CallReceipt{}, businessError(err)
	}
	if body == nil {
		return domain.CallReceipt{}, domain.ErrUnavailable
	}
	data, readErr := io.ReadAll(io.LimitReader(body, imageInspectLimit+1))
	closeErr := body.Close()
	if readErr != nil || closeErr != nil || int64(len(data)) != fact.ByteSize {
		return domain.CallReceipt{}, domain.ErrUnavailable
	}
	sum := sha256.Sum256(data)
	digest := hex.EncodeToString(sum[:])
	if digest != *fact.SHA256 {
		return domain.CallReceipt{}, domain.ErrConflict
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || config.Width < 1 || config.Height < 1 || int64(config.Width)*int64(config.Height) > 4_000_000 || format != "png" && format != "jpeg" || "image/"+format != fact.MimeType {
		return domain.CallReceipt{}, domain.ErrUnavailable
	}
	if _, _, err = image.Decode(bytes.NewReader(data)); err != nil {
		return domain.CallReceipt{}, domain.ErrUnavailable
	}
	output := inspectedImage{AssetID: fact.ID, Revision: fact.Revision, SHA256: digest, Width: config.Width, Height: config.Height, Content: []imageContent{{Type: "image", Data: base64.StdEncoding.EncodeToString(data), MIMEType: fact.MimeType}}}
	raw, err := json.Marshal(output)
	if err != nil {
		return domain.CallReceipt{}, err
	}
	return b.Store.ExecuteAtomicTool(ctx, "media_image_inspect", in, func(tx *gorm.DB, s domain.Scope, _ uuid.UUID) (domain.CallResult, error) {
		current, err := imageFact(ctx, tx, s, requested.AssetID)
		if err != nil {
			return domain.CallResult{}, err
		}
		if imageFactHash(current) != imageFactHash(fact) {
			return domain.CallResult{}, domain.ErrConflict
		}
		return domain.CallResult{Status: "completed", Result: raw}, nil
	})
}
