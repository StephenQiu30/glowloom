package media_test

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"

	project "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/adapter/editproject"
	toolapp "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/application"
	tool "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/domain"
)

func projectFixture(t *testing.T) (tool.FrozenExport, map[uuid.UUID]string) {
	t.Helper()
	timeline := exportTimeline(t)
	timeline.Clips[0].Title = "剪辑 & <测试>"
	timeline.Clips[0].StartMS = 120
	timeline.Clips[0].DurationMS = 800
	timeline.Tracks[0].Muted = true
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	img.Set(10, 10, color.RGBA{R: 255, A: 255})
	var data bytes.Buffer
	if err := png.Encode(&data, img); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "source.png")
	if err := os.WriteFile(file, data.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(data.Bytes())
	width, height := int32(64), int32(64)
	id := *timeline.Clips[0].AssetID
	frozen := tool.FrozenExport{Source: &tool.Source{CanvasID: uuid.New(), NodeID: uuid.New(), Revision: 7}, OutputKind: tool.OutputFCPXML, Timeline: timeline, Inputs: []tool.FrozenSource{{AssetID: id, Revision: 2, Kind: "image", ObjectKey: "private/key/not-in-package", MIMEType: "image/png", ByteSize: int64(data.Len()), SHA256: hex.EncodeToString(hash[:]), Width: &width, Height: &height}}}
	return frozen, map[uuid.UUID]string{id: file}
}
func archiveEntries(t *testing.T, file *os.File, size int64) map[string][]byte {
	t.Helper()
	reader, err := zip.NewReader(file, size)
	if err != nil {
		t.Fatal(err)
	}
	entries := make(map[string][]byte)
	for _, entry := range reader.File {
		r, e := entry.Open()
		if e != nil {
			t.Fatal(e)
		}
		data, e := io.ReadAll(r)
		_ = r.Close()
		if e != nil {
			t.Fatal(e)
		}
		entries[entry.Name] = data
	}
	return entries
}
func TestProjectExportsContainRealOriginalsAndParseIndependentXML(t *testing.T) {
	frozen, paths := projectFixture(t)
	for _, kind := range []tool.OutputKind{tool.OutputMediaBundle, tool.OutputFCPXML, tool.OutputXMEML} {
		t.Run(string(kind), func(t *testing.T) {
			frozen.OutputKind = kind
			result, err := project.NewRenderer(nil).Render(t.Context(), frozen, paths, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = result.Close() }()
			if result.MIMEType != "application/zip" || result.Size <= frozen.Inputs[0].ByteSize {
				t.Fatalf("not a real archive: %+v", result)
			}
			entries := archiveEntries(t, result.File, result.Size)
			var manifest project.Manifest
			if err = json.Unmarshal(entries["manifest.json"], &manifest); err != nil {
				t.Fatal(err)
			}
			if manifest.Source != *frozen.Source || manifest.Format != kind || len(manifest.Media) != 1 || manifest.Timeline.Clips[0].Title != frozen.Timeline.Clips[0].Title {
				t.Fatalf("identity/edit loss: %+v", manifest)
			}
			original, e := os.ReadFile(paths[frozen.Inputs[0].AssetID])
			if e != nil {
				t.Fatal(e)
			}
			if !bytes.Equal(entries[manifest.Media[0].Path], original) {
				t.Fatal("original media missing or modified")
			}
			if _, e = png.Decode(bytes.NewReader(entries[manifest.Media[0].Path])); e != nil {
				t.Fatal(e)
			}
			for name, data := range entries {
				if strings.Contains(string(data), frozen.Inputs[0].ObjectKey) {
					t.Fatalf("private storage key leaked in %s", name)
				}
			}
			if kind == tool.OutputFCPXML {
				var doc struct {
					XMLName xml.Name
					Version string `xml:"version,attr"`
					Clips   []struct {
						Ref       string `xml:"ref,attr"`
						Name      string `xml:"name,attr"`
						Offset    string `xml:"offset,attr"`
						Duration  string `xml:"duration,attr"`
						SrcEnable string `xml:"srcEnable,attr"`
					} `xml:"library>event>project>sequence>spine>gap>asset-clip"`
					Assets []struct {
						Media struct {
							Src string `xml:"src,attr"`
						} `xml:"media-rep"`
					} `xml:"resources>asset"`
				}
				if e = xml.Unmarshal(entries["timeline.fcpxmld/Info.fcpxml"], &doc); e != nil || doc.XMLName.Local != "fcpxml" || doc.Version != "1.10" || len(doc.Clips) != 1 || doc.Clips[0].Offset != "3/25s" || doc.Clips[0].Duration != "4/5s" || doc.Clips[0].SrcEnable != "video" || doc.Clips[0].Name != frozen.Timeline.Clips[0].Title || !strings.HasPrefix(doc.Assets[0].Media.Src, "./Media/") {
					t.Fatalf("FCPXML roundtrip: %+v %v", doc, e)
				}
			}
			if kind == tool.OutputXMEML {
				var doc struct {
					XMLName xml.Name
					Version string `xml:"version,attr"`
					Clips   []struct {
						Start int64  `xml:"start"`
						End   int64  `xml:"end"`
						Name  string `xml:"name"`
						Path  string `xml:"file>pathurl"`
					} `xml:"sequence>media>video>track>clipitem"`
				}
				if e = xml.Unmarshal(entries["timeline.xml"], &doc); e != nil || doc.XMLName.Local != "xmeml" || doc.Version != "5" || len(doc.Clips) != 1 || doc.Clips[0].Start != 3 || doc.Clips[0].End != 23 || doc.Clips[0].Name != frozen.Timeline.Clips[0].Title || !strings.HasPrefix(doc.Clips[0].Path, "./Media/") {
					t.Fatalf("xmeml roundtrip: %+v %v", doc, e)
				}
			}
			if err = project.Verify(t.Context(), result.File, result.Size, frozen); err != nil {
				t.Fatal(err)
			}
		})
	}
}
func TestProjectExportRejectsCorruptOriginalAndUnsupportedLoss(t *testing.T) {
	frozen, paths := projectFixture(t)
	frozen.Inputs[0].SHA256 = strings.Repeat("a", 64)
	if _, err := project.NewRenderer(nil).Render(t.Context(), frozen, paths, nil); !errors.Is(err, toolapp.ErrInvalidExport) {
		t.Fatalf("bad original accepted: %v", err)
	}
	frozen, paths = projectFixture(t)
	frozen.Timeline.Clips[0].FadeInMS = 20
	if _, err := project.NewRenderer(nil).Render(t.Context(), frozen, paths, nil); !errors.Is(err, toolapp.ErrInvalidExport) {
		t.Fatalf("lossy fade accepted: %v", err)
	}
	frozen.OutputKind = tool.OutputMediaBundle
	out, err := project.NewRenderer(nil).Render(t.Context(), frozen, paths, nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = out.Close()
}
func TestProjectArtifactRejectsTamperedDocumentExtraEntriesAndSymlinks(t *testing.T) {
	frozen, paths := projectFixture(t)
	result, err := project.NewRenderer(nil).Render(t.Context(), frozen, paths, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = result.Close() }()
	entries := archiveEntries(t, result.File, result.Size)
	for _, attack := range []string{"document", "extra", "symlink"} {
		t.Run(attack, func(t *testing.T) {
			file, e := os.CreateTemp(t.TempDir(), "bad-*.zip")
			if e != nil {
				t.Fatal(e)
			}
			defer func() { _ = file.Close() }()
			writer := zip.NewWriter(file)
			for name, data := range entries {
				header := &zip.FileHeader{Name: name, Method: zip.Store}
				header.SetMode(0600)
				if attack == "document" && name == "timeline.fcpxmld/Info.fcpxml" {
					data = bytes.Replace(data, []byte("version=\"1.10\""), []byte("version=\"1.9\""), 1)
				}
				if attack == "symlink" && strings.Contains(name, "/Media/") {
					header.SetMode(os.ModeSymlink | 0600)
				}
				entry, e := writer.CreateHeader(header)
				if e != nil {
					t.Fatal(e)
				}
				if _, e = entry.Write(data); e != nil {
					t.Fatal(e)
				}
			}
			if attack == "extra" {
				entry, e := writer.Create("../escape")
				if e != nil {
					t.Fatal(e)
				}
				_, _ = entry.Write([]byte("unexpected"))
			}
			if e = writer.Close(); e != nil {
				t.Fatal(e)
			}
			stat, _ := file.Stat()
			if e = project.Verify(t.Context(), file, stat.Size(), frozen); e == nil {
				t.Fatal("unsafe artifact accepted")
			}
		})
	}
}
func TestProjectFCPXMLMatchesOfficialAppleDTD(t *testing.T) {
	dtd := os.Getenv("GLOWLOOM_TEST_FCPXML_DTD")
	if dtd == "" {
		t.Skip("set locally downloaded official Apple FCPXML 1.10 DTD")
	}
	frozen, paths := projectFixture(t)
	out, err := project.NewRenderer(nil).Render(t.Context(), frozen, paths, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = out.Close() }()
	entries := archiveEntries(t, out.File, out.Size)
	xmlPath := filepath.Join(t.TempDir(), "Info.fcpxml")
	if err = os.WriteFile(xmlPath, entries["timeline.fcpxmld/Info.fcpxml"], 0600); err != nil {
		t.Fatal(err)
	}
	if output, e := exec.CommandContext(t.Context(), "xmllint", "--nonet", "--noout", "--dtdvalid", dtd, xmlPath).CombinedOutput(); e != nil {
		t.Fatalf("official DTD rejection: %v: %s", e, output)
	}
}

func TestProjectXMLPreservesZeroSequenceTimecode(t *testing.T) {
	frozen, paths := projectFixture(t)
	frozen.OutputKind = tool.OutputXMEML
	result, err := project.NewRenderer(nil).Render(t.Context(), frozen, paths, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = result.Close() }()
	var document struct {
		Timecode struct {
			String  string `xml:"string"`
			Frame   int64  `xml:"frame"`
			Display string `xml:"displayformat"`
			Rate    int64  `xml:"rate>timebase"`
		} `xml:"sequence>timecode"`
	}
	if err := xml.Unmarshal(archiveEntries(t, result.File, result.Size)["timeline.xml"], &document); err != nil {
		t.Fatal(err)
	}
	if document.Timecode.String != "00:00:00:00" || document.Timecode.Frame != 0 || document.Timecode.Display != "NDF" || document.Timecode.Rate != int64(frozen.Timeline.FPS) {
		t.Fatalf("sequence timecode not preserved: %+v", document.Timecode)
	}
}
