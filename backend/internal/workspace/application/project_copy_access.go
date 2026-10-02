package application

import "github.com/google/uuid"

// ProjectCopyBinding identifies one workspace-owned source and unpublished target.
type ProjectCopyBinding struct {
	JobID, OrgID, SourceProjectID, TargetProjectID uuid.UUID
}

// ProjectCopyAuthority is minted by admission or an actual claimed worker.
// It is an internal constructor input; public requests cannot supply it.
type ProjectCopyAuthority struct {
	Binding        ProjectCopyBinding
	ActorID        uuid.UUID
	WorkerID       uuid.UUID
	SourceRevision int64
	Phase          string
}
