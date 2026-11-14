// Package application defines the file-only contract for media normalization.
// It has no business storage, identity, or workflow dependencies.
package application

import (
	"errors"
	"os"
)

// MaxBytes bounds both source recordings and canonical output.
const MaxBytes int64 = 500 << 20

var (
	// ErrInvalidInput reports a source whose actual bytes do not match its receipt.
	ErrInvalidInput = errors.New("invalid normalization input")
	// ErrUnsupported reports content outside the supported recording contract.
	ErrUnsupported = errors.New("unsupported normalization input")
	// ErrTooLarge reports a source or canonical output exceeding MaxBytes.
	ErrTooLarge = errors.New("normalization file too large")
)

// File owns a private temporary file and its independently verified receipt.
// Passing a source to a normalizer only borrows it; ownership is not transferred.
type File struct {
	File     *os.File
	Size     int64
	SHA256   string
	MIMEType string
}

// Close closes and removes the owned temporary file. Repeated calls are safe.
func (f *File) Close() error {
	if f == nil || f.File == nil {
		return nil
	}
	name := f.File.Name()
	closeErr := f.File.Close()
	removeErr := os.Remove(name)
	f.File = nil
	return errors.Join(closeErr, removeErr)
}

// Result contains canonical H264 MP4 bytes and the verified source codec.
type Result struct {
	File        *File
	SourceCodec string
}
