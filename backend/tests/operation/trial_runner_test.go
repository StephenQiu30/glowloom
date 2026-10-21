package operation_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"image"
	"image/png"
	"io"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/adapter/credentialseal"
	catalogdomain "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/trial"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/objectstorage"
)

type trialBindings struct {
	binding trial.Binding
	deny    bool
}

func (s *trialBindings) Authorize(context.Context, identityapp.Principal) error {
	if s.deny {
		return errors.New("revoked")
	}
	return nil
}
func (s *trialBindings) Binding(context.Context, application.TrialInput) (trial.Binding, error) {
	return s.binding, nil
}

type trialObjects struct {
	data        []byte
	info        objectstorage.ObjectInfo
	writes      int
	lostReceipt bool
}

func (s *trialObjects) PutIfAbsent(_ context.Context, _ string, r io.Reader, size int64, mime, sha string) error {
	s.writes++
	if s.data != nil {
		return errors.New("exists")
	}
	s.data, _ = io.ReadAll(r)
	s.info = objectstorage.ObjectInfo{Size: size, ContentType: mime, SHA256: sha}
	if s.lostReceipt {
		return errors.New("receipt lost")
	}
	return nil
}
func (s *trialObjects) Stat(context.Context, string) (objectstorage.ObjectInfo, error) {
	if s.data == nil {
		return s.info, errors.New("missing")
	}
	return s.info, nil
}
func (s *trialObjects) Get(context.Context, string) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(s.data)), nil
}

type trialCloud struct {
	result           application.GenerationResult
	submits, queries int
	last             application.GenerationRequest
}

func (c *trialCloud) Submit(_ context.Context, r application.GenerationRequest, _ string) (application.GenerationResult, error) {
	c.submits++
	c.last = r
	return c.result, nil
}
func (c *trialCloud) Query(context.Context, catalogdomain.ProviderConfig, string, string) (application.GenerationResult, error) {
	c.queries++
	return c.result, nil
}
func (c *trialCloud) Cancel(context.Context, catalogdomain.ProviderConfig, string, string) (application.GenerationResult, error) {
	panic("not used")
}
func trialRunnerFixture(t *testing.T) (*trial.Runner, *trialCloud, *trialObjects, application.TrialInput) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	public, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	require.NoError(t, err)
	private, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err)
	seal, err := credentialseal.NewSealer("fixture", pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: public}))
	require.NoError(t, err)
	unseal, err := credentialseal.NewUnsealer("fixture", pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: private}))
	require.NoError(t, err)
	in := application.TrialInput{Request: application.TrialRequest{RequestID: uuid.New(), Source: "cloud", Kind: "image", ProviderID: uuid.New(), Model: "fixture-seedream", Prompt: "circle"}}
	credential := uuid.New()
	encrypted, err := seal.Seal(in.Request.ProviderID, credential, []byte(`{"api_key":"fixture-only"}`))
	require.NoError(t, err)
	store := &trialBindings{binding: trial.Binding{ConfigID: uuid.New(), CredentialID: credential, KeyID: "fixture", Ciphertext: encrypted}}
	cloud, objects := &trialCloud{}, &trialObjects{}
	return &trial.Runner{Store: store, Cloud: cloud, Objects: objects, Keys: unseal, StorageID: "fixture-target"}, cloud, objects, in
}
func TestTrialRunnerStoresOneImageAndRecoversLostUploadReceiptWithoutInference(t *testing.T) {
	r, cloud, objects, input := trialRunnerFixture(t)
	var data bytes.Buffer
	require.NoError(t, png.Encode(&data, image.NewRGBA(image.Rect(0, 0, 2, 2))))
	cloud.result = application.GenerationResult{Outcome: "completed", Images: []application.GeneratedImage{{Bytes: data.Bytes(), MediaType: "image/png"}}}
	objects.lostReceipt = true
	result, err := r.Run(t.Context(), input)
	require.NoError(t, err)
	require.Equal(t, "succeeded", result.State)
	require.Len(t, result.Artifacts, 1)
	require.Equal(t, 1, cloud.submits)
	require.Equal(t, int64(data.Len()), result.Artifacts[0].Size)
	require.Equal(t, "fixture-target", result.Artifacts[0].StorageID)
	input.Action = "recover"
	recovered, err := r.Run(t.Context(), input)
	require.NoError(t, err)
	require.Equal(t, "succeeded", recovered.State)
	require.Equal(t, 1, cloud.submits)
	require.Equal(t, 1, objects.writes)
	reader, err := trial.ReadArtifact(t.Context(), objects, input, recovered.Artifacts[0])
	require.NoError(t, err)
	defer func() { _ = reader.Close() }()
	saved, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.Equal(t, data.Bytes(), saved)
	objects.info.ContentType = "text/html"
	_, err = trial.ReadArtifact(t.Context(), objects, input, recovered.Artifacts[0])
	require.Error(t, err)
}
func TestTrialRunnerRejectsHeaderOnlyImageAndRevokedAuthority(t *testing.T) {
	r, cloud, objects, input := trialRunnerFixture(t)
	var data bytes.Buffer
	require.NoError(t, png.Encode(&data, image.NewRGBA(image.Rect(0, 0, 2, 2))))
	cloud.result = application.GenerationResult{Outcome: "completed", Images: []application.GeneratedImage{{Bytes: data.Bytes()[:40], MediaType: "image/png"}}}
	result, err := r.Run(t.Context(), input)
	require.NoError(t, err)
	require.Equal(t, "unknown", result.State)
	require.Zero(t, objects.writes)
	r.Store.(*trialBindings).deny = true
	_, err = r.Run(t.Context(), input)
	require.NoError(t, err)
	require.Equal(t, 1, cloud.submits)
}
func TestTrialRecoveryWithoutSavedImageNeverSubmitsAgain(t *testing.T) {
	r, cloud, _, input := trialRunnerFixture(t)
	input.Action = "recover"
	result, err := r.Run(t.Context(), input)
	require.NoError(t, err)
	require.Equal(t, "unknown", result.State)
	require.Zero(t, cloud.submits)
	require.Zero(t, cloud.queries)
}
