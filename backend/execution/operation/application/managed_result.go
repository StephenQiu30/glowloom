package application

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"time"
)

// ManagedArtifact identifies private bytes without exposing a signed URL.
type ManagedArtifact struct {
	Sequence   int    `json:"sequence"`
	Size       int64  `json:"size"`
	MIME       string `json:"mime"`
	SHA256     string `json:"sha256"`
	DurationMS int64  `json:"duration_ms,omitempty"`
}

// ManagedResultState distinguishes a recoverable accepted task from terminal output.
type ManagedResultState string

// Managed result states distinguish admitted output from unavailable or uncertain outcomes.
const (
	ManagedResultAccepted  ManagedResultState = "accepted"
	ManagedResultSucceeded ManagedResultState = "succeeded"
)

// ManagedResult is private durable evidence, never a workflow payload. It is
// written after every artifact; recovering it cannot issue another paid request.
type ManagedResult struct {
	// ReceivedAt is observed immediately after the provider call, before staging.
	// Missing timing on historical manifests remains unknown for time tariffs.
	ReceivedAt    *time.Time               `json:"received_at,omitempty"`
	Subtitle      *AudioSubtitle           `json:"subtitle,omitempty"`
	Identity      ProviderDispatchIdentity `json:"identity"`
	State         ManagedResultState       `json:"state"`
	TaskID        string                   `json:"task_id,omitempty"`
	Text          string                   `json:"text,omitempty"`
	Artifacts     []ManagedArtifact        `json:"artifacts,omitempty"`
	Usage         json.RawMessage          `json:"usage,omitempty"`
	SupplierUsage json.RawMessage          `json:"supplier_usage,omitempty"`
}

// Validate bounds the manifest and separates accepted tasks from terminal output.
func (r ManagedResult) Validate() error {
	if r.Identity.Validate() != nil || r.Identity.Attempt != 1 || len(r.TaskID) > 256 || len(r.Text) > 1<<20 || len(r.SupplierUsage) > 16384 || len(r.Artifacts) > 8 {
		return ErrInvalidProviderCall
	}
	if !r.Subtitle.Valid(240000) {
		return ErrInvalidProviderCall
	}
	if r.ReceivedAt != nil && (r.ReceivedAt.IsZero() || r.State != ManagedResultSucceeded) {
		return ErrInvalidProviderCall
	}
	if r.State == ManagedResultAccepted {
		if r.Subtitle != nil || r.TaskID == "" || r.Text != "" || len(r.Artifacts) > 0 || len(r.Usage) > 0 {
			return ErrInvalidProviderCall
		}
		return nil
	}
	if r.State != ManagedResultSucceeded || (r.Text == "") == (len(r.Artifacts) == 0) {
		return ErrInvalidProviderCall
	}
	if _, err := ParseBillableUsage(r.Usage); err != nil {
		return ErrInvalidProviderCall
	}
	for i, a := range r.Artifacts {
		if a.Sequence != i+1 || a.Size < 1 || a.Size > 512<<20 || !validReceiptSHA256(a.SHA256) {
			return ErrInvalidProviderCall
		}
		switch a.MIME {
		case "image/png", "image/jpeg", "image/webp":
			if a.Size > 32<<20 {
				return ErrInvalidProviderCall
			}
		case "audio/mpeg":
			if a.Size > 32<<20 {
				return ErrInvalidProviderCall
			}
		case "video/mp4":
		default:
			return ErrInvalidProviderCall
		}
	}
	return nil
}

// ManagedResultRepository persists exact evidence, checking the claimed dispatch.
type ManagedResultRepository interface {
	SaveManagedResult(context.Context, ManagedResult) error
	ReadManagedResult(context.Context, ProviderDispatchIdentity) (*ManagedResult, error)
}

// ManagedResults stages manifests independently of database acknowledgements.
type ManagedResults struct {
	owned   *ArtifactDelivery
	factory ProviderStagingFactory
	objects ProviderStagingObjects
	repo    ManagedResultRepository
}

