package application

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"unicode/utf8"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/script/domain"
)

// ProjectCopyTransferError preserves unknown I/O outcomes for the owning coordinator.
// A receipt is never produced by catching this error or starting another source plan.
type ProjectCopyTransferError struct {
	Code                string
	NeedsReconciliation bool
	Cause               error
}

func (e *ProjectCopyTransferError) Error() string { return "script copy " + e.Code }
func (e *ProjectCopyTransferError) Unwrap() error { return e.Cause }
func scriptCopyFailure(code string, err error) error {
	return &ProjectCopyTransferError{Code: code, NeedsReconciliation: true, Cause: err}
}

func (p *ProjectCopy) readObject(ctx context.Context, fact domain.ObjectFact) ([]byte, error) {
	reader := SourceService{objects: p.objects}
	return reader.readObject(ctx, fact)
}

// Transfer reads every frozen original and verifies the canonical semantic content.
// Remote I/O occurs outside the owning SQL transactions; every proof rechecks fencing.
func (p *ProjectCopy) Transfer(ctx context.Context, actor identityapp.Principal, b ProjectCopyBinding, snapshot ProjectCopySnapshot) (ProjectCopyReceipt, error) {
	if p == nil || p.store == nil || p.objects == nil {
		return ProjectCopyReceipt{}, ErrUnavailable
	}
	m, err := p.store.Manifest(ctx, actor, b, snapshot)
	if err != nil {
		return ProjectCopyReceipt{}, err
	}
	if err := p.verifySemanticHistory(ctx, m.Source); err != nil {
		return ProjectCopyReceipt{}, scriptCopyFailure("script_copy_content_invalid", err)
	}
	for _, object := range m.Objects {
		data, err := p.readObject(ctx, object.Source)
		if err != nil {
			return ProjectCopyReceipt{}, scriptCopyFailure("script_copy_source_unavailable", err)
		}
		_, err = p.readObject(ctx, object.Target)
		if err != nil && !errors.Is(err, ErrObjectMissing) {
			return ProjectCopyReceipt{}, scriptCopyFailure("script_copy_target_unconfirmed", err)
		}
		missing := errors.Is(err, ErrObjectMissing)
		if err := p.store.BeginObject(ctx, actor, b, snapshot, object.Target); err != nil {
			return ProjectCopyReceipt{}, err
		}
		if missing {
			if err := p.objects.PutIfAbsent(ctx, object.Target.Key, bytes.NewReader(data), object.Target.ByteSize, object.Target.MIME, object.Target.SHA256); err != nil && !errors.Is(err, ErrObjectExists) {
				return ProjectCopyReceipt{}, scriptCopyFailure("script_copy_put_unconfirmed", err)
			}
		}
		if _, err := p.readObject(ctx, object.Target); err != nil {
			return ProjectCopyReceipt{}, scriptCopyFailure("script_copy_target_unconfirmed", err)
		}
		if err := p.store.CompleteObject(ctx, actor, b, snapshot, object.Target); err != nil {
			return ProjectCopyReceipt{}, scriptCopyFailure("script_copy_proof_unconfirmed", err)
		}
	}
	return ProjectCopyReceipt{ManifestSHA256: snapshot.ManifestSHA256, ContentSHA256: snapshot.ContentSHA256, Counts: snapshot.Counts}, nil
}

func (p *ProjectCopy) verifySemanticHistory(ctx context.Context, h ProjectCopyHistory) error {
	sources := make(map[[16]byte]domain.SourceRecord, len(h.Sources))
	for _, source := range h.Sources {
		sources[source.ID] = source
		data, err := p.readObject(ctx, source.Rich)
		if err != nil {
			return err
		}
		doc, err := domain.DecodeRichDocument(data)
		if err != nil {
			return err
		}
		plain, err := doc.PlainText()
		if err != nil {
			return err
		}
		canonical, err := doc.CanonicalJSON()
		if err != nil {
			return err
		}
		if domain.ContentSHA(canonical) != source.Rich.SHA256 || domain.ContentSHA([]byte(plain)) != source.ContentHash || utf8.RuneCountInString(plain) != source.CharCount {
			return ErrObjectMismatch
		}
	}
	for _, v := range h.Versions {
		docs := make([]domain.SourceDocument, 0, len(v.SourceIDs))
		records := make([]domain.SourceRecord, 0, len(v.SourceIDs))
		for _, id := range v.SourceIDs {
			source, ok := sources[id]
			if !ok {
				return ErrObjectMismatch
			}
			data, err := p.readObject(ctx, source.Rich)
			if err != nil {
				return err
			}
			doc, err := domain.DecodeRichDocument(data)
			if err != nil {
				return err
			}
			docs = append(docs, sourceDocument(source, doc))
			records = append(records, source)
		}
		composed, err := domain.ComposeSources(docs)
		if err != nil {
			return err
		}
		textBytes, err := p.readObject(ctx, v.Text)
		if err != nil {
			return err
		}
		richBytes, err := p.readObject(ctx, v.Rich)
		if err != nil {
			return err
		}
		if !utf8.Valid(textBytes) || string(textBytes) != composed.Text || !bytes.Equal(richBytes, composed.RichJSON) || v.Text.SHA256 != v.ContentHash || v.Rich.SHA256 != v.DocumentSHA256 || v.Text.ByteSize != int64(len(composed.Text)) || v.Rich.ByteSize != int64(len(composed.RichJSON)) {
			return ErrObjectMismatch
		}

		manifest, err := json.Marshal(records)
		if err != nil {
			return err
		}
		spans, err := json.Marshal(v.Spans)
		if err != nil {
			return err
		}
		expectedSpans, err := json.Marshal(composed.Spans)
		if err != nil {
			return err
		}
		if v.CharCount != composed.CharCount || v.ContentHash != composed.ContentHash || v.DocumentSHA256 != composed.DocumentSHA256 || v.SourceManifestSHA256 != domain.ContentSHA(manifest) || string(spans) != string(expectedSpans) {
			return ErrObjectMismatch
		}
	}
	return nil
}

// Cleanup removes only frozen unpublished target objects after digest ownership proof.
// A missing unconfirmed put cannot prove that a delayed writer has ceased.
func (p *ProjectCopy) Cleanup(ctx context.Context, actor identityapp.Principal, b ProjectCopyBinding, snapshot ProjectCopySnapshot) error {
	if p == nil || p.store == nil || p.objects == nil {
		return ErrUnavailable
	}
	objects, err := p.store.BeginCleanup(ctx, actor, b, snapshot)
	if err != nil {
		return err
	}
	for _, object := range objects {
		_, readErr := p.readObject(ctx, object.Target)
		if errors.Is(readErr, ErrObjectMissing) {
			if object.PutStarted && !object.Copied && !object.Removed {
				return scriptCopyFailure("script_copy_cleanup_unconfirmed", ErrNeedsReconciliation)
			}
		} else {
			if readErr != nil {
				return scriptCopyFailure("script_copy_cleanup_mismatch", readErr)
			}
			if err := p.store.BeginCleanupObject(ctx, actor, b, snapshot, object.Target); err != nil {
				return err
			}
			removeErr := p.objects.Remove(ctx, object.Target.Key)
			if _, err := p.readObject(ctx, object.Target); !errors.Is(err, ErrObjectMissing) {
				return scriptCopyFailure("script_copy_remove_unconfirmed", errors.Join(err, removeErr))
			}
		}
		if err := p.store.CompleteCleanupObject(ctx, actor, b, snapshot, object.Target); err != nil {
			return fmt.Errorf("persist script copy cleanup: %w", err)
		}
	}
	return nil
}
