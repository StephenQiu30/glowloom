package application

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	assetapp "github.com/StephenQiu30/lanverse/backend/internal/asset/application"
	assetdomain "github.com/StephenQiu30/lanverse/backend/internal/asset/domain"
	eventingdomain "github.com/StephenQiu30/lanverse/backend/internal/eventing/domain"
	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
	bibleapp "github.com/StephenQiu30/lanverse/backend/internal/production/bible/application"
	bibledomain "github.com/StephenQiu30/lanverse/backend/internal/production/bible/domain"
	planningapp "github.com/StephenQiu30/lanverse/backend/internal/production/planning/application"
	planningdomain "github.com/StephenQiu30/lanverse/backend/internal/production/planning/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/production/world/domain"
)

var ErrProductionWorldConfirmationConflict = errors.New("Production World confirmation conflict")

type ExpectedHead struct {
	OwnerKind, VersionFamily, ScopeKind, ScopeKey, ContentHash string
	Revision                                                   int64
}

type ConfirmProductionWorldCommand struct {
	CommandID, WorkspaceID, ProjectID, ActorID string
	GateInputID, GateInputHash                 string
	ReviewDecisionID                           string
	CandidateRevisionID                        string
	CandidateRevision                          int64
	CandidateRevisionHash                      string
	IdempotencyKey                             string
	ExpectedHeads                              []ExpectedHead
	Candidate                                  domain.ProductionWorldCandidate
}

type ConfirmationReadSet struct {
	ReviewDecision domain.ReviewDecisionAuditRef
}

type ConfirmationOutbox struct {
	ID, WorkspaceID, ProjectID, AggregateID, SourceReceiptID string
	AggregateRevision                                        int64
	Payload                                                  json.RawMessage
	PayloadHash                                              string
	OccurredAt                                               time.Time
}

type ConfirmationTransaction interface {
	Assets() assetapp.ProductionWorldAssetRepository
	Bible() bibleapp.ProductionWorldBibleRepository
	Planning() planningapp.ProductionWorldPlanningRepository
	AcquireCommand(context.Context, ConfirmProductionWorldCommand, string, time.Time) error
	FindCommandReceipt(context.Context, string, string) (platformcommand.Receipt, error)
	ValidateReadSet(context.Context, ConfirmProductionWorldCommand) (ConfirmationReadSet, error)
	EnsureEmptyPlanningRebaseHead(context.Context, string, string, ExpectedHead, time.Time) error
	CreateCollectionReceipt(context.Context, domain.CollectionCommitReceipt) error
	CreateCommandReceipt(context.Context, platformcommand.Receipt) error
	AppendOutbox(context.Context, ConfirmationOutbox) error
}

type ConfirmationTransactionManager interface {
	WithinProductionWorldConfirmation(context.Context, func(ConfirmationTransaction) error) error
}

type ConfirmationService struct {
	transactions ConfirmationTransactionManager
	now          func() time.Time
	newID        func() string
}

func NewConfirmationService(
	transactions ConfirmationTransactionManager,
	now func() time.Time,
	newID func() string,
) *ConfirmationService {
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	if newID == nil {
		newID = uuid.NewString
	}
	return &ConfirmationService{transactions: transactions, now: now, newID: newID}
}

