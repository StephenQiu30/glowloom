package project_test

import (
	"testing"
	"time"

	projectdomain "github.com/StephenQiu30/lanverse/backend/internal/production/project/domain"
)

func TestProjectEpisodeCollectionReferencesImmutableEpisodeVersions(t *testing.T) {
	t.Parallel()

	workspaceID := "11111111-1111-4111-8111-111111111111"
	projectID := "22222222-2222-4222-8222-222222222222"
	createdBy := "33333333-3333-4333-8333-333333333333"
	sourceVersionID := "44444444-4444-4444-8444-444444444444"
	now := time.Date(2026, time.September, 11, 10, 0, 0, 0, time.UTC)
	build := func(position int, episodeID, versionID, scriptVersionID, name string) projectdomain.EpisodeOwnerVersion {
		value, err := projectdomain.NewEpisodeOwnerVersion(projectdomain.EpisodeOwnerVersion{
			ID: versionID, WorkspaceID: workspaceID, ProjectID: projectID, EpisodeID: episodeID,
			Revision: 1, Status: "active", Position: position, SequenceKey: projectdomain.EpisodeSequenceKey(position),
			Name: name, TargetDurationMS: 90000, SourceVersionID: sourceVersionID,
			ScriptVersionID: scriptVersionID, SourceStart: (position - 1) * 20, SourceEnd: position * 20,
			ScriptContentHash: episodeOwnerHash(name), CreatedBy: createdBy, CreatedAt: now,
		})
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
	first := build(1,
		"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
		"aaaaaaa1-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
		"aaaaaaa2-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
		"第一集",
	)
	second := build(2,
		"bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb",
		"bbbbbbb1-bbbb-4bbb-8bbb-bbbbbbbbbbbb",
		"bbbbbbb2-bbbb-4bbb-8bbb-bbbbbbbbbbbb",
		"第二集",
	)

	collection, err := projectdomain.BuildProjectEpisodeCollection(workspaceID, projectID, 7, []projectdomain.EpisodeOwnerVersion{second, first})
	if err != nil {
		t.Fatal(err)
	}
	if collection.MemberCount != 2 || len(collection.Members) != 2 {
		t.Fatalf("Project Episode member count = %d", collection.MemberCount)
	}
	for _, member := range collection.Members {
		var version projectdomain.EpisodeOwnerVersion
		switch member.OwnerLogicalID {
		case first.EpisodeID:
			version = first
		case second.EpisodeID:
			version = second
		default:
			t.Fatalf("unknown Episode logical id: %s", member.OwnerLogicalID)
		}
		if member.OwnerVersionID != version.ID || member.OwnerRevision != version.Revision ||
			member.OwnerContentHash != version.ContentHash {
			t.Fatalf("mutable Episode identity was used as a version ref: %#v", member)
		}
	}
}

func TestProjectEpisodeVersionRequiresLinearParentIdentity(t *testing.T) {
	t.Parallel()

	value := projectdomain.EpisodeOwnerVersion{
		ID:          "aaaaaaa1-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
		WorkspaceID: "11111111-1111-4111-8111-111111111111",
		ProjectID:   "22222222-2222-4222-8222-222222222222",
		EpisodeID:   "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
		Revision:    2, Status: "active", Position: 1, SequenceKey: projectdomain.EpisodeSequenceKey(1),
		Name: "第一集", TargetDurationMS: 90000,
		SourceVersionID: "44444444-4444-4444-8444-444444444444",
		ScriptVersionID: "aaaaaaa2-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
		SourceStart:     0, SourceEnd: 20, ScriptContentHash: episodeOwnerHash("第一集"),
		CreatedBy: "33333333-3333-4333-8333-333333333333", CreatedAt: time.Date(2026, time.September, 11, 10, 0, 0, 0, time.UTC),
	}
	if _, err := projectdomain.NewEpisodeOwnerVersion(value); err == nil {
		t.Fatal("revision two without an exact parent was accepted")
	}
}

func episodeOwnerHash(value string) string {
	return sha256HexEpisodeLifecycle(value)
}
