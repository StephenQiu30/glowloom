package media_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
)

func sourceDocumentAsset(data []byte) domain.MediaAsset {
	project, id, now := uuid.New(), uuid.New(), time.Now().UTC()
	hash := sha256.Sum256(data)
	sha, codec := hex.EncodeToString(hash[:]), "txt"
	return domain.MediaAsset{ID: id, ProjectID: project, Kind: domain.KindDocument, Origin: domain.OriginUpload,
		Status: domain.StatusReady, ModerationStatus: domain.ModerationPassed, ObjectKey: "projects/" + project.String() + "/document/2026/10/" + id.String() + ".txt",
		FileName: "原始剧本.txt", MimeType: domain.MIMEText, ByteSize: int64(len(data)), SHA256: &sha, Codec: &codec,
		Revision: 1, CreateTime: now, UpdateTime: now}
}

type documentSourceRepo struct {
	assets []domain.MediaAsset
	err    error
}

func (r *documentSourceRepo) FreezeDocumentSources(context.Context, identityapp.Principal, uuid.UUID, []uuid.UUID) ([]domain.MediaAsset, error) {
	return r.assets, r.err
}
func (r *documentSourceRepo) FindAsset(context.Context, identityapp.Principal, uuid.UUID, uuid.UUID) (domain.MediaAsset, error) {
	return r.assets[0], r.err
}

type documentSourceObjects struct {
	data  []byte
	calls int
	key   string
}

func (o *documentSourceObjects) Get(_ context.Context, key string) (io.ReadCloser, error) {
	o.calls++
	o.key = key
	return io.NopCloser(bytes.NewReader(o.data)), nil
}

func TestDocumentSourcesFreezeIdentityOrderAndExactOriginalRead(t *testing.T) {
	data := []byte("第一场\n原始正文\r\n")
	a := sourceDocumentAsset(data)
	b := sourceDocumentAsset([]byte("另一文件"))
	b.ProjectID = a.ProjectID
	b.ObjectKey = "projects/" + a.ProjectID.String() + "/document/2026/10/" + b.ID.String() + ".txt"
	repo, objects := &documentSourceRepo{assets: []domain.MediaAsset{b, a}}, &documentSourceObjects{data: data}
	reader := mediaapp.NewDocumentSources(repo, objects)
	got, err := reader.Freeze(t.Context(), identityapp.Principal{}, a.ProjectID, []uuid.UUID{a.ID, b.ID})
	if err != nil || len(got) != 2 || got[0].AssetID != a.ID || got[1].AssetID != b.ID || objects.calls != 0 {
		t.Fatal("freeze preserves ordered facts without object I/O", got, err)
	}
	raw, _ := json.Marshal(got)
	if bytes.Contains(raw, []byte("object_key")) || bytes.Contains(raw, []byte("projects/")) || bytes.Contains(raw, []byte("url")) {
		t.Fatal("source leaked private location")
	}
	repo.assets = []domain.MediaAsset{a}
	file, err := reader.Open(t.Context(), identityapp.Principal{}, a.ProjectID, got[0])
	if err != nil || file.Size != int64(len(data)) || file.SHA256 != *a.SHA256 || objects.key != a.ObjectKey {
		t.Fatal("verified private original", file, err)
	}
	actual, err := io.ReadAll(file.File)
	if err != nil || !bytes.Equal(actual, data) {
		t.Fatal("original bytes changed", err)
	}
	path := file.File.Name()
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("original staging file not cleaned", err)
	}
}

