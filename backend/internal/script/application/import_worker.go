package application

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"path"
	"slices"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/script/domain"
)

// ImportAuthority permits only one claimed publication using a permanent server key.
type ImportAuthority struct {
	ImportWork
	WorkerID       uuid.UUID
	PublicationKey uuid.UUID
}

// ImportWorkerStore distinguishes physical exit from workflow timeout or lost delivery.
type ImportWorkerStore interface {
	VerifyImportDelivery(context.Context, ImportDelivery) (bool, error)
	ClaimImport(context.Context, ImportDelivery, uuid.UUID, time.Time) (ImportRecord, error)
	ReportImportPhase(context.Context, ImportWork, uuid.UUID, string, time.Time) error
	RecordImportFile(context.Context, ImportWork, uuid.UUID, ImportFileResult, time.Time) error
	FinishImport(context.Context, ImportWork, uuid.UUID, string, bool, time.Time) error
	EndImportIO(context.Context, ImportWork, uuid.UUID, bool, time.Time) error
	ProveImportStop(context.Context, ImportDelivery, uuid.UUID, time.Time) error
	ImportControlRecord(context.Context, ImportDelivery) (ImportRecord, identityapp.Principal, error)
	FinishImportControl(context.Context, ImportDelivery, bool, time.Time) error
	FailImportWorkflow(context.Context, ImportWork, time.Time) error
	ImportWorkState(context.Context, ImportWork) (ImportJob, error)
}

// Run resolves only the current committed action and returns durable state for orchestration.
func (w *ImportWorker) Run(ctx context.Context, d ImportDelivery) (ImportJob, error) {
	if w == nil || w.store == nil {
		return ImportJob{}, ErrUnavailable
	}
	var err error
	switch d.Action {
	case "start":
		err = w.Execute(ctx, d)
	case "cancel":
		err = w.Control(ctx, d)
	case "reconcile":
		r, _, readErr := w.store.ImportControlRecord(ctx, d)
		if readErr != nil {
			return ImportJob{}, readErr
		}
		if r.Job.CancellationRequested || r.OwnerID != nil || r.IOState != "idle" && r.IOState != "ended" {
			err = w.Control(ctx, d)
		} else {
			err = w.Execute(ctx, d)
		}
	default:
		return ImportJob{}, domain.ErrInvalidSource
	}
	state, readErr := w.store.ImportWorkState(ctx, d.ImportWork)
	return state, errors.Join(err, readErr)
}

// ImportPublicationFactory captures the exact current job/attempt/worker authority.
type ImportPublicationFactory func(ImportAuthority) *SourceService

// ImportWorker owns synchronous document extraction, publication and cancellation.
// The registry is shared by all activities; it contains no detached execution.
type ImportWorker struct {
	store       ImportWorkerStore
	sources     SourceAssetReader
	extractor   SourceExtractor
	publication ImportPublicationFactory
	recovery    *SourceRecovery
	now         func() time.Time
	io          *sourceIORegistry
}

// NewImportWorker injects current document readers, exact publication and owned cleanup.
func NewImportWorker(store ImportWorkerStore, sources SourceAssetReader, extractor SourceExtractor, publication ImportPublicationFactory, recovery *SourceRecovery, now func() time.Time) *ImportWorker {
	return &ImportWorker{store: store, sources: sources, extractor: extractor, publication: publication, recovery: recovery, now: now, io: &sourceIORegistry{entries: make(map[uuid.UUID]*sourceIO)}}
}

