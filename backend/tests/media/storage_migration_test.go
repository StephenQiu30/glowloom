package media_test

import (
	"bytes"
	"errors"
	"testing"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

func TestStorageFrozenReferenceChecksActualBytesAndProviderFailure(t *testing.T) {
	data := []byte("immutable audio bytes")
	a := routedReadImage()
	a.Kind = domain.KindAudio
	a.ObjectKey = "projects/" + a.ProjectID.String() + "/audio/2026/11/" + a.ID.String() + ".mp3"
	a.FileName = "test.mp3"
	codec := "mp3"
	a.Codec = &codec
	duration := int32(1000)
	a.DurationMS = &duration
	a.MimeType = "audio/mpeg"
	facts := sourceDocumentAsset(data)
	a.SHA256 = facts.SHA256
	a.ByteSize = facts.ByteSize
	a.ObjectLocation = readLocation("minio-main", a.ObjectKey)
	objects := &routedReadObjects{data: map[domain.ObjectRef][]byte{a.ObjectLocation.Ref: data}}
	reader := routedReferenceReader{file: mediaapp.LibraryMediaFile{Asset: a}}
	query := mediaapp.NewRoutedReferenceFactQuery(reader, objects)
	fact, err := query.Reference(t.Context(), identityapp.Principal{}, a.ProjectID, a.ID, domain.KindAudio)
	if err != nil || fact.SHA256 != *a.SHA256 {
		t.Fatal("actual bytes not verified", err)
	}
	objects.data[a.ObjectLocation.Ref] = bytes.Repeat([]byte("x"), len(data))
	if _, err := query.Reference(t.Context(), identityapp.Principal{}, a.ProjectID, a.ID, domain.KindAudio); !errors.Is(err, mediaapp.ErrObjectMismatch) {
		t.Fatal("forged metadata accepted", err)
	}
	objects.err = errors.New("provider forbidden")
	if _, err := query.Reference(t.Context(), identityapp.Principal{}, a.ProjectID, a.ID, domain.KindAudio); !errors.Is(err, objects.err) {
		t.Fatal("provider error became absence proof", err)
	}
}
