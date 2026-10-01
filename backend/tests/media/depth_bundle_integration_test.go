package media_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	mediaflow "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/workflow"
	"github.com/StephenQiu30/lanverse/backend/internal/mediatool/adapter/videodepth"
	toolapp "github.com/StephenQiu30/lanverse/backend/internal/mediatool/application"
)

func TestDepthNativeFixedSourceRejectsModifiedAndHiddenExecutable(t *testing.T) {
	base := os.Getenv("LV_DEPTH_TEST_ROOT")
	if base == "" {
		t.Skip("actual fixed public VDA bundle was not configured")
	}
	manifestFile := filepath.Join("..", "..", "internal", "mediatool", "adapter", "videodepth", "source_manifest.json")
	manifestBytes, err := os.ReadFile(manifestFile)
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Files map[string]string `json:"files"`
	}
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"modified-python", "unexpected-python", "cached-bytecode", "symlink-directory", "unused-cli-helper"} {
		t.Run(name, func(t *testing.T) {
			source := t.TempDir()
			for relative := range manifest.Files {
				data, err := os.ReadFile(filepath.Join(base, "vda", filepath.FromSlash(relative)))
				if err != nil {
					t.Fatal(err)
				}
				path := filepath.Join(source, filepath.FromSlash(relative))
				if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, data, 0600); err != nil {
					t.Fatal(err)
				}
			}
			path := filepath.Join(source, "unexpected.py")
			switch name {
			case "modified-python":
				path = filepath.Join(source, "video_depth_anything", "video_depth.py")
			case "cached-bytecode":
				path = filepath.Join(source, "video_depth_anything", "__pycache__", "video_depth.cpython-311.pyc")
				if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
					t.Fatal(err)
				}
			case "symlink-directory":
				if err := os.Symlink(t.TempDir(), filepath.Join(source, "__pycache__")); err != nil {
					t.Fatal(err)
				}
				path = ""
			case "unused-cli-helper":
				// Even the upstream helper's exact bytes are outside the model
				// import closure. Its source license conflicts with its header.
				data, err := os.ReadFile(filepath.Join(base, "vda", "utils", "dc_utils.py"))
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(source, "utils", "dc_utils.py"), data, 0600); err != nil {
					t.Fatal(err)
				}
				path = ""
			}
			if path != "" {
				if err := os.WriteFile(path, []byte("not the fixed artifact"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			runner, err := videodepth.NewRunner(videodepth.Config{PythonPath: filepath.Join(base, "venv", "bin", "python"), SourceDir: source, ModelPath: filepath.Join(base, "video_depth_anything_vits.pth"), Device: "mps", FFmpegPath: "ffmpeg", FFprobePath: "ffprobe"}, mediaflow.FFProber{})
			if err != nil {
				t.Fatal(err)
			}
			var phases []toolapp.DepthPhase
			out, err := runner.Process(t.Context(), nil, "", func(p toolapp.DepthPhase) error { phases = append(phases, p); return nil })
			if out != nil || !errors.Is(err, toolapp.ErrDepthModelMismatch) || len(phases) != 1 || phases[0] != toolapp.DepthChecking {
				t.Fatalf("unfixed code reached AV/inference: %v %+v %v", err, out, phases)
			}
		})
	}
}
