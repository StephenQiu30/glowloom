package operation_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/StephenQiu30/lanverse/backend/internal/operation/application"
)

func TestM1ReceiptRecovery(t *testing.T) {
	for _, failure := range []string{"none", "manifest", "database", "wrong_existing_sha", "tampered_manifest"} {
		t.Run(failure, func(t *testing.T) {
			identity := codexTestInput().Identity
			data := []byte("verified-fixture-image")
			digest := sha256.Sum256(data)
			output := application.ProviderReceiptOutput{Sequence: 1, SizeBytes: int64(len(data)), MIMEType: "image/png", SHA256: hex.EncodeToString(digest[:])}
			objects := &stageTestObjects{entries: map[string]stageTestObject{}, failManifest: failure == "manifest"}
			records := &stageTestRecords{fail: failure == "database"}
			service := application.NewProviderStageService(objects, records)
			if failure == "wrong_existing_sha" {
				objects.wrongSHA = true
			}
			receipt, err := service.StageImage(t.Context(), identity, application.ProviderImageEvidence{ThreadID: "thread", TurnID: "turn", ItemID: "item", Model: "fixture-model"}, output, data)
			if failure == "none" || failure == "tampered_manifest" {
				if err != nil || receipt.Validate() != nil || len(objects.entries) != 2 || records.writes != 1 {
					t.Fatalf("stage receipt=%+v err=%v objects=%d writes=%d", receipt, err, len(objects.entries), records.writes)
				}
			} else if err == nil {
				t.Fatal("injected failure was not rejected")
			}
			if failure == "manifest" {
				if len(objects.entries) != 1 || records.writes != 0 {
					t.Fatal("manifest failure recorded completion")
				}
				got, err := service.RecoverImage(t.Context(), identity)
				if err != nil || got != nil {
					t.Fatal("orphan image must not establish completion")
				}
				return
			}
			if failure == "wrong_existing_sha" {
				if records.writes != 0 {
					t.Fatal("changed object recorded completion")
				}
				return
			}
			if failure == "database" {
				records.fail = false
				objects.puts = 0
				got, err := service.RecoverImage(t.Context(), identity)
				if err != nil || got == nil || got.Validate() != nil || objects.puts != 0 {
					t.Fatalf("recovery rewrote generation objects: %v puts=%d", err, objects.puts)
				}
				if records.last.Usage != nil || records.last.ProviderTaskID != nil {
					t.Fatal("recovery fabricated cost or task ID")
				}
			}
			if failure == "tampered_manifest" {
				for key, obj := range objects.entries {
					if strings.HasSuffix(key, "manifest.json") {
						obj.data = bytes.Replace(obj.data, []byte(`"cost_evidence_complete":false`), []byte(`"cost_evidence_complete":true`), 1)
						objects.entries[key] = obj
					}
				}
				if _, err := service.RecoverImage(t.Context(), identity); err == nil {
					t.Fatal("changed manifest accepted")
				}
			}
		})
	}
}
func TestM1ReceiptRecoveryRejectsChangedIdentityAndBytes(t *testing.T) {
	identity := codexTestInput().Identity
	data := []byte("verified-fixture-image")
	digest := sha256.Sum256(data)
	output := application.ProviderReceiptOutput{Sequence: 1, SizeBytes: int64(len(data)), MIMEType: "image/png", SHA256: hex.EncodeToString(digest[:])}
	objects := &stageTestObjects{entries: map[string]stageTestObject{}}
	records := &stageTestRecords{}
	service := application.NewProviderStageService(objects, records)
	receipt, err := service.StageImage(t.Context(), identity, application.ProviderImageEvidence{ThreadID: "thread", TurnID: "turn", ItemID: "item", Model: "fixture-model"}, output, data)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.ReadImage(t.Context(), receipt); err != nil {
		t.Fatal(err)
	}
	records.receipt.Identity.RequestKey = "different-request"
	if _, _, err := service.ReadImage(t.Context(), receipt); err == nil {
		t.Fatal("stored identity mismatch accepted")
	}
	records.receipt = receipt
	for key, obj := range objects.entries {
		if !strings.HasSuffix(key, "manifest.json") {
			obj.data[0] = 'x'
			objects.entries[key] = obj
		}
	}
	if _, _, err := service.ReadImage(t.Context(), receipt); err == nil {
		t.Fatal("changed artifact accepted on metadata alone")
	}
}
func TestM1ReceiptRecoveryRejectsDuplicateManifestFields(t *testing.T) {
	var manifest application.ProviderImageManifest
	if err := json.Unmarshal([]byte(`{"version":1,"version":1}`), &manifest); err == nil {
		t.Fatal("ambiguous manifest accepted")
	}
}

func TestM1ReceiptRecoveryRejectsNullCostEvidenceFlag(t *testing.T) {
	m := application.ProviderImageManifest{Version: 1, Identity: codexTestInput().Identity, Evidence: application.ProviderImageEvidence{ThreadID: "thread", TurnID: "turn", ItemID: "item", Model: "fixture-model"}, Outputs: codexTestReceipt(codexTestInput().Identity).Outputs}
	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	raw = bytes.Replace(raw, []byte(`"cost_evidence_complete":false`), []byte(`"cost_evidence_complete":null`), 1)
	var decoded application.ProviderImageManifest
	if err := json.Unmarshal(raw, &decoded); err == nil {
		t.Fatal("missing cost flag was accepted as an explicit incomplete-cost manifest")
	}
}

type stageTestObject struct {
	data []byte
	info application.ProviderStagedObjectInfo
}
type stageTestObjects struct {
	entries                map[string]stageTestObject
	puts                   int
	failManifest, wrongSHA bool
}

func (s *stageTestObjects) PutIfAbsent(_ context.Context, key string, r io.Reader, size int64, mime, digest string) error {
	s.puts++
	if s.failManifest && strings.HasSuffix(key, "manifest.json") {
		return errors.New("fixture manifest failure")
	}
	if _, found := s.entries[key]; found {
		return application.ErrProviderStagedObjectExists
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	if s.wrongSHA {
		digest = strings.Repeat("f", 64)
	}
	s.entries[key] = stageTestObject{data: bytes.Clone(data), info: application.ProviderStagedObjectInfo{Size: size, ContentType: mime, SHA256: digest}}
	return nil
}
func (s *stageTestObjects) Stat(_ context.Context, key string) (application.ProviderStagedObjectInfo, error) {
	value, found := s.entries[key]
	if !found {
		return application.ProviderStagedObjectInfo{}, application.ErrProviderStagedObjectMissing
	}
	return value.info, nil
}
func (s *stageTestObjects) Open(_ context.Context, key string) (io.ReadCloser, error) {
	value, found := s.entries[key]
	if !found {
		return nil, application.ErrProviderStagedObjectMissing
	}
	return io.NopCloser(bytes.NewReader(value.data)), nil
}

type stageTestRecords struct {
	fail    bool
	writes  int
	last    application.CompleteProviderCallInput
	receipt application.ProviderReceipt
}

func (r *stageTestRecords) CompleteProviderCall(_ context.Context, input application.CompleteProviderCallInput) error {
	r.writes++
	if r.fail {
		return errors.New("fixture database failure")
	}
	if err := input.Validate(); err != nil {
		return err
	}
	r.last = input
	r.receipt = *input.Receipt
	return nil
}
func (r *stageTestRecords) ProviderReceipt(_ context.Context, _ application.ProviderDispatchIdentity) (application.ProviderReceipt, bool, error) {
	return r.receipt, r.receipt.Version != 0, nil
}