// NewManagedResults injects private storage and durable call identities.
func NewManagedResults(objects ProviderStagingObjects, repo ManagedResultRepository) *ManagedResults {
	return &ManagedResults{objects: objects, repo: repo}
}

// NewOwnedManagedResults keeps manifests inside Execution and original bytes inside Media.
// The old routed constructor remains until its actual runtime has equivalent acceptance.
func NewOwnedManagedResults(repo ManagedResultRepository, artifacts *ArtifactDelivery) (*ManagedResults, error) {
	if repo == nil || artifacts == nil || artifacts.repo == nil || artifacts.media == nil {
		return nil, ErrInvalidProviderCall
	}
	return &ManagedResults{repo: repo, owned: artifacts}, nil
}

// UsesOwnedArtifacts reports the explicit opaque Media staging configuration.
func (s *ManagedResults) UsesOwnedArtifacts() bool { return s != nil && s.owned != nil }

// NewRoutedManagedResults scopes artifact and manifest I/O to the admitted operation.
func NewRoutedManagedResults(factory ProviderStagingFactory, repo ManagedResultRepository) *ManagedResults {
	return &ManagedResults{factory: factory, repo: repo}
}

func (s *ManagedResults) scoped(ctx context.Context, identity ProviderDispatchIdentity) (*ManagedResults, error) {
	if s == nil || s.factory == nil || identity.Validate() != nil {
		return nil, ErrInvalidProviderCall
	}
	objects, err := s.factory.ForProviderDispatch(ctx, identity)
	if err != nil {
		return nil, err
	}
	if objects == nil {
		return nil, ErrInvalidProviderCall
	}
	return &ManagedResults{objects: objects, repo: s.repo}, nil
}

func managedPrefix(i ProviderDispatchIdentity) string {
	return path.Join(ProviderStagePrefix(i), "managed-v1")
}
func managedArtifactKey(i ProviderDispatchIdentity, seq int) string {
	return path.Join(managedPrefix(i), fmt.Sprintf("%d.artifact", seq))
}

// StageArtifact creates immutable private bytes before the manifest is committed.
func (s *ManagedResults) StageArtifact(ctx context.Context, i ProviderDispatchIdentity, seq int, data []byte, mime string) (ManagedArtifact, error) {
	if s != nil && s.owned != nil {
		return s.owned.Stage(ctx, i, seq, data, mime)
	}
	if s != nil && s.factory != nil {
		scoped, err := s.scoped(ctx, i)
		if err != nil {
			return ManagedArtifact{}, err
		}
		return scoped.StageArtifact(ctx, i, seq, data, mime)
	}
	a := ManagedArtifact{Sequence: seq, Size: int64(len(data)), MIME: mime, SHA256: imageDigest(data)}
	if i.Validate() != nil || seq < 1 || seq > 8 || len(data) == 0 || len(data) > 512<<20 {
		return ManagedArtifact{}, ErrInvalidProviderCall
	}
	if err := s.writeArtifact(ctx, managedArtifactKey(i, seq), data, mime, a.SHA256); err != nil {
		return ManagedArtifact{}, err
	}
	return a, nil
}

// StageSource streams checked raw bytes only through Media-owned staging.
func (s *ManagedResults) StageSource(ctx context.Context, i ProviderDispatchIdentity, raw ManagedArtifact, source io.Reader) (ManagedArtifact, error) {
	if s == nil || s.owned == nil {
		return ManagedArtifact{}, ErrInvalidProviderCall
	}
	return s.owned.StageSource(ctx, i, raw, source)
}

