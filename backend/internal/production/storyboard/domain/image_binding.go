package domain

import "time"

type ShotImageBindingVersion struct {
	ID, WorkspaceID, ProjectID, EpisodeID, ShotID       string
	ShotRevision                                        int
	ShotContentHash                                     string
	CandidateSelectionID, CandidateSelectionContentHash string
	CandidateSelectionRevision                          int
	CandidateID                                         string
	CandidateRevision                                   int
	ArtifactID, ArtifactSHA256                          string
	ArtifactRevision                                    int
	Revision                                            int
	ContentHash, CreatedBy                              string
	CreatedAt                                           time.Time
}

func SameShotImageBinding(left, right ShotImageBindingVersion) bool {
	return left.WorkspaceID == right.WorkspaceID && left.ProjectID == right.ProjectID &&
		left.EpisodeID == right.EpisodeID && left.ShotID == right.ShotID &&
		left.ShotRevision == right.ShotRevision && left.ShotContentHash == right.ShotContentHash &&
		left.CandidateSelectionID == right.CandidateSelectionID &&
		left.CandidateSelectionRevision == right.CandidateSelectionRevision &&
		left.CandidateSelectionContentHash == right.CandidateSelectionContentHash &&
		left.CandidateID == right.CandidateID && left.CandidateRevision == right.CandidateRevision &&
		left.ArtifactID == right.ArtifactID && left.ArtifactRevision == right.ArtifactRevision &&
		left.ArtifactSHA256 == right.ArtifactSHA256 && left.Revision == right.Revision &&
		left.ContentHash == right.ContentHash && left.CreatedBy == right.CreatedBy
}
