package media_test

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
)

var domainNow = time.Date(2026, time.September, 28, 9, 0, 0, 0, time.UTC)

func validAsset() domain.MediaAsset {
	asset := domain.MediaAsset{
		ID:               uuid.New(),
		ProjectID:        uuid.New(),
		Kind:             domain.KindVideo,
		Origin:           domain.OriginUpload,
		Status:           domain.StatusUploading,
		FileName:         "reference.mp4",
		MimeType:         "video/mp4",
		ByteSize:         1024,
		ModerationStatus: domain.ModerationPending,
		Revision:         1,
		CreateTime:       domainNow,
		UpdateTime:       domainNow,
	}
	asset.ObjectKey = fmt.Sprintf("projects/%s/%s/2026/09/%s.mp4", asset.ProjectID, asset.Kind, asset.ID)
	return asset
}

func TestMediaAssetObjectKeyIsScopedToItsIdentity(t *testing.T) {
	asset := validAsset()
	if err := asset.Validate(); err != nil {
		t.Fatalf("valid object key: %v", err)
	}
	otherProjectID, otherAssetID := uuid.New(), uuid.New()
	tests := []struct {
		name string
		key  string
	}{
		{"other project", fmt.Sprintf("projects/%s/video/2026/09/%s.mp4", otherProjectID, asset.ID)},
		{"other kind", fmt.Sprintf("projects/%s/image/2026/09/%s.mp4", asset.ProjectID, asset.ID)},
		{"other asset", fmt.Sprintf("projects/%s/video/2026/09/%s.mp4", asset.ProjectID, otherAssetID)},
		{"traversal", fmt.Sprintf("projects/%s/video/2026/09/../%s.mp4", asset.ProjectID, asset.ID)},
		{"backslash", fmt.Sprintf("projects/%s/video/2026/09\\%s.mp4", asset.ProjectID, asset.ID)},
		{"nested object", fmt.Sprintf("projects/%s/video/2026/09/extra/%s.mp4", asset.ProjectID, asset.ID)},
		{"missing extension", fmt.Sprintf("projects/%s/video/2026/09/%s", asset.ProjectID, asset.ID)},
		{"invalid month", fmt.Sprintf("projects/%s/video/2026/13/%s.mp4", asset.ProjectID, asset.ID)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			candidate := asset
			candidate.ObjectKey = tt.key
			if err := candidate.Validate(); !errors.Is(err, domain.ErrInvalidMediaAsset) {
				t.Fatalf("Validate() = %v; want invalid key", err)
			}
		})
	}
}

func TestMediaAssetValidateProvenanceAndMetadata(t *testing.T) {
	base := validAsset()
	if err := base.Validate(); err != nil {
		t.Fatalf("valid upload: %v", err)
	}

	operationID := uuid.New()
	provider, model, region := "openrouter", "video-model", "overseas"
	generated := base
	generated.Origin = domain.OriginGenerated
	generated.Status = domain.StatusProcessing
	generated.SourceOperationID = &operationID
	generated.ProviderKey = &provider
	generated.ModelKey = &model
	generated.Region = &region
	if err := generated.Validate(); err != nil {
		t.Fatalf("valid generated asset: %v", err)
	}

	tests := []struct {
		name   string
		change func(*domain.MediaAsset)
	}{
		{"missing project", func(a *domain.MediaAsset) { a.ProjectID = uuid.Nil }},
		{"invalid kind", func(a *domain.MediaAsset) { a.Kind = "executable" }},
		{"empty object key", func(a *domain.MediaAsset) { a.ObjectKey = " " }},
		{"negative size", func(a *domain.MediaAsset) { a.ByteSize = -1 }},
		{"missing mime", func(a *domain.MediaAsset) { a.MimeType = "" }},
		{"invalid revision", func(a *domain.MediaAsset) { a.Revision = 0 }},
		{"future update before create", func(a *domain.MediaAsset) { a.UpdateTime = a.CreateTime.Add(-time.Second) }},
		{"orphan operation on upload", func(a *domain.MediaAsset) { a.SourceOperationID = &operationID }},
		{"delete time without deletion", func(a *domain.MediaAsset) { a.DeleteTime = &domainNow }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			asset := base
			tt.change(&asset)
			if err := asset.Validate(); !errors.Is(err, domain.ErrInvalidMediaAsset) {
				t.Fatalf("Validate() = %v; want ErrInvalidMediaAsset", err)
			}
		})
	}

	generated.SourceOperationID = nil
	if err := generated.Validate(); !errors.Is(err, domain.ErrInvalidMediaAsset) {
		t.Fatalf("generated without operation: %v", err)
	}
	generated.SourceOperationID = &operationID
	generated.ProviderKey = nil
	if err := generated.Validate(); !errors.Is(err, domain.ErrInvalidMediaAsset) {
		t.Fatalf("generated without provider: %v", err)
	}
	person := base
	person.ContainsRealPerson = true
	if err := person.Validate(); err != nil {
		t.Fatalf("unconsented person may be uploaded and browsed: %v", err)
	}
}

