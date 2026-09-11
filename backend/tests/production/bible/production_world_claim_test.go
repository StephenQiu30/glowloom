package bible_test

import (
	"testing"
	"time"

	"github.com/StephenQiu30/lanverse/backend/internal/production/bible/domain"
)

func TestProductionWorldNarrativeClaimOwnsExactSemanticsAndSupersession(t *testing.T) {
	now := time.Date(2026, time.September, 11, 9, 0, 0, 0, time.UTC)
	participants := []domain.ProductionWorldClaimParticipant{
		{Role: "subject", IdentityKey: "character:linzhou", AssetID: "10000000-0000-4000-8000-000000000001", AssetContentHash: claimHash("1")},
		{Role: "object", IdentityKey: "location:home", AssetID: "10000000-0000-4000-8000-000000000002", AssetContentHash: claimHash("2")},
	}
	narrative := &domain.ProductionWorldNarrativeClaim{
		ClaimSeriesKey: "claim_linzhou_protects_home", Predicate: "protects",
		Anchors:    []domain.ProductionWorldClaimAnchor{{Role: "scene", TargetKey: "scene:20000000-0000-4000-8000-000000000001"}},
		ValidScope: domain.ProductionWorldClaimScope{Kind: "scene", OwnerLogicalID: "scene:20000000-0000-4000-8000-000000000001"},
		Polarity:   "positive", Status: "asserted",
	}
	evidence := domain.FragmentRef("30000000-0000-4000-8000-000000000001", "source_evidence", "claim_linzhou_protects_home", claimHash("3"), 1)
	first, err := domain.NewProductionWorldClaim(
		"40000000-0000-4000-8000-000000000001", "50000000-0000-4000-8000-000000000001", "60000000-0000-4000-8000-000000000001",
		"claim_linzhou_protects_home", "relationship", "林舟守护故乡。", 1, participants, narrative, evidence,
		"70000000-0000-4000-8000-000000000001", now,
	)
	if err != nil {
		t.Fatalf("create exact Production World Narrative Claim: %v", err)
	}
	predecessor := domain.FragmentRef(first.ID, first.ClaimType, first.ClaimKey, first.ContentHash, first.Revision)
	secondNarrative := *narrative
	secondNarrative.Predicate = "defends"
	secondNarrative.SupersedesClaim = &predecessor
	second, err := domain.NewProductionWorldClaim(
		"40000000-0000-4000-8000-000000000002", first.WorkspaceID, first.ProjectID,
		first.ClaimKey, first.ClaimType, "林舟开始主动守护故乡。", 2, participants, &secondNarrative, evidence,
		first.CreatedBy, now.Add(time.Minute),
	)
	if err != nil || second.ContentHash == first.ContentHash {
		t.Fatalf("create superseding Narrative Claim: %#v error=%v", second, err)
	}

	invalid := secondNarrative
	invalid.SupersedesClaim = nil
	if _, err = domain.NewProductionWorldClaim(
		"40000000-0000-4000-8000-000000000003", first.WorkspaceID, first.ProjectID,
		first.ClaimKey, first.ClaimType, second.Statement, 2, participants, &invalid, evidence,
		first.CreatedBy, now.Add(2*time.Minute),
	); err == nil {
		t.Fatal("revised Narrative Claim without exact predecessor was accepted")
	}

	duplicated := append([]domain.ProductionWorldClaimParticipant(nil), participants...)
	duplicated[1] = domain.ProductionWorldClaimParticipant{
		Role: "object", IdentityKey: duplicated[0].IdentityKey,
		AssetID: duplicated[0].AssetID, AssetContentHash: duplicated[0].AssetContentHash,
	}
	if _, err = domain.NewProductionWorldClaim(
		"40000000-0000-4000-8000-000000000004", first.WorkspaceID, first.ProjectID,
		first.ClaimKey, first.ClaimType, first.Statement, 1, duplicated, narrative, evidence,
		first.CreatedBy, now.Add(3*time.Minute),
	); err == nil {
		t.Fatal("Narrative Claim accepted one identity in multiple participant roles")
	}
}

func claimHash(character string) string {
	result := ""
	for len(result) < 64 {
		result += character
	}
	return result[:64]
}
