package media_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"testing"

	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
)

type purgePhysicalFixture struct {
	objects       map[string][]byte
	started       map[string]bool
	removed       map[string]bool
	removeUnknown bool
	deleteCalls   int
}

func (f *purgePhysicalFixture) Get(_ context.Context, key string) (io.ReadCloser, error) {
	b, ok := f.objects[key]
	if !ok {
		return nil, io.EOF
	}
	return io.NopCloser(bytes.NewReader(b)), nil
}
func (f *purgePhysicalFixture) Exists(_ context.Context, key string) (bool, error) {
	_, ok := f.objects[key]
	return ok, nil
}
func (f *purgePhysicalFixture) Remove(_ context.Context, key string) error {
	f.deleteCalls++
	delete(f.objects, key)
	if f.removeUnknown {
		f.removeUnknown = false
		return io.ErrUnexpectedEOF
	}
	return nil
}
func (f *purgePhysicalFixture) RecordPurgeDigest(_ context.Context, key, digest string, size int64) error {
	b, ok := f.objects[key]
	sum := sha256.Sum256(b)
	if !ok || size != int64(len(b)) || digest != hex.EncodeToString(sum[:]) {
		return mediaapp.ErrObjectMismatch
	}
	return nil
}
func (f *purgePhysicalFixture) BeginPurgeRemoval(_ context.Context, key string) error {
	f.started[key] = true
	return nil
}
func (f *purgePhysicalFixture) ConfirmPurgeRemoval(_ context.Context, key string) error {
	if !f.started[key] {
		return mediaapp.ErrObjectMismatch
	}
	if _, exists := f.objects[key]; exists {
		return mediaapp.ErrObjectMismatch
	}
	f.removed[key] = true
	return nil
}

func TestMediaPurgeVerifiesAllBytesBeforeFirstDestructiveIO(t *testing.T) {
	original, rendition := []byte("actual original bytes"), []byte("actual rendition bytes")
	sum := sha256.Sum256(original)
	sha := hex.EncodeToString(sum[:])
	size := int64(len(original))
	bad := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	f := &purgePhysicalFixture{objects: map[string][]byte{"original": original, "thumb": rendition}, started: map[string]bool{}, removed: map[string]bool{}}
	objects := []mediaapp.PurgeObject{{ObjectKey: "original", SHA256: &sha, ByteSize: &size}, {ObjectKey: "thumb", SHA256: &bad}}
	err := mediaapp.RemovePurgeObjects(t.Context(), f, f, objects)
	if !errors.Is(err, mediaapp.ErrObjectMismatch) || f.deleteCalls != 0 || len(f.objects) != 2 {
		t.Fatal("corrupt last rendition caused partial destructive deletion", err, f.deleteCalls)
	}
}

func TestMediaPurgeUnknownRemoveRequiresOriginalIntentAndAbsentReadback(t *testing.T) {
	content := []byte("independently owned original")
	sum := sha256.Sum256(content)
	sha := hex.EncodeToString(sum[:])
	size := int64(len(content))
	f := &purgePhysicalFixture{objects: map[string][]byte{"original": content}, started: map[string]bool{}, removed: map[string]bool{}, removeUnknown: true}
	objects := []mediaapp.PurgeObject{{ObjectKey: "original", SHA256: &sha, ByteSize: &size}}
	err := mediaapp.RemovePurgeObjects(t.Context(), f, f, objects)
	if !errors.Is(err, mediaapp.ErrPurgeUnknown) || f.removed["original"] || !f.started["original"] || f.deleteCalls != 1 {
		t.Fatal("uncertain deletion was falsely completed", err, f)
	}
	objects[0].RemovalStarted = true
	objects[0].Verified = true
	if err := mediaapp.RemovePurgeObjects(t.Context(), f, f, objects); err != nil || !f.removed["original"] || f.deleteCalls != 1 {
		t.Fatal("same permanent intent could not settle actual absence", err, f)
	}
}

func TestMediaPurgeMissingUnstartedObjectIsNotProvenDeleted(t *testing.T) {
	f := &purgePhysicalFixture{objects: map[string][]byte{}, started: map[string]bool{}, removed: map[string]bool{}}
	sha := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	size := int64(1)
	err := mediaapp.RemovePurgeObjects(t.Context(), f, f, []mediaapp.PurgeObject{{ObjectKey: "missing", SHA256: &sha, ByteSize: &size}})
	if err == nil || len(f.removed) != 0 || f.deleteCalls != 0 {
		t.Fatal("absence without owning removal intent was fabricated as purge success", err)
	}
}
