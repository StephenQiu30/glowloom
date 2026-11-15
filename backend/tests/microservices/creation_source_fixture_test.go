package microservices_test

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
	pgfence "github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence/postgres"
)

type sourcePolicyFixture struct {
	admin, creation, identity, execution, media *gorm.DB
	request                                     struct {
		ActorID, SessionID, ScopeID, ProjectID uuid.UUID
		Command                                writefence.Command
	}
}

// This fixture isolates source-policy locking. Its explicit head installation is
// not evidence of public registration/project bootstrap or a running full owner.
func newSourcePolicyFixture(t *testing.T) *sourcePolicyFixture {
	t.Helper()
	f := &sourcePolicyFixture{admin: fenceDatabase(t, ""), creation: authorityDatabase(t, "creation"), identity: authorityDatabase(t, "identity"), execution: authorityDatabase(t, "execution"), media: authorityDatabase(t, "media")}
	org, actor, session, scope, project := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO workspace.organization(id,name) VALUES (?,'source policy fixture')`, []any{org}},
		{`INSERT INTO identity."user"(id,org_id,login_name,display_name,role,password_hash,must_change_password) VALUES (?,?,?,'source policy fixture','producer','fixture',false)`, []any{actor, org, actor.String()}},
		{`INSERT INTO workspace.creation_scope(id,kind,owner_user_id,physical_scope_id) VALUES (?,'personal',?,?)`, []any{scope, actor, org}},
		{`INSERT INTO workspace.project(id,org_id,owner_id,owner_scope_id,name,aspect_ratio,style_type) VALUES (?,?,?,?,'source policy fixture','16:9','realistic')`, []any{project, org, actor, scope}},
	} {
		if err := f.admin.Exec(q.sql, q.args...).Error; err != nil {
			t.Fatal(err)
		}
	}
	authorities := []writefence.Authority{{Issuer: writefence.Identity, Kind: writefence.Account, ID: actor, Epoch: 1}, {Issuer: writefence.Identity, Kind: writefence.Session, ID: session, Epoch: 1}}
	for _, a := range []writefence.Authority{{Issuer: writefence.Creation, Kind: writefence.Scope, ID: scope}, {Issuer: writefence.Creation, Kind: writefence.Project, ID: project}} {
		authorities = append(authorities, completedAuthority(t, pgfence.NewAuthorityStore(f.creation), a, false).Control.Authority)
	}
	f.request.ActorID = actor
	f.request.SessionID = session
	f.request.ScopeID = scope
	f.request.ProjectID = project
	f.request.Command = writefence.Command{ID: uuid.New(), SHA256: strings.Repeat("a", 64), Action: "execution.frozen_input.accept", Authorities: authorities}
	return f
}
