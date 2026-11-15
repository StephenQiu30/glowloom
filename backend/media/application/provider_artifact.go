package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path"
	"time"

	"github.com/google/uuid"

	contract "github.com/StephenQiu30/glowloom-server/backend/internal/contract/providerartifact"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
	"github.com/StephenQiu30/glowloom-server/backend/media/domain"
)

// ProviderArtifactWork contains Media-local frozen storage and original receipt.
type ProviderArtifactWork struct {
	Request           contract.Request
	Intent            StorageWorkIntent
	PreparedAt        time.Time
	Location          *domain.ObjectLocation
	CanonicalLocation *domain.ObjectLocation
	Canonical         *contract.Descriptor
	Receipt           *contract.Receipt
}

// ProviderArtifactRepository owns the immutable request, physical binding and ACK.
type ProviderArtifactRepository interface {
	PrepareProviderArtifact(context.Context, contract.Request) (ProviderArtifactWork, error)
	ReadProviderArtifact(context.Context, contract.Request) (ProviderArtifactWork, error)
	CommitProviderArtifact(context.Context, ProviderArtifactWork, domain.ObjectLocation) (contract.Receipt, error)
}

// ProviderArtifactSource is Execution's exact persisted source, never a provider URL.
type ProviderArtifactSource interface {
	VerifyArtifactSource(context.Context, contract.Request) error
	OpenArtifactSource(context.Context, contract.Request) (io.ReadCloser, error)
}

// ProviderArtifactReader opens only a Media-local immutable object location.
type ProviderArtifactReader interface {
	Get(context.Context, domain.ObjectRef) (io.ReadCloser, error)
}

// ProviderArtifactProcessing borrows raw bytes and returns actual canonical bytes.
type ProviderArtifactProcessing interface {
	ProcessProviderArtifact(context.Context, contract.Request, *Downloaded) (*Downloaded, int64, error)
}

// ProviderArtifacts archives original bytes without publishing an operation output.
type ProviderArtifacts struct {
	repo      ProviderArtifactRepository
	source    ProviderArtifactSource
	objects   RoutedIngestObjects
	reader    ProviderArtifactReader
	processor ProviderArtifactProcessing
}

// NewProviderArtifacts requires explicit owner-local storage and authenticated source.
func NewProviderArtifacts(repo ProviderArtifactRepository, source ProviderArtifactSource, objects RoutedIngestObjects, reader ProviderArtifactReader, processors ...ProviderArtifactProcessing) (*ProviderArtifacts, error) {
	if repo == nil || source == nil || objects == nil || reader == nil || len(processors) > 1 {
		return nil, ErrUnavailable
	}
	service := &ProviderArtifacts{repo: repo, source: source, objects: objects, reader: reader}
	if len(processors) == 1 {
		if processors[0] == nil {
			return nil, ErrUnavailable
		}
		service.processor = processors[0]
	}
	return service, nil
}

// ValidateProviderArtifact enforces Media's actual generated-byte limits.
func ValidateProviderArtifact(req contract.Request) error {
	if req.Validate() != nil || req.Sequence < 1 || req.Sequence > 8 {
		return contract.ErrInvalid
	}
	switch req.Processing {
	case "music_mp3":
		if req.ByteSize > 128<<20 {
			return contract.ErrInvalid
		}
		switch req.MIME {
		case "audio/mpeg", "audio/wave", "audio/x-wav", "video/mp4":
		default:
			return contract.ErrInvalid
		}
	case "video_probe":
		if req.MIME != "video/mp4" {
			return contract.ErrInvalid
		}
	case "", "original":
		switch req.MIME {
		case "image/png", "image/jpeg", "image/webp", "audio/mpeg":
			if req.ByteSize > 32<<20 {
				return contract.ErrInvalid
			}
		case "video/mp4":
		default:
			return contract.ErrInvalid
		}
	default:
		return contract.ErrInvalid
	}
	kinds := map[writefence.Kind]bool{}
	for _, a := range req.Command.Authorities {
		if a.Issuer == writefence.Identity && (a.Kind == writefence.Account || a.Kind == writefence.Session) || a.Issuer == writefence.Creation && (a.Kind == writefence.Scope || a.Kind == writefence.Project) {
			if kinds[a.Kind] || a.Kind == writefence.Project && a.ID != req.ProjectID {
				return contract.ErrInvalid
			}
			kinds[a.Kind] = true
		}
	}
	if !kinds[writefence.Account] || !kinds[writefence.Session] || !kinds[writefence.Scope] || !kinds[writefence.Project] {
		return contract.ErrInvalid
	}
	return nil
}