func (service *ConfirmationService) ConfirmProductionWorld(
	ctx context.Context,
	command ConfirmProductionWorldCommand,
) (domain.ConfirmProductionWorldResult, error) {
	command, err := normalizeAndValidateCommand(command)
	if err != nil {
		return domain.ConfirmProductionWorldResult{}, err
	}
	if service == nil || service.transactions == nil || service.now == nil || service.newID == nil {
		return domain.ConfirmProductionWorldResult{}, errors.New("Production World confirmation service is unavailable")
	}
	inputHash, err := platformcommand.InputHash(command)
	if err != nil {
		return domain.ConfirmProductionWorldResult{}, err
	}
	var result domain.ConfirmProductionWorldResult
	err = service.transactions.WithinProductionWorldConfirmation(ctx, func(transaction ConfirmationTransaction) error {
		now := service.now().UTC()
		if acquireErr := transaction.AcquireCommand(ctx, command, inputHash, now); acquireErr != nil {
			return acquireErr
		}
		receipt, receiptErr := transaction.FindCommandReceipt(ctx, command.WorkspaceID, command.IdempotencyKey)
		if receiptErr == nil {
			result, receiptErr = platformcommand.Replay[domain.ConfirmProductionWorldResult](receipt, inputHash)
			if receiptErr != nil {
				return receiptErr
			}
			verified, verifyErr := domain.CompleteConfirmProductionWorldResult(result)
			if verifyErr != nil || verified.ResultContentHash != result.ResultContentHash ||
				verified.ReceiptContentHash != result.ReceiptContentHash {
				return errors.New("Production World command receipt has drifted")
			}
			return nil
		}
		if !errors.Is(receiptErr, platformcommand.ErrReceiptNotFound) {
			return receiptErr
		}
		readSet, validateErr := transaction.ValidateReadSet(ctx, command)
		if validateErr != nil {
			return validateErr
		}
		expected := expectedHeadIndex(command.ExpectedHeads)
		assetResult, applyErr := service.applyAssets(ctx, transaction, command, expected)
		if applyErr != nil {
			return applyErr
		}
		bibleResult, applyErr := service.applyBible(ctx, transaction, command, expected, assetResult)
		if applyErr != nil {
			return applyErr
		}
		planningResult, applyErr := service.applyPlanning(ctx, transaction, command, expected, assetResult, bibleResult)
		if applyErr != nil {
			return applyErr
		}
		rebaseExpected := expected[expectedHeadKey("production/planning", domain.PlanningStructureRebaseFamily, "project:"+command.ProjectID)]
		if applyErr = transaction.EnsureEmptyPlanningRebaseHead(ctx, command.WorkspaceID, command.ProjectID, rebaseExpected, now); applyErr != nil {
			return applyErr
		}

		collectionReceipts, ownerRefs, buildErr := service.buildCollectionReceipts(
			command, readSet.ReviewDecision, assetResult, bibleResult, planningResult, now,
		)
		if buildErr != nil {
			return buildErr
		}
		for _, collectionReceipt := range collectionReceipts {
			if createErr := transaction.CreateCollectionReceipt(ctx, collectionReceipt); createErr != nil {
				return createErr
			}
		}
		businessPayload, _, descriptor, buildErr := confirmationEvent(command, collectionReceipts)
		if buildErr != nil {
			return buildErr
		}
		result = domain.ConfirmProductionWorldResult{
			SchemaVersion: domain.ProductionWorldCommandResultSchema,
			CommandID:     command.CommandID, CommandContractID: domain.ConfirmProductionWorldContract,
			CommandContentHash: inputHash, CommandReceiptID: service.newID(), CoordinatorKind: "production/world",
			SubjectRef: domain.CollectionMemberRef{
				OwnerKind: "agent", LogicalID: command.ProjectID, VersionID: command.CandidateRevisionID,
				Revision: command.CandidateRevision, ContentHash: command.CandidateRevisionHash,
			},
			OrderedCollectionReceiptRefs: collectionReceiptRefs(collectionReceipts),
			CommittedOwnerVersionRefs:    ownerRefs, DomainEventDescriptors: []domain.DomainEventDescriptor{descriptor},
			CommittedAt: now, CommittedBy: command.ActorID,
		}
		result, buildErr = domain.CompleteConfirmProductionWorldResult(result)
		if buildErr != nil {
			return buildErr
		}
		encoded, encodeErr := platformcommand.Result(result)
		if encodeErr != nil {
			return encodeErr
		}
		if createErr := transaction.CreateCommandReceipt(ctx, platformcommand.Receipt{
			ID: result.CommandReceiptID, WorkspaceID: command.WorkspaceID,
			Operation: domain.ConfirmProductionWorldOperation, IdempotencyKey: command.IdempotencyKey,
			InputHash: inputHash, ResourceID: command.CommandID, Result: encoded,
			CreatedBy: command.ActorID, CreatedAt: now,
		}); createErr != nil {
			return createErr
		}
		outboxPayload, encodeErr := json.Marshal(struct {
			BusinessPayload    json.RawMessage `json:"business_payload"`
			CommandReceiptID   string          `json:"command_receipt_id"`
			CommandReceiptHash string          `json:"command_receipt_hash"`
		}{businessPayload, result.CommandReceiptID, result.ReceiptContentHash})
		if encodeErr != nil {
			return encodeErr
		}
		outboxHash, hashErr := eventingdomain.HashPayload(outboxPayload)
		if hashErr != nil {
			return hashErr
		}
		return transaction.AppendOutbox(ctx, ConfirmationOutbox{
			ID: service.newID(), WorkspaceID: command.WorkspaceID, ProjectID: command.ProjectID,
			AggregateID: command.CommandID, AggregateRevision: command.CandidateRevision,
			SourceReceiptID: result.CommandReceiptID, Payload: outboxPayload, PayloadHash: outboxHash, OccurredAt: now,
		})
	})
	if errors.Is(err, platformcommand.ErrInputMismatch) {
		return domain.ConfirmProductionWorldResult{}, ErrProductionWorldConfirmationConflict
	}
	return result, err
}

