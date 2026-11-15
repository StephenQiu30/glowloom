package postgres

import (
	"bytes"
	"context"
	"errors"

	"gorm.io/gorm"

	contract "github.com/StephenQiu30/glowloom-server/backend/internal/contract/outputpublication"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
	pgfence "github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence/postgres"
)

// VerifyOutputPublication is Media's read-only minimum proof port. It requires a
// complete original persisted command and a prior durable transfer attempt. It
// neither creates an admission nor grants another paid request or deletion.
func (s *OutputDeliveryStore) VerifyOutputPublication(ctx context.Context, req contract.Request) error {
	if req.Validate() != nil {
		return contract.ErrInvalidOutputPublication
	}
	if err := s.ready(ctx); err != nil {
		return err
	}
	var row outputDeliveryRow
	err := s.db.WithContext(ctx).Where("operation_id=? AND seq_no=?", req.Output.OperationID, req.Output.Sequence).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return contract.ErrOutputPublicationUnknown
	}
	if err != nil {
		return err
	}
	work, err := decodeOutputDelivery(row)
	if err != nil {
		return err
	}
	original, err := writefence.CanonicalReceipt(outputDeliveryJSON(work.Request.Publication))
	if err != nil {
		return err
	}
	incoming, err := writefence.CanonicalReceipt(outputDeliveryJSON(req))
	if err != nil {
		return err
	}
	if !bytes.Equal(original, incoming) || !work.Attempted {
		return contract.ErrOutputPublicationConflict
	}
	// A final saved ACK is immutable recovery evidence; it cannot authorize a new
	// supplier request or output. For first publication, recheck the live original
	// worker lease and local admitted effect without any network under this lock.
	if work.Receipt != nil {
		return nil
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := pgfence.New(s.db).Guard(ctx, tx, work.Request.Command); err != nil {
			return err
		}
		if err := verifyOutputDeliveryEvidence(ctx, tx, work.Request); err != nil {
			return err
		}
		return fenceOriginalOutputAttempt(ctx, tx, work.Request)
	})
}
