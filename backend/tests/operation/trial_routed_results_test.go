package operation_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app/aitrial"
	mediadomain "github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/trial"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/objectstorage"
)

func TestTrialOfficialSDKRecoversOriginalPhysicalRefDespiteDifferentDefault(t *testing.T) {
	runner, cloud, _, input := trialRunnerFixture(t)
	body := []byte("explicit saved trial bytes")
	sum := sha256.Sum256(body)
	digest := hex.EncodeToString(sum[:])
	reads := 0
	original := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reads++
		require.True(t, strings.HasPrefix(r.Header.Get("Authorization"), "AWS4-HMAC-SHA256 "))
		require.Equal(t, "/trial-first/"+application.TrialObjectKey(input, 1), r.URL.Path)
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("X-Amz-Meta-Sha256", digest)
		w.Header().Set("Last-Modified", time.Now().UTC().Format(http.TimeFormat))
		w.Header().Set("ETag", `"fixture"`)
		if r.Method == http.MethodGet {
			_, _ = w.Write(body)
		} else {
			require.Equal(t, http.MethodHead, r.Method)
		}
	}))
	t.Cleanup(original.Close)
	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("existing trial followed new default")
		w.WriteHeader(403)
	}))
	t.Cleanup(second.Close)
	t.Setenv("TRIAL_FIXTURE_AK", "fixture-access")
	t.Setenv("TRIAL_FIXTURE_SK", "fixture-secret")
	profile := func(endpoint, bucket string) any {
		return map[string]any{"provider": "minio", "mode": config.StorageProfileReadWrite, "endpoint": endpoint, "signing_endpoint": endpoint, "addressing_style": "path", "bucket": bucket, "region": "us-east-1", "credentials": map[string]any{"source": "env", "access_key_env": "TRIAL_FIXTURE_AK", "secret_key_env": "TRIAL_FIXTURE_SK"}}
	}
	raw, err := json.Marshal(map[string]any{"schema_version": 1, "revision": "2", "default_write_profile": "trial-second", "upload": map[string]any{"mode": "relay", "signed_url_ttl": time.Minute, "session_ttl": time.Hour, "staging_retention": 24 * time.Hour, "multipart_threshold_bytes": 8 << 20, "part_size_bytes": 5 << 20, "part_concurrency": 1}, "access": map[string]any{"preview_ttl": time.Minute, "max_signed_url_ttl": 15 * time.Minute}, "limits": map[string]any{"max_image_bytes": 20 << 20, "max_video_bytes": 500 << 20, "max_audio_bytes": 100 << 20, "max_model_bytes": 64 << 20, "max_document_bytes": 20 << 20}, "profiles": map[string]any{"trial-first": profile(original.URL, "trial-first"), "trial-second": profile(second.URL, "trial-second")}})
	require.NoError(t, err)
	cfg, err := config.LoadObjectStorageEnvironment(string(raw), "local", os.Getenv)
	require.NoError(t, err)
	registry, err := objectstorage.OpenProfiles(cfg)
	require.NoError(t, err)
	t.Cleanup(registry.Close)
	objects := aitrial.NewObjects(registry)
	runner.Objects = objects
	input.OutputRef = &mediadomain.ObjectRef{ProfileID: "trial-first", Bucket: "trial-first", Key: application.TrialObjectKey(input, 1)}
	input.Action = "recover"
	result, err := runner.Run(t.Context(), input)
	require.NoError(t, err)
	require.Equal(t, "succeeded", result.State)
	require.Len(t, result.Artifacts, 1)
	require.Equal(t, input.OutputRef, result.Artifacts[0].ObjectRef)
	require.Zero(t, cloud.submits)
	require.Zero(t, cloud.queries)
	reader, err := trial.ReadArtifact(t.Context(), objects, input, result.Artifacts[0])
	require.NoError(t, err)
	data, readErr := io.ReadAll(reader)
	closeErr := reader.Close()
	require.NoError(t, readErr)
	require.NoError(t, closeErr)
	require.True(t, bytes.Equal(body, data))
	require.GreaterOrEqual(t, reads, 3)
}