func (service *ConfirmationService) applyAssets(
	ctx context.Context,
	transaction ConfirmationTransaction,
	command ConfirmProductionWorldCommand,
	expected map[string]ExpectedHead,
) (assetapp.ApplyProductionWorldAssetsResult, error) {
	identities := make([]assetapp.ProductionWorldAssetIdentityInput, len(command.Candidate.Asset.Identities))
	for index, identity := range command.Candidate.Asset.Identities {
		states := make([]assetapp.ProductionWorldAssetStateInput, len(identity.States))
		for stateIndex, state := range identity.States {
			snapshot, err := json.Marshal(state)
			if err != nil {
				return assetapp.ApplyProductionWorldAssetsResult{}, err
			}
			states[stateIndex] = assetapp.ProductionWorldAssetStateInput{StateKey: state.StateKey, Snapshot: snapshot}
		}
		identities[index] = assetapp.ProductionWorldAssetIdentityInput{IdentityKey: identity.IdentityKey, Kind: identity.Kind, States: states}
	}
	head := expected[expectedHeadKey("asset", assetdomain.AssetIdentityStateCollectionFamily, "project:"+command.ProjectID)]
	return assetapp.NewProductionWorldAssetOwner(service.now, service.newID).ApplyProductionWorldAssets(
		ctx, transaction.Assets(), assetapp.ApplyProductionWorldAssetsCommand{
			WorkspaceID: command.WorkspaceID, ProjectID: command.ProjectID, ActorID: command.ActorID,
			ExpectedHeadRevision: head.Revision, ExpectedHeadHash: head.ContentHash,
			ExpectedBusinessKeyRoot: businessKeyRoot(command.Candidate, "asset"), Identities: identities,
		},
	)
}

