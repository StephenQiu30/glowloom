package architecture_test

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"
)

var (
	markdownLinkPattern  = regexp.MustCompile(`\[[^\]]+\]\(([^)]+)\)`)
	prdAcceptancePattern = regexp.MustCompile(`(?m)^\|\s*(PRD-(?:000|[A-F])-AC-\d{3})\s*\|`)
)

func TestFormalDocumentationContainsOnlyDurableLayers(t *testing.T) {
	t.Parallel()

	docsRoot := filepath.Join(repositoryRoot(t), "docs")
	entries, err := os.ReadDir(docsRoot)
	if err != nil {
		t.Fatalf("read docs root: %v", err)
	}

	allowedDirectories := map[string]bool{
		"acceptance":  true,
		"design":      true,
		"plan":        true,
		"prd":         true,
		"requirement": true,
	}
	for _, entry := range entries {
		if entry.IsDir() {
			if !allowedDirectories[entry.Name()] {
				t.Errorf("docs/%s is not a durable formal documentation layer", entry.Name())
			}
			continue
		}
		if entry.Name() != "README.md" {
			t.Errorf("docs/%s is not the formal documentation entry point", entry.Name())
		}
	}
}

func TestFormalDocumentationLinksResolve(t *testing.T) {
	t.Parallel()

	docsRoot := filepath.Join(repositoryRoot(t), "docs")
	var failures []string
	err := filepath.WalkDir(docsRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".md" {
			return nil
		}

		contents, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		for _, match := range markdownLinkPattern.FindAllStringSubmatch(string(contents), -1) {
			target := strings.TrimSpace(strings.Trim(match[1], "<>"))
			if target == "" || strings.HasPrefix(target, "#") {
				continue
			}
			parsed, parseErr := url.Parse(target)
			if parseErr != nil {
				failures = append(failures, fmt.Sprintf("%s: invalid link %q", relativeToRepository(t, path), target))
				continue
			}
			if parsed.IsAbs() || parsed.Scheme == "mailto" {
				continue
			}

			decodedPath, decodeErr := url.PathUnescape(parsed.Path)
			if decodeErr != nil {
				failures = append(failures, fmt.Sprintf("%s: invalid escaped path %q", relativeToRepository(t, path), target))
				continue
			}
			resolved := filepath.Clean(filepath.Join(filepath.Dir(path), filepath.FromSlash(decodedPath)))
			if _, statErr := os.Stat(resolved); statErr != nil {
				failures = append(failures, fmt.Sprintf("%s: unresolved link %q", relativeToRepository(t, path), target))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk formal documentation: %v", err)
	}

	sort.Strings(failures)
	for _, failure := range failures {
		t.Error(failure)
	}
}

func TestEveryPRDAcHasAPlanEvidenceMapping(t *testing.T) {
	t.Parallel()

	repository := repositoryRoot(t)
	prdCorpus := readMarkdownTree(t, filepath.Join(repository, "docs", "prd"))
	planCorpus := readMarkdownTree(t, filepath.Join(repository, "docs", "plan"))

	seen := make(map[string]bool)
	for _, match := range prdAcceptancePattern.FindAllStringSubmatch(prdCorpus, -1) {
		id := match[1]
		if seen[id] {
			t.Errorf("duplicate PRD acceptance definition %s", id)
			continue
		}
		seen[id] = true
		if !strings.Contains(planCorpus, id) {
			t.Errorf("%s has no explicit Plan evidence mapping", id)
		}
	}
	if len(seen) == 0 {
		t.Fatal("no PRD acceptance definitions found")
	}
}

func TestBackendProductionSourceContainsNoTests(t *testing.T) {
	t.Parallel()

	sourceRoot := filepath.Join(repositoryRoot(t), "backend", "src")
	var testFiles []string
	err := filepath.WalkDir(sourceRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), "_test.go") {
			testFiles = append(testFiles, relativeToRepository(t, path))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk backend production source: %v", err)
	}

	sort.Strings(testFiles)
	for _, testFile := range testFiles {
		t.Errorf("production source contains test file %s", testFile)
	}
}

func TestApplicationProductionSourceContainsNoTests(t *testing.T) {
	t.Parallel()

	repository := repositoryRoot(t)
	productionRoots := []string{
		filepath.Join(repository, "agent", "src"),
		filepath.Join(repository, "frontend", "src"),
	}
	var testFiles []string
	for _, sourceRoot := range productionRoots {
		err := filepath.WalkDir(sourceRoot, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			name := entry.Name()
			if !entry.IsDir() && (strings.Contains(name, ".test.") || strings.Contains(name, ".spec.") || strings.HasSuffix(name, "_test.py") || strings.HasSuffix(name, ".snap")) {
				testFiles = append(testFiles, relativeToRepository(t, path))
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk application production source %s: %v", sourceRoot, err)
		}
	}

	sort.Strings(testFiles)
	for _, testFile := range testFiles {
		t.Errorf("application production source contains test file %s", testFile)
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()

	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve architecture test path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", "..", ".."))
}

func relativeToRepository(t *testing.T, path string) string {
	t.Helper()

	relative, err := filepath.Rel(repositoryRoot(t), path)
	if err != nil {
		t.Fatalf("make path relative to repository: %v", err)
	}
	return filepath.ToSlash(relative)
}

func readMarkdownTree(t *testing.T, root string) string {
	t.Helper()

	var corpus strings.Builder
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".md" {
			return nil
		}
		contents, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		corpus.Write(contents)
		corpus.WriteByte('\n')
		return nil
	})
	if err != nil {
		t.Fatalf("read Markdown tree %s: %v", root, err)
	}
	return corpus.String()
}
