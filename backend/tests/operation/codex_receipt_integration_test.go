package operation_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"gorm.io/gorm"

	"github.com/StephenQiu30/lanverse/backend/internal/operation/adapter/codex"
	pgoperation "github.com/StephenQiu30/lanverse/backend/internal/operation/adapter/postgres"
	"github.com/StephenQiu30/lanverse/backend/internal/operation/adapter/staging"
	"github.com/StephenQiu30/lanverse/backend/internal/operation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/objectstorage"
)

func freezeReceiptFixtureModel(t *testing.T, database *gorm.DB, identity application.ProviderDispatchIdentity) {
	t.Helper()
	if err := database.Exec(`UPDATE catalog.model_profile_version SET provider_model_id='fixture-model' WHERE id=?::uuid`, identity.ModelProfileVersionID.String()).Error; err != nil {
		t.Fatal("freeze synthetic protocol model")
	}
}

// This crosses actual PostgreSQL and MinIO with simulated model output; no
// account, inference, image pricing, or production bucket participates.
func TestM1ReceiptRecoveryPrivateObjectsAndPostgres(t *testing.T) {
	objects := receiptTestObjects(t)
	database := operationStoreDB(t)
	for _, failure := range []string{"none", "manifest", "database"} {
		t.Run(failure, func(t *testing.T) {
			identity, store := seedDispatchCall(t, database, true)
			freezeReceiptFixtureModel(t, database, identity)
			objectPort := &receiptFaultObjects{ProviderStagingObjects: staging.NewObjects(objects), manifestFailure: failure == "manifest"}
			recordPort := &receiptFaultRecords{Store: store, writeFailure: failure == "database"}
			receiptService := application.NewProviderStageService(objectPort, recordPort)
			launch := &codexTestLauncher{t: t, behavior: "success"}
			adapter, err := codex.New(store, launch, receiptService, codex.Config{WorkRoot: t.TempDir()})
			if err != nil {
				t.Fatal(err)
			}
			input := application.ProviderImageInput{Identity: identity, Model: "fixture-model", Prompt: "生成测试图片"}
			result, err := adapter.Submit(t.Context(), input)
			if err != nil {
				t.Fatal(err)
			}
			want := "completed"
			if failure != "none" {
				want = "unknown"
			}
			if result.Outcome != want || launch.starts != 1 {
				t.Fatalf("first outcome=%s starts=%d want=%s", result.Outcome, launch.starts, want)
			}
			if result.Outcome == "unknown" {
				if err := store.CompleteProviderCall(t.Context(), application.CompleteProviderCallInput{OperationID: identity.OperationID, Action: "submit", Attempt: 1, Outcome: "unknown", State: "unknown"}); err != nil {
					t.Fatal(err)
				}
			}
			recordPort.writeFailure = false
			recovered, err := adapter.Submit(t.Context(), input)
			if err != nil {
				t.Fatal(err)
			}
			want = "completed"
			if failure == "manifest" {
				want = "unknown"
			}
			if recovered.Outcome != want || launch.starts != 1 || launch.opens != 1 {
				t.Fatalf("recovery=%s starts=%d opens=%d want=%s", recovered.Outcome, launch.starts, launch.opens, want)
			}
			if recovered.Outcome == "completed" {
				if recovered.Receipt == nil {
					t.Fatal("completed receipt missing")
				}
				output, data, err := receiptService.ReadImage(t.Context(), *recovered.Receipt)
				if err != nil || output.MIMEType != "image/png" || int64(len(data)) != output.SizeBytes {
					t.Fatalf("private result read: %v", err)
				}
				var saved struct {
					State    string
					Cost     *int64
					HasUsage bool
					Count    int64
				}
				if err := database.Raw(`SELECT response_summary->>'state' AS state,cost_micros AS cost,usage IS NOT NULL AS has_usage,COUNT(*) OVER() AS count FROM operation.provider_call WHERE operation_id=?::uuid AND action='submit'`, identity.OperationID.String()).Scan(&saved).Error; err != nil {
					t.Fatal(err)
				}
				if saved.State != "completed" || saved.Cost != nil || saved.HasUsage || saved.Count != 1 {
					t.Fatalf("completion invented cost or duplicated call: %+v", saved)
				}
			} else {
				var hasReceipt bool
				if err := database.Raw(`SELECT receipt IS NOT NULL FROM operation.provider_call WHERE operation_id=?::uuid AND action='submit'`, identity.OperationID.String()).Scan(&hasReceipt).Error; err != nil {
					t.Fatal(err)
				}
				if hasReceipt {
					t.Fatal("orphan artifact inferred completion")
				}
			}
			var budget struct{ Reserved, Spent int64 }
			if err := database.Raw(`SELECT reserved_micros AS reserved,settled_micros AS spent FROM billing.budget WHERE project_id=?::uuid`, identity.ProjectID.String()).Scan(&budget).Error; err != nil {
				t.Fatal(err)
			}
			if budget.Reserved != 10 || budget.Spent != 0 {
				t.Fatal("staging released or settled unknown cost")
			}
		})
	}
}

