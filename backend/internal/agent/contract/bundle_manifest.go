package contract

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf8"

	platformcanonical "github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
)

const StoryGraphBundleContractID = "storygraph-bundle-content-production"

const (
	storyGraphBundleEntrypoint = "SKILL.md"
	storyGraphNoticePath       = "NOTICE.md"
)

type BundleFile struct {
	Path       string `json:"path"`
	ByteLength int64  `json:"byte_length"`
	SHA256     string `json:"sha256"`
}

type BundleContentManifest struct {
	ContractID             string       `json:"contract_id"`
	BundleEntrypoint       string       `json:"bundle_entrypoint"`
	Files                  []BundleFile `json:"bundle_file_manifest"`
	ProvenanceManifestHash string       `json:"provenance_manifest_hash"`
	NoticeHash             string       `json:"notice_hash"`
	IsolationScanHash      string       `json:"isolation_scan_hash"`
	ContentHash            string       `json:"bundle_content_hash"`
}

func BuildStoryGraphBundleManifest(root string) (BundleContentManifest, json.RawMessage, error) {
	if err := validateBundleRoot(root); err != nil {
		return BundleContentManifest{}, nil, err
	}
	paths := StoryGraphBundlePaths()
	files := make([]BundleFile, 0, len(paths))
	for _, relative := range paths {
		content, err := readBundleFile(root, relative)
		if err != nil {
			return BundleContentManifest{}, nil, err
		}
		digest := sha256.Sum256(content)
		files = append(files, BundleFile{Path: relative, ByteLength: int64(len(content)), SHA256: hex.EncodeToString(digest[:])})
	}
	noticeIndex := slices.IndexFunc(files, func(file BundleFile) bool { return file.Path == storyGraphNoticePath })
	if noticeIndex < 0 {
		return BundleContentManifest{}, nil, errors.New("StoryGraph bundle NOTICE is missing")
	}
	provenanceHash, err := storyGraphProvenanceHash(files[noticeIndex].SHA256)
	if err != nil {
		return BundleContentManifest{}, nil, err
	}
	isolationHash, err := storyGraphIsolationHash(files)
	if err != nil {
		return BundleContentManifest{}, nil, err
	}
	manifest := BundleContentManifest{
		ContractID: StoryGraphBundleContractID, BundleEntrypoint: storyGraphBundleEntrypoint,
		Files: files, ProvenanceManifestHash: provenanceHash, NoticeHash: files[noticeIndex].SHA256,
		IsolationScanHash: isolationHash,
	}
	contentHash, err := storyGraphBundleManifestHash(manifest)
	if err != nil {
		return BundleContentManifest{}, nil, err
	}
	manifest.ContentHash = contentHash
	encoded, err := encodeBundleManifest(manifest)
	if err != nil {
		return BundleContentManifest{}, nil, err
	}
	return manifest, encoded, nil
}

func DecodeStoryGraphBundleManifest(raw json.RawMessage) (BundleContentManifest, json.RawMessage, error) {
	var manifest BundleContentManifest
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return BundleContentManifest{}, nil, errors.New("invalid StoryGraph bundle manifest")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return BundleContentManifest{}, nil, errors.New("invalid StoryGraph bundle manifest")
	}
	if err := validateBundleManifest(manifest); err != nil {
		return BundleContentManifest{}, nil, err
	}
	encoded, err := encodeBundleManifest(manifest)
	if err != nil {
		return BundleContentManifest{}, nil, err
	}
	return manifest, encoded, nil
}

func validateBundleRoot(root string) error {
	for current, depth := filepath.Clean(root), 0; depth < 3; current, depth = filepath.Dir(current), depth+1 {
		info, err := os.Lstat(current)
		if err != nil || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("invalid StoryGraph bundle root")
		}
	}
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("invalid StoryGraph bundle root")
	}
	expected := StoryGraphBundlePaths()
	actual := make([]string, 0, len(expected))
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return errors.New("StoryGraph bundle contains a symlink")
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		relative, relativeErr := filepath.Rel(root, path)
		if relativeErr != nil || strings.HasPrefix(relative, "..") {
			return errors.New("StoryGraph bundle path escapes root")
		}
		actual = append(actual, filepath.ToSlash(relative))
		return nil
	})
	if err != nil {
		return err
	}
	slices.Sort(actual)
	if !slices.Equal(actual, expected) {
		return errors.New("StoryGraph bundle file set is invalid")
	}
	return nil
}

