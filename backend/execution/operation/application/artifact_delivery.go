package application

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"

	contract "github.com/StephenQiu30/glowloom-server/backend/internal/contract/providerartifact"
)

// ArtifactDeliveryRecord retains the original request and transfer certainty.
type ArtifactDeliveryRecord struct {
	Request   contract.Request
	State     string
	Attempted bool
	Receipt   *contract.Receipt
}

// ArtifactDeliveryRepository owns immutable source chunks and ACK adoption.
type ArtifactDeliveryRepository interface {
	PrepareArtifact(context.Context, ProviderDispatchIdentity, ManagedArtifact) (ArtifactDeliveryRecord, error)
	PutArtifactChunk(context.Context, contract.Request, int, []byte) error
	SealArtifact(context.Context, contract.Request) error
	StartArtifact(context.Context, contract.Request) (bool, error)
	AdoptArtifact(context.Context, contract.Request, contract.Receipt) error
	FindArtifact(context.Context, ProviderDispatchIdentity, int) (ArtifactDeliveryRecord, error)
	ArtifactDeliveries(context.Context, ProviderDispatchIdentity) ([]ArtifactDeliveryRecord, error)
}

// ArtifactMedia accepts or resolves only one original byte responsibility.
type ArtifactMedia interface {
	StageArtifact(context.Context, contract.Request) (contract.Receipt, error)
	ResolveArtifact(context.Context, contract.Request) (contract.Receipt, error)
	OpenArtifact(context.Context, contract.Request) (io.ReadCloser, error)
}

// ArtifactDelivery persists complete bounded chunks before any Media call.
type ArtifactDelivery struct {
	repo  ArtifactDeliveryRepository
	media ArtifactMedia
}

// NewArtifactDelivery injects the two owners without physical storage identities.
func NewArtifactDelivery(repo ArtifactDeliveryRepository, media ArtifactMedia) *ArtifactDelivery {
	return &ArtifactDelivery{repo: repo, media: media}
}

func artifactReceipt(req contract.Request, ack contract.Receipt) (ManagedArtifact, error) {
	var out ManagedArtifact
	if !ack.Matches(req) {
		return out, contract.ErrConflict
	}
	d := contract.Descriptor{ByteSize: req.ByteSize, SHA256: req.SHA256, MIME: req.MIME}
	if ack.Canonical != nil {
		d = *ack.Canonical
	}
	switch req.Processing {
	case "music_mp3":
		if ack.Canonical == nil || d.MIME != "audio/mpeg" || d.ByteSize > 32<<20 || d.DurationMS <= 0 || d.DurationMS > 241000 {
			return out, contract.ErrConflict
		}
	case "video_probe":
		if ack.Canonical == nil || d.ByteSize != req.ByteSize || d.SHA256 != req.SHA256 || d.MIME != req.MIME || d.DurationMS <= 0 {
			return out, contract.ErrConflict
		}
	case "", "original":
		if d.ByteSize != req.ByteSize || d.SHA256 != req.SHA256 || d.MIME != req.MIME {
			return out, contract.ErrConflict
		}
	default:
		return out, contract.ErrConflict
	}
	return ManagedArtifact{Sequence: req.Sequence, Size: d.ByteSize, SHA256: d.SHA256, MIME: d.MIME, DurationMS: d.DurationMS}, nil
}

func (s *ArtifactDelivery) resolve(ctx context.Context, record ArtifactDeliveryRecord) (ManagedArtifact, error) {
	req := record.Request
	if record.Receipt != nil {
		return artifactReceipt(req, *record.Receipt)
	}
	if record.State != "prepared" {
		return ManagedArtifact{}, contract.ErrUnknown
	}
	var ack contract.Receipt
	var err error
	if record.Attempted {
		ack, err = s.media.ResolveArtifact(ctx, req)
	} else {
		var started bool
		started, err = s.repo.StartArtifact(ctx, req)
		if err == nil {
			if started {
				ack, err = s.media.StageArtifact(ctx, req)
			} else {
				ack, err = s.media.ResolveArtifact(ctx, req)
			}
		}
	}
	if err != nil {
		return ManagedArtifact{}, err
	}
	artifact, err := artifactReceipt(req, ack)
	if err != nil {
		return ManagedArtifact{}, err
	}
	if err := s.repo.AdoptArtifact(ctx, req, ack); err != nil {
		return ManagedArtifact{}, err
	}
	return artifact, nil
}

