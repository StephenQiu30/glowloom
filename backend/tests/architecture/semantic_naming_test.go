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
	numericReleaseName    = regexp.MustCompile(`[vV][0-9]+`)
	externalSemver        = regexp.MustCompile(`\bv[0-9]+\.[0-9]+\.[0-9]+\b`)
	externalGoModuleMajor = regexp.MustCompile(`(?:github\.com|go\.temporal\.io|gorm\.io|google\.golang\.org|go\.opentelemetry\.io|golang\.org|gonum\.org)/[^\x60\x22[:space:]]*/v[0-9]+`)
	externalModuleAlias   = regexp.MustCompile(`\bvalidator/v[0-9]+\b`)
	externalGitHubAction  = regexp.MustCompile(`uses:[[:space:]]+[^[:space:]]+@v[0-9]+`)
)

func TestProjectContractsUseSemanticNames(t *testing.T) {
	t.Parallel()

	repositoryRoot := repositoryDirectory(t)
	for _, relativeRoot := range []string{
		"backend/api", "backend/cmd", "backend/internal", ".github/ci/observability", "backend/tests",
		"agent/app", "agent/skills", "agent/tests", "frontend/src", "frontend/tests", ".github", "docs",
		"frontend/scripts",
	} {
		root := filepath.Join(repositoryRoot, relativeRoot)
		err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				if path != root && (strings.HasPrefix(entry.Name(), ".") || entry.Name() == "__pycache__") {
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
	for _, relativePath := range []string{
		"README.md", "AGENTS.md", "AGENTS.local.md", "LICENSE",
		"backend/Dockerfile", "agent/Dockerfile", "frontend/Dockerfile",
		"agent/pyproject.toml", "agent/requirements.txt",
		"frontend/components.json", "frontend/eslint.config.mjs", "frontend/next.config.ts",
		"frontend/package.json", "frontend/playwright.config.ts", "frontend/postcss.config.mjs",
		"frontend/tsconfig.json", "frontend/vitest.config.ts",
		"docker-compose.yml", ".github/ci/compose.dependencies.yml", ".github/ci/compose.application.yml",
	} {
		path := filepath.Join(repositoryRoot, relativePath)
		if numericReleaseName.MatchString(relativePath) {
			t.Errorf("项目文件名必须表达业务语义，不得使用数字发布序号：%s", relativePath)
		}
		if err := inspectSemanticNames(t, path, relativePath); err != nil {
			t.Fatalf("检查 %s 的语义命名：%v", relativePath, err)
		}
	}
}

func TestSemanticNameInspectorDistinguishesProjectAndExternalNames(t *testing.T) {
	t.Parallel()

	for _, value := range []string{
		"/api/" + "v" + "1/projects",
		"StoryGraph" + "V" + "2",
		"candidate_" + "v" + "3",
		"wire-version-" + "v" + "4",
		"from temporalio.api.workflowservice.v1 import DescribeNamespaceRequest # flow_" + "v" + "2",
	} {
		if !hasProjectNumericReleaseName(value) {
			t.Errorf("应拒绝项目自有数字发布序号命名：%s", value)
		}
	}
	for _, value := range []string{
		"storygraph-stage-wire-production",
		"uses: actions/checkout@v6",
		`go.temporal.io/api/enums/v1`,
		"from temporalio.api.workflowservice.v1 import DescribeNamespaceRequest",
		"from temporalio.api.common.v1 import WorkflowExecution",
		"from temporalio.api.enums.v1 import EventType, ResetReapplyType",
		`github.com/minio/minio-go/v7/pkg/credentials`,
		"dependency v1.31.2",
		"NewStaticV4",
		"multi_agent_" + "v" + "2",
		"interactions-v1beta-image",
		"https://api.openai.com/v1/images/generations",
		"/v1/images/generations",
	} {
		if hasProjectNumericReleaseName(value) {
			t.Errorf("不应拒绝第三方正式版本或标识：%s", value)
		}
	}
}

func inspectSemanticNames(t *testing.T, path, relativePath string) error {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	scanner := bufio.NewScanner(file)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		if hasProjectNumericReleaseName(scanner.Text()) {
			t.Errorf("%s:%d 含项目自有数字发布序号命名：%s", relativePath, lineNumber, strings.TrimSpace(scanner.Text()))
		}
	}
	return scanner.Err()
}

func hasProjectNumericReleaseName(line string) bool {
	codexFeature := "multi_agent_" + "v" + "2"
	googleInteractionAPI := "interactions-" + "v" + "1beta-image"
	minioCredentialConstructor := "NewStatic" + "V" + "4"
	vitestCoveragePackage := "@vitest/coverage-" + "v" + "8"
	line = strings.ReplaceAll(line, "/v1/images/generations", "/openai-images-generations")
	line = strings.ReplaceAll(line, codexFeature, "codex-external-feature")
	line = strings.ReplaceAll(line, googleInteractionAPI, "google-external-api")
	line = strings.ReplaceAll(line, minioCredentialConstructor, "minio-external-constructor")
	line = strings.ReplaceAll(line, vitestCoveragePackage, "vitest-external-package")
	for _, module := range []string{"workflowservice", "common", "enums"} {
		line = strings.ReplaceAll(line, "temporalio.api."+module+".v1", "temporal-external-protobuf")
	}
	line = externalGoModuleMajor.ReplaceAllString(line, "external-go-module")
	line = externalModuleAlias.ReplaceAllString(line, "external-module-alias")
	line = externalGitHubAction.ReplaceAllString(line, "external-github-action")
	line = externalSemver.ReplaceAllString(line, "external-semver")
	return numericReleaseName.MatchString(line)
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
