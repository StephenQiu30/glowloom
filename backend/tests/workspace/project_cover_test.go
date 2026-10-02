package workspace_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	identitydomain "github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	workspacehttp "github.com/StephenQiu30/lanverse/backend/internal/workspace/adapter/http"
	workspaceapp "github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workspace/domain"
)

func TestProjectCoverNilLegacyProjectGolden(t *testing.T) {
	raw, err := os.ReadFile("testdata/project_copy_legacy_golden.json")
	if err != nil {
		t.Fatal(err)
	}
	var golden struct {
		WorkspaceBytes string `json:"workspace_bytes"`
	}
	if err := json.Unmarshal(raw, &golden); err != nil {
		t.Fatal(err)
	}
	var snapshot struct{ Target json.RawMessage }
	if err := json.Unmarshal([]byte(golden.WorkspaceBytes), &snapshot); err != nil {
		t.Fatal(err)
	}
	var project domain.Project
	if err := json.Unmarshal(snapshot.Target, &project); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(project)
	if err != nil || !bytes.Equal(encoded, snapshot.Target) {
		t.Fatalf("legacy project bytes changed: %s %v", encoded, err)
	}
	asset := uuid.New()
	project.CoverAssetID = &asset
	encoded, err = json.Marshal(project)
	if err != nil || !bytes.Contains(encoded, []byte(`"CoverAssetID":"`+asset.String()+`"`)) {
		t.Fatal("new cover missing", err)
	}
	zero := uuid.Nil
	project.CoverAssetID = &zero
	if !errors.Is(project.Validate(), domain.ErrInvalidProject) {
		t.Fatal("zero cover accepted")
	}
}

type coverFacts struct {
	fact mediaapp.AssetSummary
	err  error
}

func (f coverFacts) Reference(context.Context, identityapp.Principal, uuid.UUID, uuid.UUID) (mediaapp.AssetSummary, error) {
	return f.fact, f.err
}

func TestProjectCoverFreezeMapAndPublicationClosedIdentities(t *testing.T) {
	actor := projectCommandActor(identitydomain.RoleProducer)
	source, target, asset, copied := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	if err := workspaceapp.FreezeProjectCover(t.Context(), nil, actor, source, nil); err != nil {
		t.Fatal("nil legacy cover requires owner", err)
	}
	if err := workspaceapp.FreezeProjectCover(t.Context(), nil, actor, source, &asset); !errors.Is(err, workspaceapp.ErrProjectDependencyUnavailable) {
		t.Fatal("missing owner accepted", err)
	}
	if _, err := workspaceapp.MapProjectCover(&asset, nil); !errors.Is(err, workspaceapp.ErrProjectCoverUnavailable) {
		t.Fatal("missing mapping accepted", err)
	}
	if _, err := workspaceapp.MapProjectCover(&asset, map[uuid.UUID]uuid.UUID{asset: asset}); !errors.Is(err, workspaceapp.ErrProjectCoverUnavailable) {
		t.Fatal("source ID reused", err)
	}
	mapped, err := workspaceapp.MapProjectCover(&asset, map[uuid.UUID]uuid.UUID{asset: copied})
	if err != nil || mapped == nil || *mapped != copied {
		t.Fatal("target mapping", err)
	}
	reader := coverFacts{fact: mediaapp.AssetSummary{ID: copied, ProjectID: target, Kind: "image", Revision: 1}}
	if err := workspaceapp.VerifyProjectCover(t.Context(), reader, actor, target, mapped, &copied); err != nil {
		t.Fatal("publication cover rejected", err)
	}
	if err := workspaceapp.VerifyProjectCover(t.Context(), reader, actor, target, &asset, &copied); !errors.Is(err, workspaceapp.ErrProjectCoverUnavailable) {
		t.Fatal("wrong frozen cover published", err)
	}
	reader.fact.ProjectID = source
	if err := workspaceapp.VerifyProjectCover(t.Context(), reader, actor, target, mapped, &copied); !errors.Is(err, workspaceapp.ErrProjectCoverUnavailable) {
		t.Fatal("foreign target media published", err)
	}
	reader.fact.ProjectID = target
	reader.fact.Kind = "audio"
	if err := workspaceapp.VerifyProjectCover(t.Context(), reader, actor, target, mapped, &copied); !errors.Is(err, workspaceapp.ErrProjectCoverUnavailable) {
		t.Fatal("non-image published", err)
	}
}

func TestProjectCoverLifecycleOldProductionJSONGolden(t *testing.T) {
	raw, err := os.ReadFile("testdata/project_change_legacy_golden.json")
	if err != nil {
		t.Fatal(err)
	}
	var golden struct {
		InputBytes    string `json:"input_bytes"`
		ResponseBytes string `json:"response_bytes"`
	}
	if err := json.Unmarshal(raw, &golden); err != nil {
		t.Fatal(err)
	}
	var input workspaceapp.ProjectChangeInput
	if err := json.Unmarshal([]byte(golden.InputBytes), &input); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(input)
	if err != nil || !bytes.Equal(encoded, []byte(golden.InputBytes)) {
		t.Fatal("old input nil bytes changed", string(encoded), err)
	}
	var snapshot workspaceapp.ProjectSnapshot
	if err := json.Unmarshal([]byte(golden.ResponseBytes), &snapshot); err != nil {
		t.Fatal(err)
	}
	encoded, err = json.Marshal(snapshot)
	if err != nil || !bytes.Equal(encoded, []byte(golden.ResponseBytes)) {
		t.Fatal("old response nil bytes changed", string(encoded), err)
	}
}

func TestProjectCoverPatchPresenceAndDomainTransition(t *testing.T) {
	for _, tc := range []struct {
		raw string
		set bool
		id  *uuid.UUID
	}{
		{`{"expected_revision":1,"name":"name"}`, false, nil},
		{`{"expected_revision":1,"cover_asset_id":null}`, true, nil},
	} {
		var request workspacehttp.ProjectUpdateRequest
		if err := json.Unmarshal([]byte(tc.raw), &request); err != nil || request.SetCover != tc.set || request.CoverAssetID != nil {
			t.Fatalf("presence %+v %v", request, err)
		}
	}
	actor := projectCommandActor(identitydomain.RoleProducer)
	project := updateProjectFixture(actor)
	asset := uuid.New()
	input := projectChange(project.ID, project.Revision, "patch")
	input.Patch.SetCover, input.Patch.CoverAssetID = true, &asset
	after, events, err := workspaceapp.PrepareProjectChange(actor, input, project, time.Now(), false)
	if err != nil || after.CoverAssetID == nil || *after.CoverAssetID != asset || after.Revision != project.Revision+1 || len(events) != 2 {
		t.Fatalf("set cover %+v %v", after, err)
	}
	input.Patch.ExpectedRevision++
	input.Patch.CoverAssetID = nil
	cleared, _, err := workspaceapp.PrepareProjectChange(actor, input, after, time.Now(), false)
	if err != nil || cleared.CoverAssetID != nil || cleared.Revision != after.Revision+1 {
		t.Fatalf("clear cover %+v %v", cleared, err)
	}
	input.Patch.ExpectedRevision++
	unchanged, events, err := workspaceapp.PrepareProjectChange(actor, input, cleared, time.Now(), false)
	if err != nil || unchanged.Revision != cleared.Revision || len(events) != 0 {
		t.Fatal("clear no-op changed revision", err)
	}
	input.Patch.SetCover = false
	input.Patch.CoverAssetID = &asset
	if !errors.Is(input.Validate(), workspaceapp.ErrInvalidProjectChange) {
		t.Fatal("unannounced cover accepted")
	}
}