// Execute processes only a committed attempt and publishes successes once in original order.
func (w *ImportWorker) Execute(ctx context.Context, d ImportDelivery) (err error) {
	if w == nil || w.store == nil || w.sources == nil || w.extractor == nil || w.publication == nil || w.now == nil {
		return ErrUnavailable
	}
	if d.Action != "start" && d.Action != "reconcile" {
		return domain.ErrInvalidSource
	}
	allowed, err := w.store.VerifyImportDelivery(ctx, d)
	if err != nil || !allowed {
		return err
	}
	owner := uuid.New()
	r, err := w.store.ClaimImport(ctx, d, owner, w.now().UTC())
	if err != nil {
		return err
	}
	ioCtx, entry := w.io.begin(ctx, owner)
	uncertain := false
	defer func() {
		exit, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		if err != nil {
			uncertain = errors.Is(err, ErrNeedsReconciliation) || errors.Is(err, ErrObjectMismatch)
			if finishErr := w.store.FinishImport(exit, d.ImportWork, owner, importFailureCode(err), uncertain, w.now().UTC()); finishErr != nil {
				err = errors.Join(err, finishErr)
				uncertain = true
			}
		}
		endErr := w.store.EndImportIO(exit, d.ImportWork, owner, uncertain, w.now().UTC())
		w.io.end(owner, entry, endErr == nil)
		err = errors.Join(err, endErr)
	}()
	prepared := []PreparedImportFile{}
	for _, file := range r.Files {
		if filePublished(r.Job.Files, file.Position) {
			continue
		}
		prior, found := findImportResult(r.Results, file.Position)
		if found && prior.Status == "failed" {
			continue
		}
		if err := w.store.ReportImportPhase(ioCtx, d.ImportWork, owner, "extracting", w.now().UTC()); err != nil {
			return err
		}
		out, readErr := w.extractFile(ioCtx, r.Actor, r.Job.ProjectID, file)
		result := ImportFileResult{Position: file.Position, Status: "failed", FailureCode: importFailureCode(readErr), Warnings: []ExtractionWarning{}}
		if readErr == nil {
			result, readErr = importResult(file.Position, out.Extracted)
			if readErr == nil {
				prepared = append(prepared, out)
			} else {
				result = ImportFileResult{Position: file.Position, Status: "failed", FailureCode: importFailureCode(readErr), Warnings: []ExtractionWarning{}}
			}
		}
		if readErr != nil && (ioCtx.Err() != nil || errors.Is(readErr, identityapp.ErrForbidden) || errors.Is(readErr, ErrUnavailable)) {
			return readErr
		}
		if found {
			original, _ := json.Marshal(prior)
			current, _ := json.Marshal(result)
			if !slices.Equal(original, current) {
				return ErrObjectMismatch
			}
		} else if err := w.store.RecordImportFile(ioCtx, d.ImportWork, owner, result, w.now().UTC()); err != nil {
			return err
		}
	}
	if len(prepared) == 0 {
		return w.store.FinishImport(ioCtx, d.ImportWork, owner, "all_files_failed", false, w.now().UTC())
	}
	if err := w.store.ReportImportPhase(ioCtx, d.ImportWork, owner, "normalizing", w.now().UTC()); err != nil {
		return err
	}
	pub := w.publication(ImportAuthority{ImportWork: d.ImportWork, WorkerID: owner, PublicationKey: r.PublicationKey})
	if pub == nil {
		return ErrUnavailable
	}
	if err := w.store.ReportImportPhase(ioCtx, d.ImportWork, owner, "storing", w.now().UTC()); err != nil {
		return err
	}
	order := make([]uuid.UUID, len(r.Files))
	for i, file := range r.Files {
		order[i] = file.SourceID
	}
	_, err = pub.WriteFiles(ioCtx, r.Actor, SourceCommand{ProjectID: r.Job.ProjectID, Key: r.PublicationKey, RequestID: d.RequestID, Action: "import", ExpectedRevision: r.Job.LatestScriptRevision, BaseVersionID: r.BaseVersionID, RightsConfirmed: true}, prepared, r.Job.CreatedAt, order)
	return err
}

func filePublished(files []ImportFile, position int) bool {
	for _, f := range files {
		if f.Position == position {
			return f.SourceID != nil
		}
	}
	return false
}
func findImportResult(results []ImportFileResult, position int) (ImportFileResult, bool) {
	for _, r := range results {
		if r.Position == position {
			return r, true
		}
	}
	return ImportFileResult{}, false
}

// PreparedImportFile is server-only extracted content, exact original bytes and lineage.
type PreparedImportFile struct {
	Frozen    FrozenImportFile
	Extracted ExtractedDocument
	Original  []byte
	Title     string
}

func (w *ImportWorker) extractFile(ctx context.Context, actor identityapp.Principal, project uuid.UUID, file FrozenImportFile) (out PreparedImportFile, err error) {
	download, err := w.sources.Open(ctx, actor, project, file.Source)
	if err != nil {
		return out, err
	}
	if download == nil || download.File == nil {
		return out, ErrUnavailable
	}
	defer func() { err = errors.Join(err, download.Close()) }()
	extracted, err := w.extractor.Extract(ctx, download)
	if err != nil {
		return out, err
	}
	if _, err := download.File.Seek(0, io.SeekStart); err != nil {
		return out, err
	}
	original, err := io.ReadAll(io.LimitReader(download.File, file.Source.ByteSize+1))
	if err != nil {
		return out, err
	}
	if int64(len(original)) != file.Source.ByteSize || domain.ContentSHA(original) != file.Source.SHA256 {
		return out, ErrObjectMismatch
	}
	name := file.Source.FileName
	title := name[:len(name)-len(path.Ext(name))]
	if title == "" || utf8.RuneCountInString(title) > 512 {
		return out, domain.ErrInvalidSource
	}
	return PreparedImportFile{Frozen: file, Extracted: extracted, Original: original, Title: title}, ctx.Err()
}

