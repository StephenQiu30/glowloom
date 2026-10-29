package media_test

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"

	project "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/adapter/editproject"
	tool "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/domain"
	"github.com/google/uuid"
)

func TestJianyingNativeDraftPreservesEditsAndOriginals(t *testing.T) {
	f, paths := projectAVFixture(t)
	f.OutputKind = tool.OutputJianying
	id := uuid.New()
	f.ExportID = &id
	// Reuse one audio source with a separate range; archive it once.
	original := f.Timeline.Clips[2]
	repeat := original
	repeat.ID = uuid.New()
	repeat.StartMS = 1400
	repeat.DurationMS = 300
	repeat.SourceStartMS = 500
	f.Timeline.Clips = append(f.Timeline.Clips, repeat)
	out, e := project.NewRenderer(nil).Render(t.Context(), f, paths, nil)
	if e != nil {
		t.Fatal(e)
	}
	defer out.Close()
	entries := archiveEntries(t, out.File, out.Size)
	var doc struct {
		Version    int    `json:"version"`
		NewVersion string `json:"new_version"`
		Duration   int64  `json:"duration"`
		Tracks     []struct {
			Segments []struct {
				Material string                          `json:"material_id"`
				Source   struct{ Start, Duration int64 } `json:"source_timerange"`
				Target   struct{ Start, Duration int64 } `json:"target_timerange"`
				Volume   float64                         `json:"volume"`
				Last     float64                         `json:"last_nonzero_volume"`
				Refs     []string                        `json:"extra_material_refs"`
			} `json:"segments"`
		} `json:"tracks"`
		Materials map[string][]map[string]any `json:"materials"`
	}
	if e = json.Unmarshal(entries["template.json"], &doc); e != nil {
		t.Fatal(e)
	}
	if doc.Version != 360000 || doc.NewVersion != "134.0.0" || doc.Duration != 1700000 || len(doc.Tracks) != 3 || len(entries) != 5 {
		t.Fatalf("native draft lost structure: %+v entries %d", doc, len(entries))
	}
	v := doc.Tracks[1].Segments[0]
	a := doc.Tracks[2].Segments
	if v.Source.Start != 200000 || v.Target.Start != 400000 || v.Source.Duration != 800000 || v.Volume != .5 || len(a) != 2 || a[0].Volume != 0 || a[0].Last != .5 || a[1].Source.Start != 500000 || a[1].Target.Start != 1400000 {
		t.Fatalf("edit loss: video %+v audio %+v", v, a)
	}
	ids := map[string]bool{}
	for _, list := range doc.Materials {
		for _, m := range list {
			ids[m["id"].(string)] = true
		}
	}
	for _, track := range doc.Tracks {
		for _, s := range track.Segments {
			if !ids[s.Material] {
				t.Fatal("missing material")
			}
			for _, ref := range s.Refs {
				if !ids[ref] {
					t.Fatal("missing effect reference")
				}
			}
		}
	}
	for _, in := range f.Inputs {
		b, e := os.ReadFile(paths[in.AssetID])
		if e != nil {
			t.Fatal(e)
		}
		found := false
		for name, body := range entries {
			if name != "template.json" && name != "manifest.json" && bytes.Equal(b, body) {
				found = true
			}
		}
		if !found {
			t.Fatal("original was trimmed/transcoded or lost")
		}
	}
	for _, m := range doc.Materials["audios"] {
		p := m["path"].(string)
		if _, ok := entries[p]; !ok {
			t.Fatal("relative media path unresolved")
		}
	}
	if e = project.Verify(t.Context(), out.File, out.Size, f); e != nil {
		t.Fatal(e)
	}
	f.Timeline.Clips[0].DurationMS++
	if e = project.Verify(t.Context(), out.File, out.Size, f); e == nil {
		t.Fatal("artifact with different frozen edits accepted")
	}
}

func TestJianyingNativeDraftIdentityFollowsExportJob(t *testing.T) {
	f, paths := projectFixture(t)
	f.OutputKind = tool.OutputJianying
	job := uuid.New()
	f.ExportID = &job
	render := func() ([]byte, string) {
		t.Helper()
		out, e := project.NewRenderer(nil).Render(t.Context(), f, paths, nil)
		if e != nil {
			t.Fatal(e)
		}
		defer out.Close()
		body := archiveEntries(t, out.File, out.Size)["template.json"]
		var doc struct {
			ID string `json:"id"`
		}
		if e = json.Unmarshal(body, &doc); e != nil {
			t.Fatal(e)
		}
		return body, doc.ID
	}
	first, id := render()
	again, same := render()
	if !bytes.Equal(first, again) || id != same {
		t.Fatal("same export changed draft identity")
	}
	next := uuid.New()
	f.ExportID = &next
	_, different := render()
	if id == different {
		t.Fatal("different export jobs reuse native draft identity")
	}
	f.ExportID = nil
	if _, e := project.NewRenderer(nil).Render(t.Context(), f, paths, nil); e == nil {
		t.Fatal("unbound native draft accepted")
	}
}