func readBundleFile(root, relative string) ([]byte, error) {
	path := filepath.Join(root, filepath.FromSlash(relative))
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("StoryGraph bundle resource is invalid")
	}
	content, err := os.ReadFile(path)
	if err != nil || !utf8.Valid(content) {
		return nil, errors.New("StoryGraph bundle contains invalid UTF-8")
	}
	return content, nil
}

func validateBundleManifest(manifest BundleContentManifest) error {
	if manifest.ContractID != StoryGraphBundleContractID || manifest.BundleEntrypoint != storyGraphBundleEntrypoint ||
		!hashPattern.MatchString(manifest.ProvenanceManifestHash) || !hashPattern.MatchString(manifest.NoticeHash) ||
		!hashPattern.MatchString(manifest.IsolationScanHash) || !hashPattern.MatchString(manifest.ContentHash) {
		return errors.New("invalid StoryGraph bundle manifest")
	}
	expectedPaths := StoryGraphBundlePaths()
	if len(manifest.Files) != len(expectedPaths) {
		return errors.New("invalid StoryGraph bundle file manifest")
	}
	for index, file := range manifest.Files {
		if file.Path != expectedPaths[index] || file.ByteLength < 1 || !hashPattern.MatchString(file.SHA256) {
			return errors.New("invalid StoryGraph bundle file manifest")
		}
	}
	noticeIndex := slices.IndexFunc(manifest.Files, func(file BundleFile) bool { return file.Path == storyGraphNoticePath })
	if noticeIndex < 0 || manifest.NoticeHash != manifest.Files[noticeIndex].SHA256 {
		return errors.New("invalid StoryGraph bundle NOTICE")
	}
	provenanceHash, err := storyGraphProvenanceHash(manifest.NoticeHash)
	if err != nil || provenanceHash != manifest.ProvenanceManifestHash {
		return errors.New("invalid StoryGraph bundle provenance")
	}
	isolationHash, err := storyGraphIsolationHash(manifest.Files)
	if err != nil || isolationHash != manifest.IsolationScanHash {
		return errors.New("invalid StoryGraph bundle isolation proof")
	}
	contentHash, err := storyGraphBundleManifestHash(manifest)
	if err != nil || contentHash != manifest.ContentHash {
		return errors.New("StoryGraph bundle content hash has drifted")
	}
	return nil
}

func storyGraphProvenanceHash(noticeHash string) (string, error) {
	return hashBundleValue(struct {
		ContractID string `json:"contract_id"`
		Origin     string `json:"origin"`
		SourceURL  string `json:"source_url"`
		License    string `json:"license_spdx"`
		NoticeHash string `json:"notice_hash"`
	}{
		ContractID: "storygraph-bundle-provenance-production",
		Origin:     "project_owned",
		SourceURL:  "https://github.com/StephenQiu30/lanverse",
		License:    "MIT",
		NoticeHash: noticeHash,
	})
}

func storyGraphIsolationHash(files []BundleFile) (string, error) {
	return hashBundleValue(struct {
		ContractID string       `json:"contract_id"`
		Rules      []string     `json:"rules"`
		Files      []BundleFile `json:"files"`
	}{
		ContractID: "storygraph-bundle-filesystem-isolation-production",
		Rules:      []string{"exact_file_set", "no_symlink", "relative_posix_path", "utf8"},
		Files:      files,
	})
}

func storyGraphBundleManifestHash(manifest BundleContentManifest) (string, error) {
	return hashBundleValue(struct {
		ContractID             string       `json:"contract_id"`
		BundleEntrypoint       string       `json:"bundle_entrypoint"`
		Files                  []BundleFile `json:"bundle_file_manifest"`
		ProvenanceManifestHash string       `json:"provenance_manifest_hash"`
		NoticeHash             string       `json:"notice_hash"`
		IsolationScanHash      string       `json:"isolation_scan_hash"`
	}{
		ContractID:             manifest.ContractID,
		BundleEntrypoint:       manifest.BundleEntrypoint,
		Files:                  manifest.Files,
		ProvenanceManifestHash: manifest.ProvenanceManifestHash,
		NoticeHash:             manifest.NoticeHash,
		IsolationScanHash:      manifest.IsolationScanHash,
	})
}

func hashBundleValue(value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return platformcanonical.Hash(raw)
}

func encodeBundleManifest(manifest BundleContentManifest) (json.RawMessage, error) {
	raw, err := json.Marshal(manifest)
	if err != nil {
		return nil, err
	}
	canonical, err := platformcanonical.JSON(raw)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(canonical), nil
}
