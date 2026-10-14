package credentialproof

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"os"

	"github.com/google/uuid"
)

type keyFile struct {
	KeyID         string `json:"key_id"`
	ProofKey      []byte `json:"proof_key"`
	EncryptionKey []byte `json:"encryption_key"`
}

// CreateFile is an explicit operator action. Process startup never replaces keys.
func CreateFile(path string) error {
	k := keyFile{KeyID: uuid.NewString(), ProofKey: make([]byte, 32), EncryptionKey: make([]byte, 32)}
	defer clear(k.ProofKey)
	defer clear(k.EncryptionKey)
	if _, err := rand.Read(k.ProofKey); err != nil {
		return err
	}
	if _, err := rand.Read(k.EncryptionKey); err != nil {
		return err
	}
	data, err := json.Marshal(k)
	if err != nil {
		return err
	}
	defer clear(data)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return errors.New("create identity key file failed; existing files are never replaced")
	}
	_, writeErr := f.Write(data)
	syncErr := f.Sync()
	closeErr := f.Close()
	if writeErr != nil || syncErr != nil || closeErr != nil {
		return errors.New("persist identity key file failed; inspect the file before retrying")
	}
	return nil
}

// LoadFile accepts only a bounded, private regular file. Errors never include key data.
func LoadFile(path string) (*Protector, error) {
	invalid := errors.New("identity key file must be a private regular file containing valid independent keys")
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 4096 {
		return nil, invalid
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, invalid
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil || !os.SameFile(info, opened) || !opened.Mode().IsRegular() || opened.Mode().Perm()&0077 != 0 {
		return nil, invalid
	}
	data, err := io.ReadAll(io.LimitReader(f, 4097))
	if err != nil || len(data) > 4096 {
		return nil, invalid
	}
	defer clear(data)
	var k keyFile
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&k); err != nil {
		return nil, invalid
	}
	defer clear(k.ProofKey)
	defer clear(k.EncryptionKey)
	if decoder.Decode(new(any)) != io.EOF || len(k.KeyID) > 128 {
		return nil, invalid
	}
	p, err := New(k.KeyID, k.ProofKey, k.EncryptionKey)
	if err != nil {
		return nil, invalid
	}
	return p, nil
}
