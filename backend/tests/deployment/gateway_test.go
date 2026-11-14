package deployment_test

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func TestIndependentImagesKeepBackendModuleBuildContext(t *testing.T) {
	for _, overlay := range []bool{false, true} {
		v := readDeployment(t, overlay)
		for _, service := range []string{"gateway", "creation", "media"} {
			prefix := "services." + service + ".build."
			if got := v.GetString(prefix + "context"); got != "./backend" {
				t.Errorf("%s build context = %q; must retain the root Go module", service, got)
			}
			if got := v.GetString(prefix + "dockerfile"); got != service+"/Dockerfile" {
				t.Errorf("%s Dockerfile = %q; must belong to the service", service, got)
			}
		}
	}
	for _, name := range []string{"go.mod", "go.sum"} {
		deploymentSource(t, "backend/"+name)
	}
	for _, service := range []string{"gateway", "creation", "media"} {
		source := deploymentSource(t, "backend/"+service+"/Dockerfile")
		for _, required := range []string{"COPY go.mod go.sum ./", "COPY . .", " -o /out/" + service + " ./" + service + "\n"} {
			if !strings.Contains(source, required) {
				t.Errorf("%s image lost module-root build instruction %q", service, required)
			}
		}
	}
}

func TestBackendBuildContextKeepsExplicitSourceAllowlist(t *testing.T) {
	var got []string
	for line := range strings.SplitSeq(deploymentSource(t, "backend/.dockerignore"), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			got = append(got, line)
		}
	}
	want := []string{
		"*", "!go.mod", "!go.sum", "!cmd/", "!internal/",
		"!gateway/", "!identity/", "!creation/", "!media/", "!execution/",
		"**/*_test.go", "**/.env*", "**/*.pem", "**/*.key", "**/secrets", "**/.git",
	}
	if !slices.Equal(got, want) {
		t.Errorf("backend context allowlist = %q; want %q (do not admit tests, credentials or unrelated roots)", got, want)
	}
}

func TestDevelopmentWatchTracksOnlyIndependentServiceDependencies(t *testing.T) {
	v := readDeployment(t, true)
	for service, want := range map[string][]string{
		"gateway": {
			"./backend/gateway",
			"./backend/internal/platform/httpapi", "./backend/internal/platform/log",
			"./backend/go.mod", "./backend/go.sum",
		},
		"creation": {
			"./backend/creation/main.go", "./backend/creation/app", "./backend/creation/document", "./backend/creation/script/domain",
			"./backend/internal/platform/serviceauth", "./backend/internal/contract/gen/creation", "./backend/api/proto/glowloom/creation",
			"./backend/go.mod", "./backend/go.sum", "./backend/creation/Dockerfile",
		},
		"media": {
			"./backend/media/main.go", "./backend/media/app", "./backend/media/processing",
			"./backend/internal/platform/serviceauth", "./backend/internal/contract/gen/media", "./backend/api/proto/glowloom/media",
			"./backend/go.mod", "./backend/go.sum", "./backend/media/Dockerfile",
		},
	} {
		var watch []struct {
			Action string
			Path   string
		}
		if err := v.UnmarshalKey("services."+service+".develop.watch", &watch); err != nil {
			t.Fatalf("parse %s development watch: %v", service, err)
		}
		var got []string
		for _, entry := range watch {
			if entry.Action != "rebuild" {
				t.Errorf("%s source watch action = %q; want rebuild", service, entry.Action)
			}
			got = append(got, entry.Path)
		}
		if !slices.Equal(got, want) {
			t.Errorf("%s development watch = %q; want exact independent dependencies %q", service, got, want)
		}
	}
}

func TestFrontendCatalogUsesCanonicalCreationResource(t *testing.T) {
	const catalog = "backend/creation/prompt/domain/cinema_catalog.json"
	const copyInstruction = "COPY " + catalog + " /" + catalog
	if got := strings.Count(deploymentSource(t, "frontend/Dockerfile"), copyInstruction); got != 2 {
		t.Errorf("frontend catalog COPY count = %d; dev and build must use the same canonical source and target", got)
	}
	var got []string
	for line := range strings.SplitSeq(deploymentSource(t, ".dockerignore"), "\n") {
		if strings.HasPrefix(line, "!backend") {
			got = append(got, line)
		}
	}
	want := []string{"!backend/", "!backend/creation/", "!backend/creation/prompt/", "!backend/creation/prompt/domain/", "!" + catalog}
	if !slices.Equal(got, want) {
		t.Errorf("frontend backend-resource allowlist = %q; want only %q", got, want)
	}
	v := readDeployment(t, true)
	var watch []struct {
		Action string
		Path   string
	}
	if err := v.UnmarshalKey("services.frontend.develop.watch", &watch); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, entry := range watch {
		if entry.Path == "./"+catalog && entry.Action == "rebuild" {
			found = true
		}
		if strings.Contains(entry.Path, "backend/internal/") {
			t.Errorf("frontend watches retired catalog path %q", entry.Path)
		}
	}
	if !found {
		t.Error("frontend does not rebuild for the canonical Creation catalog")
	}
	deploymentSource(t, catalog)
}

func TestLegacyImageKeepsEntrypointAndRelocatedResourceLicenses(t *testing.T) {
	source := deploymentSource(t, "backend/Dockerfile")
	if got := strings.Count(source, " ./cmd/glowloom"); got != 2 {
		t.Errorf("legacy dev and runtime build targets changed: got %d cmd/glowloom builds", got)
	}
	for _, instruction := range []string{
		"COPY media/adapter/gltf/schema/COPYING.adoc media/adapter/gltf/schema/LICENSE.adoc media/adapter/gltf/schema/PROVENANCE.md /usr/share/glowloom/licenses/gltf/",
		"COPY media/adapter/gltf/schema/LICENSES/ /usr/share/glowloom/licenses/gltf/LICENSES/",
		"COPY media/mediatool/adapter/ffmpeg/fonts/OFL.txt media/mediatool/adapter/ffmpeg/fonts/PROVENANCE.md /usr/share/glowloom/licenses/noto-cjk/",
	} {
		if !strings.Contains(source, instruction) {
			t.Errorf("legacy image omits relocated license COPY %q", instruction)
		}
		fields := strings.Fields(instruction)
		for _, path := range fields[1 : len(fields)-1] {
			info, err := os.Stat(filepath.Join(deploymentRoot(t), "backend", filepath.FromSlash(path)))
			if err != nil || info == nil {
				t.Errorf("legacy image license source %s is unavailable: %v", path, err)
			}
		}
	}
}

func deploymentSource(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(deploymentRoot(t), filepath.FromSlash(name)))
	if err != nil {
		t.Fatalf("read deployment source %s: %v", name, err)
	}
	return string(data)
}

func deploymentRoot(t *testing.T) string {
	t.Helper()
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate deployment sources")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(source), "../../.."))
}
