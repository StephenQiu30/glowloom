package workflow_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	bibledomain "github.com/StephenQiu30/lanverse/backend/internal/production/bible/domain"
	worldapp "github.com/StephenQiu30/lanverse/backend/internal/production/world/application"
	worlddomain "github.com/StephenQiu30/lanverse/backend/internal/production/world/domain"
)

func TestConfirmedVisualFoundationSourceBindsGate2CandidateAndBibleRoot(t *testing.T) {
	candidate, _, err := worlddomain.NewProductionWorldCandidate(productionWorldCandidateDraft(t))
	if err != nil {
		t.Fatal(err)
	}
	revisionHash := strings.Repeat("a", 64)
	version := visualFoundationBibleVersion(t, candidate, revisionHash)
	collection, err := bibledomain.BuildProductionWorldBibleCollection(version)
	if err != nil {
		t.Fatal(err)
	}
	source, err := worlddomain.NewConfirmedVisualFoundationSource(
		version,
		collection.CollectionRootHash,
		version.Candidate.VersionID,
		version.Candidate.Revision,
		revisionHash,
		candidate.ContentHash,
		candidate,
	)
	if err != nil {
		t.Fatal(err)
	}
	if source.WorkspaceID != candidate.WorkspaceID || source.ProjectID != candidate.ProjectID ||
		source.BibleVersionID != version.ID || source.BibleCollectionRootHash != collection.CollectionRootHash ||
		source.CandidateRevisionID != version.Candidate.VersionID || source.CandidateRevisionHash != revisionHash ||
		source.CandidateContentHash != candidate.ContentHash || source.ContentHash == "" ||
		len(source.Candidate.SharedProof.DesignGaps) != len(candidate.SharedProof.DesignGaps) {
		t.Fatalf("confirmed Visual Foundation source is incomplete: %#v", source)
	}

	repository := &visualFoundationSourceRepositoryStub{source: source}
	service, err := worldapp.NewVisualFoundationSourceService(repository)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := service.Current(context.Background(), candidate.WorkspaceID, candidate.ProjectID)
	if err != nil || loaded.ContentHash != source.ContentHash || repository.calls != 1 {
		t.Fatalf("load current Visual Foundation source: source=%#v calls=%d err=%v", loaded, repository.calls, err)
	}
}

func TestConfirmedVisualFoundationSourceRejectsDriftedCandidatePreimage(t *testing.T) {
	candidate, _, err := worlddomain.NewProductionWorldCandidate(productionWorldCandidateDraft(t))
	if err != nil {
		t.Fatal(err)
	}
	revisionHash := strings.Repeat("b", 64)
	version := visualFoundationBibleVersion(t, candidate, revisionHash)
	collection, err := bibledomain.BuildProductionWorldBibleCollection(version)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name, collectionRoot, revisionID, revisionHash, candidateHash string
	}{
		{name: "collection root", collectionRoot: strings.Repeat("c", 64), revisionID: version.Candidate.VersionID, revisionHash: revisionHash, candidateHash: candidate.ContentHash},
		{name: "candidate revision", collectionRoot: collection.CollectionRootHash, revisionID: uuid.NewString(), revisionHash: revisionHash, candidateHash: candidate.ContentHash},
		{name: "candidate revision hash", collectionRoot: collection.CollectionRootHash, revisionID: version.Candidate.VersionID, revisionHash: strings.Repeat("d", 64), candidateHash: candidate.ContentHash},
		{name: "candidate content", collectionRoot: collection.CollectionRootHash, revisionID: version.Candidate.VersionID, revisionHash: revisionHash, candidateHash: strings.Repeat("e", 64)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := worlddomain.NewConfirmedVisualFoundationSource(
				version, test.collectionRoot, test.revisionID, version.Candidate.Revision,
				test.revisionHash, test.candidateHash, candidate,
			); err == nil {
				t.Fatal("accepted a drifted Gate 2 visual source")
			}
		})
	}
}

type visualFoundationSourceRepositoryStub struct {
	source worlddomain.ConfirmedVisualFoundationSource
	calls  int
}

func (repository *visualFoundationSourceRepositoryStub) GetCurrentVisualFoundationSource(
	context.Context,
	string,
	string,
) (worlddomain.ConfirmedVisualFoundationSource, error) {
	repository.calls++
	return repository.source, nil
}

func visualFoundationBibleVersion(
	t *testing.T,
	candidate worlddomain.ProductionWorldCandidate,
	revisionHash string,
) bibledomain.ProductionWorldBibleVersion {
	t.Helper()
	businessKeyRoot := ""
	for _, root := range candidate.SharedProof.ExpectedBusinessKeyRoots {
		if root.Partition == "bible" {
			businessKeyRoot = root.Root
			break
		}
	}
	version, err := bibledomain.NewProductionWorldBibleVersion(bibledomain.ProductionWorldBibleVersion{
		ID: uuid.NewString(), WorkspaceID: candidate.WorkspaceID, ProjectID: candidate.ProjectID, Revision: 1,
		StructureIdentitySet: bibledomain.ProductionWorldOwnerRef{
			OwnerKind:   candidate.StructureIdentitySetVersion.OwnerKind,
			LogicalID:   candidate.StructureIdentitySetVersion.LogicalID,
			VersionID:   candidate.StructureIdentitySetVersion.VersionID,
			Revision:    candidate.StructureIdentitySetVersion.Revision,
			ContentHash: candidate.StructureIdentitySetVersion.ContentHash,
		},
		Candidate: bibledomain.ProductionWorldOwnerRef{
			OwnerKind: "agent", LogicalID: candidate.ProjectID, VersionID: uuid.NewString(), Revision: 1,
			ContentHash: revisionHash,
		},
		ReviewDecisionID: uuid.NewString(), PartitionHash: candidate.PartitionRoots.Bible,
		BusinessKeyRoot: businessKeyRoot,
		Evidence:        []bibledomain.ProductionWorldFragmentRef{},
		Specifications: []bibledomain.ProductionWorldFragmentRef{{
			ID: uuid.NewString(), Kind: "specification", BusinessKey: "specification_visual_source",
			Revision: 1, ContentHash: strings.Repeat("1", 64),
		}},
		Claims: []bibledomain.ProductionWorldFragmentRef{},
		Bindings: []bibledomain.ProductionWorldFragmentRef{{
			ID: uuid.NewString(), Kind: "production_binding", BusinessKey: "character:visual_source",
			Revision: 1, ContentHash: strings.Repeat("2", 64),
		}},
		CreatedBy: uuid.NewString(), CreatedAt: time.Date(2026, time.September, 12, 1, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	return version
}
