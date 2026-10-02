package script_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"

	app "github.com/StephenQiu30/lanverse/backend/internal/script/application"
	"github.com/StephenQiu30/lanverse/backend/internal/script/domain"
)

func TestScriptCopyRemapsFullIdentityClosureAndPreservesSemanticHashes(t *testing.T) {
	org, pid, target, job := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	source, version, split, episode, structure, scene, line, action := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	sourceFact := domain.ObjectFact{Key: "projects/" + pid.String() + "/script/" + source.String() + "/original", SHA256: domain.ContentSHA([]byte("x")), ByteSize: 1, MIME: "application/json"}
	doc := manualStructure()
	doc.Scenes[0].Key = scene
	doc.Scenes[0].Items[0].Key = action
	doc.Scenes[0].Items[1].Key = line
	history := app.ProjectCopyHistory{State: &domain.ProjectState{ProjectID: pid, OrgID: org, Revision: 4, DraftVersionID: &version, AdoptedVersionID: &version}, Sources: []domain.SourceRecord{{ID: source, OrgID: org, ProjectID: pid, LineageID: source, Revision: 1, Origin: "manual", Kind: "chapter", Title: "旧章", Status: "draft", Original: sourceFact, Rich: sourceFact, ContentHash: sourceFact.SHA256, CharCount: 1, CreatedAt: time.Now().UTC()}}, Versions: []domain.ScriptVersion{{ID: version, OrgID: org, ProjectID: pid, VersionNo: 1, SourceIDs: []uuid.UUID{source}, Text: sourceFact, Rich: sourceFact, ContentHash: sourceFact.SHA256, DocumentSHA256: sourceFact.SHA256, SourceManifestSHA256: sourceFact.SHA256, CharCount: 1}}, VersionHeads: []app.VersionHead{{VersionID: version, CandidateSetID: split, ConfirmedSetID: &split}}, SplitSets: []domain.SplitSet{{ID: split, OrgID: org, ProjectID: pid, VersionID: version, Kind: "formal", Origin: "manual"}}, Episodes: []domain.Episode{{ID: episode, OrgID: org, ProjectID: pid, VersionID: version, SplitSetID: split, Revision: 1, SeqNo: 1, Start: 0, End: 9, CurrentStructureID: &structure, ConfirmedStructureID: &structure, IsDelete: true}}, Structures: []domain.EpisodeStructure{{ID: structure, OrgID: org, ProjectID: pid, EpisodeID: episode, VersionNo: 1, SourceHash: sourceFact.SHA256, Document: doc}}, Scenes: []app.ProjectCopyScene{{ID: scene, OrgID: org, ProjectID: pid, StructureID: structure, SceneKey: scene, SeqNo: 1, Heading: doc.Scenes[0].Heading, LocationText: doc.Scenes[0].LocationText, TimeOfDay: doc.Scenes[0].TimeOfDay, Start: 0, End: 9}}, Dialogue: []app.ProjectCopyDialogue{{ID: line, OrgID: org, ProjectID: pid, SceneID: scene, LineKey: line, SeqNo: 2, Kind: "dialogue", Speaker: doc.Scenes[0].Items[1].Speaker, Emotion: doc.Scenes[0].Items[1].Emotion, Content: "你好😀", ContentHash: domain.ContentSHA([]byte("你好😀")), Start: 5, End: 8}}, Actions: []app.ProjectCopyAction{{ID: action, OrgID: org, ProjectID: pid, SceneID: scene, LineKey: action, SeqNo: 1, Content: "走进客厅", Start: 0, End: 4}}}
	encoded, err := json.Marshal(history.Sources)
	if err != nil {
		t.Fatal(err)
	}
	history.Versions[0].SourceManifestSHA256 = domain.ContentSHA(encoded)
	history.VersionSources = []app.ProjectCopyVersionSource{{OrgID: org, ProjectID: pid, VersionID: version, SourceID: source, Position: 0}}
	binding := app.ProjectCopyBinding{JobID: job, OrgID: org, SourceProjectID: pid, TargetProjectID: target}
	manifest, err := app.RemapProjectHistory(binding, history, nil)
	if err != nil {
		t.Fatal(err)
	}
	copied := manifest.Target
	if copied.Sources[0].ID == source || copied.Sources[0].ID != copied.Sources[0].LineageID || copied.Versions[0].SourceIDs[0] != copied.Sources[0].ID || *copied.State.DraftVersionID != copied.Versions[0].ID || *copied.Episodes[0].ConfirmedStructureID != copied.Structures[0].ID || !copied.Episodes[0].IsDelete {
		t.Fatal("lost historical identity closure")
	}
	if copied.Sources[0].ContentHash != history.Sources[0].ContentHash || copied.Versions[0].ContentHash != history.Versions[0].ContentHash || copied.Versions[0].SourceManifestSHA256 == history.Versions[0].SourceManifestSHA256 {
		t.Fatal("plain/rich/manifest identities conflated")
	}
	if manifest.Counts.Objects != 1 || len(manifest.Objects) != 1 || manifest.Objects[0].Source.Key == manifest.Objects[0].Target.Key || manifest.Counts.Episodes != 1 || manifest.Counts.Structures != 1 || manifest.Counts.VersionSources != 1 {
		t.Fatal("full historical counts or independent keys")
	}
	if copied.Structures[0].Document.Scenes[0].Items[0].Key == action || copied.Scenes[0].StructureID != copied.Structures[0].ID || copied.Actions[0].SceneID != copied.Scenes[0].ID {
		t.Fatal("stable scene/line keys not remapped")
	}
	again, err := app.RemapProjectHistory(binding, history, nil)
	if err != nil || again.ContentSHA256 != manifest.ContentSHA256 || again.Target.Sources[0].ID != copied.Sources[0].ID {
		t.Fatal("non-deterministic frozen manifest", err)
	}
	file := uuid.New()
	history.Sources[0].MediaAssetID = &file
	if _, err := app.RemapProjectHistory(binding, history, nil); err == nil {
		t.Fatal("missing owning media mapping silently dropped")
	}
}