func (service *ConfirmationService) applyBible(
	ctx context.Context,
	transaction ConfirmationTransaction,
	command ConfirmProductionWorldCommand,
	expected map[string]ExpectedHead,
	assets assetapp.ApplyProductionWorldAssetsResult,
) (bibleapp.ApplyProductionWorldBibleResult, error) {
	specifications := make([]bibleapp.ProductionWorldSpecificationInput, len(command.Candidate.Bible.Specifications))
	for index, value := range command.Candidate.Bible.Specifications {
		specifications[index] = bibleapp.ProductionWorldSpecificationInput{
			SpecificationKey: value.SpecificationKey, IdentityKey: value.IdentityKey,
			Kind: value.Kind, Slots: value.SpecificationSlots, Basis: value.Basis,
		}
	}
	claims := make([]bibleapp.ProductionWorldClaimInput, len(command.Candidate.Bible.WorldClaims))
	for index, value := range command.Candidate.Bible.WorldClaims {
		claims[index] = bibleapp.ProductionWorldClaimInput{
			ClaimKey: value.ClaimKey, ClaimType: value.ClaimType, Statement: value.Statement,
			Participants: value.Participants, Narrative: value.Narrative, Basis: value.Basis,
		}
	}
	head := expected[expectedHeadKey("production/bible", bibledomain.BibleProductionWorldFamily, "project:"+command.ProjectID)]
	return bibleapp.NewProductionWorldBibleOwner(service.now, service.newID).ApplyProductionWorldBible(
		ctx, transaction.Bible(), bibleapp.ApplyProductionWorldBibleCommand{
			WorkspaceID: command.WorkspaceID, ProjectID: command.ProjectID, ActorID: command.ActorID,
			ReviewDecisionID: command.ReviewDecisionID, ExpectedHeadRevision: head.Revision,
			ExpectedHeadHash: head.ContentHash, ExpectedBusinessKeyRoot: businessKeyRoot(command.Candidate, "bible"),
			PartitionHash: command.Candidate.PartitionRoots.Bible,
			StructureIdentitySet: bibledomain.ProductionWorldOwnerRef{
				OwnerKind:   command.Candidate.StructureIdentitySetVersion.OwnerKind,
				LogicalID:   command.Candidate.StructureIdentitySetVersion.LogicalID,
				VersionID:   command.Candidate.StructureIdentitySetVersion.VersionID,
				Revision:    command.Candidate.StructureIdentitySetVersion.Revision,
				ContentHash: command.Candidate.StructureIdentitySetVersion.ContentHash,
			},
			Candidate: bibledomain.ProductionWorldOwnerRef{
				OwnerKind: "agent", LogicalID: command.ProjectID, VersionID: command.CandidateRevisionID,
				Revision: command.CandidateRevision, ContentHash: command.CandidateRevisionHash,
			},
			Specifications: specifications, Claims: claims, Assets: assets.Assets, States: assets.States,
		},
	)
}

func (service *ConfirmationService) applyPlanning(
	ctx context.Context,
	transaction ConfirmationTransaction,
	command ConfirmProductionWorldCommand,
	expected map[string]ExpectedHead,
	assets assetapp.ApplyProductionWorldAssetsResult,
	bible bibleapp.ApplyProductionWorldBibleResult,
) (planningapp.ApplyProductionWorldPlanningResult, error) {
	heads := make([]planningapp.ExpectedProductionWorldPlanningHead, len(command.Candidate.SharedProof.PlanningEpisodeScopes))
	for index, scope := range command.Candidate.SharedProof.PlanningEpisodeScopes {
		head := expected[expectedHeadKey("production/planning", planningdomain.PlanningSceneCollectionFamily, scope.ScopeKey)]
		heads[index] = planningapp.ExpectedProductionWorldPlanningHead{
			EpisodeID: scope.EpisodeID, Revision: head.Revision, ContentHash: head.ContentHash,
		}
	}
	return planningapp.NewProductionWorldPlanningOwner(service.now, service.newID).ApplyProductionWorldPlanning(
		ctx, transaction.Planning(), planningapp.ApplyProductionWorldPlanningCommand{
			WorkspaceID: command.WorkspaceID, ProjectID: command.ProjectID, ActorID: command.ActorID,
			ExpectedBusinessKeyRoot: businessKeyRoot(command.Candidate, "planning"), ExpectedHeads: heads,
			EpisodeScopes: command.Candidate.SharedProof.PlanningEpisodeScopes, Planning: command.Candidate.Planning,
			Assets: assets.Assets, States: assets.States, Specifications: bible.Specifications, Bindings: bible.Bindings,
		},
	)
}

