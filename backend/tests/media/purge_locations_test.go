package media_test

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/google/uuid"

	media "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

type locatedPurgeIO struct {
	read, removed int
	missing       bool
	location      *domain.ObjectLocation
}

func (*locatedPurgeIO) Get(context.Context, string) (io.ReadCloser, error) { panic("bare read") }
func (*locatedPurgeIO) Exists(context.Context, string) (bool, error)       { panic("bare stat") }
func (*locatedPurgeIO) Remove(context.Context, string) error               { panic("bare delete") }
func (p *locatedPurgeIO) GetPurge(_ context.Context, o media.PurgeObject) (io.ReadCloser, error) {
	if o.Location == nil || o.Location.ID != p.location.ID {
		return nil, media.ErrObjectMismatch
	}
	p.read++
	return io.NopCloser(strings.NewReader("owned")), nil
}
func (p *locatedPurgeIO) ExistsPurge(_ context.Context, o media.PurgeObject) (bool, error) {
	if o.Location == nil || o.Location.ID != p.location.ID {
		return false, media.ErrObjectMismatch
	}
	return !p.missing, nil
}
func (p *locatedPurgeIO) RemovePurge(_ context.Context, o media.PurgeObject) error {
	if o.Location == nil || o.Location.ID != p.location.ID {
		return media.ErrObjectMismatch
	}
	p.removed++
	p.missing = true
	return nil
}

type locatedPurgeIntents struct{}

func (locatedPurgeIntents) RecordPurgeDigest(context.Context, string, string, int64) error {
	return nil
}
func (locatedPurgeIntents) BeginPurgeRemoval(context.Context, string) error   { return nil }
func (locatedPurgeIntents) ConfirmPurgeRemoval(context.Context, string) error { return nil }
func TestPurgeUsesFrozenLocationInsteadOfActiveBareKey(t *testing.T) {
	l := &domain.ObjectLocation{ID: uuid.New(), Ref: domain.ObjectRef{ProfileID: "old-main", Bucket: "original-files", Key: "owned/file"}, State: "present"}
	objects := &locatedPurgeIO{location: l}
	if err := media.RemovePurgeObjects(t.Context(), objects, locatedPurgeIntents{}, []media.PurgeObject{{ObjectKey: l.Ref.Key, Location: l}}); err != nil {
		t.Fatal(err)
	}
	if objects.read != 1 || objects.removed != 1 {
		t.Fatal("wrong physical object set", objects)
	}
}
