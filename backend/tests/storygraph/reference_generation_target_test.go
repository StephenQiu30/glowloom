package storygraph_test

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	agentapp "github.com/StephenQiu30/lanverse/backend/internal/agent/application"
	contract "github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	generationapp "github.com/StephenQiu30/lanverse/backend/internal/generation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
)

func TestReferenceGenerationTargetPublishesAndRevalidatesReplay(t *testing.T) {
	repo, service, actor, command := referenceTargetFixture(t)
	published, err := service.BuildInitial(context.Background(), actor, command)
	if err != nil || len(repo.targets) != 1 || len(repo.receipts) != 2 {
		t.Fatalf("Target publication: %v", err)
	}
	replayed, err := service.BuildInitial(context.Background(), actor, command)
	if err != nil || !reflect.DeepEqual(replayed, published) || len(repo.targets) != 1 || len(repo.receipts) != 2 {
		t.Fatalf("Target replay: %v", err)
	}
	newRound := command
	newRound.IdempotencyKey = "another"
	if _, err = service.BuildInitial(context.Background(), actor, newRound); err == nil {
		t.Fatal("new key bypassed Head")
	}
	wrongAuthorization := command
	wrongAuthorization.AuthorizationHash = strings.Repeat("f", 64)
	if _, err = service.BuildInitial(context.Background(), actor, wrongAuthorization); err == nil {
		t.Fatal("wrong authorization was accepted")
	}
	repo.capabilityError = errors.New("preset_capability_missing")
	if _, err = service.BuildInitial(context.Background(), actor, command); err == nil {
		t.Fatal("replay skipped capability validation")
	}
	if len(repo.targets) != 1 || len(repo.receipts) != 2 {
		t.Fatal("rejected publication changed facts")
	}
}

func referenceTargetFixture(t *testing.T) (*referenceTargetMemory, *generationapp.ReferenceGenerationTargetService, generationapp.Actor, generationapp.BuildReferenceGenerationTargetCommand) {
	t.Helper()
	world := visualFoundationProductionVersion(t)
	input, candidate := referenceGenerationAnchorBrief(t, world)
	source, err := generationapp.CompileBaseReferenceGenerationSource(input, candidate, world)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(candidate)
	if err != nil {
		t.Fatal(err)
	}
	contentHash, err := canonical.Hash(raw)
	if err != nil {
		t.Fatal(err)
	}
	repo := &referenceTargetMemory{brief: agentapp.AcceptedReferenceBrief{RevisionID: uuid.NewString(), Revision: 1, RevisionHash: strings.Repeat("e", 64), ContentHash: contentHash, Input: input, Candidate: candidate}, source: source, receipts: map[string]platformcommand.Receipt{}, targets: map[string]generationapp.ReferenceGenerationTarget{}}
	clock := func() time.Time { return time.Date(2026, 9, 12, 1, 0, 0, 0, time.UTC) }
	actor := generationapp.Actor{UserID: uuid.NewString(), TokenVersion: 1}
	authorizer, err := generationapp.NewReferenceGenerationAuthorizationService(repo, clock, uuid.NewString)
	if err != nil {
		t.Fatal(err)
	}
	authorization, err := authorizer.AuthorizeInitial(context.Background(), actor, generationapp.AuthorizeInitialReferenceGenerationCommand{WorkspaceID: input.WorkspaceID, ProjectID: input.ProjectID, PlanVersionID: input.ApprovedReferencePlanVersionRef.OwnerVersionID, PlanContentHash: input.ApprovedReferencePlanVersionRef.OwnerContentHash, TargetVersionID: input.ReferencePlanTargetRef.OwnerVersionID, TargetContentHash: input.ReferencePlanTargetRef.OwnerContentHash, BriefRevisionID: repo.brief.RevisionID, BriefRevisionHash: repo.brief.RevisionHash, CandidateBundleCount: 2, IdempotencyKey: "authorize"})
	if err != nil {
		t.Fatal(err)
	}
	service, err := generationapp.NewReferenceGenerationTargetService(repo, clock, uuid.NewString)
	if err != nil {
		t.Fatal(err)
	}
	command := generationapp.BuildReferenceGenerationTargetCommand{WorkspaceID: input.WorkspaceID, ProjectID: input.ProjectID, AuthorizationID: authorization.HumanActionRef, AuthorizationHash: authorization.ContentHash, BriefRevisionID: repo.brief.RevisionID, BriefRevisionHash: repo.brief.RevisionHash, IdempotencyKey: "publish"}
	for _, role := range input.RequiredViewRoles {
		command.SlotPolicies = append(command.SlotPolicies, generationapp.ReferenceOutputSlotPolicy{ViewRole: role, AllowedMediaTypes: []string{"image/png"}, AspectRatio: "1:1", MinWidth: 1024, MinHeight: 1024, MaxBytes: 8 << 20})
	}
	return repo, service, actor, command
}

