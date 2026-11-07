package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/url"
	"strings"

	"github.com/google/uuid"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

var (
	// ErrInvalidVideoLink rejects unsupported or unsafe share-page input.
	ErrInvalidVideoLink = errors.New("invalid_video_link")
	// ErrVideoLinkUnresolvable means no public verified video was available.
	ErrVideoLinkUnresolvable = errors.New("video_link_unresolvable")
	// ErrVideoLinkUnavailable means a bounded external dependency failed.
	ErrVideoLinkUnavailable = errors.New("video_link_unavailable")
)

// VideoLinkRequest is an explicitly reviewed public platform share page, never
// an arbitrary remote URL or a browser-supplied assertion of successful parsing.
type VideoLinkRequest struct {
	URL                  string `json:"url"`
	LocalReviewConfirmed bool   `json:"local_review_confirmed"`
}

// CanonicalVideoLinkURL closes supported share-page origins before network I/O.
func CanonicalVideoLinkURL(raw string) (string, error) {
	if len(raw) > 4096 {
		return "", ErrInvalidVideoLink
	}
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" || u.User != nil || u.Fragment != "" || u.Opaque != "" || u.Port() != "" && u.Port() != "443" {
		return "", ErrInvalidVideoLink
	}
	host := strings.ToLower(u.Hostname())
	switch host {
	case "douyin.com", "www.douyin.com", "v.douyin.com", "iesdouyin.com", "www.iesdouyin.com", "toutiao.com", "www.toutiao.com", "m.toutiao.com", "v.toutiao.com", "ixigua.com", "www.ixigua.com", "m.ixigua.com", "v.ixigua.com":
	default:
		return "", ErrInvalidVideoLink
	}
	u.Host = host
	return u.String(), nil
}

// VideoLinkResolver returns actual verified bytes owned by the caller.
type VideoLinkResolver interface {
	Resolve(context.Context, string, int64) (*Downloaded, error)
}

// VideoLinkUploads keeps personal authorization and publication in the media owner.
type VideoLinkUploads interface {
	AuthorizePersonal(context.Context, identity.Principal) error
	BeginUpload(context.Context, identity.Principal, domain.LibraryScope, uuid.UUID) (UploadTarget, error)
	LookupPersonalVideoLink(context.Context, identity.Principal, uuid.UUID, string) (PersonalUploadResult, bool, error)
	UploadPersonal(context.Context, UploadInput) (PersonalUploadResult, error)
}

// VideoLinkService resolves bounded public metadata and publishes through the
// same reviewed, idempotent private-upload pipeline as a local video.
type VideoLinkService struct {
	uploads  VideoLinkUploads
	resolver VideoLinkResolver
}

// NewVideoLinkService keeps authorization and publication with the upload owner.
func NewVideoLinkService(uploads VideoLinkUploads, resolver VideoLinkResolver) *VideoLinkService {
	return &VideoLinkService{uploads: uploads, resolver: resolver}
}

// Import returns a current permanent receipt or publishes actual source bytes.
func (s *VideoLinkService) Import(ctx context.Context, actor identity.Principal, key, requestID uuid.UUID, in VideoLinkRequest) (PersonalUploadResult, error) {
	if s == nil || s.uploads == nil || s.resolver == nil {
		return PersonalUploadResult{}, ErrUnavailable
	}
	raw, err := CanonicalVideoLinkURL(in.URL)
	if err != nil || !in.LocalReviewConfirmed || key == uuid.Nil || requestID == uuid.Nil {
		return PersonalUploadResult{}, ErrInvalidVideoLink
	}
	if err := s.uploads.AuthorizePersonal(ctx, actor); err != nil {
		return PersonalUploadResult{}, err
	}
	// The URL itself is neither persisted nor exposed in a filename. Its complete
	// SHA binds the share-page request to the existing permanent upload receipt.
	sum := sha256.Sum256([]byte(raw))
	name := "video-link-" + hex.EncodeToString(sum[:]) + ".mp4"
	if result, found, err := s.uploads.LookupPersonalVideoLink(ctx, actor, key, name); err != nil || found {
		if err == nil && (result.Asset.ID == uuid.Nil || result.Asset.Kind != "video") {
			return PersonalUploadResult{}, ErrUploadConflict
		}
		return result, err
	}
	target, err := s.uploads.BeginUpload(ctx, actor, domain.LibraryScope{Kind: domain.LibraryPersonal}, key)
	if err != nil {
		return PersonalUploadResult{}, err
	}
	if target.Policy.MaxVideoBytes < 1 || target.Policy.MaxVideoBytes > MaxUploadVideoBytes {
		return PersonalUploadResult{}, ErrStorageNotReady
	}
	file, err := s.resolver.Resolve(ctx, raw, target.Policy.MaxVideoBytes)
	if err != nil {
		return PersonalUploadResult{}, err
	}
	if file == nil {
		return PersonalUploadResult{}, ErrVideoLinkUnresolvable
	}
	defer func() { _ = file.Close() }()
	if file.MIMEType != "video/mp4" || file.Size < 1 || file.Size > target.Policy.MaxVideoBytes {
		return PersonalUploadResult{}, ErrUnsupportedUpload
	}
	result, err := s.uploads.UploadPersonal(ctx, UploadInput{Actor: actor, Request: UploadRequest{Key: key, RequestID: requestID, FileName: name, VideoLink: true}, File: file, LocalReviewConfirmed: true, Target: &target})
	if err != nil {
		return PersonalUploadResult{}, err
	}
	if result.Asset.ID == uuid.Nil || result.Asset.Kind != "video" {
		return PersonalUploadResult{}, ErrUnavailable
	}
	return result, nil
}

type personalVideoLinkReceipts interface {
	LookupPersonalVideoLink(context.Context, identity.Principal, uuid.UUID, string) (PersonalUploadResult, bool, error)
}

// LookupPersonalVideoLink reauthorizes an existing receipt before any new egress.
func (s *UploadService) LookupPersonalVideoLink(ctx context.Context, actor identity.Principal, key uuid.UUID, name string) (PersonalUploadResult, bool, error) {
	if s == nil {
		return PersonalUploadResult{}, false, ErrUnavailable
	}
	store, ok := s.personal.(personalVideoLinkReceipts)
	if !ok {
		return PersonalUploadResult{}, false, ErrUnavailable
	}
	return store.LookupPersonalVideoLink(ctx, actor, key, name)
}
