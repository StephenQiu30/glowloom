package subject_test

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

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	media "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	mediadomain "github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

type subjectSourceReader struct {
	file media.LibraryMediaFile
	err  error
}

func (r *subjectSourceReader) FindSubjectSource(context.Context, identity.Principal, mediadomain.LibraryScope, uuid.UUID) (media.LibraryMediaFile, error) {
	return r.file, r.err
}
func (r *subjectSourceReader) ReadBoundSubjectSource(context.Context, uuid.UUID) (media.LibraryMediaFile, error) {
	return r.file, r.err
}

type subjectObjects struct {
	data   []byte
	reads  int
	signed int
	closed int
}

type closedSubjectReader struct {
	*bytes.Reader
	owner *subjectObjects
}

func (r closedSubjectReader) Close() error { r.owner.closed++; return nil }
func (o *subjectObjects) Get(context.Context, string) (io.ReadCloser, error) {
	o.reads++
	return closedSubjectReader{Reader: bytes.NewReader(o.data), owner: o}, nil
}
func (o *subjectObjects) PresignGet(context.Context, string, time.Duration) (string, error) {
	o.signed++
	return "https://storage.invalid/synthetic-signed-input", nil
}

func sourceFixture() (identity.Principal, *subjectSourceReader, *subjectObjects, media.SubjectSourceFact) {
	actor := identity.Principal{ID: uuid.New(), OrgID: uuid.New()}
	data := []byte("verified original fixture bytes")
	digest := sha256.Sum256(data)
	sha := hex.EncodeToString(digest[:])
	a := mediadomain.MediaAsset{ID: uuid.New(), Personal: &mediadomain.PersonalOwnership{OrgID: actor.OrgID, ActorID: actor.ID}, Kind: mediadomain.KindImage, Origin: mediadomain.OriginUpload, Status: mediadomain.StatusReady, FileName: "original.png", MimeType: "image/png", ByteSize: int64(len(data)), SHA256: &sha, ModerationStatus: mediadomain.ModerationPassed, Revision: 1, CreateTime: time.Now().UTC(), UpdateTime: time.Now().UTC()}
	a.ObjectKey = "personal/" + actor.OrgID.String() + "/" + actor.ID.String() + "/image/2026/10/" + a.ID.String() + ".png"
	return actor, &subjectSourceReader{file: media.LibraryMediaFile{Asset: a}}, &subjectObjects{data: data}, media.SubjectSourceFact{ReferenceFact: media.ReferenceFact{AssetID: a.ID, Revision: a.Revision, Kind: a.Kind, SHA256: sha, ByteSize: a.ByteSize}, Personal: &mediadomain.PersonalOwnership{OrgID: actor.OrgID, ActorID: actor.ID}}
}

func TestSubjectSourceUsesOriginalPersonalBytesWithoutProjectReferenceGate(t *testing.T) {
	actor, reader, objects, _ := sourceFixture()
	if reader.file.Asset.Validate() != nil || reader.file.Asset.CanReference() {
		t.Fatal("fixture must be a valid personal file rejected by ordinary project reference rules")
	}
	query := media.NewSubjectSourceQuery(reader, objects, objects)
	scope := mediadomain.LibraryScope{Kind: mediadomain.LibraryPersonal}
	file, err := query.Reference(t.Context(), actor, scope, reader.file.Asset.ID)
	if err != nil || file.Asset.ID != reader.file.Asset.ID || objects.reads != 1 || objects.closed != 1 {
		t.Fatal("personal original adoption", err)
	}
	objects.data[0] ^= 1
	if _, err := query.Reference(t.Context(), actor, scope, reader.file.Asset.ID); !errors.Is(err, media.ErrObjectMismatch) {
		t.Fatal("same-length original corruption was accepted", err)
	}
	objects.data = objects.data[:len(objects.data)-1]
	if _, err := query.Reference(t.Context(), actor, scope, reader.file.Asset.ID); !errors.Is(err, media.ErrObjectMismatch) {
		t.Fatal("truncated original was accepted", err)
	}
	if objects.reads != objects.closed {
		t.Fatal("original reader leaked")
	}
	other := actor
	other.ID = uuid.New()
	before := objects.reads
	if _, err := query.Reference(t.Context(), other, scope, reader.file.Asset.ID); !errors.Is(err, media.ErrNotFound) || objects.reads != before {
		t.Fatal("foreign personal source exposed bytes", err)
	}
	reader.err = media.ErrNotFound
	if _, err := query.Reference(t.Context(), actor, scope, reader.file.Asset.ID); !errors.Is(err, media.ErrNotFound) || objects.reads != before {
		t.Fatal("source-library rejection was bypassed", err)
	}
}