type referenceTargetMemory struct {
	brief           agentapp.AcceptedReferenceBrief
	source          generationapp.ReferenceGenerationSourceCompilation
	receipts        map[string]platformcommand.Receipt
	targets         map[string]generationapp.ReferenceGenerationTarget
	head            string
	capabilityError error
	deniedUser      string
}

func (repo *referenceTargetMemory) WithinReferenceGenerationAuthorization(_ context.Context, operation func(generationapp.ReferenceGenerationAuthorizationRepository) error) error {
	return operation(repo)
}
func (repo *referenceTargetMemory) WithinReferenceGenerationTarget(_ context.Context, operation func(generationapp.ReferenceGenerationTargetRepository) error) error {
	receipts, targets, head := maps.Clone(repo.receipts), maps.Clone(repo.targets), repo.head
	if err := operation(repo); err != nil {
		repo.receipts, repo.targets, repo.head = receipts, targets, head
		return err
	}
	return nil
}
func (repo *referenceTargetMemory) AuthorizeReferenceGenerationProject(_ context.Context, actor generationapp.Actor, workspace, project string) error {
	if actor.TokenVersion != 1 || actor.UserID == repo.deniedUser || workspace != repo.brief.Input.WorkspaceID || project != repo.brief.Input.ProjectID {
		return errors.New("forbidden")
	}
	return nil
}
func (repo *referenceTargetMemory) ReadReferenceGenerationBrief(_ context.Context, workspace, project, id, hash string) (agentapp.AcceptedReferenceBrief, error) {
	if workspace != repo.brief.Input.WorkspaceID || project != repo.brief.Input.ProjectID || id != repo.brief.RevisionID || hash != repo.brief.RevisionHash {
		return agentapp.AcceptedReferenceBrief{}, errors.New("Brief unavailable")
	}
	return repo.brief, nil
}
func (repo *referenceTargetMemory) ReadReferenceGenerationSource(context.Context, agentapp.AcceptedReferenceBrief) (generationapp.ReferenceGenerationSourceCompilation, error) {
	return repo.source, nil
}
func (repo *referenceTargetMemory) FindReceipt(_ context.Context, workspace, operation, key string) (platformcommand.Receipt, error) {
	if value, found := repo.receipts[operation+":"+key]; found && value.WorkspaceID == workspace {
		return value, nil
	}
	return platformcommand.Receipt{}, platformcommand.ErrReceiptNotFound
}
func (repo *referenceTargetMemory) EnsureReceipt(_ context.Context, value platformcommand.Receipt) (platformcommand.Receipt, error) {
	repo.receipts[value.Operation+":"+value.IdempotencyKey] = value
	return value, nil
}
func (repo *referenceTargetMemory) FindReferenceAuthorization(_ context.Context, id string) (platformcommand.Receipt, error) {
	for _, value := range repo.receipts {
		if value.ID == id {
			return value, nil
		}
	}
	return platformcommand.Receipt{}, platformcommand.ErrReceiptNotFound
}
func (repo *referenceTargetMemory) ValidateReferenceGenerationCapabilities(context.Context, contract.ReferenceBriefInput) error {
	return repo.capabilityError
}
func (repo *referenceTargetMemory) FindReferenceGenerationTarget(_ context.Context, workspace, project, id string) (generationapp.ReferenceGenerationTarget, error) {
	value, found := repo.targets[id]
	if !found || value.WorkspaceID != workspace || value.ProjectID != project {
		return generationapp.ReferenceGenerationTarget{}, errors.New("Target unavailable")
	}
	return value, nil
}
func (repo *referenceTargetMemory) PublishInitialReferenceGenerationTarget(_ context.Context, value generationapp.ReferenceGenerationTarget) error {
	if repo.head != "" {
		return errors.New("Head conflict")
	}
	repo.targets[value.ID], repo.head = value, value.ID
	return nil
}
func (repo *referenceTargetMemory) ValidateReferenceGenerationTargetHead(_ context.Context, value generationapp.ReferenceGenerationTarget) error {
	if repo.head != value.ID {
		return errors.New("Head drift")
	}
	return nil
}
