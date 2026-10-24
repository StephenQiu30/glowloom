package canvas_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"
)

func TestCanvasLocationResolvesTeamBeforeScopeSelection(t *testing.T) {
	a, b, team, _ := newTeamLiveFixtures(t)
	read := func(token, id string) (int, map[string]any) {
		t.Helper()
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, a.server.URL+"/api/canvas-locations/"+id, nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Cookie", "glowloom_session="+token)
		req.Header.Set("X-Creation-Scope", uuid.NewString())
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() {
			if err := res.Body.Close(); err != nil {
				t.Error(err)
			}
		}()
		var body map[string]any
		if err = json.NewDecoder(res.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if res.Header.Get("Cache-Control") != "no-store" {
			t.Error("location must not be cached")
		}
		return res.StatusCode, body
	}
	for _, f := range []liveFixture{a, b} {
		code, body := read(f.auth.Token, a.doc.ID.String())
		if code != 200 || body["scope_id"] != a.actor.ScopeID.String() || body["project_id"] != a.doc.ProjectID.String() {
			t.Fatalf("location %d %+v", code, body)
		}
		if len(body) != 3 {
			t.Fatalf("unexpected disclosure: %+v", body)
		}
	}

	privateProject, privateCanvas := uuid.New(), uuid.New()
	var personal struct{ ID uuid.UUID }
	if err := a.db.Raw(`SELECT id FROM workspace.creation_scope WHERE kind='personal' AND owner_user_id=?`, a.actor.ID).Scan(&personal).Error; err != nil {
		t.Fatal(err)
	}
	if err := a.db.Exec(`INSERT INTO workspace.project(id,org_id,owner_id,owner_scope_id,name,aspect_ratio,style_type) VALUES(?,?,?,?,'Private routing test','16:9','realistic')`, privateProject, a.actor.OrgID, a.actor.ID, personal.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := a.db.Exec(`INSERT INTO canvas.document(id,project_id,name) VALUES(?,?,'Private canvas')`, privateCanvas, privateProject).Error; err != nil {
		t.Fatal(err)
	}
	if code, body := read(a.auth.Token, privateCanvas.String()); code != 200 || body["scope_id"] != personal.ID.String() {
		t.Fatalf("personal owner: %d %+v", code, body)
	}
	if code, body := read(b.auth.Token, privateCanvas.String()); code != 404 || body["scope_id"] != nil {
		t.Fatalf("private disclosure: %d %+v", code, body)
	}
	if code, _ := read(b.auth.Token, uuid.NewString()); code != 404 {
		t.Fatalf("missing: %d", code)
	}
	if code, _ := read("", a.doc.ID.String()); code != 401 {
		t.Fatalf("anonymous: %d", code)
	}
	if err := a.db.Exec(`UPDATE workspace.team_member SET status='removed',auth_epoch=auth_epoch+1 WHERE team_id=? AND user_id=?`, team, b.actor.ID).Error; err != nil {
		t.Fatal(err)
	}
	if code, body := read(b.auth.Token, a.doc.ID.String()); code != 404 {
		t.Fatalf("removed member: %d %+v", code, body)
	}
	if err := a.db.Exec(`UPDATE identity.user_session SET revoked_at=now() WHERE id=?`, a.auth.Session.ID).Error; err != nil {
		t.Fatal(err)
	}
	if code, _ := read(a.auth.Token, a.doc.ID.String()); code != 401 {
		t.Fatalf("revoked: %d", code)
	}
}