func TestMediaAssetTransitionsAndReferenceGate(t *testing.T) {
	asset := validAsset()
	if asset.CanReference() {
		t.Fatal("uploading asset is referenceable")
	}
	if _, err := asset.Transition(domain.StatusReady, domain.ModerationPassed, "", domainNow.Add(time.Minute)); !errors.Is(err, domain.ErrMediaStateConflict) {
		t.Fatalf("uploading to ready = %v", err)
	}

	processing, err := asset.Transition(domain.StatusProcessing, domain.ModerationPending, "", domainNow.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if processing.Status != domain.StatusProcessing || processing.Revision != 2 || asset.Status != domain.StatusUploading {
		t.Fatalf("transition should return a changed copy: old=%+v next=%+v", asset, processing)
	}
	if processing.CanReference() {
		t.Fatal("processing asset is referenceable")
	}
	corruptReady := processing
	corruptReady.Status = domain.StatusReady
	if corruptReady.CanReference() {
		t.Fatal("ready asset with pending moderation is referenceable")
	}
	if _, err := processing.Transition(domain.StatusReady, domain.ModerationPending, "", domainNow.Add(2*time.Minute)); !errors.Is(err, domain.ErrInvalidMediaAsset) {
		t.Fatalf("ready before passed = %v", err)
	}

	ready, err := processing.Transition(domain.StatusReady, domain.ModerationPassed, "", domainNow.Add(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if !ready.CanReference() {
		t.Fatal("ready and passed asset cannot be referenced")
	}
	person := ready
	person.ContainsRealPerson = true
	if person.CanReference() {
		t.Fatal("real person without consent is referenceable")
	}
	consentID := uuid.New()
	person.ConsentRecordID = &consentID
	if !person.CanReference() {
		t.Fatal("real person with a linked consent cannot pass the local reference gate")
	}
	if _, err := ready.Transition(domain.StatusProcessing, domain.ModerationPending, "", domainNow.Add(3*time.Minute)); !errors.Is(err, domain.ErrMediaStateConflict) {
		t.Fatalf("terminal state reversal = %v", err)
	}

	rejected, err := processing.Transition(domain.StatusRejected, domain.ModerationRejected, "blocked content", domainNow.Add(2*time.Minute))
	if err != nil || rejected.CanReference() {
		t.Fatalf("rejected = %+v, %v", rejected, err)
	}
	failed, err := processing.Transition(domain.StatusFailed, domain.ModerationPending, "file type mismatch", domainNow.Add(2*time.Minute))
	if err != nil || failed.CanReference() {
		t.Fatalf("failed = %+v, %v", failed, err)
	}
	if _, err := processing.Transition(domain.StatusFailed, domain.ModerationPending, "", domainNow.Add(2*time.Minute)); !errors.Is(err, domain.ErrInvalidMediaAsset) {
		t.Fatalf("failed without reason = %v", err)
	}
}

func TestMediaAssetDeleteSchedulesThirtyDayPurge(t *testing.T) {
	asset := validAsset()
	processing, err := asset.Transition(domain.StatusProcessing, domain.ModerationPending, "", domainNow.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	ready, err := processing.Transition(domain.StatusReady, domain.ModerationPassed, "", domainNow.Add(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	deletedAt := domainNow.Add(3 * time.Minute)
	deleted, err := ready.Delete(deletedAt)
	if err != nil {
		t.Fatal(err)
	}
	if !deleted.IsDelete || deleted.DeleteTime == nil || deleted.PurgeAfter == nil ||
		!deleted.DeleteTime.Equal(deletedAt) || !deleted.PurgeAfter.Equal(deletedAt.Add(30*24*time.Hour)) ||
		deleted.CanReference() || deleted.Revision != ready.Revision+1 {
		t.Fatalf("invalid deletion state: %+v", deleted)
	}
	if _, err := deleted.Delete(deletedAt.Add(time.Minute)); !errors.Is(err, domain.ErrMediaStateConflict) {
		t.Fatalf("delete replay = %v", err)
	}
	if _, err := asset.Delete(domainNow.Add(time.Minute)); !errors.Is(err, domain.ErrMediaStateConflict) {
		t.Fatalf("delete uploading = %v", err)
	}
}

func TestRenditionValidate(t *testing.T) {
	width, height, size := int32(256), int32(256), int64(512)
	base := domain.Rendition{
		ID:           uuid.New(),
		MediaAssetID: uuid.New(),
		Kind:         domain.RenditionThumb256,
		ObjectKey:    "projects/project/video/2026/09/asset/thumb_256.webp",
		Width:        &width,
		Height:       &height,
		ByteSize:     &size,
		CreateTime:   domainNow,
		UpdateTime:   domainNow,
	}
	if err := base.Validate(); err != nil {
		t.Fatalf("valid rendition: %v", err)
	}
	base.Kind = "raw"
	if err := base.Validate(); !errors.Is(err, domain.ErrInvalidRendition) {
		t.Fatalf("invalid kind = %v", err)
	}
	base.Kind = domain.RenditionThumb256
	base.MediaAssetID = uuid.Nil
	if err := base.Validate(); !errors.Is(err, domain.ErrInvalidRendition) {
		t.Fatalf("missing parent = %v", err)
	}
}