// ValidateProviderArtifactReceipt binds the actual canonical result to Media policy.
func ValidateProviderArtifactReceipt(req contract.Request, ack contract.Receipt) error {
	if !ack.Matches(req) {
		return contract.ErrConflict
	}
	d := ack.Canonical
	switch req.Processing {
	case "music_mp3":
		if d == nil || d.MIME != "audio/mpeg" || d.ByteSize > 32<<20 || d.DurationMS <= 0 || d.DurationMS > 241000 {
			return contract.ErrConflict
		}
	case "video_probe":
		if d == nil || d.ByteSize != req.ByteSize || d.SHA256 != req.SHA256 || d.MIME != req.MIME || d.DurationMS <= 0 {
			return contract.ErrConflict
		}
	case "", "original":
		if d != nil && (d.ByteSize != req.ByteSize || d.SHA256 != req.SHA256 || d.MIME != req.MIME) {
			return contract.ErrConflict
		}
	default:
		return contract.ErrConflict
	}
	return nil
}

// StageArtifact seals original responsibility before any object I/O.
func (s *ProviderArtifacts) StageArtifact(ctx context.Context, req contract.Request) (contract.Receipt, error) {
	if ValidateProviderArtifact(req) != nil {
		return contract.Receipt{}, contract.ErrInvalid
	}
	if prior, err := s.repo.ReadProviderArtifact(ctx, req); err == nil {
		if prior.Receipt != nil {
			return *prior.Receipt, nil
		}
	} else if !errors.Is(err, contract.ErrUnknown) {
		return contract.Receipt{}, err
	}
	if err := s.source.VerifyArtifactSource(ctx, req); err != nil {
		return contract.Receipt{}, err
	}
	work, err := s.repo.PrepareProviderArtifact(ctx, req)
	if err != nil {
		return contract.Receipt{}, err
	}
	if work.Receipt != nil {
		return *work.Receipt, nil
	}
	return s.archive(ctx, work)
}

// ResolveArtifact recovers only an original durable request. Missing is unknown.
func (s *ProviderArtifacts) ResolveArtifact(ctx context.Context, req contract.Request) (contract.Receipt, error) {
	if ValidateProviderArtifact(req) != nil {
		return contract.Receipt{}, contract.ErrInvalid
	}
	work, err := s.repo.ReadProviderArtifact(ctx, req)
	if err != nil {
		return contract.Receipt{}, err
	}
	if work.Receipt != nil {
		return *work.Receipt, nil
	}
	return s.archive(ctx, work)
}