func (service *ConfirmationService) buildCollectionReceipts(
	command ConfirmProductionWorldCommand,
	decision domain.ReviewDecisionAuditRef,
	assets assetapp.ApplyProductionWorldAssetsResult,
	bible bibleapp.ApplyProductionWorldBibleResult,
	planning planningapp.ApplyProductionWorldPlanningResult,
	now time.Time,
) ([]domain.CollectionCommitReceipt, []domain.CollectionMemberRef, error) {
	allScopes := append([]string(nil), command.Candidate.SharedProof.ScopeKeys...)
	assetMembers := assetCollectionRefs(assets)
	bibleMembers := []domain.CollectionMemberRef{{
		OwnerKind: "production/bible", LogicalID: command.ProjectID, VersionID: bible.Version.ID,
		Revision: bible.Version.Revision, ContentHash: bible.Version.ContentHash,
	}}
	inputs := []domain.CollectionCommitReceiptInput{
		collectionReceiptInput(command, decision, "asset", assetdomain.AssetIdentityStateCollectionFamily,
			"project", assets.Head.ScopeKey, assets.Head.ScopeRevision, assets.Head.ScopeContentHash,
			assets.Head.MembersHash, assets.Head.CollectionRootHash, assetMembers, allScopes, now, service.newID()),
		collectionReceiptInput(command, decision, "production/bible", bibledomain.BibleProductionWorldFamily,
			"project", bible.Head.ScopeKey, bible.Head.ScopeRevision, bible.Head.ScopeContentHash,
			bible.Head.MembersHash, bible.Head.CollectionRootHash, bibleMembers, allScopes, now, service.newID()),
	}
	headByEpisode := make(map[string]planningdomain.ProductionWorldPlanningEpisodeHead, len(planning.Heads))
	for _, head := range planning.Heads {
		headByEpisode[head.EpisodeID] = head
	}
	for _, scope := range command.Candidate.SharedProof.PlanningEpisodeScopes {
		head := headByEpisode[scope.EpisodeID]
		members := make([]domain.CollectionMemberRef, len(head.Members))
		for index, member := range head.Members {
			members[index] = domain.CollectionMemberRef{
				OwnerKind: "production/planning", LogicalID: member.Fact.BusinessKey,
				VersionID: member.Fact.ID, Revision: int64(member.Fact.Revision), ContentHash: member.Fact.ContentHash,
			}
		}
		sortCollectionRefs(members)
		inputs = append(inputs, collectionReceiptInput(command, decision, "production/planning",
			planningdomain.PlanningSceneCollectionFamily, "episode", head.ScopeKey, head.ScopeRevision,
			head.ScopeContentHash, head.MembersHash, head.CollectionRootHash, members,
			append([]string(nil), scope.SceneScopeKeys...), now, service.newID()))
	}
	receipts := make([]domain.CollectionCommitReceipt, len(inputs))
	ownerRefs := append(append([]domain.CollectionMemberRef(nil), assetMembers...), bibleMembers...)
	for index, input := range inputs {
		value, err := domain.NewCollectionCommitReceipt(input)
		if err != nil {
			return nil, nil, err
		}
		receipts[index] = value
		if input.VersionFamily == planningdomain.PlanningSceneCollectionFamily {
			ownerRefs = append(ownerRefs, input.Members...)
		}
	}
	slices.SortFunc(receipts, func(left, right domain.CollectionCommitReceipt) int {
		return strings.Compare(left.OwnerKind+"\x00"+left.VersionFamily+"\x00"+left.ScopeKey,
			right.OwnerKind+"\x00"+right.VersionFamily+"\x00"+right.ScopeKey)
	})
	sortCollectionRefs(ownerRefs)
	return receipts, ownerRefs, nil
}

