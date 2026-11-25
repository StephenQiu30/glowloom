package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"reflect"

	"github.com/google/uuid"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

// YjsRenameRuntime computes a trusted title delta without resetting shared history.
type YjsRenameRuntime interface {
	YjsRuntime
	Rename(context.Context, YjsState, uuid.UUID, string) (YjsCandidate, error)
}

// RenameAsset keeps title, binary state and the durable projection in one acceptance.
// The asset command owner holds its revision fence and enclosing receipt transaction.
func (s *YjsService) RenameAsset(ctx context.Context, actor identity.Principal, id, key uuid.UUID, expected Revision, name string) (YjsReceipt, error) {
	runtime, ok := s.runtime.(YjsRenameRuntime)
	if !ok {
		return YjsReceipt{}, fmt.Errorf("canvas rename runtime unavailable")
	}
	base, err := s.store.LoadYjs(ctx, actor, id)
	if err != nil {
		return YjsReceipt{}, err
	}
	if base.Document.Revision != expected {
		return YjsReceipt{}, &RevisionConflict{CurrentRevision: int64(base.Document.Revision)}
	}
	if len(base.State) == 0 {
		return YjsReceipt{}, ErrYjsNotEnabled
	}
	candidate, err := runtime.Rename(ctx, base, key, name)
	if err != nil {
		return YjsReceipt{}, err
	}
	if err := validateYjsCandidate(base, candidate); err != nil {
		return YjsReceipt{}, err
	}
	wanted := base.Document
	wanted.Name = name
	if !reflect.DeepEqual(wanted, candidate.Projection) || candidate.UpdateID != key {
		return YjsReceipt{}, fmt.Errorf("rename runtime changed graph or identity")
	}
	hash := sha256.Sum256(candidate.AppliedUpdate)
	update := YjsUpdate{Epoch: base.Epoch, SessionID: actor.SessionID, UpdateID: key, Update: candidate.AppliedUpdate, PayloadHash: hex.EncodeToString(hash[:])}
	if err := validYjsUpdate(update); err != nil {
		return YjsReceipt{}, err
	}
	candidate.PayloadHash = update.PayloadHash
	return s.store.CommitYjs(ctx, actor, base, update, &candidate, "")
}