// Save publishes a complete private manifest then records it in the database.
func (s *ManagedResults) Save(ctx context.Context, r ManagedResult) error {
	if s != nil && s.owned != nil {
		if r.Validate() != nil {
			return ErrInvalidProviderCall
		}
		for _, a := range r.Artifacts {
			if err := s.owned.Verify(ctx, r.Identity, a); err != nil {
				return err
			}
		}
		return s.repo.SaveManagedResult(ctx, r)
	}
	if s != nil && s.factory != nil {
		scoped, err := s.scoped(ctx, r.Identity)
		if err != nil {
			return err
		}
		return scoped.Save(ctx, r)
	}
	if r.Validate() != nil {
		return ErrInvalidProviderCall
	}
	raw, err := json.Marshal(r)
	if err != nil {
		return err
	}
	stage := ProviderStageService{objects: s.objects}
	if err := stage.write(ctx, path.Join(managedPrefix(r.Identity), string(r.State)+".json"), raw, "application/json", imageDigest(raw)); err != nil {
		return err
	}
	return s.repo.SaveManagedResult(ctx, r)
}

// Recover checks terminal evidence first; an absent manifest remains unknown.
func (s *ManagedResults) Recover(ctx context.Context, i ProviderDispatchIdentity) (*ManagedResult, error) {
	return s.recover(ctx, i, true)
}

// Verify reads the original staged manifest without rewriting terminal evidence.
func (s *ManagedResults) Verify(ctx context.Context, i ProviderDispatchIdentity) (*ManagedResult, error) {
	return s.recover(ctx, i, false)
}

func (s *ManagedResults) recover(ctx context.Context, i ProviderDispatchIdentity, persist bool) (*ManagedResult, error) {
	if s != nil && s.owned != nil {
		result, err := s.repo.ReadManagedResult(ctx, i)
		if err != nil {
			return nil, err
		}
		if result != nil {
			for _, a := range result.Artifacts {
				if err := s.owned.Verify(ctx, i, a); err != nil {
					return nil, err
				}
			}
			return result, nil
		}
		if persist {
			if err := s.owned.Recover(ctx, i); err != nil {
				return nil, err
			}
		}
		if source, ok := s.repo.(SupplierObservationReader); ok {
			observation, err := source.ReadSupplierObservation(ctx, i)
			if err != nil {
				return nil, err
			}
			// A saved original task can be queried read-only. Missing original
			// synchronous bytes cannot be reconstructed by another paid request.
			if observation != nil && observation.TaskID != "" && (observation.Outcome == ProviderOutcomeAccepted || observation.Outcome == ProviderOutcomeCompleted) {
				return &ManagedResult{Identity: i, State: ManagedResultAccepted, TaskID: observation.TaskID}, nil
			}
		}
		return nil, nil
	}
	if s != nil && s.factory != nil {
		scoped, err := s.scoped(ctx, i)
		if err != nil {
			return nil, err
		}
		return scoped.recover(ctx, i, persist)
	}
	if i.Validate() != nil {
		return nil, ErrInvalidProviderCall
	}
	for _, state := range []string{"succeeded", "accepted"} {
		key := path.Join(managedPrefix(i), state+".json")
		info, err := s.objects.Stat(ctx, key)
		if err != nil {
			if errors.Is(err, ErrProviderStagedObjectMissing) {
				continue
			}
			return nil, err
		}
		if info.Size < 1 || info.Size > 2<<20 || info.ContentType != "application/json" {
			return nil, ErrProviderCallConflict
		}
		body, err := s.objects.Open(ctx, key)
		if err != nil {
			return nil, err
		}
		raw, readErr := io.ReadAll(io.LimitReader(body, info.Size+1))
		closeErr := body.Close()
		if readErr != nil || closeErr != nil {
			return nil, ErrProviderCallConflict
		}
		if int64(len(raw)) != info.Size || imageDigest(raw) != info.SHA256 {
			return nil, ErrProviderCallConflict
		}
		var r ManagedResult
		if json.Unmarshal(raw, &r) != nil || r.Validate() != nil || r.Identity != i || string(r.State) != state {
			return nil, ErrProviderCallConflict
		}
		for _, a := range r.Artifacts {
			stored, e := s.objects.Stat(ctx, managedArtifactKey(i, a.Sequence))
			if e != nil {
				return nil, e
			}
			if stored.Size != a.Size || stored.ContentType != a.MIME || stored.SHA256 != a.SHA256 {
				return nil, ErrProviderCallConflict
			}
		}
		if persist {
			if err := s.repo.SaveManagedResult(ctx, r); err != nil {
				return nil, err
			}
		}
		return &r, nil
	}
	return nil, nil
}

