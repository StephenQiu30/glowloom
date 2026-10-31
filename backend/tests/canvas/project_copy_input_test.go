package canvas_test

import (
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
	prompt "github.com/StephenQiu30/glowloom-server/backend/internal/prompt/domain"
)

func TestProjectCopyResourceInputAndLocalSubjectEdge(t *testing.T) {
	doc, source, target := subjectConnectionFixture(t)
	doc.Name, doc.Revision, doc.Scope = "完整生成输入", 8, raw(`{}`)
	doc.Viewport.Zoom = 1
	asset, copiedAsset := uuid.New(), uuid.New()
	doc.Nodes[1].Config.Subject = &domain.SubjectConfig{Version: 1, Local: &domain.SubjectLocal{Primary: &domain.SubjectMedia{AssetID: asset, AssetRevision: 3, Kind: "image"}, References: []domain.SubjectMedia{}, Description: "局部"}}
	doc.Nodes[0].InputRevision = 7
	copied, err := domain.CopyProjectDocument(doc, uuid.New(), uuid.New(), map[uuid.UUID]uuid.UUID{asset: copiedAsset})
	if err != nil {
		t.Fatal(err)
	}
	if copied.Document.Nodes[0].ID == target.ID || copied.Document.Nodes[1].ID == source.ID || copied.Document.Nodes[0].InputRevision != 1 || copied.Document.Nodes[1].InputRevision != 1 {
		t.Fatal("copy reused input identities", copied)
	}
	input, err := domain.GenerationInput(copied.Document, copied.Document.Nodes[0])
	if err != nil || len(input.Composer.References) != 1 {
		t.Fatal(input, err)
	}
	ref := input.Composer.References[0]
	if ref.Source.SourceID != copied.Document.Nodes[1].ID.String() || ref.Source.SourceVersion != "1" || ref.Source.Media[0].SourceID != copiedAsset.String() || ref.Source.Media[0].SourceVersion != "1" {
		t.Fatal("copied reference did not use target owner", ref)
	}
	if len(doc.Nodes[0].Config.Draft.Composer.References) != 0 || doc.Nodes[0].InputRevision != 7 || doc.Nodes[1].Config.Subject.Local.Primary.AssetID != asset {
		t.Fatal("source mutated")
	}
}

func TestProjectCopyStillRejectsUnmigratedManualInputSources(t *testing.T) {
	doc, _, _ := subjectConnectionFixture(t)
	doc.Name, doc.Revision, doc.Scope = "外部固定引用", 1, raw(`{}`)
	composer := doc.Nodes[0].Config.Draft.Composer
	composer.References = []prompt.Reference{{ID: "reference", BindingGeneration: 1, MediaKind: "image", Role: "reference", Order: 0, Source: prompt.ReferenceSource{Kind: "media", SourceID: uuid.NewString(), SourceVersion: "1"}}}
	composer.LabelState.HighWater["image"] = 1
	composer.LabelState.Allocations["reference"] = prompt.LabelAllocation{LabelKind: "image", Ordinal: 1}
	if err := composer.Validate(); err != nil {
		t.Fatal(err)
	}
	if _, err := domain.CopyProjectDocument(doc, uuid.New(), uuid.New(), nil); !errors.Is(err, domain.ErrUnsupportedProjectCopy) {
		t.Fatal("unmigrated fixed reference escaped", err)
	}
}

func TestProjectCopyCinemaSourcesAreRecompiledForTarget(t *testing.T) {
	doc, source, _ := cinemaFixture(t)
	doc.Name, doc.Revision, doc.Scope = "相机输入副本", 1, raw(`{}`)
	doc.Viewport.Zoom = 1
	doc.Nodes[1] = resourceInputNode(t, "image")
	doc.Edges[0].TargetNodeID = doc.Nodes[1].ID
	// A hand-authored palette coexists with the connected camera.
	doc.Nodes[1].Config.CinemaKit = &prompt.CinemaKit{Version: 1, Palette: &prompt.CinemaPalette{Name: "手工配色", Colors: []string{"#112233", "#FFFFFF"}}}
	before, err := domain.GenerationInput(doc, doc.Nodes[1])
	if err != nil {
		t.Fatal(err)
	}
	copied, err := domain.CopyProjectDocument(doc, uuid.New(), uuid.New(), nil)
	if err != nil {
		t.Fatal(err)
	}
	after, err := domain.GenerationInput(copied.Document, copied.Document.Nodes[1])
	if err != nil {
		t.Fatal(err)
	}
	kit := after.Composer.CinemaKit
	if kit == nil || len(kit.Sources) != 1 || kit.Sources[0].NodeID != copied.Document.Nodes[0].ID || kit.Sources[0].NodeID == source.ID || kit.Sources[0].Hash != before.Composer.CinemaKit.Sources[0].Hash {
		t.Fatal("toolkit provenance did not rebind target owner", kit)
	}
	if kit.Camera.FocalLength != "35mm" || kit.Palette.Name != "手工配色" || kit.Palette.Colors[0] != "#112233" || copied.Document.Nodes[1].Config.Draft.Composer.CinemaKit != nil {
		t.Fatal("authored settings changed or derived provenance entered draft", kit)
	}
	if before.Composer.CinemaKit.Sources[0].NodeID != source.ID || doc.Nodes[1].Config.CinemaKit.Palette.Colors[0] != "#112233" {
		t.Fatal("copy changed original")
	}
}

func TestProjectCopyRejectsForgedStoredCinemaProvenance(t *testing.T) {
	for _, location := range []string{"node config", "draft composer"} {
		t.Run(location, func(t *testing.T) {
			doc, _, _ := cinemaFixture(t)
			doc.Name, doc.Revision, doc.Scope = "非法来源", 1, raw(`{}`)
			doc.Viewport.Zoom = 1
			doc.Nodes[1] = resourceInputNode(t, "image")
			doc.Edges[0].TargetNodeID = doc.Nodes[1].ID
			input, err := domain.GenerationInput(doc, doc.Nodes[1])
			if err != nil {
				t.Fatal(err)
			}
			if location == "node config" {
				doc.Nodes[1].Config.CinemaKit = input.Composer.CinemaKit
			} else {
				doc.Nodes[1].Config.Draft.Composer.CinemaKit = input.Composer.CinemaKit
			}
			if domain.ValidateGraph(doc) == nil {
				t.Fatal("server-derived sources admitted as authored graph")
			}
			if _, err := domain.CopyProjectDocument(doc, uuid.New(), uuid.New(), nil); err == nil {
				t.Fatal("copy accepted forged stored source identity")
			}
		})
	}
}
