package application

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
)

var (
	// ErrProviderStagedObjectExists makes immutable writes inspect existing bytes.
	ErrProviderStagedObjectExists = errors.New("provider staged object already exists")
	// ErrProviderStagedObjectMissing prevents orphan artifacts implying completion.
	ErrProviderStagedObjectMissing = errors.New("provider staged object not found")
)

// ProviderImageEvidence identifies the one image item in its frozen-model turn.
// It is bounded audit evidence, never an executable command or a task ID.
type ProviderImageEvidence struct {
	ThreadID string `json:"thread_id"`
	TurnID   string `json:"turn_id"`
	ItemID   string `json:"item_id"`
	Model    string `json:"model"`
}

// ProviderImageManifest is published last, independently recording that the
// artifact is complete while this local protocol has no established cost.
type ProviderImageManifest struct {
	Version              int                      `json:"version"`
	Identity             ProviderDispatchIdentity `json:"identity"`
	Evidence             ProviderImageEvidence    `json:"evidence"`
	Outputs              []ProviderReceiptOutput  `json:"outputs"`
	CostEvidenceComplete bool                     `json:"cost_evidence_complete"`
}

// Validate checks the first-image manifest, whose cost must remain unknown.
func (m ProviderImageManifest) Validate() error {
	receipt := ProviderReceipt{Version: m.Version, Identity: m.Identity, ManifestSHA256: strings.Repeat("0", 64), Outputs: m.Outputs}
	if receipt.Validate() != nil || m.CostEvidenceComplete {
		return ErrInvalidProviderCall
	}
	for _, id := range []string{m.Evidence.ThreadID, m.Evidence.TurnID, m.Evidence.ItemID} {
		if id == "" || len(id) > 256 || strings.TrimSpace(id) != id {
			return ErrInvalidProviderCall
		}
	}
	if m.Evidence.Model == "" || len(m.Evidence.Model) > 200 || strings.TrimSpace(m.Evidence.Model) != m.Evidence.Model {
		return ErrInvalidProviderCall
	}
	return nil
}

// UnmarshalJSON rejects unknown, duplicated, and incomplete manifest fields.
func (m *ProviderImageManifest) UnmarshalJSON(raw []byte) error {
	if len(raw) > 8*1024 {
		return ErrInvalidProviderCall
	}
	type wireManifest ProviderImageManifest
	var wire wireManifest
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&wire) != nil {
		return ErrInvalidProviderCall
	}
	fields, err := receiptObjectFields(raw, "version", "identity", "evidence", "outputs", "cost_evidence_complete")
	if err != nil || len(fields) != 5 {
		return ErrInvalidProviderCall
	}
	if !bytes.Equal(bytes.TrimSpace(fields["cost_evidence_complete"]), []byte("false")) {
		return ErrInvalidProviderCall
	}
	if _, err := receiptObjectFields(fields["identity"], "project_id", "operation_id", "action", "attempt", "request_key", "model_profile_version_id", "price_rule_version_id"); err != nil {
		return err
	}
	if _, err := receiptObjectFields(fields["evidence"], "thread_id", "turn_id", "item_id", "model"); err != nil {
		return err
	}
	var outputs []json.RawMessage
	if json.Unmarshal(fields["outputs"], &outputs) != nil || len(outputs) != 1 {
		return ErrInvalidProviderCall
	}
	if _, err := receiptObjectFields(outputs[0], "sequence", "size_bytes", "mime_type", "sha256"); err != nil {
		return err
	}
	manifest := ProviderImageManifest(wire)
	if err := manifest.Validate(); err != nil {
		return err
	}
	*m = manifest
	return nil
}

// ProviderStagedObjectInfo describes an immutable private object's checked bytes.
type ProviderStagedObjectInfo struct {
	Size                int64
	ContentType, SHA256 string
}

// ProviderStagingObjects never signs or exposes the private staging objects.
type ProviderStagingObjects interface {
	PutIfAbsent(context.Context, string, io.Reader, int64, string, string) error
	Stat(context.Context, string) (ProviderStagedObjectInfo, error)
	Open(context.Context, string) (io.ReadCloser, error)
}

// ProviderReceiptRepository persists completion and reads an exact call's receipt.
type ProviderReceiptRepository interface {
	CompleteProviderCall(context.Context, CompleteProviderCallInput) error
	ProviderReceipt(context.Context, ProviderDispatchIdentity) (ProviderReceipt, bool, error)
}

