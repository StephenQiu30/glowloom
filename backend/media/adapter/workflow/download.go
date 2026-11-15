// Package workflow implements Media-owned fetch and processing policy.
package workflow

import "github.com/StephenQiu30/glowloom-server/backend/internal/platform/fetch"

// maxResultBytes retains Media's existing renderer envelope as well as fetch policy.
const maxResultBytes int64 = 2_000_000_000

var (
	// ErrUnsafeResultURL means an origin, redirect or address violates transfer policy.
	ErrUnsafeResultURL = fetch.ErrUnsafeResultURL
	// ErrResultTooLarge means the body exceeded the original byte bound.
	ErrResultTooLarge = fetch.ErrResultTooLarge
	// ErrUnsupportedMedia means actual bytes are outside Media's MIME contract.
	ErrUnsupportedMedia = fetch.ErrUnsupportedMedia
	// ErrResultUnavailable means no usable remote result was established.
	ErrResultUnavailable = fetch.ErrResultUnavailable
	// ErrResultExpired means the original remote result is inaccessible.
	ErrResultExpired = fetch.ErrResultExpired
)

// IPResolver is the same injected technical DNS port used by all downloads.
type IPResolver = fetch.IPResolver

// DownloadPolicy retains the original approved-origin, TLS and size configuration.
type DownloadPolicy = fetch.DownloadPolicy

// SecureDownloader uses pinned DNS and Media's fixed accepted MIME policy.
type SecureDownloader = fetch.SecureDownloader

// Downloaded is an owned technical byte file, without any Media business identity.
type Downloaded = fetch.Downloaded

// NewSecureDownloader preserves Media's MIME policy over shared transfer machinery.
func NewSecureDownloader(policy DownloadPolicy) (*SecureDownloader, error) {
	policy.AcceptedMIME = []string{"image/jpeg", "image/png", "image/gif", "image/webp", "video/mp4", "audio/mpeg", "audio/wave", "audio/x-wav", "audio/ogg"}
	return fetch.NewSecureDownloader(policy)
}
