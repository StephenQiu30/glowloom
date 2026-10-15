package identity_test

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	pgidentity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/postgres"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

// This double deliberately performs no external object writes.
type directoryTestObjects struct {
	values       map[string][]byte
	onGet, onPut func()
}

func (o *directoryTestObjects) Put(_ context.Context, key string, r io.Reader, _ int64, _ string) error {
	b, err := io.ReadAll(r)
	o.values[key] = b
	if o.onPut != nil {
		o.onPut()
	}
	return err
}
func (o *directoryTestObjects) Get(_ context.Context, key string) (io.ReadCloser, error) {
	if o.onGet != nil {
		o.onGet()
	}
	return io.NopCloser(bytes.NewReader(o.values[key])), nil
}
func (o *directoryTestObjects) Remove(_ context.Context, key string) error {
	delete(o.values, key)
	return nil
}
func installDirectoryExport(t *testing.T, tx *gorm.DB) {
	t.Helper()
	raw, err := os.ReadFile("../../db/migrations/011_identity_directory_export.sql")
	if err != nil {
		t.Fatal(err)
	}
	// Never allow transaction-control SQL to escape the reviewed outer rollback.
	statements := []string{}
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.TrimSpace(line) == "BEGIN;" || strings.TrimSpace(line) == "COMMIT;" {
			continue
		}
		statements = append(statements, line)
	}
	sql := strings.Join(statements, "\n")
	if strings.Contains(sql, "COMMIT") || strings.Contains(sql, "BEGIN;") {
		t.Fatal("unsafe migration transaction control")
	}
	var installed bool
	if err := tx.Raw(`SELECT to_regclass('identity.directory_export') IS NOT NULL`).Scan(&installed).Error; err != nil {
		t.Fatal(err)
	}
	if installed {
		t.Fatal("unexpected persistent directory export schema: test requires reviewed absent table")
	}
	if err := tx.Exec(sql).Error; err != nil {
		t.Fatal(err)
	}
}
func TestDirectoryExportPostgresSnapshotStatisticsAndRevocation(t *testing.T) {
	tx, _, actor, now := governanceFixture(t)
	installDirectoryExport(t, tx)
	_, target := verifiedTestActor(t, tx, "export-target@example.invalid", now)
	label := "export-" + uuid.NewString()
	if err := tx.Exec(`UPDATE identity."user" SET display_name=?,create_time=? WHERE id IN ?`, label, now.Add(-time.Second), []uuid.UUID{actor.ID, target.ID}).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(`UPDATE identity."user" SET is_delete=true WHERE id=?`, target.ID).Error; err != nil {
		t.Fatal(err)
	}
	directory := identity.NewPlatformDirectory(pgidentity.NewPlatformDirectoryStore(tx), completionProtector(t), func() time.Time { return now })
	stats := identity.NewDirectoryStatistics(pgidentity.NewPlatformDirectoryStore(tx), func() time.Time { return now })
	all, err := stats.Users(t.Context(), actor, identity.ExportFilter{Search: label})
	if err != nil || all.TotalUsers != 2 || all.AccountStates["deleted"] != 1 {
		t.Fatalf("consistent deleted counts: %+v %v", all, err)
	}
	page, err := directory.List(t.Context(), actor, identity.DirectoryFilter{Search: label, Status: "deleted"})
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != target.ID {
		t.Fatal("deleted list filter", err)
	}
	detail, err := directory.Detail(t.Context(), actor, target.ID)
	if err != nil || detail.User.Status != "deleted" || len(detail.AllowedActions) != 0 {
		t.Fatal("deleted action projection", err)
	}
	objects := &directoryTestObjects{values: map[string][]byte{}}
	exports := identity.NewDirectoryExport(pgidentity.NewDirectoryExportStore(tx), completionProtector(t), objects, func() time.Time { return now })
	filter := identity.ExportFilter{Search: label, Status: "deleted"}
	key := uuid.New()
	job, err := exports.Create(t.Context(), actor, nil, filter, key)
	if err != nil || job.RowCount != 1 {
		t.Fatal("create snapshot", err)
	}
	again, err := exports.Create(t.Context(), actor, nil, filter, key)
	if err != nil || again.ID != job.ID {
		t.Fatal("idempotency", err)
	}
	if _, err := exports.Create(t.Context(), actor, nil, identity.ExportFilter{Search: label}, key); !errors.Is(err, identity.ErrExportConflict) {
		t.Fatal("mismatched replay", err)
	}
	if err := tx.Exec(`UPDATE identity."user" SET display_name='later change' WHERE id=?`, target.ID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := exports.RunOnce(t.Context(), 2); err != nil {
		t.Fatal("generate", err)
	}
	var output bytes.Buffer
	if err := exports.Stream(t.Context(), actor, nil, job.ID, func(r io.Reader) error { _, err := io.Copy(&output, r); return err }); err != nil {
		t.Fatal("download", err)
	}
	rows, err := csv.NewReader(&output).ReadAll()
	if err != nil || len(rows) != 2 || rows[1][2] != label || rows[1][3] != "deleted" {
		t.Fatal("snapshot changed", err, rows)
	}
	objects.onGet = func() {
		if err := tx.Exec(`UPDATE identity."user" SET platform_epoch=platform_epoch+1 WHERE id=?`, actor.ID).Error; err != nil {
			t.Fatal(err)
		}
	}
	wrote := false
	if err := exports.Stream(t.Context(), actor, nil, job.ID, func(io.Reader) error { wrote = true; return nil }); !errors.Is(err, identity.ErrForbidden) || wrote {
		t.Fatal("revoked during GET emitted bytes", err)
	}
}
func TestDirectoryExportTeamRoleFilterAndIndependentAuthority(t *testing.T) {
	tx, _, actor, now := governanceFixture(t)
	installDirectoryExport(t, tx)
	_, target := verifiedTestActor(t, tx, "export-member@example.invalid", now)
	team := uuid.New()
	if err := tx.Exec(`INSERT INTO workspace.team(id,name,created_by) VALUES(?,'Export team',?)`, team, target.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(`INSERT INTO workspace.team_member(team_id,user_id,role,status,joined_at,source) VALUES(?,?,'team_admin','active',?,'created'),(?,?,'member','suspended',?,'invited')`, team, target.ID, now, team, actor.ID, now).Error; err != nil {
		t.Fatal(err)
	}
	stats := identity.NewDirectoryStatistics(pgidentity.NewPlatformDirectoryStore(tx), func() time.Time { return now })
	if _, err := stats.Members(t.Context(), actor, team, identity.ExportFilter{}); !errors.Is(err, identity.ErrForbidden) {
		t.Fatal("platform bypassed team authority", err)
	}
	if err := tx.Exec(`UPDATE identity.user_session SET reauth_at=?,reauth_epoch=? WHERE id=?`, now, target.CredentialEpoch, target.SessionID).Error; err != nil {
		t.Fatal(err)
	}
	view, err := stats.Members(t.Context(), target, team, identity.ExportFilter{Role: "member", Status: "suspended"})
	if err != nil || view.TotalMembers != 1 || view.MemberStates["suspended"] != 1 {
		t.Fatalf("member filtered stats %+v %v", view, err)
	}
	exports := identity.NewDirectoryExport(pgidentity.NewDirectoryExportStore(tx), completionProtector(t), &directoryTestObjects{values: map[string][]byte{}}, func() time.Time { return now })
	job, err := exports.Create(t.Context(), target, &team, identity.ExportFilter{Role: "member", Status: "suspended"}, uuid.New())
	if err != nil || job.RowCount != 1 {
		t.Fatal("member role filter lost", err)
	}
	if _, err := exports.RunOnce(t.Context(), 2); err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(`UPDATE workspace.team SET revision=revision+1 WHERE id=?`, team).Error; err != nil {
		t.Fatal(err)
	}
	wrote := false
	if err := exports.Stream(t.Context(), target, &team, job.ID, func(io.Reader) error { wrote = true; return nil }); !errors.Is(err, identity.ErrForbidden) || wrote {
		t.Fatal("stale team revision downloaded", err)
	}
}
func TestPlatformTeamInvitationMetadataAuthority(t *testing.T) {
	tx, gov, actor, now := governanceFixture(t)
	_, teamAdmin := verifiedTestActor(t, tx, "export-inviter@example.invalid", now)
	team := uuid.New()
	if err := tx.Exec(`INSERT INTO workspace.team(id,name,created_by) VALUES(?,'Governed invitations',?)`, team, teamAdmin.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(`INSERT INTO workspace.team_member(team_id,user_id,role,status,joined_at,source) VALUES(?,?,'team_admin','active',?,'created')`, team, teamAdmin.ID, now).Error; err != nil {
		t.Fatal(err)
	}
	for _, email := range []string{"metadata-one@example.invalid", "metadata-two@example.invalid"} {
		if _, err := gov.CreateTeamInvitation(t.Context(), teamAdmin, uuid.New(), team, email, "member"); err != nil {
			t.Fatal(err)
		}
	}
	page, err := gov.ListPlatformTeamInvitations(t.Context(), actor, team, identity.GovernanceFilter{Limit: 1})
	if err != nil || len(page.Items) != 1 || page.NextCursor == nil {
		t.Fatal("platform metadata directory", err)
	}
	if _, err := gov.ListPlatformTeamInvitations(t.Context(), teamAdmin, team, identity.GovernanceFilter{}); !errors.Is(err, identity.ErrForbidden) {
		t.Fatal("team admin acquired platform directory", err)
	}
	if _, err := gov.ListInvitations(t.Context(), actor, &team, identity.GovernanceFilter{}); err == nil {
		t.Fatal("platform reader acquired team authority")
	}
	if _, err := gov.ListInvitations(t.Context(), teamAdmin, &team, identity.GovernanceFilter{Limit: 1, Cursor: *page.NextCursor}); !errors.Is(err, identity.ErrInvalidGovernance) {
		t.Fatal("cross scope cursor accepted", err)
	}
	if err := tx.Exec(`UPDATE identity."user" SET platform_epoch=platform_epoch+1 WHERE id=?`, actor.ID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := gov.ListPlatformTeamInvitations(t.Context(), actor, team, identity.GovernanceFilter{}); !errors.Is(err, identity.ErrForbidden) {
		t.Fatal("stale platform epoch accepted", err)
	}
}
