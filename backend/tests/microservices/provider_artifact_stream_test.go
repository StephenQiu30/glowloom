package microservices_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	contract "github.com/StephenQiu30/glowloom-server/backend/internal/contract/providerartifact"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/artifactrpc"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/media/application"
	mediadomain "github.com/StephenQiu30/glowloom-server/backend/media/domain"
)

func artifactRequest(t *testing.T, body []byte) contract.Request {
	t.Helper()
	project := uuid.New()
	sum := sha256.Sum256(body)
	req := contract.Request{Command: writefence.Command{ID: uuid.New(), Action: "media.provider_artifact.stage", Authorities: []writefence.Authority{{Issuer: writefence.Identity, Kind: writefence.Account, ID: uuid.New(), Epoch: 1}, {Issuer: writefence.Identity, Kind: writefence.Session, ID: uuid.New(), Epoch: 1}, {Issuer: writefence.Creation, Kind: writefence.Scope, ID: uuid.New(), Epoch: 1}, {Issuer: writefence.Creation, Kind: writefence.Project, ID: project, Epoch: 1}}}, DeliveryID: uuid.New(), ProviderCallID: uuid.New(), ProviderCallCreatedAt: time.Now().UTC(), OperationID: uuid.New(), ProjectID: project, CallAction: "submit", CallAttempt: 1, RequestKey: "original supplier request", Sequence: 1, ByteSize: int64(len(body)), SHA256: hex.EncodeToString(sum[:]), ManifestSHA256: strings.Repeat("a", 64), MIME: "image/png"}
	req.Command.SHA256 = req.Digest()
	if err := req.Validate(); err != nil {
		t.Fatal(err)
	}
	return req
}

type artifactWorkFake struct {
	prepared, committed int
}

func (r *artifactWorkFake) PrepareProviderArtifact(_ context.Context, req contract.Request) (mediaapp.ProviderArtifactWork, error) {
	r.prepared++
	return mediaapp.ProviderArtifactWork{Request: req, Intent: mediaapp.StorageWorkIntent{ID: uuid.New(), State: mediaapp.StorageWorkOpen}, PreparedAt: time.Now().UTC()}, nil
}
func (r *artifactWorkFake) ReadProviderArtifact(context.Context, contract.Request) (mediaapp.ProviderArtifactWork, error) {
	return mediaapp.ProviderArtifactWork{}, contract.ErrUnknown
}
func (r *artifactWorkFake) CommitProviderArtifact(context.Context, mediaapp.ProviderArtifactWork, mediadomain.ObjectLocation) (contract.Receipt, error) {
	r.committed++
	return contract.Receipt{}, errors.New("unexpected commit")
}

type artifactObjectsFake struct{ writes int }

func (o *artifactObjectsFake) CreateOperationObject(context.Context, mediaapp.StorageWorkIntent, string, *mediaapp.Downloaded) (mediadomain.ObjectLocation, error) {
	o.writes++
	return mediadomain.ObjectLocation{}, errors.New("unexpected object write")
}
func (*artifactObjectsFake) StatOperationObject(context.Context, mediadomain.ObjectLocation) (mediaapp.ObjectInfo, error) {
	return mediaapp.ObjectInfo{}, errors.New("unexpected stat")
}
func (*artifactObjectsFake) Get(context.Context, mediadomain.ObjectRef) (io.ReadCloser, error) {
	return nil, errors.New("unexpected read")
}

type artifactSourceFake struct{ open func() io.ReadCloser }

func (*artifactSourceFake) VerifyArtifactSource(context.Context, contract.Request) error { return nil }
func (s *artifactSourceFake) OpenArtifactSource(context.Context, contract.Request) (io.ReadCloser, error) {
	return s.open(), nil
}

type artifactFinalError struct{ *bytes.Reader }

func (r *artifactFinalError) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	if err == io.EOF {
		return n, errors.New("synthetic final source failure")
	}
	return n, err
}
func (*artifactFinalError) Close() error { return nil }

func TestProviderArtifactRequiresExactBytesHashEOFAndFinalRPCStatus(t *testing.T) {
	original := []byte("complete original source")
	for _, test := range []struct {
		name       string
		body       []byte
		finalError bool
		rpc        bool
	}{
		{"short", original[:len(original)-1], false, false},
		{"extra", append(append([]byte(nil), original...), 1), false, false},
		{"wrong_sha", bytes.Repeat([]byte{1}, len(original)), false, false},
		{"full_bytes_final_error", original, true, false},
		{"full_frames_final_non_ok", original, true, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			req := artifactRequest(t, original)
			repo := &artifactWorkFake{}
			objects := &artifactObjectsFake{}
			fake := &artifactSourceFake{open: func() io.ReadCloser {
				if test.finalError {
					return &artifactFinalError{bytes.NewReader(test.body)}
				}
				return io.NopCloser(bytes.NewReader(test.body))
			}}
			var source mediaapp.ProviderArtifactSource = fake
			if test.rpc {
				transport, peer, _ := ownerPeerTLS(t, "media")
				server, err := artifactrpc.NewSourceListener(fake, strings.Repeat("s", 40), 1, transport)
				if err != nil {
					t.Fatal(err)
				}
				source, err = artifactrpc.NewClient(artifactrpc.Target{Address: serveFrozenRPC(t, server), Token: strings.Repeat("s", 40), Transport: peer})
				if err != nil {
					t.Fatal(err)
				}
			}
			service, err := mediaapp.NewProviderArtifacts(repo, source, objects, objects)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := service.StageArtifact(t.Context(), req); err == nil {
				t.Fatal("incomplete source archived")
			}
			if repo.prepared != 1 || repo.committed != 0 || objects.writes != 0 {
				t.Fatalf("invalid bytes crossed archive boundary: prepared=%d writes=%d commits=%d", repo.prepared, objects.writes, repo.committed)
			}
		})
	}
}