func normalizeAndValidateCommand(command ConfirmProductionWorldCommand) (ConfirmProductionWorldCommand, error) {
	for _, identifier := range []string{command.CommandID, command.WorkspaceID, command.ProjectID, command.ActorID,
		command.GateInputID, command.ReviewDecisionID, command.CandidateRevisionID} {
		if _, err := uuid.Parse(identifier); err != nil {
			return ConfirmProductionWorldCommand{}, errors.New("invalid Production World confirmation identity")
		}
	}
	raw, err := json.Marshal(command.Candidate)
	if err != nil {
		return ConfirmProductionWorldCommand{}, err
	}
	candidate, canonicalCandidate, err := domain.DecodeProductionWorldCandidate(raw)
	if err != nil || string(raw) != string(canonicalCandidate) || candidate.WorkspaceID != command.WorkspaceID ||
		candidate.ProjectID != command.ProjectID || command.CandidateRevision < 1 ||
		!validHash(command.GateInputHash) || !validHash(command.CandidateRevisionHash) ||
		strings.TrimSpace(command.IdempotencyKey) == "" {
		return ConfirmProductionWorldCommand{}, errors.New("invalid Production World confirmation command")
	}
	command.Candidate = candidate
	slices.SortFunc(command.ExpectedHeads, func(left, right ExpectedHead) int {
		return strings.Compare(expectedHeadKey(left.OwnerKind, left.VersionFamily, left.ScopeKey),
			expectedHeadKey(right.OwnerKind, right.VersionFamily, right.ScopeKey))
	})
	expectedCount := 3 + len(candidate.SharedProof.PlanningEpisodeScopes)
	if len(command.ExpectedHeads) != expectedCount {
		return ConfirmProductionWorldCommand{}, errors.New("Production World expected Head set is incomplete")
	}
	want := map[string]string{
		expectedHeadKey("asset", assetdomain.AssetIdentityStateCollectionFamily, "project:"+command.ProjectID):     "project",
		expectedHeadKey("production/bible", bibledomain.BibleProductionWorldFamily, "project:"+command.ProjectID):  "project",
		expectedHeadKey("production/planning", domain.PlanningStructureRebaseFamily, "project:"+command.ProjectID): "project",
	}
	for _, scope := range candidate.SharedProof.PlanningEpisodeScopes {
		want[expectedHeadKey("production/planning", planningdomain.PlanningSceneCollectionFamily, scope.ScopeKey)] = "episode"
	}
	for _, head := range command.ExpectedHeads {
		key := expectedHeadKey(head.OwnerKind, head.VersionFamily, head.ScopeKey)
		scopeKind, exists := want[key]
		if !exists || head.Revision < 0 || (head.Revision == 0) != (head.ContentHash == "") ||
			(head.Revision > 0 && !validHash(head.ContentHash)) ||
			head.ScopeKind != scopeKind {
			return ConfirmProductionWorldCommand{}, errors.New("invalid Production World expected Head")
		}
		delete(want, key)
	}
	if len(want) != 0 {
		return ConfirmProductionWorldCommand{}, errors.New("Production World expected Head set has drifted")
	}
	return command, nil
}

func collectionReceiptInput(
	command ConfirmProductionWorldCommand,
	decision domain.ReviewDecisionAuditRef,
	ownerKind, family, scopeKind, scopeKey string,
	scopeRevision int64,
	scopeHash, membersHash, rootHash string,
	members []domain.CollectionMemberRef,
	coveredScopes []string,
	now time.Time,
	id string,
) domain.CollectionCommitReceiptInput {
	return domain.CollectionCommitReceiptInput{
		ID: id, CommandID: command.CommandID, IdempotencyKey: command.IdempotencyKey,
		WorkspaceID: command.WorkspaceID, ProjectID: command.ProjectID,
		DecisionCheckpointID: command.ReviewDecisionID, OwnerKind: ownerKind, VersionFamily: family,
		ScopeKind: scopeKind, ScopeKey: scopeKey, ScopeRevision: scopeRevision,
		ScopeContentHash: scopeHash, MembersHash: membersHash, CollectionRootHash: rootHash,
		Members: members, CommittedOwnerVersionRefs: append([]domain.CollectionMemberRef(nil), members...),
		CoveredScopeKeys: coveredScopes, ReviewDecisionAuditRef: decision,
		CommittedAt: now, CommittedBy: command.ActorID,
	}
}

