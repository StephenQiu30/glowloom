package workspace_test

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/workspace/domain"
)

func bibleCopySnapshot(digest string) *domain.ProjectCopyBibleSnapshot {
	return &domain.ProjectCopyBibleSnapshot{ID: uuid.New(), ManifestSHA256: digest, ContentSHA256: digest, Counts: domain.ProjectCopyBibleCounts{Characters: 1, CharacterVersions: 2, CharacterConfirmations: 1, Looks: 2, LookVersions: 3, References: 4, Voices: 1}, Identities: []domain.ProjectCopyBibleMapping{{Kind: "character", SourceID: uuid.New(), TargetID: uuid.New()}}, Versions: []domain.ProjectCopyBibleMapping{{Kind: "character", SourceID: uuid.New(), TargetID: uuid.New()}, {Kind: "character", SourceID: uuid.New(), TargetID: uuid.New()}}}
}

func TestProjectCopyBibleRequiresCompleteReceiptBeforeScriptAndCanvases(t *testing.T) {
	job := projectCopyJob()
	digest := job.Manifest.MediaSHA256
	job.Manifest.Bible = bibleCopySnapshot(digest)
	job.Manifest.Script = &domain.ProjectCopyScriptSnapshot{ID: uuid.New(), ManifestSHA256: digest, ContentSHA256: digest}
	worker := uuid.New()
	if err := job.Start(worker, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := job.AcceptMediaReceipt(worker, domain.ProjectCopyReceipt{ManifestSHA256: digest, ContentSHA256: digest, PrimaryCount: 3, SecondaryCount: 4}); err != nil || job.Stage != "bible" {
		t.Fatal("Bible was skipped", job.Stage, err)
	}
	before := job
	if err := job.AcceptScriptReceipt(worker, domain.ProjectCopyScriptReceipt{ManifestSHA256: digest, ContentSHA256: digest}); !errors.Is(err, domain.ErrProjectCopyStateConflict) || !reflect.DeepEqual(before, job) {
		t.Fatal("Script bypassed Bible", err)
	}
	receipt := domain.ProjectCopyBibleReceipt{ManifestSHA256: digest, ContentSHA256: digest, Counts: job.Manifest.Bible.Counts}
	for _, corruption := range []string{"worker", "history", "references", "hash"} {
		t.Run(corruption, func(t *testing.T) {
			changed, fence := receipt, worker
			switch corruption {
			case "worker":
				fence = uuid.New()
			case "history":
				changed.Counts.CharacterVersions--
			case "references":
				changed.Counts.References--
			case "hash":
				changed.ContentSHA256 = ""
			}
			if err := job.AcceptBibleReceipt(fence, changed); err == nil || !reflect.DeepEqual(before, job) {
				t.Fatal("corrupt proof advanced", err)
			}
		})
	}
	if err := job.AcceptBibleReceipt(worker, receipt); err != nil || job.Stage != "script" {
		t.Fatal("Bible did not precede Script", err)
	}
	if err := job.AcceptScriptReceipt(worker, domain.ProjectCopyScriptReceipt{ManifestSHA256: digest, ContentSHA256: digest}); err != nil {
		t.Fatal(err)
	}
	if err := job.AcceptCanvasReceipt(worker, domain.ProjectCopyReceipt{ManifestSHA256: digest, ContentSHA256: digest, PrimaryCount: 2}); err != nil {
		t.Fatal(err)
	}
	if err := job.Publish(worker); err != nil || job.Validate() != nil {
		t.Fatal("complete copy rejected", err)
	}
}

func TestProjectCopyBibleNilBytesAndCorruptMappingRejected(t *testing.T) {
	legacy := projectCopyJob()
	body, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]json.RawMessage
	if err := json.Unmarshal(body, &value); err != nil {
		t.Fatal(err)
	}
	if _, found := value["BibleReceipt"]; found {
		t.Fatal("nil receipt altered old response")
	}
	var manifest map[string]json.RawMessage
	if err := json.Unmarshal(value["Manifest"], &manifest); err != nil {
		t.Fatal(err)
	}
	if _, found := manifest["bible"]; found {
		t.Fatal("nil snapshot altered old manifest")
	}
	for _, corruption := range []string{"missing_snapshot", "orphan_receipt", "negative_count", "unknown_kind", "same_target", "missing_version", "duplicate_target"} {
		t.Run(corruption, func(t *testing.T) {
			job := legacy
			job.Manifest.Bible = bibleCopySnapshot(job.Manifest.MediaSHA256)
			switch corruption {
			case "missing_snapshot":
				job.Manifest.Bible = nil
				job.Stage = "bible"
			case "orphan_receipt":
				job.BibleReceipt = &domain.ProjectCopyBibleReceipt{}
			case "negative_count":
				job.Manifest.Bible.Counts.Voices = -1
			case "unknown_kind":
				job.Manifest.Bible.Identities[0].Kind = "unowned"
			case "same_target":
				job.Manifest.Bible.Identities[0].TargetID = job.Manifest.Bible.Identities[0].SourceID
			case "missing_version":
				job.Manifest.Bible.Versions = job.Manifest.Bible.Versions[:1]
			case "duplicate_target":
				job.Manifest.Bible.Versions[1].TargetID = job.Manifest.Bible.Versions[0].TargetID
			}
			if !errors.Is(job.Validate(), domain.ErrInvalidProjectCopy) {
				t.Fatal("corrupt Bible accepted", corruption)
			}
		})
	}
}
