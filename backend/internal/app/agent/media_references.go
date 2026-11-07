package agent

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	app "github.com/StephenQiu30/glowloom-server/backend/internal/agent/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain"
	mediawire "github.com/StephenQiu30/glowloom-server/backend/internal/app/media"
	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	mediaflow "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/workflow"
	media "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	mediadomain "github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
	scriptextract "github.com/StephenQiu30/glowloom-server/backend/internal/script/adapter/extract"
)

type referenceContent struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	Data     string `json:"data,omitempty"`
	MIMEType string `json:"mimeType,omitempty"`
}
type inspectedReference struct {
	AssetID   uuid.UUID          `json:"asset_id"`
	Revision  int64              `json:"revision"`
	SHA256    string             `json:"sha256"`
	MediaKind string             `json:"media_kind"`
	Content   []referenceContent `json:"content"`
}

func (b *Boundary) registerMediaReferences() {
	for _, binding := range b.bindings {
		if binding.descriptor.Name == "media_reference_inspect" {
			return
		}
	}
	binding := tool("media_reference_inspect", "真实读取项目图片、视频或TXT/DOCX正文。视频最多500MiB，返回10%/50%/90%三个取样目标时间的实际画面，不含音轨、不是全片理解。必须提供被冻结的素材版本。", "read", `{"type":"object","properties":{"asset_id":{"type":"string","format":"uuid"},"revision":{"type":"integer","minimum":1},"media_kind":{"type":"string","enum":["image","video","document"]}},"required":["asset_id","revision","media_kind"],"additionalProperties":false}`, nil)
	b.bindings = append(b.bindings, binding)
	b.Capabilities.Tools = append(b.Capabilities.Tools, app.Tool{Descriptor: binding.descriptor})
}

func referenceLimit(kind mediadomain.Kind) int64 {
	switch kind {
	case mediadomain.KindImage, mediadomain.KindDocument:
		return 20 << 20
	case mediadomain.KindVideo:
		return 500 << 20
	default:
		return 0
	}
}
func referenceFact(ctx context.Context, db *gorm.DB, scope domain.Scope, id uuid.UUID) (mediadomain.MediaAsset, error) {
	a, err := pgmedia.NewScopedStore(db, mediawire.NewMediaWorkerScopeAccess).FindAsset(ctx, scope.Principal(), scope.ProjectID, id)
	if err != nil {
		return a, businessError(err)
	}
	if a.Status != mediadomain.StatusReady || a.ModerationStatus != mediadomain.ModerationPassed || a.IsDelete || a.ContainsRealPerson || a.ConsentRecordID != nil {
		return a, domain.ErrNotFound
	}
	limit := referenceLimit(a.Kind)
	if limit == 0 || a.ByteSize < 1 || a.ByteSize > limit || a.SHA256 == nil || len(*a.SHA256) != 64 || !a.ObjectLocation.Readable(a.ObjectKey) {
		return a, domain.ErrUnavailable
	}
	return a, nil
}

func (b *Boundary) executeMediaReference(ctx context.Context, in domain.CallInput) (domain.CallReceipt, error) {
	if b.mediaObjects == nil {
		return domain.CallReceipt{}, domain.ErrUnavailable
	}
	if in.MaxMicros < 0 || in.Call.ContractVersion != "v1" || in.Call.InputHash != domain.Hash(in.Input) {
		return domain.CallReceipt{}, domain.ErrInvalid
	}
	var requested struct {
		AssetID   uuid.UUID `json:"asset_id"`
		Revision  int64     `json:"revision"`
		MediaKind string    `json:"media_kind"`
	}
	if decode(in.Input, &requested) != nil || requested.AssetID == uuid.Nil || requested.Revision < 1 || referenceLimit(mediadomain.Kind(requested.MediaKind)) == 0 {
		return domain.CallReceipt{}, domain.ErrInvalid
	}
	// A cached private-content receipt still requires a live capability for the
	// current run, segment, owner epoch and authorization revision.
	grant, err := b.Store.ValidateGrant(ctx, in.Grant)
	if err != nil {
		return domain.CallReceipt{}, err
	}
	old, err := b.Store.ReadCall(ctx, in.Grant.RootRunID, in.Grant.SubmissionID, in.Call)
	if err == nil {
		if old.Kind != "tool" || old.Name != "media_reference_inspect" || old.Call.InputHash != in.Call.InputHash || old.MaxMicros != in.MaxMicros {
			return old, domain.ErrConflict
		}
		var prior inspectedReference
		if json.Unmarshal(old.Result, &prior) != nil {
			return old, domain.ErrConflict
		}
		authority, err := b.Store.CurrentAuthority(ctx, in.Grant.RootRunID)
		if err != nil {
			return old, err
		}
		fact, err := referenceFact(ctx, b.db, authority.Scope, requested.AssetID)
		if err != nil {
			return domain.CallReceipt{}, err
		}
		if prior.AssetID != fact.ID || prior.Revision != fact.Revision || prior.SHA256 != *fact.SHA256 || requested.Revision != fact.Revision || prior.MediaKind != string(fact.Kind) || requested.MediaKind != string(fact.Kind) {
			return domain.CallReceipt{}, domain.ErrConflict
		}
		return old, nil
	}
	if !errors.Is(err, domain.ErrNotFound) {
		return domain.CallReceipt{}, err
	}
	fact, err := referenceFact(ctx, b.db, grant.Scope, requested.AssetID)
	if err != nil {
		return domain.CallReceipt{}, err
	}
	if fact.Revision != requested.Revision || string(fact.Kind) != requested.MediaKind {
		return domain.CallReceipt{}, domain.ErrConflict
	}
	readCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	body, err := b.mediaObjects.Get(readCtx, fact.ObjectLocation.Ref)
	if err != nil {
		return domain.CallReceipt{}, businessError(err)
	}
	file, err := readReferenceOriginal(readCtx, body, fact)
	if err != nil {
		return domain.CallReceipt{}, err
	}
	defer func() { _ = file.Close() }()
	result, err := inspectReferenceFile(readCtx, fact, file)
	if err != nil {
		return domain.CallReceipt{}, err
	}
	raw, err := json.Marshal(result)
	if err != nil {
		return domain.CallReceipt{}, err
	}
	return b.Store.ExecuteAtomicTool(ctx, "media_reference_inspect", in, func(tx *gorm.DB, scope domain.Scope, _ uuid.UUID) (domain.CallResult, error) {
		current, err := referenceFact(ctx, tx, scope, requested.AssetID)
		if err != nil {
			return domain.CallResult{}, err
		}
		if current.Kind != fact.Kind || imageFactHash(current) != imageFactHash(fact) {
			return domain.CallResult{}, domain.ErrConflict
		}
		return domain.CallResult{Status: "completed", Result: raw}, nil
	})
}

