package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"go.yaml.in/yaml/v3"
)

// Skill errors classify invalid packages and unavailable skill resources.
var (
	ErrSkillInvalidName   = errors.New("invalid skill name")
	ErrSkillPathTraversal = errors.New("path traversal detected in skill resource path")
	ErrSkillNotFound      = errors.New("skill not found")
	ErrSkillEmptyContent  = errors.New("skill SKILL.md cannot be empty")
	ErrSkillMetadata      = errors.New("invalid SKILL.md frontmatter")
)

var skillNamePattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// SkillPackage represents an immutable, versioned skill bundle.
type SkillPackage struct {
	ID          uuid.UUID         `json:"id"`
	Name        string            `json:"name"`
	Version     string            `json:"version"`
	Description string            `json:"description"`
	SkillMD     string            `json:"skill_md"`
	Resources   map[string][]byte `json:"resources,omitempty"` // relative path -> original bytes
	Hash        string            `json:"hash"`
	CreatedAt   time.Time         `json:"created_at"`
}

// NewSkillPackage constructs and validates a new SkillPackage, computing its cryptographic hash.
func NewSkillPackage(name, version, description, skillMD string, resources map[string][]byte) (*SkillPackage, error) {
	if len(name) > 64 || !skillNamePattern.MatchString(name) {
		return nil, fmt.Errorf("%w: %q", ErrSkillInvalidName, name)
	}
	if strings.TrimSpace(skillMD) == "" {
		return nil, ErrSkillEmptyContent
	}
	if strings.TrimSpace(version) == "" {
		version = "1.0.0"
	}

	cleanResources := make(map[string][]byte, len(resources))
	for relPath, content := range resources {
		// Strict path traversal defense
		cleaned := path.Clean(relPath)
		if cleaned != relPath || cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") || path.IsAbs(cleaned) || strings.ContainsAny(relPath, "\\\x00") || relPath == "SKILL.md" {
			return nil, fmt.Errorf("%w: %q", ErrSkillPathTraversal, relPath)
		}
		cleanResources[cleaned] = content
	}

	if !utf8.ValidString(skillMD) || strings.ContainsRune(skillMD, 0) {
		return nil, ErrSkillMetadata
	}
	lines := strings.Split(strings.ReplaceAll(skillMD, "\r\n", "\n"), "\n")
	if lines[0] != "---" {
		return nil, ErrSkillMetadata
	}
	end := 1
	for end < len(lines) && lines[end] != "---" {
		end++
	}
	var meta struct {
		Name        string `yaml:"name"`
		Description string `yaml:"description"`
	}
	if end == len(lines) || yaml.Unmarshal([]byte(strings.Join(lines[1:end], "\n")), &meta) != nil ||
		meta.Name != name || strings.TrimSpace(meta.Description) == "" || utf8.RuneCountInString(meta.Description) > 1024 ||
		strings.TrimSpace(strings.Join(lines[end+1:], "\n")) == "" {
		return nil, ErrSkillMetadata
	}
	if description != "" && description != meta.Description {
		return nil, ErrSkillMetadata
	}

	pkg := &SkillPackage{
		ID:          uuid.New(),
		Name:        name,
		Version:     version,
		Description: meta.Description,
		SkillMD:     skillMD,
		Resources:   cleanResources,
		CreatedAt:   time.Now().UTC(),
	}

	pkg.Hash = pkg.ComputeHash()
	return pkg, nil
}

// ComputeHash generates a deterministic SHA256 checksum over the package contents.
func (p *SkillPackage) ComputeHash() string {
	// JSON object keys are sorted by the encoder and byte values are base64.
	data, _ := json.Marshal(struct {
		Name        string
		Version     string
		Description string
		Body        string
		Files       map[string][]byte
	}{p.Name, p.Version, p.Description, p.SkillMD, p.Resources})
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
