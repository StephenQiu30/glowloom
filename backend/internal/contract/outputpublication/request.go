package outputpublication

import "github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"

func digestValid(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, c := range value {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// Request is the immutable private wire envelope, not an authorization policy.
type Request struct {
	Command writefence.Command      `json:"command"`
	Output  OperationOutputIdentity `json:"output"`
}

// Validate checks only bounded command/wire shape; each owner applies its own policy.
func (r Request) Validate() error {
	if _, err := r.Command.AuthorityDigest(); err != nil {
		return err
	}
	return r.Output.Validate()
}

// Matches binds a recovered final ACK to the exact original immutable payload.
func (r Request) Matches(receipt OperationOutputReceipt) error {
	if receipt.Validate() != nil || receipt.CommandID != r.Command.ID || receipt.RequestSHA256 != r.Command.SHA256 || receipt.PayloadSHA256 != r.Output.Digest() {
		return ErrOutputPublicationConflict
	}
	return nil
}
