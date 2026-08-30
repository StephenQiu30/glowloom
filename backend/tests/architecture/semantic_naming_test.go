package architecture_test

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var (
	numericReleaseName = regexp.MustCompile(`(^|[^[:alnum:]])[vV][12]([^[:alnum:]]|$)|[[:alnum:]][Vv][12]([A-Z_]|$)`)
	externalSemver     = regexp.MustCompile(`\bv[12]\.[0-9]+\.[0-9]+\b`)
)

func TestProjectContractsUseSemanticNames(t *testing.T) {
	t.Parallel()

	repositoryRoot := repositoryDirectory(t)
	for _, relativeRoot := range []string{
		"backend/api", "backend/cmd", "backend/internal", "backend/tests",
		"agent/app", "agent/tests", "frontend/src", "frontend/tests", ".github", "docs",
	} {
		root := filepath.Join(repositoryRoot, relativeRoot)
		err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				if strings.HasPrefix(entry.Name(), ".") || entry.Name() == "__pycache__" {
					return filepath.SkipDir
				}
				return nil
			}
			if !isSemanticNamingSource(path) || entry.Name() == "semantic_naming_test.go" {
				return nil
			}
			relativePath, err := filepath.Rel(repositoryRoot, path)
			if err != nil {
				return err
			}
			if numericReleaseName.MatchString(filepath.ToSlash(relativePath)) {
				t.Errorf("项目文件名必须表达业务语义，不得使用数字发布序号：%s", filepath.ToSlash(relativePath))
			}
			return inspectSemanticNames(t, path, filepath.ToSlash(relativePath))
		})
		if err != nil {
			t.Fatalf("检查 %s 的语义命名：%v", relativeRoot, err)
		}
	}
}

func inspectSemanticNames(t *testing.T, path, relativePath string) error {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	temporalAPIVersion := "/" + "v" + "1"
	franzGoVersion := "franz-go " + "v" + "1.21.6"
	codexFeature := "multi_agent_" + "v" + "2"
	googleInteractionAPI := "interactions-" + "v" + "1beta-image"
	scanner := bufio.NewScanner(file)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := scanner.Text()
		if strings.Contains(line, "go.temporal.io/api/") {
			line = strings.ReplaceAll(line, temporalAPIVersion, "/external-api")
		}
		line = strings.ReplaceAll(line, franzGoVersion, "franz-go external-version")
		line = strings.ReplaceAll(line, codexFeature, "codex-external-feature")
		line = strings.ReplaceAll(line, googleInteractionAPI, "google-external-api")
		line = externalSemver.ReplaceAllString(line, "external-semver")
		if numericReleaseName.MatchString(line) {
			t.Errorf("%s:%d 含项目自有数字发布序号命名：%s", relativePath, lineNumber, strings.TrimSpace(scanner.Text()))
		}
	}
	return scanner.Err()
}

func isSemanticNamingSource(path string) bool {
	extension := filepath.Ext(path)
	if extension == "" {
		return filepath.Base(path) == "Dockerfile"
	}
	return map[string]bool{
		".go": true, ".py": true, ".ts": true, ".tsx": true,
		".json": true, ".yaml": true, ".yml": true, ".toml": true, ".sh": true, ".md": true,
	}[extension]
}
