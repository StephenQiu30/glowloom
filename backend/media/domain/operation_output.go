package domain

import "github.com/StephenQiu30/glowloom-server/backend/internal/contract/outputpublication"

type (
	// OutputProviderEvidence carries immutable supplier facts, not Media policy.
	OutputProviderEvidence = outputpublication.OutputProviderEvidence
	// OutputWorkerEvidence retains the independent original worker lease evidence.
	OutputWorkerEvidence = outputpublication.OutputWorkerEvidence
	// OperationOutputIdentity is the immutable private wire identity consumed by Media.
	OperationOutputIdentity = outputpublication.OperationOutputIdentity
	// OperationOutputReceipt is Media's durable original archive/review acknowledgement.
	OperationOutputReceipt = outputpublication.OperationOutputReceipt
)

// Publication errors preserve unresolved physical responsibility separately from rejection.
var (
	ErrInvalidOutputPublication  = outputpublication.ErrInvalidOutputPublication
	ErrOutputPublicationConflict = outputpublication.ErrOutputPublicationConflict
	ErrOutputPublicationUnknown  = outputpublication.ErrOutputPublicationUnknown
	ErrOutputPublicationOwner    = outputpublication.ErrOutputPublicationOwner
)

// ValidateOperationOutput is Media's format and archive policy. The private wire
// package deliberately has no authority over accepted media formats or limits.
func ValidateOperationOutput(o OperationOutputIdentity) error {
	if o.Validate() != nil || len(o.Sequences) > 8 || (o.Region != "domestic" && o.Region != "overseas") || (o.AspectRatio != "9:16" && o.AspectRatio != "16:9" && o.AspectRatio != "1:1") {
		return ErrInvalidOutputPublication
	}
	switch o.Kind {
	case "image":
		if o.ByteSize > 32<<20 || (o.MIMEType != "image/png" && o.MIMEType != "image/jpeg" && o.MIMEType != "image/webp") {
			return ErrInvalidOutputPublication
		}
	case "audio":
		if o.ByteSize > 32<<20 || o.MIMEType != "audio/mpeg" {
			return ErrInvalidOutputPublication
		}
	case "video":
		if o.ByteSize > 512<<20 || o.MIMEType != "video/mp4" {
			return ErrInvalidOutputPublication
		}
	default:
		return ErrInvalidOutputPublication
	}
	return nil
}

// ValidateOperationOutputReceipt enforces Media's actual archive/review outcome.
func ValidateOperationOutputReceipt(r OperationOutputReceipt) error {
	if r.Validate() != nil || ValidateOperationOutput(r.Output) != nil {
		return ErrOutputPublicationConflict
	}
	if (r.State == "published" && (r.ModerationStatus == "passed" || r.ModerationStatus == "skipped")) || (r.State == "rejected" && r.ModerationStatus == "rejected") {
		return nil
	}
	return ErrOutputPublicationConflict
}