func receiptTestObjects(t *testing.T) *objectstorage.Client {
	t.Helper()
	file := os.Getenv("LV_TEST_RECEIPT_STORAGE_CONFIG")
	if file == "" {
		t.Skip("set isolated LV_TEST_RECEIPT_STORAGE_CONFIG with newly generated test credentials")
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal("read temporary storage fixture config")
	}
	var config struct {
		Endpoint  string `json:"endpoint"`
		Bucket    string `json:"bucket"`
		AccessKey string `json:"access_key"`
		SecretKey string `json:"secret_key"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&config) != nil {
		t.Fatal("invalid temporary storage fixture config")
	}
	endpoint, err := url.Parse(config.Endpoint)
	if err != nil || endpoint.Scheme != "http" || (endpoint.Hostname() != "127.0.0.1" && endpoint.Hostname() != "localhost") || config.Bucket != "lanverse-receipt-test" {
		t.Fatal("storage fixture must be a dedicated loopback test bucket")
	}
	sdk, err := minio.New(endpoint.Host, &minio.Options{Creds: credentials.NewStaticV4(config.AccessKey, config.SecretKey, ""), Secure: false})
	if err != nil {
		t.Fatal("configure MinIO fixture")
	}
	exists, err := sdk.BucketExists(t.Context(), config.Bucket)
	if err != nil {
		t.Fatalf("check isolated MinIO bucket: %v", err)
	}
	if !exists {
		if err := sdk.MakeBucket(t.Context(), config.Bucket, minio.MakeBucketOptions{}); err != nil {
			t.Fatal("create isolated test bucket")
		}
	}
	client, err := objectstorage.Open(config.Endpoint, config.Bucket, config.AccessKey, config.SecretKey, "")
	if err != nil {
		t.Fatal("configure private staging client")
	}
	return client
}

type receiptFaultObjects struct {
	application.ProviderStagingObjects
	manifestFailure bool
}

func (o *receiptFaultObjects) PutIfAbsent(ctx context.Context, key string, r io.Reader, size int64, mime, digest string) error {
	if o.manifestFailure && strings.HasSuffix(key, "manifest.json") {
		return errors.New("fixture manifest interruption")
	}
	return o.ProviderStagingObjects.PutIfAbsent(ctx, key, r, size, mime, digest)
}

type receiptFaultRecords struct {
	*pgoperation.Store
	writeFailure bool
}

func (r *receiptFaultRecords) CompleteProviderCall(ctx context.Context, input application.CompleteProviderCallInput) error {
	if r.writeFailure {
		return errors.New("fixture database interruption")
	}
	return r.Store.CompleteProviderCall(ctx, input)
}