func TestSubjectSourceSigningAndDownloadsRejectObjectCorruption(t *testing.T) {
	_, reader, objects, expected := sourceFixture()
	query := media.NewSubjectSourceQuery(reader, objects, objects)
	download, summary, err := query.DownloadBound(t.Context(), expected)
	if err != nil {
		t.Fatal(err)
	}
	data, readErr := io.ReadAll(download.File)
	closeErr := download.Close()
	if readErr != nil || closeErr != nil || !bytes.Equal(data, objects.data) || summary.FileName != "original.png" {
		t.Fatal("verified download did not preserve original bytes", readErr, closeErr)
	}
	if grant, err := query.SignBound(t.Context(), expected, time.Minute); err != nil || grant.URL == "" || objects.signed != 1 {
		t.Fatal("verified original did not receive a bounded lease", err)
	}
	objects.data = []byte(strings.Repeat("x", len(objects.data)))
	if download, _, err := query.DownloadBound(t.Context(), expected); !errors.Is(err, media.ErrObjectMismatch) || download != nil {
		t.Fatal("corrupt download exposed a stream", err)
	}
	if _, err := query.SignBound(t.Context(), expected, time.Minute); !errors.Is(err, media.ErrObjectMismatch) || objects.signed != 1 {
		t.Fatal("corrupt original received a provider lease", err)
	}
}

func TestSubjectSourceBoundRejectsChangedFrozenFactsAndSourceOwner(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*mediadomain.MediaAsset)
	}{
		{"revision", func(a *mediadomain.MediaAsset) { a.Revision++ }},
		{"SHA", func(a *mediadomain.MediaAsset) { sha := strings.Repeat("a", 64); a.SHA256 = &sha }},
		{"size", func(a *mediadomain.MediaAsset) { a.ByteSize++ }},
		{"personal actor", func(a *mediadomain.MediaAsset) {
			a.Personal.ActorID = uuid.New()
			a.ObjectKey = "personal/" + a.Personal.OrgID.String() + "/" + a.Personal.ActorID.String() + "/image/2026/10/" + a.ID.String() + ".png"
		}},
		{"personal organization", func(a *mediadomain.MediaAsset) {
			a.Personal.OrgID = uuid.New()
			a.ObjectKey = "personal/" + a.Personal.OrgID.String() + "/" + a.Personal.ActorID.String() + "/image/2026/10/" + a.ID.String() + ".png"
		}},
		{"owner kind", func(a *mediadomain.MediaAsset) {
			a.Personal, a.ProjectID = nil, uuid.New()
			a.ObjectKey = "projects/" + a.ProjectID.String() + "/image/2026/10/" + a.ID.String() + ".png"
		}},
		{"moderation", func(a *mediadomain.MediaAsset) { a.ModerationStatus = mediadomain.ModerationRejected }},
		{"consent requirement", func(a *mediadomain.MediaAsset) { a.ContainsRealPerson = true }},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, reader, objects, expected := sourceFixture()
			query := media.NewSubjectSourceQuery(reader, objects, objects)
			if _, err := query.Bound(t.Context(), expected); err != nil {
				t.Fatal("unchanged frozen source", err)
			}
			test.change(&reader.file.Asset)
			if _, err := query.Bound(t.Context(), expected); !errors.Is(err, media.ErrNotFound) || objects.reads != 0 {
				t.Fatal("changed binding fact reached physical object", err)
			}
		})
	}
	_, reader, objects, expected := sourceFixture()
	reader.file.Asset.ProjectID, reader.file.Asset.Personal = uuid.New(), nil
	reader.file.Asset.ObjectKey = "projects/" + reader.file.Asset.ProjectID.String() + "/image/2026/10/" + reader.file.Asset.ID.String() + ".png"
	expected.ProjectID, expected.Personal = reader.file.Asset.ProjectID, nil
	query := media.NewSubjectSourceQuery(reader, objects, objects)
	if _, err := query.Bound(t.Context(), expected); err != nil {
		t.Fatal("unchanged project binding", err)
	}
	reader.file.Asset.ProjectID = uuid.New()
	reader.file.Asset.ObjectKey = "projects/" + reader.file.Asset.ProjectID.String() + "/image/2026/10/" + reader.file.Asset.ID.String() + ".png"
	if _, err := query.Bound(t.Context(), expected); !errors.Is(err, media.ErrNotFound) {
		t.Fatal("changed source project accepted", err)
	}
}
