package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"

	contract "github.com/StephenQiu30/glowloom-server/backend/internal/contract/outputpublication"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

// OutputDeliveryRequest is Execution's local effect command and original Media intent.
// The separate command IDs cannot change writer or reuse another owner's receipt.
type OutputDeliveryRequest struct {
	Command     writefence.Command `json:"command"`
	Publication contract.Request   `json:"publication"`
}

// Digest binds the local adoption command to the complete original Media intent.
func (r OutputDeliveryRequest) Digest() string {
	ad, err := r.Command.AuthorityDigest()
	if err != nil {
		return ""
	}
	raw, err := json.Marshal(struct {
		ID          string           `json:"id"`
		Action      string           `json:"action"`
		Authorities string           `json:"authorities"`
		Publication contract.Request `json:"publication"`
	}{r.Command.ID.String(), r.Command.Action, ad, r.Publication})
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// ValidateOutputDelivery applies Execution's manifest/evidence/authority policy.
// Media independently owns physical format and moderation policy.
func ValidateOutputDelivery(r OutputDeliveryRequest) error {
	if r.Publication.Validate() != nil || r.Command.Action != "execution.operation_output.adopt" || r.Publication.Command.Action != "media.operation_output.publish" || r.Command.ID == r.Publication.Command.ID || r.Command.SHA256 != r.Digest() {
		return contract.ErrInvalidOutputPublication
	}
	ad, err := r.Command.AuthorityDigest()
	if err != nil {
		return err
	}
	pd, err := r.Publication.Command.AuthorityDigest()
	if err != nil || pd != ad {
		return contract.ErrOutputPublicationConflict
	}
	o := r.Publication.Output
	if len(o.Sequences) > 8 {
		return contract.ErrInvalidOutputPublication
	}
	var account, session, scope, project bool
	for _, a := range r.Command.Authorities {
		account = account || (a.Issuer == writefence.Identity && a.Kind == writefence.Account)
		session = session || (a.Issuer == writefence.Identity && a.Kind == writefence.Session)
		scope = scope || (a.Issuer == writefence.Creation && a.Kind == writefence.Scope && a.ID == o.ScopeID)
		project = project || (a.Issuer == writefence.Creation && a.Kind == writefence.Project && a.ID == o.ProjectID)
	}
	if !account || !session || !scope || !project {
		return contract.ErrInvalidOutputPublication
	}
	return nil
}

// OutputDeliveryWork is durable state returned before external transfer.
type OutputDeliveryWork struct {
	Request   OutputDeliveryRequest
	Attempted bool
	Receipt   *contract.OperationOutputReceipt
}

// OutputDeliveries must atomically guard/adopt output, settle original cost and
// reservation, close the operation and append its Outbox when the manifest is complete.
type OutputDeliveries interface {
	PrepareOutputDelivery(context.Context, OutputDeliveryRequest) (OutputDeliveryWork, error)
	StartOutputDelivery(context.Context, OutputDeliveryRequest) (bool, error)
	AdoptOutputDelivery(context.Context, OutputDeliveryRequest, contract.OperationOutputReceipt) error
}

// OutputMedia accepts bytes once and resolves only the original command after ambiguity.
type OutputMedia interface {
	Publish(context.Context, contract.Request, io.Reader) (contract.OperationOutputReceipt, error)
	Resolve(context.Context, contract.Request) (contract.OperationOutputReceipt, error)
}

// OutputDelivery coordinates owner boundaries without keeping any SQL transaction open.
type OutputDelivery struct {
	store OutputDeliveries
	media OutputMedia
}

// NewOutputDelivery requires explicit persistence and authenticated Media ports.
func NewOutputDelivery(store OutputDeliveries, media OutputMedia) (*OutputDelivery, error) {
	if store == nil || media == nil {
		return nil, ErrInvalidTransitionInput
	}
	return &OutputDelivery{store: store, media: media}, nil
}

// Deliver opens staged source bytes only for the first unambiguous transfer.
// Every ambiguous attempt, including after process restart, queries its original ACK.
func (d *OutputDelivery) Deliver(ctx context.Context, req OutputDeliveryRequest, open func(context.Context) (io.ReadCloser, error)) error {
	if d == nil || d.store == nil || d.media == nil || open == nil {
		return ErrInvalidTransitionInput
	}
	if err := ValidateOutputDelivery(req); err != nil {
		return err
	}
	work, err := d.store.PrepareOutputDelivery(ctx, req)
	if err != nil {
		return err
	}
	if work.Receipt != nil {
		return d.store.AdoptOutputDelivery(ctx, req, *work.Receipt)
	}
	if work.Attempted {
		receipt, err := d.media.Resolve(ctx, req.Publication)
		if err != nil {
			return err
		}
		return d.adopt(ctx, req, receipt)
	}
	source, err := open(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = source.Close() }()
	started, err := d.store.StartOutputDelivery(ctx, req)
	if err != nil {
		return err
	}
	if !started {
		receipt, err := d.media.Resolve(ctx, req.Publication)
		if err != nil {
			return err
		}
		return d.adopt(ctx, req, receipt)
	}
	receipt, publishErr := d.media.Publish(ctx, req.Publication, source)
	if publishErr != nil {
		// A closed error is not proof that Media did not archive or commit.
		receipt, err = d.media.Resolve(ctx, req.Publication)
		if err != nil {
			return errors.Join(publishErr, err)
		}
	}
	return d.adopt(ctx, req, receipt)
}
func (d *OutputDelivery) adopt(ctx context.Context, req OutputDeliveryRequest, receipt contract.OperationOutputReceipt) error {
	if err := req.Publication.Matches(receipt); err != nil {
		return err
	}
	// Execution refuses to publish a mixed archive/review outcome even if its
	// technical wire shape was valid. Supplier costs remain original in either case.
	if (receipt.State == "published" && (receipt.ModerationStatus != "passed" && receipt.ModerationStatus != "skipped")) || (receipt.State == "rejected" && receipt.ModerationStatus != "rejected") {
		return contract.ErrOutputPublicationConflict
	}
	return d.store.AdoptOutputDelivery(ctx, req, receipt)
}
