package operation

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	catalog "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/providerhttp"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/tests/storagefixture"
)

type imageReviewTransport func(*http.Request) (*http.Response, error)

func (f imageReviewTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestLocalProjectImageReachesArkAsVerifiedBytes(t *testing.T) {
	var imageData bytes.Buffer
	require.NoError(t, png.Encode(&imageData, image.NewRGBA(image.Rect(0, 0, 32, 32))))
	data := imageData.Bytes()
	reads := 0
	storage := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/fixture/input.png", r.URL.Path)
		reads++
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Last-Modified", "Wed, 04 Nov 2026 00:00:00 GMT")
		_, _ = w.Write(data)
	}))
	defer storage.Close()
	objects, err := storagefixture.Open(t, storage.URL, "fixture")
	require.NoError(t, err)
	sum := sha256.Sum256(data)
	fact := application.ProjectMediaObject{AssetID: uuid.New(), ProjectID: uuid.New(), Key: "input.png", SHA256: hex.EncodeToString(sum[:]), Kind: "image", Role: "reference_image", ByteSize: int64(len(data))}
	locationID := uuid.New()
	fact.LocationID, fact.ProfileID, fact.Bucket = &locationID, "test-main", "fixture"
	signer := NewProjectMediaSigner(objects)
	ref, err := signer.SignProjectMedia(t.Context(), fact, time.Minute)
	require.NoError(t, err)
	require.Equal(t, 1, reads)
	var config catalog.ProviderConfig
	for _, p := range catalog.BuiltinPresets() {
		if p.Config.AdapterKey == "volcengine_ark" {
			config = p.Config
		}
	}
	calls := 0
	provider := providerhttp.NewClient(&http.Client{Transport: imageReviewTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		require.Equal(t, "/api/v3/images/generations", r.URL.Path)
		var payload struct {
			Images []string `json:"image"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
		require.Len(t, payload.Images, 1)
		require.Equal(t, "data:image/png;base64,"+base64.StdEncoding.EncodeToString(data), payload.Images[0])
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"data":[{"b64_json":"` + base64.StdEncoding.EncodeToString(data) + `"}]}`))}, nil
	})})
	result, err := provider.Submit(t.Context(), application.GenerationRequest{Config: config, ModelID: "doubao-seedream-5-0-flash-260915", Capability: "image.generate", Mode: "image2image", Prompt: "fixture", Params: map[string]any{"size": "2K"}, References: []application.ProviderReference{ref}, OutputCount: 1}, "fixture-key")
	require.NoError(t, err)
	require.Equal(t, 1, calls, "local storage URL must not be sent to the cloud or block a verified image")
	require.Equal(t, application.ProviderTaskSucceeded, result.State)
	encoded, err := json.Marshal(ref)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), base64.StdEncoding.EncodeToString(data), "private bytes must stay outside workflow serialization")
	fact.SHA256 = strings.Repeat("0", 64)
	_, err = signer.SignProjectMedia(t.Context(), fact, time.Minute)
	require.ErrorIs(t, err, application.ErrWorkflowInputNotReady)
}
