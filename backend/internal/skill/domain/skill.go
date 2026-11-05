package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Skill errors classify invalid packages and unavailable skill resources.
var (
	ErrSkillInvalidName   = errors.New("invalid skill name")
	ErrSkillPathTraversal = errors.New("path traversal detected in skill resource path")
	ErrSkillNotFound      = errors.New("skill not found")
	ErrSkillEmptyContent  = errors.New("skill SKILL.md cannot be empty")
)

var skillNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_.-]{1,63}$`)

// SkillPackage represents an immutable, versioned skill bundle.
type SkillPackage struct {
	ID          uuid.UUID         `json:"id"`
	Name        string            `json:"name"`
	Version     string            `json:"version"`
	Description string            `json:"description"`
	SkillMD     string            `json:"skill_md"`
	Resources   map[string]string `json:"resources,omitempty"` // relative_path -> text content
	Hash        string            `json:"hash"`
	CreatedAt   time.Time         `json:"created_at"`
}

// NewSkillPackage constructs and validates a new SkillPackage, computing its cryptographic hash.
func NewSkillPackage(name, version, description, skillMD string, resources map[string]string) (*SkillPackage, error) {
	if !skillNamePattern.MatchString(name) {
		return nil, fmt.Errorf("%w: %q", ErrSkillInvalidName, name)
	}
	if strings.TrimSpace(skillMD) == "" {
		return nil, ErrSkillEmptyContent
	}
	if strings.TrimSpace(version) == "" {
		version = "1.0.0"
	}

	cleanResources := make(map[string]string, len(resources))
	for relPath, content := range resources {
		// Strict path traversal defense
		cleaned := filepath.Clean(relPath)
		if strings.HasPrefix(cleaned, "..") || filepath.IsAbs(cleaned) || strings.Contains(relPath, "../") || strings.Contains(relPath, `..\`) {
			return nil, fmt.Errorf("%w: %q", ErrSkillPathTraversal, relPath)
		}
		cleanResources[cleaned] = content
	}

	pkg := &SkillPackage{
		ID:          uuid.New(),
		Name:        name,
		Version:     version,
		Description: description,
		SkillMD:     skillMD,
		Resources:   cleanResources,
		CreatedAt:   time.Now().UTC(),
	}

	pkg.Hash = pkg.ComputeHash()
	return pkg, nil
}

// ComputeHash generates a deterministic SHA256 checksum over the package contents.
func (p *SkillPackage) ComputeHash() string {
	resKeys := make([]string, 0, len(p.Resources))
	for k := range p.Resources {
		resKeys = append(resKeys, k)
	}
	sort.Strings(resKeys)

	var sb strings.Builder
	fmt.Fprintf(&sb, "%s|%s|%s|%s|", p.Name, p.Version, p.Description, p.SkillMD)
	for _, k := range resKeys {
		fmt.Fprintf(&sb, "%s:%s|", k, p.Resources[k])
	}

	sum := sha256.Sum256([]byte(sb.String()))
	return hex.EncodeToString(sum[:])
}
