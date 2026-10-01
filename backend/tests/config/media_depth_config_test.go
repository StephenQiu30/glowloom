package config_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/StephenQiu30/lanverse/backend/internal/platform/config"
)

func TestLoadVideoDepthRequiresCompleteOfflineRuntime(t *testing.T) {
	keys := []string{"LV_VIDEO_DEPTH_PYTHON_PATH", "LV_VIDEO_DEPTH_SOURCE_DIR", "LV_VIDEO_DEPTH_MODEL_PATH", "LV_VIDEO_DEPTH_DEVICE"}
	values := []string{"/configured/venv/bin/python", "/configured/vda", "/configured/weights/small.pth", "mps"}
	for mask := 0; mask < 16; mask++ {
		t.Run(fmt.Sprintf("configured_%04b", mask), func(t *testing.T) {
			for index, key := range keys {
				value := ""
				if mask&(1<<index) != 0 {
					value = values[index]
				}
				t.Setenv(key, value)
			}
			_, err := config.Load()
			if mask == 0 || mask == 15 {
				if err != nil {
					t.Fatal("disabled or complete offline runtime rejected", err)
				}
			} else if !errors.Is(err, config.ErrInvalid) {
				t.Fatal("partial runtime must fail before admission", err)
			}
		})
	}
	for index, key := range keys {
		t.Run(key, func(t *testing.T) {
			for i, k := range keys {
				t.Setenv(k, values[i])
			}
			value := "relative/path"
			if index == 3 {
				value = "automatic-gpu-fallback"
			}
			t.Setenv(key, value)
			if _, err := config.Load(); !errors.Is(err, config.ErrInvalid) {
				t.Fatal("unowned path or unspecified device accepted", err)
			}
		})
	}
	t.Run("unverified_cpu_device", func(t *testing.T) {
		for index, key := range keys {
			t.Setenv(key, values[index])
		}
		t.Setenv("LV_VIDEO_DEPTH_DEVICE", "cpu")
		if _, err := config.Load(); !errors.Is(err, config.ErrInvalid) {
			t.Fatal("CPU admission must remain disabled until its receipt profile is verified", err)
		}
	})
}
