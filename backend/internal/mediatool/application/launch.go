package application

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"strings"
	"time"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/domain"
	"github.com/google/uuid"
)

// LaunchReceipt is an expiring capability, never a new authority to sign media.
type LaunchReceipt struct {
	TokenHash                    string
	JobID, ArtifactID, ProjectID uuid.UUID
	Revision                     int64
	Actor                        identity.Principal
	Body                         []byte
	ExpiresAt                    time.Time
}

// LaunchStore owns capability persistence and live authorization revalidation.
type LaunchStore interface {
	SaveLaunch(context.Context, LaunchReceipt) error
	ReadLaunch(context.Context, string, time.Time) ([]byte, error)
	PruneLaunches(context.Context, time.Time, int) (int64, error)
}

func (q *ExportQuery) WithLaunchOrigin(origin string) *ExportQuery {
	u, e := url.Parse(origin)
	if e == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Host != "" && u.User == nil && u.Path == "" && u.RawQuery == "" && u.Fragment == "" {
		q.launchOrigin = strings.TrimRight(origin, "/")
	}
	return q
}
func launchHash(token string) (string, bool) {
	if len(token) != 64 || strings.Trim(token, "0123456789abcdef") != "" {
		return "", false
	}
	b, e := hex.DecodeString(token)
	if e != nil {
		return "", false
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:]), true
}
func (q *ExportQuery) nativeLaunch(ctx context.Context, actor identity.Principal, job domain.ExportJob, a domain.Artifact, preview *ExportPreview) error {
	if job.OutputKind != domain.OutputJianying || job.Status != domain.Succeeded {
		return nil
	}
	store, ok := q.store.(LaunchStore)
	if !ok || q.launchOrigin == "" {
		return ErrUnavailable
	}
	now := time.Now().UTC()
	expires := preview.ExpiresAt
	if expires.After(now.Add(10 * time.Minute)) {
		expires = now.Add(10 * time.Minute)
	}
	if !expires.After(now) {
		return ErrConflict
	}
	name := "时间线"
	if job.Settings != nil {
		name = job.Settings.FileName
	}
	body, e := json.Marshal(map[string]any{"short_video_draft": map[string]any{"batch": false, "folder_name": name, "drafts": []map[string]string{{"draft_url": preview.URL, "draft_name": name}}}})
	if e != nil {
		return e
	}
	var entropy [32]byte
	if _, e = rand.Read(entropy[:]); e != nil {
		return e
	}
	token := hex.EncodeToString(entropy[:])
	hash, _ := launchHash(token)
	actor.LoginName = ""
	actor.DisplayName = ""
	if e = store.SaveLaunch(ctx, LaunchReceipt{TokenHash: hash, JobID: job.ID, ArtifactID: a.ID, ProjectID: job.ProjectID, Revision: job.Revision, Actor: actor, Body: body, ExpiresAt: expires}); e != nil {
		return e
	}
	preview.LaunchURL = q.launchOrigin + "/api/mediaexports/launch?token=" + token
	preview.ExpiresAt = expires
	return nil
}

// Launch serves only the already-issued envelope; it cannot refresh or sign URLs.
func (q *ExportQuery) Launch(ctx context.Context, token string) ([]byte, error) {
	hash, ok := launchHash(token)
	if !ok || q == nil {
		return nil, ErrNotFound
	}
	store, ok := q.store.(LaunchStore)
	if !ok {
		return nil, ErrUnavailable
	}
	return store.ReadLaunch(ctx, hash, time.Now().UTC())
}