func (s *ProviderArtifacts) archive(ctx context.Context, work ProviderArtifactWork) (contract.Receipt, error) {
	req := work.Request
	if work.Intent.ID == uuid.Nil || work.Intent.State != StorageWorkOpen || work.PreparedAt.IsZero() {
		return contract.Receipt{}, ErrStorageRouteConflict
	}
	source, err := s.source.OpenArtifactSource(ctx, req)
	if err != nil {
		return contract.Receipt{}, err
	}
	defer func() { _ = source.Close() }()
	file, err := os.CreateTemp("", "glowloom-provider-artifact-*")
	if err != nil {
		return contract.Receipt{}, err
	}
	defer func() { _ = os.Remove(file.Name()) }()
	defer func() { _ = file.Close() }()
	hash := sha256.New()
	size, err := io.Copy(io.MultiWriter(file, hash), io.LimitReader(source, req.ByteSize+1))
	if err != nil {
		return contract.Receipt{}, err
	}
	if err := source.Close(); err != nil {
		return contract.Receipt{}, err
	}
	if size != req.ByteSize || hex.EncodeToString(hash.Sum(nil)) != req.SHA256 {
		return contract.Receipt{}, contract.ErrConflict
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return contract.Receipt{}, err
	}
	key := path.Join("projects", req.ProjectID.String(), "provider-artifacts", work.PreparedAt.UTC().Format("2006/01"), req.DeliveryID.String()+".bin")
	location, err := s.objects.CreateOperationObject(ctx, work.Intent, key, &Downloaded{File: file, Size: size, MIMEType: req.MIME, SHA256: req.SHA256})
	if err != nil {
		return contract.Receipt{}, err
	}
	if !validIngestLocation(&work.Intent, &location, key, req.ByteSize, req.MIME, req.SHA256) {
		return contract.Receipt{}, ErrObjectMismatch
	}
	if req.Processing != "" && req.Processing != "original" {
		if s.processor == nil {
			return contract.Receipt{}, ErrUnavailable
		}
		input := &Downloaded{File: file, Size: size, MIMEType: req.MIME, SHA256: req.SHA256}
		canonical, duration, err := s.processor.ProcessProviderArtifact(ctx, req, input)
		if err != nil {
			return contract.Receipt{}, err
		}
		if canonical == nil || canonical.File == nil {
			return contract.Receipt{}, ErrObjectMismatch
		}
		if canonical != input {
			defer func() { _ = canonical.Close() }()
		}
		descriptor := &contract.Descriptor{ByteSize: canonical.Size, SHA256: canonical.SHA256, MIME: canonical.MIMEType, DurationMS: duration}
		ack := contract.Receipt{Request: req, ArtifactID: req.DeliveryID, State: "present", Canonical: descriptor}
		if err := ValidateProviderArtifactReceipt(req, ack); err != nil {
			return contract.Receipt{}, err
		}
		canonicalLocation := location
		if canonical != input {
			if _, err := canonical.File.Seek(0, io.SeekStart); err != nil {
				return contract.Receipt{}, err
			}
			canonicalKey := path.Join(path.Dir(key), req.DeliveryID.String()+".canonical.mp3")
			canonicalLocation, err = s.objects.CreateOperationObject(ctx, work.Intent, canonicalKey, canonical)
			if err != nil {
				return contract.Receipt{}, err
			}
			if !validIngestLocation(&work.Intent, &canonicalLocation, canonicalKey, descriptor.ByteSize, descriptor.MIME, descriptor.SHA256) {
				return contract.Receipt{}, ErrObjectMismatch
			}
		}
		work.Canonical, work.CanonicalLocation = descriptor, &canonicalLocation
	}
	return s.repo.CommitProviderArtifact(ctx, work, location)
}

// OpenArtifact returns only verified original Media-owned bytes to Execution.
func (s *ProviderArtifacts) OpenArtifact(ctx context.Context, req contract.Request) (io.ReadCloser, error) {
	if ValidateProviderArtifact(req) != nil {
		return nil, contract.ErrInvalid
	}
	work, err := s.repo.ReadProviderArtifact(ctx, req)
	if err != nil {
		return nil, err
	}
	if work.Receipt == nil || work.Location == nil || !work.Receipt.Matches(req) {
		return nil, contract.ErrUnknown
	}
	if err := ValidateProviderArtifactReceipt(req, *work.Receipt); err != nil {
		return nil, err
	}
	location := work.Location
	descriptor := contract.Descriptor{ByteSize: req.ByteSize, SHA256: req.SHA256, MIME: req.MIME}
	if work.Receipt.Canonical != nil {
		if work.CanonicalLocation == nil {
			return nil, contract.ErrConflict
		}
		location = work.CanonicalLocation
		descriptor = *work.Receipt.Canonical
	}
	if location.State != domain.ObjectLocationAccepted || location.ByteSize == nil || *location.ByteSize != descriptor.ByteSize || location.SHA256 == nil || *location.SHA256 != descriptor.SHA256 || location.MIMEType == nil || *location.MIMEType != descriptor.MIME {
		return nil, ErrObjectMismatch
	}
	return s.reader.Get(ctx, location.Ref)
}
