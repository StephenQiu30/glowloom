package application

import "github.com/StephenQiu30/glowloom-server/backend/media/domain"

// CompleteOutputRenditions reports whether renditions satisfy the asset kind's required outputs.
func CompleteOutputRenditions(kind domain.Kind, renditions []domain.Rendition) bool {
	return completeRenditions(kind, renditions)
}
