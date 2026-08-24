package migrations

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"io/fs"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
)

var migrationNamePattern = regexp.MustCompile(`^(\d{6})_([a-z0-9_]+)\.sql$`)

//go:embed *.sql
var migrationFiles embed.FS

type Migration struct {
	Version  int64
	Name     string
	SQL      string
	Checksum string
}

func All() ([]Migration, error) {
	entries, err := fs.ReadDir(migrationFiles, ".")
	if err != nil {
		return nil, fmt.Errorf("read embedded migrations: %w", err)
	}
	migrations := make([]Migration, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".sql" {
			continue
		}
		matches := migrationNamePattern.FindStringSubmatch(entry.Name())
		if matches == nil {
			return nil, fmt.Errorf("migration filename %q does not follow the versioned contract", entry.Name())
		}
		version, parseErr := strconv.ParseInt(matches[1], 10, 64)
		if parseErr != nil {
			return nil, fmt.Errorf("parse migration version %q: %w", entry.Name(), parseErr)
		}
		content, readErr := migrationFiles.ReadFile(entry.Name())
		if readErr != nil {
			return nil, fmt.Errorf("read migration %q: %w", entry.Name(), readErr)
		}
		digest := sha256.Sum256(content)
		migrations = append(migrations, Migration{
			Version:  version,
			Name:     matches[2],
			SQL:      string(content),
			Checksum: hex.EncodeToString(digest[:]),
		})
	}
	sort.Slice(migrations, func(left, right int) bool {
		return migrations[left].Version < migrations[right].Version
	})
	for index, migration := range migrations {
		expected := int64(index + 1)
		if migration.Version != expected {
			return nil, fmt.Errorf(
				"migration versions must be contiguous: got %d, want %d",
				migration.Version,
				expected,
			)
		}
	}
	return migrations, nil
}
