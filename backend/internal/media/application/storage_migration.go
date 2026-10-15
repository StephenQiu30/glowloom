package application

import (
	"context"
	"errors"
	"time"

	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

// LegacyObjectCandidate freezes every currently unresolved owning row for one
// key. The source is an explicitly proved original resource, never the route.
type LegacyObjectCandidate struct {
	Retired        bool
	Ref            domain.ObjectRef
	SnapshotSHA256 string
	References     int
	ByteSize       *int64
	SHA256         *string
	MIMEType       *string
}

// LegacyMigrationRepository fences facts checked outside the SQL transaction.
type LegacyMigrationRepository interface {
	LegacyCandidates(context.Context) ([]LegacyObjectCandidate, error)
	BackfillLegacyObject(context.Context, LegacyObjectCandidate, ObjectInfo) (domain.ObjectLocation, error)
	BackfillMissingLegacyObject(context.Context, LegacyObjectCandidate) (domain.ObjectLocation, error)
}

// LegacyObjectVerifier reads actual bounded bytes from the proved original.
type LegacyObjectVerifier interface {
	VerifyLegacyObject(context.Context, LegacyObjectCandidate) (ObjectInfo, error)
}

// LegacyMigrationReport is progress, not blanket migration acceptance.
type LegacyMigrationReport struct{ Inspected, Bound, Remaining int }

// LegacyMigration performs resumable bounded work without holding SQL locks
// during provider calls. A failed proof leaves that reference unresolved.
type LegacyMigration struct {
	repo    LegacyMigrationRepository
	objects LegacyObjectVerifier
}

// NewLegacyMigration injects the inventory owner and actual byte verifier.
func NewLegacyMigration(repo LegacyMigrationRepository, objects LegacyObjectVerifier) *LegacyMigration {
	return &LegacyMigration{repo: repo, objects: objects}
}

// BackfillBatch stops on the first failed or changed proof; earlier receipts
// remain durable and an identical retry resumes without rewriting accepted rows.
func (s *LegacyMigration) BackfillBatch(ctx context.Context, limit int) (LegacyMigrationReport, error) {
	var result LegacyMigrationReport
	if s == nil || s.repo == nil || s.objects == nil || limit < 1 || limit > 1000 {
		return result, ErrUnavailable
	}
	candidates, err := s.repo.LegacyCandidates(ctx)
	if err != nil {
		return result, err
	}
	result.Remaining = len(candidates)
	for _, candidate := range candidates[:min(limit, len(candidates))] {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		verifyCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		info, err := s.objects.VerifyLegacyObject(verifyCtx, candidate)
		cancel()
		result.Inspected++
		if errors.Is(err, ErrObjectMissing) {
			if _, err := s.repo.BackfillMissingLegacyObject(ctx, candidate); err != nil {
				return result, err
			}
			result.Bound++
			result.Remaining--
			continue
		}
		if err != nil {
			return result, err
		}
		if _, err := s.repo.BackfillLegacyObject(ctx, candidate, info); err != nil {
			return result, err
		}
		result.Bound++
		result.Remaining--
	}
	return result, nil
}
