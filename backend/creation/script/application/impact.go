package application

import "github.com/google/uuid"

// AffectedEpisode reports only actual owning script facts requiring impact review.
// It never substitutes a guessed downstream shot, link or audio count.
type AffectedEpisode struct {
	EpisodeID            uuid.UUID `json:"episode_id"`
	ConfirmedStructureID uuid.UUID `json:"confirmed_structure_id"`
	Reason               string    `json:"reason"`
}

// ImpactError retains the known affected identities while missing owner evidence blocks mutation.
type ImpactError struct {
	Affected []AffectedEpisode
	NeedsAck bool
}

func (e *ImpactError) Error() string {
	if e.NeedsAck {
		return ErrConfirmationRequired.Error()
	}
	return ErrContextUnavailable.Error()
}
func (e *ImpactError) Unwrap() error {
	if e.NeedsAck {
		return ErrConfirmationRequired
	}
	return ErrContextUnavailable
}
