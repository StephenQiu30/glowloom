package domain

import (
	"encoding/json"
	"errors"
	"reflect"

	"github.com/google/uuid"

	bibledomain "github.com/StephenQiu30/lanverse/backend/internal/production/bible/domain"
)

const confirmedVisualFoundationSourceSchema = "confirmed-visual-foundation-source-production"

// ConfirmedVisualFoundationSource binds the current Gate 2 Bible root to the
// exact aggregate Candidate that supplied its Design Gaps. It is a Backend
// read proof and is never sent to the Agent as a world fact.
type ConfirmedVisualFoundationSource struct {
	WorkspaceID, ProjectID                     string
	BibleVersion                               bibledomain.ProductionWorldBibleVersion
	BibleVersionID, BibleVersionContentHash    string
	BibleCollectionRootHash                    string
	CandidateRevisionID, CandidateRevisionHash string
	CandidateRevision                          int64
	CandidateContentHash, ContentHash          string
	Candidate                                  ProductionWorldCandidate
}

func NewConfirmedVisualFoundationSource(
	version bibledomain.ProductionWorldBibleVersion,
	bibleCollectionRootHash string,
	candidateRevisionID string,
	candidateRevision int64,
	candidateRevisionHash string,
	candidateContentHash string,
	candidate ProductionWorldCandidate,
) (ConfirmedVisualFoundationSource, error) {
	rebuiltVersion, err := bibledomain.NewProductionWorldBibleVersion(version)
	if err != nil || !reflect.DeepEqual(rebuiltVersion, version) {
		return ConfirmedVisualFoundationSource{}, errors.New("Production World Bible source has drifted")
	}
	collection, err := bibledomain.BuildProductionWorldBibleCollection(version)
	if err != nil || collection.CollectionRootHash != bibleCollectionRootHash {
		return ConfirmedVisualFoundationSource{}, errors.New("Production World Bible root has drifted")
	}
	if identifier, parseErr := uuid.Parse(candidateRevisionID); parseErr != nil || identifier == uuid.Nil ||
		candidateRevision < 1 || !productionWorldContentHashPattern.MatchString(candidateRevisionHash) ||
		!productionWorldContentHashPattern.MatchString(candidateContentHash) ||
		version.Candidate.VersionID != candidateRevisionID || version.Candidate.Revision != candidateRevision ||
		version.Candidate.ContentHash != candidateRevisionHash {
		return ConfirmedVisualFoundationSource{}, errors.New("Production World Candidate revision has drifted")
	}
	raw, err := json.Marshal(candidate)
	if err != nil {
		return ConfirmedVisualFoundationSource{}, err
	}
	decoded, _, err := DecodeProductionWorldCandidate(raw)
	if err != nil || decoded.WorkspaceID != version.WorkspaceID || decoded.ProjectID != version.ProjectID ||
		decoded.ContentHash != candidateContentHash || decoded.PartitionRoots.Bible != version.PartitionHash ||
		productionWorldBusinessKeyRoot(decoded, "bible") != version.BusinessKeyRoot {
		return ConfirmedVisualFoundationSource{}, errors.New("Production World Candidate content has drifted")
	}
	value := ConfirmedVisualFoundationSource{
		WorkspaceID: version.WorkspaceID, ProjectID: version.ProjectID,
		BibleVersion:   version,
		BibleVersionID: version.ID, BibleVersionContentHash: version.ContentHash,
		BibleCollectionRootHash: bibleCollectionRootHash,
		CandidateRevisionID:     candidateRevisionID, CandidateRevision: candidateRevision,
		CandidateRevisionHash: candidateRevisionHash, CandidateContentHash: candidateContentHash,
		Candidate: decoded,
	}
	value.ContentHash, err = confirmationHash(struct {
		Schema, WorkspaceID, ProjectID             string
		BibleVersionID, BibleVersionContentHash    string
		BibleCollectionRootHash                    string
		CandidateRevisionID, CandidateRevisionHash string
		CandidateRevision                          int64
		CandidateContentHash                       string
	}{
		confirmedVisualFoundationSourceSchema, value.WorkspaceID, value.ProjectID,
		value.BibleVersionID, value.BibleVersionContentHash, value.BibleCollectionRootHash,
		value.CandidateRevisionID, value.CandidateRevisionHash, value.CandidateRevision,
		value.CandidateContentHash,
	})
	if err != nil {
		return ConfirmedVisualFoundationSource{}, err
	}
	return value, nil
}

func (value ConfirmedVisualFoundationSource) Validate() error {
	if !productionWorldContentHashPattern.MatchString(value.ContentHash) {
		return errors.New("invalid confirmed Visual Foundation source")
	}
	rebuilt, err := NewConfirmedVisualFoundationSource(
		value.BibleVersion, value.BibleCollectionRootHash,
		value.CandidateRevisionID, value.CandidateRevision, value.CandidateRevisionHash,
		value.CandidateContentHash, value.Candidate,
	)
	if err != nil || !reflect.DeepEqual(rebuilt, value) {
		return errors.New("confirmed Visual Foundation source has drifted")
	}
	return nil
}

func productionWorldBusinessKeyRoot(candidate ProductionWorldCandidate, partition string) string {
	for _, root := range candidate.SharedProof.ExpectedBusinessKeyRoots {
		if root.Partition == partition {
			return root.Root
		}
	}
	return ""
}