// ProviderStageService makes private artifact and manifest writes recoverable.
// It does not release a reservation, select a result, or infer image pricing.
type ProviderStageService struct {
	objects ProviderStagingObjects
	repo    ProviderReceiptRepository
}

// NewProviderStageService injects immutable private storage and durable calls.
func NewProviderStageService(objects ProviderStagingObjects, repo ProviderReceiptRepository) *ProviderStageService {
	return &ProviderStageService{objects: objects, repo: repo}
}

// StageImage writes the artifact, verifies it, publishes the full manifest last,
// then records completion. A failed DB write can recover that same manifest.
func (s *ProviderStageService) StageImage(ctx context.Context, identity ProviderDispatchIdentity, evidence ProviderImageEvidence, output ProviderReceiptOutput, data []byte) (ProviderReceipt, error) {
	manifest := ProviderImageManifest{Version: 1, Identity: identity, Evidence: evidence, Outputs: []ProviderReceiptOutput{output}, CostEvidenceComplete: false}
	if s == nil || s.objects == nil || s.repo == nil || manifest.Validate() != nil || int64(len(data)) != output.SizeBytes || imageDigest(data) != output.SHA256 {
		return ProviderReceipt{}, ErrInvalidProviderCall
	}
	if err := s.write(ctx, providerStageImageKey(identity), data, output.MIMEType, output.SHA256); err != nil {
		return ProviderReceipt{}, err
	}
	raw, err := json.Marshal(manifest)
	if err != nil {
		return ProviderReceipt{}, fmt.Errorf("encode provider image manifest: %w", err)
	}
	manifestSHA := imageDigest(raw)
	if err := s.write(ctx, providerStageManifestKey(identity), raw, "application/json", manifestSHA); err != nil {
		return ProviderReceipt{}, err
	}
	receipt := ProviderReceipt{Version: 1, Identity: identity, ManifestSHA256: manifestSHA, Outputs: manifest.Outputs}
	if err := s.record(ctx, receipt); err != nil {
		return ProviderReceipt{}, err
	}
	return receipt, nil
}

// RecoverImage only examines server-derived keys for this same attempt. Missing
// manifests stay uncertain; this method cannot issue a new provider request.
func (s *ProviderStageService) RecoverImage(ctx context.Context, identity ProviderDispatchIdentity) (*ProviderReceipt, error) {
	if s == nil || s.objects == nil || s.repo == nil || identity.Validate() != nil {
		return nil, ErrInvalidProviderCall
	}
	manifest, raw, err := s.manifest(ctx, identity)
	if errors.Is(err, ErrProviderStagedObjectMissing) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	output := manifest.Outputs[0]
	if _, err := s.read(ctx, providerStageImageKey(identity), output.SizeBytes, output.MIMEType, output.SHA256); err != nil {
		return nil, err
	}
	receipt := ProviderReceipt{Version: 1, Identity: identity, ManifestSHA256: imageDigest(raw), Outputs: manifest.Outputs}
	if err := s.record(ctx, receipt); err != nil {
		return nil, err
	}
	return &receipt, nil
}

// VerifyReceipt checks the exact durable call without requiring staging bytes.
// An already ingested result may outlive temporary provider staging objects.
func (s *ProviderStageService) VerifyReceipt(ctx context.Context, receipt ProviderReceipt) error {
	if s == nil || s.repo == nil || receipt.Validate() != nil {
		return ErrInvalidProviderCall
	}
	stored, found, err := s.repo.ProviderReceipt(ctx, receipt.Identity)
	if err != nil {
		return fmt.Errorf("read durable provider receipt: %w", err)
	}
	if !found || stored.Validate() != nil || stored.Identity != receipt.Identity || stored.ManifestSHA256 != receipt.ManifestSHA256 || stored.Outputs[0] != receipt.Outputs[0] {
		return ErrProviderCallConflict
	}
	return nil
}