func assetCollectionRefs(result assetapp.ApplyProductionWorldAssetsResult) []domain.CollectionMemberRef {
	refs := make([]domain.CollectionMemberRef, 0, len(result.Assets)+len(result.States))
	for _, value := range result.Assets {
		refs = append(refs, domain.CollectionMemberRef{OwnerKind: "asset", LogicalID: value.IdentityKey,
			VersionID: value.ID, Revision: int64(value.Revision), ContentHash: value.ContentHash})
	}
	for _, value := range result.States {
		refs = append(refs, domain.CollectionMemberRef{OwnerKind: "asset", LogicalID: value.StateKey,
			VersionID: value.ID, Revision: int64(value.Revision), ContentHash: value.ContentHash})
	}
	sortCollectionRefs(refs)
	return refs
}

func sortCollectionRefs(values []domain.CollectionMemberRef) {
	slices.SortFunc(values, func(left, right domain.CollectionMemberRef) int {
		return strings.Compare(left.OwnerKind+"\x00"+left.LogicalID+"\x00"+left.VersionID,
			right.OwnerKind+"\x00"+right.LogicalID+"\x00"+right.VersionID)
	})
}

func collectionReceiptRefs(values []domain.CollectionCommitReceipt) []domain.CollectionReceiptRef {
	result := make([]domain.CollectionReceiptRef, len(values))
	for index, value := range values {
		result[index] = domain.CollectionReceiptRef{
			ID: value.ID, OwnerKind: value.OwnerKind, VersionFamily: value.VersionFamily,
			ScopeKind: value.ScopeKind, ScopeKey: value.ScopeKey,
			CollectionRootHash: value.CollectionRootHash, ReceiptContentHash: value.ReceiptContentHash,
		}
	}
	return result
}

func confirmationEvent(
	command ConfirmProductionWorldCommand,
	receipts []domain.CollectionCommitReceipt,
) (json.RawMessage, string, domain.DomainEventDescriptor, error) {
	payload, err := json.Marshal(struct {
		Schema               string                        `json:"schema"`
		CommandID            string                        `json:"command_id"`
		CandidateRevisionID  string                        `json:"candidate_revision_id"`
		CandidateRevision    int64                         `json:"candidate_revision"`
		CandidateContentHash string                        `json:"candidate_content_hash"`
		CollectionReceipts   []domain.CollectionReceiptRef `json:"collection_receipts"`
	}{"production-world-confirmed-production", command.CommandID, command.CandidateRevisionID,
		command.CandidateRevision, command.Candidate.ContentHash, collectionReceiptRefs(receipts)})
	if err != nil {
		return nil, "", domain.DomainEventDescriptor{}, err
	}
	hash, err := eventingdomain.HashPayload(payload)
	if err != nil {
		return nil, "", domain.DomainEventDescriptor{}, err
	}
	descriptor := domain.DomainEventDescriptor{
		EventType: domain.ProductionWorldConfirmedEvent,
		AggregateOwnerRef: domain.CollectionMemberRef{
			OwnerKind: "production/world", LogicalID: command.ProjectID, VersionID: command.CommandID,
			Revision: command.CandidateRevision, ContentHash: command.Candidate.ContentHash,
		},
		AggregateRevision: command.CandidateRevision, AggregateContentHash: command.Candidate.ContentHash,
		PayloadContractID: "production-world-confirmed-production", PayloadContentHash: hash,
	}
	return payload, hash, descriptor, nil
}

func businessKeyRoot(candidate domain.ProductionWorldCandidate, partition string) string {
	for _, value := range candidate.SharedProof.ExpectedBusinessKeyRoots {
		if value.Partition == partition {
			return value.Root
		}
	}
	return ""
}

func expectedHeadKey(owner, family, scope string) string {
	return owner + "\x00" + family + "\x00" + scope
}

func expectedHeadIndex(values []ExpectedHead) map[string]ExpectedHead {
	result := make(map[string]ExpectedHead, len(values))
	for _, value := range values {
		result[expectedHeadKey(value.OwnerKind, value.VersionFamily, value.ScopeKey)] = value
	}
	return result
}

func validHash(value string) bool {
	return len(value) == 64 && strings.IndexFunc(value, func(r rune) bool {
		return (r < '0' || r > '9') && (r < 'a' || r > 'f')
	}) == -1
}
