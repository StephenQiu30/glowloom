package postgres

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
	pgfence "github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/media/domain"
)

var _ application.ReferencePinReceiptReader = (*ReferencePinStore)(nil)

// ReadReferencePinReceipt authenticates original acquisition evidence using only
// Media's restricted pool. It does not register, resolve, bind or release a pin.
func (s *ReferencePinStore) ReadReferencePinReceipt(ctx context.Context, key application.ReferencePinReceiptKey) (domain.ReferencePin, error) {
	if err := s.ready(ctx); err != nil {
		return domain.ReferencePin{}, err
	}
	if err := key.Validate(); err != nil {
		return domain.ReferencePin{}, err
	}
	var pin domain.ReferencePin
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row referencePinRow
		err := tx.Where("pin_id = ?", key.PinID).Take(&row).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.ErrReferencePinNotFound
		}
		if err != nil {
			return err
		}
		i := row.ReferencePinIdentity
		if row.AcquireCommandID != key.AcquireCommandID || i.Consumer != key.Consumer ||
			i.ConsumerCommandID != key.ConsumerCommandID || i.ConsumerCommandSHA256 != key.ConsumerCommandSHA256 ||
			i.ConsumerResourceID != key.ConsumerResourceID || i.ScopeID != key.ScopeID || i.ProjectID != key.ProjectID {
			return domain.ErrReferencePinConflict
		}
		pin, err = loadReferencePin(tx, i, false)
		if err != nil {
			return err
		}
		original, err := pgfence.New(tx).Load(ctx, key.AcquireCommandID)
		if err != nil {
			return err
		}
		if original.State != writefence.Committed || original.Command.Action != "media.reference_pin.acquire" {
			return domain.ErrReferencePinProof
		}
		digest, err := referenceRequestDigest(i)
		if err != nil {
			return err
		}
		receipt, err := decodePinReceipt(original, digest)
		if err != nil {
			return err
		}
		if receipt.Pin == nil || receipt.Pin.Identity != i || receipt.Pin.AcquireCommandID != key.AcquireCommandID ||
			receipt.Pin.State != "held" || receipt.Pin.Kind == "" || receipt.Pin.SnapshotSHA256() != pin.SnapshotSHA256() {
			return domain.ErrReferencePinProof
		}
		return nil
	})
	if err != nil {
		return domain.ReferencePin{}, err
	}
	return pin, nil
}
