package application

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"image"
	_ "image/jpeg" // Register the allowed JPEG decoder.
	"image/png"
	"io"
	"path"
	"time"

	"github.com/google/uuid"
	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp" // Register the allowed WebP decoder.

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

// AvatarMaxBytes bounds uploaded bytes before decoding.
const AvatarMaxBytes = 5 * 1024 * 1024

// AvatarOutputMaxBytes safely bounds an eight-bit 1024x1024 PNG, independently
// of the compressed input cap. Raw RGBA pixels use at most four MiB.
const AvatarOutputMaxBytes = 6 * 1024 * 1024

// AvatarOutputMaxDimension bounds the long edge without enlarging small inputs.
const AvatarOutputMaxDimension = 1024

// AvatarImage holds the metadata derived from a bounded decoded image.
type AvatarImage struct {
	Bytes         []byte
	Width, Height int32
	MIMEType      string
}

// NormalizeAvatar checks real image encoding and rejects animated containers.
func NormalizeAvatar(data []byte, mime string) (AvatarImage, error) {
	if len(data) > AvatarMaxBytes {
		return AvatarImage{}, ErrUploadTooLarge
	}
	if len(data) == 0 {
		return AvatarImage{}, ErrUnsupportedUpload
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return AvatarImage{}, ErrUnsupportedUpload
	}
	expected := map[string]string{"jpeg": "image/jpeg", "png": "image/png", "webp": "image/webp"}
	if expected[format] == "" || expected[format] != mime {
		return AvatarImage{}, ErrUnsupportedUpload
	}
	if cfg.Width < 1 || cfg.Height < 1 || cfg.Width > 4096 || cfg.Height > 4096 {
		return AvatarImage{}, ErrUploadTooLarge
	}
	// APNG and animated WebP carry explicit chunk markers. Parse lengths instead of
	// scanning pixel bytes, which could reject valid static images accidentally.
	if animatedAvatar(data, format) {
		return AvatarImage{}, ErrUnsupportedUpload
	}
	im, decoded, err := image.Decode(bytes.NewReader(data))
	if err != nil || decoded != format || im.Bounds().Dx() != cfg.Width || im.Bounds().Dy() != cfg.Height {
		return AvatarImage{}, ErrUnsupportedUpload
	}
	width, height := cfg.Width, cfg.Height
	if width > AvatarOutputMaxDimension || height > AvatarOutputMaxDimension {
		if width >= height {
			height = max(1, height*AvatarOutputMaxDimension/width)
			width = AvatarOutputMaxDimension
		} else {
			width = max(1, width*AvatarOutputMaxDimension/height)
			height = AvatarOutputMaxDimension
		}
	}
	normalized := image.NewNRGBA(image.Rect(0, 0, width, height))
	draw.BiLinear.Scale(normalized, normalized.Bounds(), im, im.Bounds(), draw.Src, nil)
	var out bytes.Buffer
	w := &avatarWriter{dst: &out, remaining: AvatarOutputMaxBytes}
	if err := png.Encode(w, normalized); err != nil {
		return AvatarImage{}, ErrUploadTooLarge
	}
	return AvatarImage{Bytes: out.Bytes(), Width: int32(width), Height: int32(height), MIMEType: "image/png"}, nil
}
func animatedAvatar(b []byte, format string) bool {
	if format == "png" {
		for pos := 8; pos+12 <= len(b); {
			n := int64(b[pos])<<24 | int64(b[pos+1])<<16 | int64(b[pos+2])<<8 | int64(b[pos+3])
			if string(b[pos+4:pos+8]) == "acTL" {
				return true
			}
			if n > int64(len(b)-pos-12) {
				return true
			}
			pos += int(n) + 12
		}
	}
	if format == "webp" {
		for pos := 12; pos+8 <= len(b); {
			n := int64(b[pos+4]) | int64(b[pos+5])<<8 | int64(b[pos+6])<<16 | int64(b[pos+7])<<24
			kind := string(b[pos : pos+4])
			if kind == "ANIM" || kind == "ANMF" || (kind == "VP8X" && n > 0 && pos+8 < len(b) && b[pos+8]&2 != 0) {
				return true
			}
			if n > int64(len(b)-pos-8) {
				return true
			}
			pos += 8 + int(n) + (int(n) & 1)
		}
	}
	return false
}

type avatarWriter struct {
	dst       io.Writer
	remaining int
}

func (w *avatarWriter) Write(b []byte) (int, error) {
	if len(b) > w.remaining {
		return 0, ErrUploadTooLarge
	}
	n, err := w.dst.Write(b)
	w.remaining -= n
	return n, err
}

// AvatarActorAccess consumes the identity-owned live-session authority.
type AvatarActorAccess interface {
	CheckSession(context.Context, identityapp.Principal, time.Time) error
}

// AvatarRepository publishes only service-reencoded, account-owned images.
type AvatarRepository interface {
	AuthorizeAvatar(context.Context, identityapp.Principal, time.Time) error
	CommitAvatar(context.Context, identityapp.Principal, domain.MediaAsset, time.Time) error
	AvatarExists(context.Context, uuid.UUID) (bool, error)
}

// AvatarService reuses the configured media object pipeline and asset storage.
type AvatarService struct {
	repo    AvatarRepository
	objects UploadObjects
	now     func() time.Time
}