type referenceReader struct {
	context context.Context
	reader  io.Reader
}

func (r referenceReader) Read(p []byte) (int, error) {
	if err := r.context.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}

// readReferenceOriginal hashes the entire bounded original while streaming to
// a private temporary file. It closes remote input on every exit path.
func readReferenceOriginal(ctx context.Context, body io.ReadCloser, fact mediadomain.MediaAsset) (*media.Downloaded, error) {
	if body == nil {
		return nil, domain.ErrUnavailable
	}
	var closeOnce sync.Once
	var closeErr error
	closeBody := func() { closeOnce.Do(func() { closeErr = body.Close() }) }
	// Cancel a blocked source Read as well as checking between chunks. Remote
	// objects remain owned by this call and Close runs at most once.
	stopClose := context.AfterFunc(ctx, closeBody)
	defer stopClose()
	defer closeBody()
	limit := referenceLimit(fact.Kind)
	if limit == 0 || fact.ByteSize < 1 || fact.ByteSize > limit || fact.SHA256 == nil {
		return nil, domain.ErrUnavailable
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	file, err := os.CreateTemp("", "glowloom-agent-reference-*")
	if err != nil {
		return nil, err
	}
	out := &media.Downloaded{File: file, Size: fact.ByteSize, MIMEType: fact.MimeType, SHA256: *fact.SHA256}
	keep := false
	defer func() {
		if !keep {
			_ = out.Close()
		}
	}()
	hash := sha256.New()
	n, err := io.Copy(io.MultiWriter(file, hash), io.LimitReader(referenceReader{ctx, body}, fact.ByteSize+1))
	if err != nil {
		return nil, err
	}
	closeBody()
	if closeErr != nil {
		return nil, closeErr
	}
	if n != fact.ByteSize || hex.EncodeToString(hash.Sum(nil)) != *fact.SHA256 {
		return nil, domain.ErrConflict
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	keep = true
	return out, nil
}

func inspectReferenceFile(ctx context.Context, fact mediadomain.MediaAsset, file *media.Downloaded) (inspectedReference, error) {
	result := inspectedReference{AssetID: fact.ID, Revision: fact.Revision, SHA256: *fact.SHA256, MediaKind: string(fact.Kind), Content: []referenceContent{}}
	if fact.Kind == mediadomain.KindDocument {
		extracted, err := scriptextract.NewExtractor().Extract(ctx, file)
		if err != nil {
			return result, err
		}
		text, err := extracted.Document.PlainText()
		if err != nil {
			return result, err
		}
		if text == "" {
			return result, domain.ErrInvalid
		}
		// Complete extracted text crosses the owner boundary. The Agent checks it
		// against its frozen model context and refuses instead of truncating.
		result.Content = append(result.Content, referenceContent{Type: "text", Text: text})
		return result, nil
	}
	frames, err := mediaflow.InspectReferenceVisuals(ctx, file)
	if err != nil {
		return result, err
	}
	if fact.Kind == mediadomain.KindImage && len(frames) != 1 || fact.Kind == mediadomain.KindVideo && len(frames) != 3 {
		return result, domain.ErrConflict
	}
	for _, frame := range frames {
		if fact.Kind == mediadomain.KindVideo {
			result.Content = append(result.Content, referenceContent{Type: "text", Text: fmt.Sprintf("Reference video %s revision %d: requested sampling time %d ms. These are sampled visual frames only; the full clip and audio have not been inspected.", fact.ID, fact.Revision, frame.TimestampMS)})
		}
		result.Content = append(result.Content, referenceContent{Type: "image", Data: base64.StdEncoding.EncodeToString(frame.Data), MIMEType: "image/jpeg"})
	}
	return result, ctx.Err()
}
