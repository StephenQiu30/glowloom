package application

import (
	"errors"

	"github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
)

var ErrAcceptedReferenceBriefUnavailable = errors.New("accepted Reference Brief is unavailable or has drifted")

// AcceptedReferenceBrief is an exact Owner read, not an authorization to generate.
// Consumers publishing facts must keep this read inside their write transaction.
type AcceptedReferenceBrief struct {
	RevisionID   string
	Revision     int64
	RevisionHash string
	ContentHash  string
	Input        contract.ReferenceBriefInput
	Candidate    contract.ReferenceBriefCandidate
}
