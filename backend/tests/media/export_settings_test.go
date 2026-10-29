package media_test

import (
	"encoding/json"
	"encoding/xml"
	"strings"
	"testing"

	mediaflow "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/workflow"
	project "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/adapter/editproject"
	exportff "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/adapter/ffmpeg"
	app "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/application"
	tool "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/domain"
)

func TestTimelineExportSettingsFreezeAutoDimensionsAndDestination(t *testing.T) {
	frozen, _ := projectFixture(t)
	frozen.Timeline.AspectRatio = "auto"
	w, h := int32(320), int32(180)
	frozen.Inputs[0].Width, frozen.Inputs[0].Height = &w, &h
	settings := tool.ExportSettings{FileName: "时间线 甲"}
	if err := app.ApplyExportSettings(&frozen, settings); err != nil {
		t.Fatal(err)
	}
	if frozen.Settings.Destination != "canvas" || frozen.Settings.Resolution != 720 {
		t.Fatalf("defaults: %+v", frozen.Settings)
	}
	width, height, err := frozen.VideoDimensions()
	if err != nil || width != 1280 || height != 720 || frozen.Timeline.AspectRatio != "auto" {
		t.Fatalf("dimensions %d %d %v", width, height, err)
	}
	raw, _ := json.Marshal(frozen)
	var restored tool.FrozenExport
	if err := json.Unmarshal(raw, &restored); err != nil {
		t.Fatal(err)
	}
	if restored.Settings.FileName != "时间线 甲" || restored.Settings.Destination != "canvas" {
		t.Fatal("settings lost")
	}
}
func TestTimelineExportSettingsRejectInvalidAndResolvePortrait(t *testing.T) {
	for _, settings := range []tool.ExportSettings{{FileName: strings.Repeat("中", 100)}, {FileName: "../bad"}, {FileName: "bad\nname"}, {FileName: "ok", Resolution: 721}, {FileName: "ok", Destination: "unknown"}} {
		frozen, _ := projectFixture(t)
		if err := app.ApplyExportSettings(&frozen, settings); err == nil {
			t.Fatalf("accepted %+v", settings)
		}
	}
	frozen, _ := projectFixture(t)
	frozen.Timeline.AspectRatio = "9:16"
	if err := app.ApplyExportSettings(&frozen, tool.ExportSettings{FileName: "竖屏", Resolution: 480, Destination: "local"}); err != nil {
		t.Fatal(err)
	}
	w, h, e := frozen.VideoDimensions()
	if e != nil || w != 480 || h != 854 {
		t.Fatalf("portrait %d %d %v", w, h, e)
	}
	frozen.Timeline.AspectRatio = "auto"
	frozen.Timeline.Clips[0].Kind = "audio"
	if err := app.ApplyExportSettings(&frozen, tool.ExportSettings{FileName: "音频", Resolution: 720}); err != nil {
		t.Fatal(err)
	}
	w, h, e = frozen.VideoDimensions()
	if e != nil || w != 1280 || h != 720 {
		t.Fatalf("fallback %d %d %v", w, h, e)
	}
}

func TestTimelineExportSelectedResolutionRendersAutoAndItsRealProxy(t *testing.T) {
	frozen, paths := projectFixture(t)
	frozen.Timeline.AspectRatio = "auto"
	frozen.OutputKind = tool.OutputVideo
	if err := app.ApplyExportSettings(&frozen, tool.ExportSettings{FileName: "自动比例", Resolution: 480}); err != nil {
		t.Fatal(err)
	}
	renderer, err := exportff.NewRenderer(mediaflow.FFProber{})
	if err != nil {
		t.Fatal(err)
	}
	output, err := renderer.Render(t.Context(), frozen, paths, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer output.Close()
	probe, err := (mediaflow.FFProber{}).Probe(t.Context(), output)
	if err != nil || probe.Width == nil || *probe.Width != 480 || probe.Height == nil || *probe.Height != 480 {
		t.Fatalf("actual 480P auto output %+v %v", probe, err)
	}
	previews, err := (mediaflow.FFRenderer{}).Render(t.Context(), output, probe, "auto")
	if err != nil {
		t.Fatal(err)
	}
	defer closeRenditions(previews)
	if len(previews) != 2 || previews[1].Width != 720 || previews[1].Height != 720 {
		t.Fatal("auto proxy changed composition")
	}
}

func TestTimelineExportSettingsPortableProjectsKeepNameAndDimensions(t *testing.T) {
	for _, kind := range []tool.OutputKind{tool.OutputFCPXML, tool.OutputXMEML} {
		t.Run(string(kind), func(t *testing.T) {
			frozen, paths := projectFixture(t)
			frozen.OutputKind = kind
			frozen.Timeline.AspectRatio = "auto"
			if err := app.ApplyExportSettings(&frozen, tool.ExportSettings{FileName: "实际工程", Resolution: 720, Destination: "local"}); err != nil {
				t.Fatal(err)
			}
			output, err := project.NewRenderer(nil).Render(t.Context(), frozen, paths, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer output.Close()
			entries := archiveEntries(t, output.File, output.Size)
			if kind == tool.OutputFCPXML {
				var doc struct {
					Project struct {
						Name string `xml:"name,attr"`
					} `xml:"library>event>project"`
					Formats []struct {
						Width  int `xml:"width,attr"`
						Height int `xml:"height,attr"`
					} `xml:"resources>format"`
				}
				if err := xml.Unmarshal(entries["timeline.fcpxmld/Info.fcpxml"], &doc); err != nil || doc.Project.Name != "实际工程" || len(doc.Formats) == 0 || doc.Formats[0].Width != 720 || doc.Formats[0].Height != 720 {
					t.Fatalf("named auto project %+v %v", doc, err)
				}
			} else {
				var doc struct {
					Sequence struct {
						Name   string `xml:"name"`
						Width  int    `xml:"media>video>format>samplecharacteristics>width"`
						Height int    `xml:"media>video>format>samplecharacteristics>height"`
					} `xml:"sequence"`
				}
				if err := xml.Unmarshal(entries["timeline.xml"], &doc); err != nil || doc.Sequence.Name != "实际工程" || doc.Sequence.Width != 720 || doc.Sequence.Height != 720 {
					t.Fatalf("named auto XML %+v %v", doc, err)
				}
			}
		})
	}
}
