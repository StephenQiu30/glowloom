package sessiontoken

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
)

// readControlled rejects relative paths, symlinks and writable-by-other files.
func readControlled(path string, private bool) ([]byte, error) {
	invalid := errors.New("invalid controlled identity key file")
	if !filepath.IsAbs(path) {
		return nil, invalid
	}
	st, e := os.Lstat(path)
	if e != nil || !st.Mode().IsRegular() || st.Size() > 65536 || st.Mode().Perm()&0022 != 0 || private && st.Mode().Perm()&0077 != 0 {
		return nil, invalid
	}
	f, e := os.Open(path)
	if e != nil {
		return nil, invalid
	}
	defer func() { _ = f.Close() }()
	opened, e := f.Stat()
	if e != nil || !os.SameFile(st, opened) {
		return nil, invalid
	}
	b, e := io.ReadAll(io.LimitReader(f, 65537))
	if e != nil || len(b) > 65536 {
		return nil, invalid
	}
	return b, nil
}

// LoadFiles loads only the operator's trusted KID-to-absolute-public-path JSON map.
// With signing disabled, verification keys are still required for rollback.
func LoadFiles(privatePath, mapPath, kid string, signing bool) (*Codec, error) {
	b, e := readControlled(mapPath, false)
	if e != nil {
		return nil, e
	}
	var paths map[string]string
	if e = json.Unmarshal(b, &paths); e != nil {
		return nil, errors.New("invalid verification key map")
	}
	keys := make(map[string][]byte, len(paths))
	for id, path := range paths {
		key, e := readControlled(path, false)
		if e != nil {
			return nil, e
		}
		keys[id] = key
	}
	if !signing {
		return NewVerifier(keys)
	}
	private, e := readControlled(privatePath, true)
	if e != nil {
		return nil, e
	}
	defer clear(private)
	return New(kid, private, keys)
}

// LoadReceiptFile loads a dedicated raw 32-byte receipt key.
func LoadReceiptFile(path string) (*ReceiptProtector, error) {
	b, e := readControlled(path, true)
	if e != nil {
		return nil, e
	}
	defer clear(b)
	return NewReceiptProtector(b)
}