// Recover resumes complete original sources without the supplier response bytes.
// Incomplete collection remains unknown and never permits another paid submit.
func (s *ArtifactDelivery) Recover(ctx context.Context, i ProviderDispatchIdentity) error {
	records, err := s.repo.ArtifactDeliveries(ctx, i)
	if err != nil {
		return err
	}
	incomplete := false
	for _, record := range records {
		if record.State == "collecting" {
			incomplete = true
			continue
		}
		if _, err := s.resolve(ctx, record); err != nil {
			return err
		}
	}
	if incomplete {
		return contract.ErrUnknown
	}
	return nil
}

// Verify compares canonical facts with the exact permanent Media responsibility.
func (s *ArtifactDelivery) Verify(ctx context.Context, i ProviderDispatchIdentity, a ManagedArtifact) error {
	record, err := s.repo.FindArtifact(ctx, i, a.Sequence)
	if err != nil {
		return err
	}
	actual, err := s.resolve(ctx, record)
	if err != nil {
		return err
	}
	if actual != a {
		return contract.ErrConflict
	}
	return nil
}

// Open streams exact Media-owned canonical bytes after original receipt recovery.
func (s *ArtifactDelivery) Open(ctx context.Context, i ProviderDispatchIdentity, a ManagedArtifact) (io.ReadCloser, error) {
	if err := s.Verify(ctx, i, a); err != nil {
		return nil, err
	}
	record, err := s.repo.FindArtifact(ctx, i, a.Sequence)
	if err != nil {
		return nil, err
	}
	return s.media.OpenArtifact(ctx, record.Request)
}

// OpenManagedArtifact supplies the existing output-publication worker's opaque port.
func (s *ArtifactDelivery) OpenManagedArtifact(ctx context.Context, i ProviderDispatchIdentity, a ManagedArtifact) (io.ReadCloser, error) {
	return s.Open(ctx, i, a)
}

// Stage persists raw original bytes before Media processing and adopts canonical facts.
func (s *ArtifactDelivery) Stage(ctx context.Context, i ProviderDispatchIdentity, sequence int, data []byte, mime string) (ManagedArtifact, error) {
	if s == nil || s.repo == nil || s.media == nil || i.Validate() != nil || sequence < 1 || sequence > 8 || len(data) < 1 || len(data) > 512<<20 {
		return ManagedArtifact{}, ErrInvalidProviderCall
	}
	sum := sha256.Sum256(data)
	raw := ManagedArtifact{Sequence: sequence, Size: int64(len(data)), MIME: mime, SHA256: hex.EncodeToString(sum[:])}
	return s.StageSource(ctx, i, raw, bytes.NewReader(data))
}

// StageSource persists a raw file in bounded immutable chunks without materializing it.
func (s *ArtifactDelivery) StageSource(ctx context.Context, i ProviderDispatchIdentity, raw ManagedArtifact, source io.Reader) (ManagedArtifact, error) {
	if s == nil || s.repo == nil || s.media == nil || source == nil || i.Validate() != nil || raw.Sequence < 1 || raw.Sequence > 8 || raw.Size < 1 || raw.Size > 512<<20 {
		return ManagedArtifact{}, ErrInvalidProviderCall
	}
	record, err := s.repo.PrepareArtifact(ctx, i, raw)
	if err != nil {
		return ManagedArtifact{}, err
	}
	if record.State == "collecting" {
		for total, seq := int64(0), 0; total < raw.Size; seq++ {
			size := min(int64(contract.ChunkSize), raw.Size-total)
			chunk := make([]byte, int(size))
			if _, err := io.ReadFull(source, chunk); err != nil {
				return ManagedArtifact{}, err
			}
			if err := s.repo.PutArtifactChunk(ctx, record.Request, seq, chunk); err != nil {
				return ManagedArtifact{}, err
			}
			total += size
		}
		var extra [1]byte
		if n, err := source.Read(extra[:]); n != 0 || err != io.EOF {
			return ManagedArtifact{}, contract.ErrConflict
		}
		if err := s.repo.SealArtifact(ctx, record.Request); err != nil {
			return ManagedArtifact{}, err
		}
		record.State = "prepared"
	}
	return s.resolve(ctx, record)
}
