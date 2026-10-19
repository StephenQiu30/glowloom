package application

import "github.com/google/uuid"

// ProjectNavigation is workspace-owned identity and presentation, not a graph revision.
type ProjectNavigation struct {
	ProjectID, MainCanvasID uuid.UUID
	Name, Status            string
	IsDelete                bool
}