// ReadImage binds an existing receipt to its durable call and complete private
// manifest, then verifies the artifact bytes. It accepts no caller object key.
func (s *ProviderStageService) ReadImage(ctx context.Context, receipt ProviderReceipt) (ProviderReceiptOutput, []byte, error) {
	if s == nil || s.objects == nil {
		return ProviderReceiptOutput{}, nil, ErrInvalidProviderCall
	}
	if err := s.VerifyReceipt(ctx, receipt); err != nil {
		return ProviderReceiptOutput{}, nil, err
	}
	manifest, raw, err := s.manifest(ctx, receipt.Identity)
	if err != nil {
		return ProviderReceiptOutput{}, nil, err
	}
	if imageDigest(raw) != receipt.ManifestSHA256 || manifest.Outputs[0] != receipt.Outputs[0] {
		return ProviderReceiptOutput{}, nil, ErrProviderCallConflict
	}
	output := manifest.Outputs[0]
	data, err := s.read(ctx, providerStageImageKey(receipt.Identity), output.SizeBytes, output.MIMEType, output.SHA256)
	if err != nil {
		return ProviderReceiptOutput{}, nil, err
	}
	return output, data, nil
}

func (s *ProviderStageService) record(ctx context.Context, receipt ProviderReceipt) error {
	if err := s.repo.CompleteProviderCall(ctx, CompleteProviderCallInput{OperationID: receipt.Identity.OperationID, Action: receipt.Identity.Action, Attempt: receipt.Identity.Attempt, Outcome: "ok", State: "completed", Receipt: &receipt}); err != nil {
		return fmt.Errorf("record provider image receipt: %w", err)
	}
	return nil
}
func (s *ProviderStageService) manifest(ctx context.Context, identity ProviderDispatchIdentity) (ProviderImageManifest, []byte, error) {
	key := providerStageManifestKey(identity)
	info, err := s.objects.Stat(ctx, key)
	if err != nil {
		return ProviderImageManifest{}, nil, err
	}
	if info.Size < 1 || info.Size > 8*1024 || info.ContentType != "application/json" || !validReceiptSHA256(info.SHA256) {
		return ProviderImageManifest{}, nil, ErrProviderCallConflict
	}
	raw, err := s.read(ctx, key, info.Size, info.ContentType, info.SHA256)
	if err != nil {
		return ProviderImageManifest{}, nil, err
	}
	var manifest ProviderImageManifest
	if json.Unmarshal(raw, &manifest) != nil || manifest.Identity != identity {
		return ProviderImageManifest{}, nil, ErrProviderCallConflict
	}
	return manifest, raw, nil
}
func (s *ProviderStageService) write(ctx context.Context, key string, data []byte, mime, digest string) error {
	err := s.objects.PutIfAbsent(ctx, key, bytes.NewReader(data), int64(len(data)), mime, digest)
	if err != nil && !errors.Is(err, ErrProviderStagedObjectExists) {
		return fmt.Errorf("create provider staging object: %w", err)
	}
	if _, err := s.read(ctx, key, int64(len(data)), mime, digest); err != nil {
		return err
	}
	return nil
}
func (s *ProviderStageService) read(ctx context.Context, key string, size int64, mime, digest string) ([]byte, error) {
	if size < 1 || size > maxProviderImageBytes || !validReceiptSHA256(digest) {
		return nil, ErrInvalidProviderCall
	}
	info, err := s.objects.Stat(ctx, key)
	if err != nil {
		return nil, err
	}
	if info.Size != size || info.ContentType != mime || info.SHA256 != digest {
		return nil, ErrProviderCallConflict
	}
	object, err := s.objects.Open(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("open provider staging object: %w", err)
	}
	defer func() { _ = object.Close() }()
	data, err := io.ReadAll(io.LimitReader(object, size+1))
	if err != nil {
		return nil, fmt.Errorf("read provider staging object: %w", err)
	}
	if int64(len(data)) != size || imageDigest(data) != digest {
		return nil, ErrProviderCallConflict
	}
	return data, nil
}
func imageDigest(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}
func providerStagePrefix(i ProviderDispatchIdentity) string {
	return path.Join("private", "provider-results", i.ProjectID.String(), i.OperationID.String(), i.Action, fmt.Sprint(i.Attempt))
}
func providerStageImageKey(i ProviderDispatchIdentity) string {
	return path.Join(providerStagePrefix(i), "1.image")
}
func providerStageManifestKey(i ProviderDispatchIdentity) string {
	return path.Join(providerStagePrefix(i), "manifest.json")
}