func importResult(position int, e ExtractedDocument) (ImportFileResult, error) {
	if err := domain.ValidateSourceWarnings(e.Warnings); err != nil {
		return ImportFileResult{}, err
	}
	rich, err := e.Document.CanonicalJSON()
	if err != nil {
		return ImportFileResult{}, err
	}
	plain, err := e.Document.PlainText()
	if err != nil {
		return ImportFileResult{}, err
	}
	warnings := e.Warnings
	if warnings == nil {
		warnings = []ExtractionWarning{}
	}
	return ImportFileResult{Position: position, Status: "succeeded", RichSHA256: domain.ContentSHA(rich), ContentHash: domain.ContentSHA([]byte(plain)), CharCount: utf8.RuneCountInString(plain), Encoding: e.Encoding, Mapping: e.Mapping, Warnings: warnings}, nil
}

func importFailureCode(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, identityapp.ErrForbidden):
		return "import_actor_forbidden"
	case errors.Is(err, ErrConflict):
		return "script_revision_conflict"
	case errors.Is(err, ErrObjectMismatch):
		return "import_object_mismatch"
	case errors.Is(err, ErrNeedsReconciliation):
		return "import_object_unknown"
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return "import_interrupted"
	case errors.Is(err, domain.ErrInvalidSource), errors.Is(err, domain.ErrInvalidDocument):
		return "import_content_invalid"
	default:
		return "source_extraction_failed"
	}
}

// Control waits for this worker's actual exit before cleanup; unknown ownership stays fenced.
func (w *ImportWorker) Control(ctx context.Context, d ImportDelivery) error {
	if w == nil || w.store == nil || w.recovery == nil || w.now == nil {
		return ErrUnavailable
	}
	if d.Action != "cancel" && d.Action != "reconcile" {
		return domain.ErrInvalidSource
	}
	allowed, err := w.store.VerifyImportDelivery(ctx, d)
	if err != nil || !allowed {
		return err
	}
	r, controller, err := w.store.ImportControlRecord(ctx, d)
	if err != nil {
		return err
	}
	if r.OwnerID != nil {
		owner := *r.OwnerID
		if err := w.io.stop(ctx, owner); err != nil {
			return w.store.FinishImportControl(ctx, d, true, w.now().UTC())
		}
		if err := w.store.ProveImportStop(ctx, d, owner, w.now().UTC()); err != nil {
			return err
		}
		w.io.forget(owner)
		r, controller, err = w.store.ImportControlRecord(ctx, d)
		if err != nil {
			return err
		}
	}
	if r.IOState != "idle" && r.IOState != "ended" {
		return w.store.FinishImportControl(ctx, d, true, w.now().UTC())
	}
	if r.Job.CancellationRequested {
		intentID := uuid.NewSHA1(r.PublicationKey, []byte("source-write/"+r.Actor.ID.String()+"/"+r.Job.ProjectID.String()))
		intent, findErr := w.recovery.Get(ctx, controller, r.Job.ProjectID, intentID)
		if findErr != nil && !errors.Is(findErr, ErrNotFound) {
			return findErr
		}
		if findErr == nil && intent.Status == "pending" {
			_, err := w.recovery.Control(ctx, controller, SourceControlCommand{ProjectID: r.Job.ProjectID, IntentID: intent.ID, Key: uuid.NewSHA1(d.EventID, []byte("source-cleanup")), RequestID: d.RequestID, Action: "cancel", ExpectedRevision: intent.Revision})
			if err != nil {
				return err
			}
			intent, err = w.recovery.Get(ctx, controller, r.Job.ProjectID, intentID)
			if err != nil {
				return err
			}
			if intent.Status != "cancelled" {
				return w.store.FinishImportControl(ctx, d, true, w.now().UTC())
			}
		}
	}
	return w.store.FinishImportControl(ctx, d, false, w.now().UTC())
}
