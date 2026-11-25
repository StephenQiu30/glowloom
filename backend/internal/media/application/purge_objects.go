package application

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"

	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

// ErrPurgeUnknown preserves uncertainty after an owning removal intent. It must
// be reconciled under the same durable object identity before reporting success.
var ErrPurgeUnknown = errors.New("media purge outcome unknown")

// PurgeObject is an internal immutable key plus its persisted physical receipt.
// Neither this object nor the original bytes cross a public job response.
type PurgeObject struct {
	Location       *domain.ObjectLocation `json:"-"`
	ObjectKey      string                 `json:"object_key"`
	ByteSize       *int64                 `json:"byte_size"`
	SHA256         *string                `json:"sha256"`
	Verified       bool                   `json:"verified"`
	RemovalStarted bool                   `json:"removal_started"`
	Removed        bool                   `json:"removed"`
}

// PurgeObjects exposes only exact-key operations selected by the media owner.
type PurgeObjects interface {
	Get(context.Context, string) (io.ReadCloser, error)
	Exists(context.Context, string) (bool, error)
	Remove(context.Context, string) error
}

// PurgeObjectIntents must bind every callback to its current SQL worker fence.
// Digest and removal intents are committed before the corresponding mutation.
type PurgeObjectIntents interface {
	RecordPurgeDigest(context.Context, string, string, int64) error
	BeginPurgeRemoval(context.Context, string) error
	ConfirmPurgeRemoval(context.Context, string) error
}

// RemovePurgeObjects verifies the whole original/rendition set before the first
// deletion. Only a preexisting owning intent can settle an already absent key.
func RemovePurgeObjects(ctx context.Context, objects PurgeObjects, intents PurgeObjectIntents, original []PurgeObject) error {
	if objects == nil || intents == nil || len(original) < 1 || len(original) > 17 {
		return ErrUnavailable
	}
	held := slices.Clone(original)
	seen := make(map[string]bool, len(held))
	for i, object := range held {
		if object.ObjectKey == "" || len(object.ObjectKey) > 2048 || seen[object.ObjectKey] || object.RemovalStarted && (!object.Verified || object.SHA256 == nil || object.ByteSize == nil) || object.Removed && !object.RemovalStarted {
			return ErrObjectMismatch
		}
		seen[object.ObjectKey] = true
		if err := ctx.Err(); err != nil {
			return err
		}
		if object.RemovalStarted {
			present, err := purgeExists(ctx, objects, object)
			if err != nil {
				return errors.Join(ErrPurgeUnknown, err)
			}
			if !present {
				continue
			}
			if object.Removed {
				return ErrObjectMismatch
			}
		}
		reader, err := purgeGet(ctx, objects, object)
		if err != nil {
			return err
		}
		size, digest, readErr := copyObjectBytes(ctx, reader, io.Discard, object.ByteSize, object.SHA256)
		if err := errors.Join(readErr, reader.Close()); err != nil {
			return err
		}
		if err := intents.RecordPurgeDigest(ctx, object.ObjectKey, digest, size); err != nil {
			return err
		}
		held[i].SHA256, held[i].ByteSize, held[i].Verified = &digest, &size, true
	}
	for _, object := range held {
		if err := ctx.Err(); err != nil {
			return err
		}
		if object.RemovalStarted {
			present, err := purgeExists(ctx, objects, object)
			if err != nil {
				return errors.Join(ErrPurgeUnknown, err)
			}
			if !present {
				if err := intents.ConfirmPurgeRemoval(ctx, object.ObjectKey); err != nil {
					return errors.Join(ErrPurgeUnknown, err)
				}
				continue
			}
		} else if err := intents.BeginPurgeRemoval(ctx, object.ObjectKey); err != nil {
			return err
		}
		if err := purgeRemove(ctx, objects, object); err != nil {
			return errors.Join(ErrPurgeUnknown, fmt.Errorf("remove owned media object: %w", err))
		}
		present, err := purgeExists(ctx, objects, object)
		if err != nil {
			return errors.Join(ErrPurgeUnknown, err)
		}
		if present {
			return ErrPurgeUnknown
		}
		if err := intents.ConfirmPurgeRemoval(ctx, object.ObjectKey); err != nil {
			return errors.Join(ErrPurgeUnknown, err)
		}
	}
	return nil
}

// LocatedPurgeObjects consumes only the owning repository's physical receipt.
type LocatedPurgeObjects interface {
	GetPurge(context.Context, PurgeObject) (io.ReadCloser, error)
	ExistsPurge(context.Context, PurgeObject) (bool, error)
	RemovePurge(context.Context, PurgeObject) error
}

func purgeGet(ctx context.Context, objects PurgeObjects, o PurgeObject) (io.ReadCloser, error) {
	if located, ok := objects.(LocatedPurgeObjects); ok {
		return located.GetPurge(ctx, o)
	}
	if o.Location != nil {
		return nil, ErrUnavailable
	}
	return objects.Get(ctx, o.ObjectKey)
}
func purgeExists(ctx context.Context, objects PurgeObjects, o PurgeObject) (bool, error) {
	if located, ok := objects.(LocatedPurgeObjects); ok {
		return located.ExistsPurge(ctx, o)
	}
	if o.Location != nil {
		return false, ErrUnavailable
	}
	return objects.Exists(ctx, o.ObjectKey)
}
func purgeRemove(ctx context.Context, objects PurgeObjects, o PurgeObject) error {
	if located, ok := objects.(LocatedPurgeObjects); ok {
		return located.RemovePurge(ctx, o)
	}
	if o.Location != nil {
		return ErrUnavailable
	}
	return objects.Remove(ctx, o.ObjectKey)
}