// ReadArtifact verifies server-derived object metadata and bounded bytes.
func (s *ManagedResults) ReadArtifact(ctx context.Context, i ProviderDispatchIdentity, seq int) (ManagedArtifact, []byte, error) {
	if s != nil && s.owned != nil {
		r, err := s.repo.ReadManagedResult(ctx, i)
		if err != nil {
			return ManagedArtifact{}, nil, err
		}
		if r == nil || r.State != ManagedResultSucceeded || seq < 1 || seq > len(r.Artifacts) {
			return ManagedArtifact{}, nil, ErrProviderCallConflict
		}
		a := r.Artifacts[seq-1]
		body, err := s.owned.Open(ctx, i, a)
		if err != nil {
			return ManagedArtifact{}, nil, err
		}
		data, readErr := io.ReadAll(io.LimitReader(body, a.Size+1))
		closeErr := body.Close()
		if readErr != nil || closeErr != nil || int64(len(data)) != a.Size || imageDigest(data) != a.SHA256 {
			return ManagedArtifact{}, nil, ErrProviderCallConflict
		}
		return a, data, nil
	}
	if s != nil && s.factory != nil {
		scoped, err := s.scoped(ctx, i)
		if err != nil {
			return ManagedArtifact{}, nil, err
		}
		return scoped.ReadArtifact(ctx, i, seq)
	}
	r, err := s.repo.ReadManagedResult(ctx, i)
	if err != nil {
		return ManagedArtifact{}, nil, err
	}
	if r == nil || r.State != ManagedResultSucceeded || seq < 1 || seq > len(r.Artifacts) {
		return ManagedArtifact{}, nil, ErrProviderCallConflict
	}
	a := r.Artifacts[seq-1]
	key := managedArtifactKey(i, seq)
	info, err := s.objects.Stat(ctx, key)
	if err != nil {
		return ManagedArtifact{}, nil, err
	}
	if info.Size != a.Size || info.ContentType != a.MIME || info.SHA256 != a.SHA256 {
		return ManagedArtifact{}, nil, ErrProviderCallConflict
	}
	body, err := s.objects.Open(ctx, key)
	if err != nil {
		return ManagedArtifact{}, nil, err
	}
	data, readErr := io.ReadAll(io.LimitReader(body, a.Size+1))
	closeErr := body.Close()
	if readErr != nil || closeErr != nil || int64(len(data)) != a.Size || imageDigest(data) != a.SHA256 {
		return ManagedArtifact{}, nil, ErrProviderCallConflict
	}
	return a, data, nil
}

func (s *ManagedResults) writeArtifact(ctx context.Context, key string, data []byte, mime, digest string) error {
	err := s.objects.PutIfAbsent(ctx, key, bytes.NewReader(data), int64(len(data)), mime, digest)
	if err != nil && !errors.Is(err, ErrProviderStagedObjectExists) {
		return err
	}
	info, err := s.objects.Stat(ctx, key)
	if err != nil {
		return err
	}
	if info.Size != int64(len(data)) || info.ContentType != mime || info.SHA256 != digest {
		return ErrProviderCallConflict
	}
	body, err := s.objects.Open(ctx, key)
	if err != nil {
		return err
	}
	actual, readErr := io.ReadAll(io.LimitReader(body, int64(len(data))+1))
	closeErr := body.Close()
	if readErr != nil || closeErr != nil || !bytes.Equal(actual, data) {
		return ErrProviderCallConflict
	}
	return nil
}
