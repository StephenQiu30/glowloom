package microservices_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"testing"

	"github.com/google/uuid"

	catalogdomain "github.com/StephenQiu30/glowloom-server/backend/execution/catalog/domain"
	"github.com/StephenQiu30/glowloom-server/backend/execution/operation/adapter/managed"
	executionpg "github.com/StephenQiu30/glowloom-server/backend/execution/operation/adapter/postgres"
	executionapp "github.com/StephenQiu30/glowloom-server/backend/execution/operation/application"
	operationdomain "github.com/StephenQiu30/glowloom-server/backend/execution/operation/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/fetch"
)

type ownedMusicKeys struct{}

func (ownedMusicKeys) Open(string, uuid.UUID, uuid.UUID, []byte) ([]byte, error) {
	return []byte(`{"access_key_id":"synthetic-test-ak","secret_access_key":"synthetic-test-sk"}`), nil
}

type ownedMusicDownload struct {
	before func() error
	file   *fetch.Downloaded
	calls  int
}

func (d *ownedMusicDownload) Music(context.Context, catalogdomain.ProviderConfig, string, string) (*fetch.Downloaded, error) {
	if err := d.before(); err != nil {
		return nil, err
	}
	d.calls++
	body := providerMusicWAV()
	file, err := os.CreateTemp("", "glowloom-owned-music-*.wav")
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(body)
	d.file = &fetch.Downloaded{File: file, Size: int64(len(body)), SHA256: hex.EncodeToString(sum[:]), MIMEType: "audio/wave"}
	if _, err := file.Write(body); err != nil {
		_ = d.file.Close()
		return nil, err
	}
	return d.file, nil
}
func (*ownedMusicDownload) Video(context.Context, catalogdomain.ProviderConfig, string, string, string) (*fetch.Downloaded, error) {
	return nil, errors.New("unexpected video download")
}

func TestSupplierOwnedMusicActivitySealsCostBeforeRawDownloadAndClosesFailure(t *testing.T) {
	f, o := supplierObservationFixture(t)
	for _, query := range []string{`DELETE FROM operation.provider_result WHERE operation_id=?`, `UPDATE operation.operation SET capability='audio.generate' WHERE id=?`} {
		if err := f.admin.Exec(query, o.Identity.OperationID).Error; err != nil {
			t.Fatal(err)
		}
	}
	spec, ok := catalogdomain.GetAdapterSpec("volcengine_music")
	if !ok {
		t.Fatal("music protocol unavailable")
	}
	config := catalogdomain.ProviderConfig{AdapterKey: "volcengine_music", BaseURL: spec.AllowedBaseURL, Region: spec.AllowedRegion, Capabilities: []string{"audio"}, TimeoutSeconds: 5, ConcurrencyLimit: 1, RateLimitPerMin: 1}
	if err := config.Validate(); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(config)
	store := &observedActivityStore{Store: executionpg.NewStore(f.execution), config: raw, loaded: executionapp.WorkflowOperation{ProviderRequestKey: o.Identity.RequestKey, Provider: executionapp.WorkflowProvider{AdapterKey: config.AdapterKey}, Operation: operationdomain.Operation{ID: o.Identity.OperationID, ProjectID: o.Identity.ProjectID, ModelProfileVersionID: &o.Identity.ModelProfileVersionID, PriceRuleVersionID: &o.Identity.PriceRuleVersionID, Status: operationdomain.StatusSubmitting, Capability: "audio.generate", Mode: "music", OutputCount: 1, Params: json.RawMessage(`{}`)}}}
	check := func() error {
		var cost *int64
		var original []byte
		if err := f.execution.Raw(`SELECT cost_micros,response_summary->'supplier_observation' FROM operation.provider_call WHERE operation_id=? AND action='submit'`, o.Identity.OperationID).Row().Scan(&cost, &original); err != nil {
			return err
		}
		if cost == nil || *cost != 1234 || len(original) == 0 {
			return errors.New("raw download began before durable liability")
		}
		return nil
	}
	artifacts := &failObservedArtifact{check: check}
	results, err := executionapp.NewOwnedManagedResults(store, executionapp.NewArtifactDelivery(artifacts, unusedArtifactMedia{}))
	if err != nil {
		t.Fatal(err)
	}
	provider := &observedActivityProvider{result: &executionapp.GenerationResult{Outcome: executionapp.ProviderOutcomeCompleted, State: executionapp.ProviderTaskSucceeded, URLs: []string{"https://synthetic.invalid/original.wav"}, Usage: json.RawMessage(`{"duration_seconds":1,"original":9007199254740993}`)}}
	files := &ownedMusicDownload{before: check}
	activity, err := managed.NewOwnedActivities(store, ownedMusicKeys{}, provider, results, files)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := activity.Step(t.Context(), executionapp.ManagedStep{OperationID: o.Identity.OperationID, Action: "submit"}); err == nil || err.Error() != "synthetic Media stage failure" {
		t.Fatalf("unexpected raw Stage failure: %v", err)
	}
	if files.calls != 1 || files.file == nil || files.file.File != nil {
		t.Fatal("raw file was not closed after Stage failure")
	}
	if _, err := activity.Step(t.Context(), executionapp.ManagedStep{OperationID: o.Identity.OperationID, Action: "submit"}); err != nil {
		t.Fatal(err)
	}
	if files.calls != 1 || provider.calls != 1 {
		t.Fatalf("unknown original result was regenerated: downloads=%d supplier=%d", files.calls, provider.calls)
	}
}