func TestDocumentSourcesRejectChangedScopeFactsAndPrivateBytes(t *testing.T) {
	data := []byte("原始剧本")
	for _, tc := range []struct {
		name   string
		change func(*domain.MediaAsset, *mediaapp.DocumentSource, *documentSourceObjects)
		want   error
		calls  int
	}{
		{"revision", func(a *domain.MediaAsset, _ *mediaapp.DocumentSource, _ *documentSourceObjects) { a.Revision++ }, mediaapp.ErrDocumentSourceConflict, 0},
		{"name", func(a *domain.MediaAsset, _ *mediaapp.DocumentSource, _ *documentSourceObjects) {
			a.FileName = "改名.txt"
		}, mediaapp.ErrDocumentSourceConflict, 0},
		{"cross project", func(_ *domain.MediaAsset, s *mediaapp.DocumentSource, _ *documentSourceObjects) {
			s.ProjectID = uuid.New()
		}, mediaapp.ErrNotFound, 0},
		{"unapproved", func(a *domain.MediaAsset, _ *mediaapp.DocumentSource, _ *documentSourceObjects) {
			a.Status = domain.StatusProcessing
			a.ModerationStatus = domain.ModerationPending
		}, mediaapp.ErrNotFound, 0},
		{"wrong SHA", func(_ *domain.MediaAsset, _ *mediaapp.DocumentSource, o *documentSourceObjects) {
			o.data = []byte("篡改剧本")
		}, mediaapp.ErrObjectMismatch, 1},
		{"extra bytes", func(_ *domain.MediaAsset, _ *mediaapp.DocumentSource, o *documentSourceObjects) {
			o.data = append(bytes.Clone(data), '!')
		}, mediaapp.ErrObjectMismatch, 1},
		{"short bytes", func(_ *domain.MediaAsset, _ *mediaapp.DocumentSource, o *documentSourceObjects) { o.data = data[:2] }, mediaapp.ErrObjectMismatch, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := sourceDocumentAsset(data)
			repo, objects := &documentSourceRepo{assets: []domain.MediaAsset{a}}, &documentSourceObjects{data: bytes.Clone(data)}
			reader := mediaapp.NewDocumentSources(repo, objects)
			frozen, err := reader.Freeze(t.Context(), identityapp.Principal{}, a.ProjectID, []uuid.UUID{a.ID})
			if err != nil {
				t.Fatal(err)
			}
			tc.change(&repo.assets[0], &frozen[0], objects)
			if file, err := reader.Open(t.Context(), identityapp.Principal{}, a.ProjectID, frozen[0]); !errors.Is(err, tc.want) || file != nil || objects.calls != tc.calls {
				t.Fatal("invalid source accepted", err, objects.calls)
			}
		})
	}
}

func TestDocumentSourcesRejectInvalidBatchAndCancelledReadBeforeObjectGet(t *testing.T) {
	a := sourceDocumentAsset([]byte("原文"))
	repo, objects := &documentSourceRepo{assets: []domain.MediaAsset{a}}, &documentSourceObjects{}
	reader := mediaapp.NewDocumentSources(repo, objects)
	for _, ids := range [][]uuid.UUID{nil, {uuid.Nil}, {a.ID, a.ID}, make([]uuid.UUID, 201)} {
		if _, err := reader.Freeze(t.Context(), identityapp.Principal{}, a.ProjectID, ids); !errors.Is(err, mediaapp.ErrInvalidQuery) {
			t.Fatal("invalid source batch", err)
		}
	}
	frozen, err := reader.Freeze(t.Context(), identityapp.Principal{}, a.ProjectID, []uuid.UUID{a.ID})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := reader.Open(ctx, identityapp.Principal{}, a.ProjectID, frozen[0]); !errors.Is(err, context.Canceled) || objects.calls != 0 {
		t.Fatal("cancel reached private object I/O", err, objects.calls)
	}
}

func TestDocumentDomainRejectsLegacyMalformedFacts(t *testing.T) {
	for _, change := range []func(*domain.MediaAsset){
		func(a *domain.MediaAsset) { a.ByteSize = 21 << 20 },
		func(a *domain.MediaAsset) { a.SHA256 = nil },
		func(a *domain.MediaAsset) { bad := strings.Repeat("g", 64); a.SHA256 = &bad },
		func(a *domain.MediaAsset) { a.MimeType = "application/pdf" },
		func(a *domain.MediaAsset) { a.Codec = nil },
		func(a *domain.MediaAsset) { v := int32(1); a.DurationMS = &v },
		func(a *domain.MediaAsset) { a.FileName = "../source.txt" },
	} {
		a := sourceDocumentAsset([]byte("内容"))
		change(&a)
		if a.Validate() == nil {
			t.Fatal("invalid document facts accepted")
		}
	}
}
