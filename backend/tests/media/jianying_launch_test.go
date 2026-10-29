package media_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	toolhttp "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/adapter/http"
	app "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/application"
	tool "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/domain"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type launchStore struct {
	app.ExportStore
	job      tool.ExportJob
	artifact tool.Artifact
	saved    app.LaunchReceipt
	denied   bool
	reads    int
}

func (s *launchStore) Get(context.Context, identity.Principal, uuid.UUID, uuid.UUID) (tool.ExportJob, error) {
	if s.denied {
		return tool.ExportJob{}, app.ErrNotFound
	}
	return s.job, nil
}
func (s *launchStore) PreviewArtifact(context.Context, identity.Principal, uuid.UUID, uuid.UUID) (tool.ExportJob, tool.Artifact, error) {
	return s.job, s.artifact, nil
}
func (s *launchStore) SaveLaunch(_ context.Context, r app.LaunchReceipt) error {
	s.saved = r
	return nil
}
func (s *launchStore) ReadLaunch(_ context.Context, hash string, now time.Time) ([]byte, error) {
	s.reads++
	if s.denied || hash != s.saved.TokenHash || !now.Before(s.saved.ExpiresAt) || s.saved.Revision != s.job.Revision || s.job.Status != tool.Succeeded {
		return nil, app.ErrNotFound
	}
	return s.saved.Body, nil
}
func (s *launchStore) PruneLaunches(context.Context, time.Time, int) (int64, error) { return 0, nil }
func launchFixture(t *testing.T) (*launchStore, *app.ExportQuery) {
	t.Helper()
	a := tool.Artifact{ID: uuid.New(), JobID: uuid.New(), ProjectID: uuid.New(), Format: tool.OutputJianying, ObjectKey: "private/native.zip", SHA256: strings.Repeat("a", 64)}
	s := &launchStore{artifact: a, job: tool.ExportJob{ID: a.JobID, ProjectID: a.ProjectID, Revision: 3, OutputKind: tool.OutputJianying, Status: tool.Succeeded, ArtifactID: &a.ID, Settings: &tool.ExportSettings{FileName: "命名验收"}}}
	q := app.NewExportQuery(s, &projectObjects{}, nil).WithLaunchOrigin("http://localhost:3000")
	return s, q
}
func TestJianyingLaunchCapabilityBoundToAuthorizedArtifactAndExpiry(t *testing.T) {
	s, q := launchFixture(t)
	actor := identity.Principal{ID: uuid.New(), OrgID: uuid.New(), LoginName: "must-not-copy", DisplayName: "must-not-copy"}
	before := time.Now()
	p, e := q.Preview(t.Context(), actor, s.job.ProjectID, s.job.ID)
	if e != nil {
		t.Fatal(e)
	}
	u, e := url.Parse(p.LaunchURL)
	if e != nil || u.Host != "localhost:3000" || u.Path != "/api/mediaexports/launch" {
		t.Fatalf("invalid launch URL: %v", e)
	}
	token := u.Query().Get("token")
	entropy, e := hex.DecodeString(token)
	if e != nil || len(entropy) != 32 {
		t.Fatal("weak capability")
	}
	hash := sha256.Sum256(entropy)
	if s.saved.TokenHash != hex.EncodeToString(hash[:]) || s.saved.TokenHash == token || s.saved.JobID != s.job.ID || s.saved.ArtifactID != s.artifact.ID || s.saved.Revision != 3 || s.saved.Actor.ID != actor.ID || s.saved.Actor.LoginName != "" || s.saved.Actor.DisplayName != "" || s.saved.ExpiresAt.After(before.Add(10*time.Minute+time.Second)) || !p.ExpiresAt.Equal(s.saved.ExpiresAt) {
		t.Fatal("unbound or overshared receipt")
	}
	body, e := q.Launch(t.Context(), token)
	if e != nil {
		t.Fatal(e)
	}
	var envelope struct {
		Draft struct {
			Batch  bool   `json:"batch"`
			Folder string `json:"folder_name"`
			Drafts []struct {
				URL  string `json:"draft_url"`
				Name string `json:"draft_name"`
			} `json:"drafts"`
		} `json:"short_video_draft"`
	}
	if json.Unmarshal(body, &envelope) != nil || envelope.Draft.Batch || envelope.Draft.Folder != "命名验收" || len(envelope.Draft.Drafts) != 1 || envelope.Draft.Drafts[0].URL != p.URL || envelope.Draft.Drafts[0].Name != "命名验收" {
		t.Fatal("wrong native envelope")
	}
	for _, bad := range []string{"", strings.Repeat("a", 63), strings.Repeat("A", 64), "https://evil.invalid/"} {
		n := s.reads
		if _, e = q.Launch(t.Context(), bad); !errors.Is(e, app.ErrNotFound) || s.reads != n {
			t.Fatal("bad token reached store")
		}
	}
	s.saved.ExpiresAt = time.Now().Add(-time.Second)
	if _, e = q.Launch(t.Context(), token); !errors.Is(e, app.ErrNotFound) {
		t.Fatal("expired capability accepted")
	}
	s.saved.ExpiresAt = time.Now().Add(time.Minute)
	s.job.Revision++
	if _, e = q.Launch(t.Context(), token); !errors.Is(e, app.ErrNotFound) {
		t.Fatal("changed job accepted")
	}
	s.job.Revision--
	s.job.Status = tool.Cancelled
	if _, e = q.Launch(t.Context(), token); !errors.Is(e, app.ErrNotFound) {
		t.Fatal("cancelled output accepted")
	}
	s.denied = true
	if _, e = q.Preview(t.Context(), actor, s.job.ProjectID, s.job.ID); !errors.Is(e, app.ErrNotFound) {
		t.Fatal("denied preview minted capability")
	}
}
func TestJianyingLaunchHTTPHasNoSessionOrIdentityFallback(t *testing.T) {
	s, q := launchFixture(t)
	p, e := q.Preview(t.Context(), identity.Principal{}, s.job.ProjectID, s.job.ID)
	if e != nil {
		t.Fatal(e)
	}
	u, _ := url.Parse(p.LaunchURL)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	toolhttp.NewHandler(s, q).RegisterLaunch(r.Group("/api"))
	for _, query := range []string{u.RawQuery, u.RawQuery + "&job_id=" + s.job.ID.String(), u.RawQuery + "&" + u.RawQuery, "token=bad", ""} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", "/api/media-export-launch?"+query, nil))
		want := 404
		if query == u.RawQuery {
			want = 200
		}
		if w.Code != want || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Referrer-Policy") != "no-referrer" {
			t.Fatalf("response status/header: %d", w.Code)
		}
		if strings.Contains(w.Body.String(), s.saved.TokenHash) {
			t.Fatal("receipt hash leaked")
		}
	}
}