// NewAvatarService supplies the existing media storage connection.
func NewAvatarService(repo AvatarRepository, objects UploadObjects, now func() time.Time) *AvatarService {
	return &AvatarService{repo: repo, objects: objects, now: now}
}

// Authorize rejects invalid live sessions before reading an upload body.
func (s *AvatarService) Authorize(ctx context.Context, a identityapp.Principal) error {
	if s == nil || s.repo == nil || s.objects == nil {
		return ErrUnavailable
	}
	return s.repo.AuthorizeAvatar(ctx, a, s.now().UTC())
}

// Upload returns an eligible asset; binding still uses identity's profile CAS.
func (s *AvatarService) Upload(ctx context.Context, a identityapp.Principal, data []byte, mime string) (LibraryAssetSummary, error) {
	if err := s.Authorize(ctx, a); err != nil {
		return LibraryAssetSummary{}, err
	}
	im, err := NormalizeAvatar(data, mime)
	if err != nil {
		return LibraryAssetSummary{}, err
	}
	now := s.now().UTC()
	id := uuid.New()
	key := path.Join("personal", a.OrgID.String(), a.ID.String(), "image", now.Format("2006"), now.Format("01"), id.String()+".png")
	digest := sha256.Sum256(im.Bytes)
	hash := hex.EncodeToString(digest[:])
	size := int64(len(im.Bytes))
	codec := "png"
	detail, _ := json.Marshal(map[string]any{"avatar": map[string]any{"version": 1, "method": "server_reencode_png"}})
	asset := domain.MediaAsset{ID: id, Personal: &domain.PersonalOwnership{OrgID: a.OrgID, ActorID: a.ID}, Kind: domain.KindImage, Origin: domain.OriginUpload, Status: domain.StatusReady, ObjectKey: key, FileName: "avatar.png", MimeType: "image/png", ByteSize: size, SHA256: &hash, Width: &im.Width, Height: &im.Height, Codec: &codec, ModerationStatus: domain.ModerationPassed, ModerationDetail: detail, Revision: 1, CreateTime: now, UpdateTime: now}
	if err := asset.Validate(); err != nil {
		return LibraryAssetSummary{}, err
	}
	if err := s.objects.PutIfAbsent(ctx, key, bytes.NewReader(im.Bytes), size, "image/png", hash); err != nil {
		return LibraryAssetSummary{}, err
	}
	info, err := s.objects.Stat(ctx, key)
	if err == nil && (info.Size != size || info.ContentType != "image/png" || info.SHA256 != hash) {
		err = ErrObjectMismatch
	}
	if err == nil {
		err = s.repo.CommitAvatar(ctx, a, asset, s.now().UTC())
	}
	if err != nil {
		exists, checkErr := s.repo.AvatarExists(ctx, id)
		if checkErr == nil && !exists {
			err = errors.Join(err, s.objects.Remove(ctx, key))
		}
		return LibraryAssetSummary{}, err
	}
	return PersonalUploadSummary(asset), nil
}

// AvatarReader loads only the current account's bound, eligible avatar.
type AvatarReader interface {
	AvatarTransaction(context.Context, identityapp.Principal, time.Time, func(domain.MediaAsset) error) error
}

// AvatarPreview contains a short-lived private URL without exposing its object key.
type AvatarPreview struct {
	Asset     LibraryAssetSummary `json:"asset"`
	URL       string              `json:"url"`
	ExpiresAt time.Time           `json:"expires_at"`
}

// AvatarQuery authorizes every preview issuance using the identity-owned session checker.
type AvatarQuery struct {
	reader AvatarReader
	signer PreviewSigner
	now    func() time.Time
}

// NewAvatarQuery injects the existing private bucket signer and live avatar reader.
func NewAvatarQuery(reader AvatarReader, signer PreviewSigner, now func() time.Time) *AvatarQuery {
	return &AvatarQuery{reader: reader, signer: signer, now: now}
}

// Read grants only a sixty-second preview of the currently bound avatar.
func (q *AvatarQuery) Read(ctx context.Context, a identityapp.Principal) (AvatarPreview, error) {
	if q == nil || q.reader == nil || q.signer == nil {
		return AvatarPreview{}, ErrUnavailable
	}
	now := q.now().UTC()
	var asset domain.MediaAsset
	if err := q.reader.AvatarTransaction(ctx, a, now, func(current domain.MediaAsset) error { asset = current; return nil }); err != nil {
		return AvatarPreview{}, err
	}
	// MinIO may resolve an uncached bucket region over the network. Sign without
	// database locks, then recheck the current binding before exposing the URL.
	signCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	url, err := q.signer.PresignGet(signCtx, asset.ObjectKey, time.Minute)
	if err != nil {
		return AvatarPreview{}, err
	}
	err = q.reader.AvatarTransaction(ctx, a, q.now().UTC(), func(current domain.MediaAsset) error {
		if current.ID != asset.ID || current.ObjectKey != asset.ObjectKey {
			return ErrNotFound
		}
		return nil
	})
	if err != nil {
		return AvatarPreview{}, err
	}
	return AvatarPreview{Asset: PersonalUploadSummary(asset), URL: url, ExpiresAt: now.Add(time.Minute)}, nil
}
