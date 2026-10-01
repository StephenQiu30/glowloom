package media_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestDepthPreviewMatchesFixedWholeClipNormalizationByteForByte(t *testing.T) {
	base, source := os.Getenv("LV_DEPTH_TEST_ROOT"), os.Getenv("LV_BEEFTV_SOURCE_ROOT")
	if base == "" || source == "" {
		t.Skip("fixed native Python and BeefTV public source were not configured")
	}
	worker, err := filepath.Abs(filepath.Join("..", "..", "internal", "mediatool", "adapter", "videodepth", "worker.py"))
	if err != nil {
		t.Fatal(err)
	}
	code := `import importlib.util,sys,numpy as np
def load(name,path):
    spec=importlib.util.spec_from_file_location(name,path)
    module=importlib.util.module_from_spec(spec)
    sys.modules[name]=module
    spec.loader.exec_module(module)
    return module
own=load("native_depth_worker",sys.argv[1])
source=load("fixed_beeftv_pipeline",sys.argv[2])
generator=np.random.default_rng(17)
cases=[np.zeros((3,5,7),dtype=np.float32),np.full((2,11,13),8,dtype=np.float32),
       generator.normal(3,7,size=(4,63,65)).astype(np.float32),
       np.geomspace(1e-6,1e7,5*47*53).reshape(5,47,53).astype(np.float32)]
for value in cases:
    expected=source.normalize_depth_clip(value,gamma=1.25)
    actual=own.relative_preview(value,np)
    assert np.array_equal(expected,actual),"whole-clip P2/P98 or pointwise rounding changed"
print("four fixed-source normalization cases have exactly identical bytes")
`
	cmd := exec.CommandContext(t.Context(), filepath.Join(base, "venv", "bin", "python"), "-I", "-B", "-c", code, worker, filepath.Join(source, "tools", "depth-capture", "depth_capture", "pipeline.py"))
	cmd.Dir = t.TempDir()
	cmd.Env = []string{"PATH=/usr/bin:/bin", "LANG=C.UTF-8", "HOME=" + cmd.Dir}
	data, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("actual fixed-source NumPy comparison: %v %s", err, data)
	}
	t.Log(string(data))
}
