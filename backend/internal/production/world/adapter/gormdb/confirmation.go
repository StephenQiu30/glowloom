package gormdb

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	assetgorm "github.com/StephenQiu30/lanverse/backend/internal/asset/adapter/gormdb"
	assetapp "github.com/StephenQiu30/lanverse/backend/internal/asset/application"
	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
	commandgorm "github.com/StephenQiu30/lanverse/backend/internal/platform/command/adapter/gormdb"
	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/ownercollection"
	biblegorm "github.com/StephenQiu30/lanverse/backend/internal/production/bible/adapter/gormdb"
	bibleapp "github.com/StephenQiu30/lanverse/backend/internal/production/bible/application"
	planninggorm "github.com/StephenQiu30/lanverse/backend/internal/production/planning/adapter/gormdb"
	planningapp "github.com/StephenQiu30/lanverse/backend/internal/production/planning/application"
	projectdomain "github.com/StephenQiu30/lanverse/backend/internal/production/project/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/production/world/application"
	"github.com/StephenQiu30/lanverse/backend/internal/production/world/domain"
	workflowdomain "github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

type Store struct {
	database *gorm.DB
}

func NewStore(database *gorm.DB) *Store { return &Store{database: database} }

func (store *Store) WithinProductionWorldConfirmation(
	ctx context.Context,
	operation func(application.ConfirmationTransaction) error,
) error {
	if store == nil || store.database == nil {
		return errors.New("Production World confirmation store is unavailable")
	}
	return platformdatabase.WithinTransaction(ctx, store.database, func(transaction *gorm.DB) error {
		return operation(&confirmationTransaction{database: transaction})
	})
}

type confirmationTransaction struct {
	database *gorm.DB
}

func (transaction *confirmationTransaction) Assets() assetapp.ProductionWorldAssetRepository {
	return assetgorm.NewProductionWorldRepository(transaction.database)
}

func (transaction *confirmationTransaction) Bible() bibleapp.ProductionWorldBibleRepository {
	return biblegorm.NewProductionWorldRepository(transaction.database)
}

func (transaction *confirmationTransaction) Planning() planningapp.ProductionWorldPlanningRepository {
	return planninggorm.NewProductionWorldRepository(transaction.database)
}

func (transaction *confirmationTransaction) AcquireCommand(
	ctx context.Context,
	command application.ConfirmProductionWorldCommand,
	inputHash string,
	createdAt time.Time,
) error {
	workspaceID, workspaceErr := uuid.Parse(command.WorkspaceID)
	commandID, commandErr := uuid.Parse(command.CommandID)
	if workspaceErr != nil || commandErr != nil {
		return application.ErrProductionWorldConfirmationConflict
	}
	record := model.ProductionWorldCommandDedup{
		ID:          uuid.NewSHA1(workspaceID, []byte(domain.ConfirmProductionWorldContract+":"+command.IdempotencyKey)),
		WorkspaceID: workspaceID, CommandContract: domain.ConfirmProductionWorldContract,
		IdempotencyKey: command.IdempotencyKey, CommandID: commandID, InputHash: inputHash, CreatedAt: createdAt,
	}
	if err := transaction.database.WithContext(ctx).Omit(clause.Associations).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "workspace_id"}, {Name: "command_contract"}, {Name: "idempotency_key"}},
		DoNothing: true,
	}).Create(&record).Error; err != nil {
		return err
	}
	var persisted model.ProductionWorldCommandDedup
	if err := transaction.database.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("workspace_id = ? AND command_contract = ? AND idempotency_key = ?", workspaceID,
			domain.ConfirmProductionWorldContract, command.IdempotencyKey).First(&persisted).Error; err != nil {
		return err
	}
	if persisted.CommandID != commandID || persisted.InputHash != inputHash {
		return platformcommand.ErrInputMismatch
	}
	return nil
}

func (transaction *confirmationTransaction) FindCommandReceipt(
	ctx context.Context,
	workspaceID, idempotencyKey string,
) (platformcommand.Receipt, error) {
	return commandgorm.Find(ctx, transaction.database, workspaceID, domain.ConfirmProductionWorldOperation, idempotencyKey)
}

func (transaction *confirmationTransaction) ValidateReadSet(
	ctx context.Context,
	command application.ConfirmProductionWorldCommand,
) (application.ConfirmationReadSet, error) {
	workspaceID, _ := uuid.Parse(command.WorkspaceID)
	projectID, _ := uuid.Parse(command.ProjectID)
	actorID, _ := uuid.Parse(command.ActorID)
	gateInputID, _ := uuid.Parse(command.GateInputID)
	reviewDecisionID, _ := uuid.Parse(command.ReviewDecisionID)
	candidateRevisionID, _ := uuid.Parse(command.CandidateRevisionID)

	var project model.Project
	if err := transaction.database.WithContext(ctx).Clauses(clause.Locking{Strength: "SHARE"}).
		First(&project, "id = ?", projectID).Error; err != nil {
		return application.ConfirmationReadSet{}, err
	}
	var membership model.Membership
	if err := transaction.database.WithContext(ctx).Clauses(clause.Locking{Strength: "SHARE"}).
		Where("workspace_id = ? AND user_id = ? AND status = ?", workspaceID, actorID, "active").
		First(&membership).Error; err != nil {
		return application.ConfirmationReadSet{}, err
	}
	if project.WorkspaceID != workspaceID || project.Status != "active" ||
		(membership.Role != "owner" && membership.Role != "editor") {
		return application.ConfirmationReadSet{}, application.ErrProductionWorldConfirmationConflict
	}

	var gateRecord model.WorkflowHumanGateInput
	if err := transaction.database.WithContext(ctx).Clauses(clause.Locking{Strength: "SHARE"}).
		First(&gateRecord, "id = ?", gateInputID).Error; err != nil {
		return application.ConfirmationReadSet{}, err
	}
	gate, _, err := workflowdomain.DecodeProductionWorldGateInput(json.RawMessage(gateRecord.Input))
	if err != nil || gateRecord.WorkspaceID != workspaceID || gateRecord.ProjectID != projectID ||
		gateRecord.GateKey != workflowdomain.ProductionWorldGateKey || gateRecord.InputHash != command.GateInputHash ||
		gate.InputHash != command.GateInputHash || gate.Subject.ProductionWorldCandidate.CandidateRevisionID != command.CandidateRevisionID ||
		gate.Subject.ProductionWorldCandidate.CandidateRevision != command.CandidateRevision ||
		gate.Subject.ProductionWorldCandidate.CandidateRevisionHash != command.CandidateRevisionHash ||
		gate.Subject.ProductionWorldCandidate.CandidateContentHash != command.Candidate.ContentHash ||
		gate.Subject.StructureIdentitySetVersion != command.Candidate.StructureIdentitySetVersion ||
		!sameExpectedHeads(gate.Subject.ExpectedHeads, command.ExpectedHeads) {
		return application.ConfirmationReadSet{}, application.ErrProductionWorldConfirmationConflict
	}

	var decision model.ReviewDecision
	if err = transaction.database.WithContext(ctx).Clauses(clause.Locking{Strength: "SHARE"}).
		First(&decision, "id = ?", reviewDecisionID).Error; err != nil {
		return application.ConfirmationReadSet{}, err
	}
	var task model.HumanTask
	if err = transaction.database.WithContext(ctx).Clauses(clause.Locking{Strength: "SHARE"}).
		First(&task, "id = ?", decision.HumanTaskID).Error; err != nil {
		return application.ConfirmationReadSet{}, err
	}
	if decision.WorkspaceID != workspaceID || decision.Decision != "approved" || decision.CreatedBy != actorID ||
		decision.SubjectRevision != task.SubjectRevision || decision.SubjectHash != task.SubjectHash ||
		task.WorkspaceID != workspaceID || task.ProjectID != projectID || task.NodeRunID != gateRecord.NodeRunID ||
		task.SubjectID != gateRecord.ID || task.SubjectHash != gateRecord.InputHash || task.Status != "COMPLETED" {
		return application.ConfirmationReadSet{}, application.ErrProductionWorldConfirmationConflict
	}

	var candidateRecord model.StageCandidateRevision
	if err = transaction.database.WithContext(ctx).Clauses(clause.Locking{Strength: "SHARE"}).
		First(&candidateRecord, "id = ?", candidateRevisionID).Error; err != nil {
		return application.ConfirmationReadSet{}, err
	}
	var candidateHead model.StageCandidateHead
	if err = transaction.database.WithContext(ctx).Clauses(clause.Locking{Strength: "SHARE"}).
		First(&candidateHead, "stage_instance_key = ?", candidateRecord.StageInstanceKey).Error; err != nil {
		return application.ConfirmationReadSet{}, err
	}
	persistedCandidate, _, decodeErr := domain.DecodeProductionWorldCandidate(json.RawMessage(candidateRecord.Candidate))
	if decodeErr != nil || candidateRecord.WorkspaceID != workspaceID || candidateRecord.OriginKind != "aggregate" ||
		candidateRecord.RevisionNo != command.CandidateRevision || candidateRecord.CandidateRevisionHash != command.CandidateRevisionHash ||
		candidateRecord.CandidateContentHash != command.Candidate.ContentHash || !reflect.DeepEqual(persistedCandidate, command.Candidate) ||
		candidateHead.WorkspaceID != workspaceID || candidateHead.CurrentRevisionID != candidateRecord.ID ||
		candidateHead.CurrentCandidateRevisionHash != candidateRecord.CandidateRevisionHash ||
		candidateHead.Revision != candidateRecord.RevisionNo {
		return application.ConfirmationReadSet{}, application.ErrProductionWorldConfirmationConflict
	}
	if err = transaction.validateStructureIdentity(ctx, workspaceID, projectID, command); err != nil {
		return application.ConfirmationReadSet{}, err
	}
	if err = transaction.validateUpstreamCandidateHeads(ctx, workspaceID, projectID, command); err != nil {
		return application.ConfirmationReadSet{}, err
	}
	decisionHash, err := platformcommand.InputHash(struct {
		ID, HumanTaskID, Decision, SubjectHash, DecisionPayloadHash, CreatedBy string
		SubjectRevision                                                        int
	}{decision.ID.String(), decision.HumanTaskID.String(), decision.Decision, decision.SubjectHash,
		decision.DecisionPayloadHash, decision.CreatedBy.String(), decision.SubjectRevision})
	if err != nil {
		return application.ConfirmationReadSet{}, err
	}
	return application.ConfirmationReadSet{ReviewDecision: domain.ReviewDecisionAuditRef{
		ID: decision.ID.String(), Revision: 1, ContentHash: decisionHash,
	}}, nil
}

func (transaction *confirmationTransaction) validateStructureIdentity(
	ctx context.Context,
	workspaceID, projectID uuid.UUID,
	command application.ConfirmProductionWorldCommand,
) error {
	versionID, _ := uuid.Parse(command.Candidate.StructureIdentitySetVersion.VersionID)
	var version model.StructureIdentitySetVersion
	if err := transaction.database.WithContext(ctx).Clauses(clause.Locking{Strength: "SHARE"}).
		First(&version, "id = ?", versionID).Error; err != nil {
		return err
	}
	var head model.StructureIdentityScopeHead
	if err := transaction.database.WithContext(ctx).Clauses(clause.Locking{Strength: "SHARE"}).
		First(&head, "project_id = ?", projectID).Error; err != nil {
		return err
	}
	var collection model.StructureIdentityCollectionReceipt
	if err := transaction.database.WithContext(ctx).Clauses(clause.Locking{Strength: "SHARE"}).
		Where("project_id = ? AND version_id = ?", projectID, versionID).First(&collection).Error; err != nil {
		return err
	}
	var projectReceipt model.ProjectEpisodeCollectionReceipt
	if err := transaction.database.WithContext(ctx).Clauses(clause.Locking{Strength: "SHARE"}).
		First(&projectReceipt, "id = ?", version.ProjectEpisodeReceiptID).Error; err != nil {
		return err
	}
	var projectMembers, committedProjectMembers []ownercollection.VersionRef
	var coveredProjectScopes []string
	if json.Unmarshal(projectReceipt.Members, &projectMembers) != nil ||
		json.Unmarshal(projectReceipt.CommittedOwnerVersionRefs, &committedProjectMembers) != nil ||
		json.Unmarshal(projectReceipt.CoveredScopeKeys, &coveredProjectScopes) != nil {
		return application.ErrProductionWorldConfirmationConflict
	}
	projectCollection, collectionErr := ownercollection.Build(ownercollection.Scope{
		WorkspaceID: workspaceID.String(), ProjectID: projectID.String(), OwnerKind: projectReceipt.OwnerKind,
		VersionFamily: projectReceipt.VersionFamily, ScopeKind: projectReceipt.ScopeKind,
		ScopeKey: projectReceipt.ScopeKey, ScopeRevision: projectReceipt.ScopeRevision,
	}, projectMembers)
	if collectionErr != nil {
		return application.ErrProductionWorldConfirmationConflict
	}
	rebuiltProjectReceipt, receiptErr := projectdomain.NewProjectEpisodeCollectionReceipt(
		projectReceipt.ID.String(), projectReceipt.CommandID.String(), projectReceipt.IdempotencyKey,
		projectReceipt.ReviewDecisionID.String(), projectCollection, projectReceipt.CommittedAt,
		projectReceipt.CommittedBy.String(),
	)
	ref := command.Candidate.StructureIdentitySetVersion
	if version.WorkspaceID != workspaceID || version.ProjectID != projectID || version.ReviewDecisionID == uuid.Nil ||
		int64(version.Version) != ref.Revision || version.ContentHash != ref.ContentHash ||
		head.WorkspaceID != workspaceID || head.CurrentVersionID != version.ID || head.HeadRevision != ref.Revision ||
		head.HeadHash != ref.ContentHash || collection.WorkspaceID != workspaceID ||
		collection.CollectionRootHash == "" || collection.ReceiptContentHash == "" ||
		receiptErr != nil || projectReceipt.WorkspaceID != workspaceID || projectReceipt.ProjectID != projectID ||
		projectReceipt.DecisionCheckpointID != projectdomain.ProjectEpisodeCheckpoint ||
		projectReceipt.OwnerKind != "production/project" || projectReceipt.VersionFamily != projectdomain.ProjectEpisodeCollectionFamily ||
		projectReceipt.ScopeKind != "project" || projectReceipt.ScopeKey != "project:"+projectID.String() ||
		projectReceipt.ScopeContentHash != projectCollection.ScopeContentHash ||
		projectReceipt.MemberCount != projectCollection.MemberCount || projectReceipt.MembersHash != projectCollection.MembersHash ||
		projectReceipt.CollectionRootHash != projectCollection.CollectionRootHash ||
		projectReceipt.ReceiptContentHash != rebuiltProjectReceipt.ReceiptContentHash ||
		!reflect.DeepEqual(projectMembers, rebuiltProjectReceipt.Members) ||
		!reflect.DeepEqual(committedProjectMembers, rebuiltProjectReceipt.CommittedOwnerVersionRefs) ||
		!reflect.DeepEqual(coveredProjectScopes, rebuiltProjectReceipt.CoveredScopeKeys) {
		return application.ErrProductionWorldConfirmationConflict
	}
	return nil
}

func (transaction *confirmationTransaction) validateUpstreamCandidateHeads(
	ctx context.Context,
	workspaceID, projectID uuid.UUID,
	command application.ConfirmProductionWorldCommand,
) error {
	values := []struct {
		stage    string
		id, hash string
	}{
		{"derive_production_entities", command.Candidate.UpstreamCandidates.ProductionEntity.CandidateRevisionID,
			command.Candidate.UpstreamCandidates.ProductionEntity.CandidateRevisionHash},
		{"bind_scene_occurrences", command.Candidate.UpstreamCandidates.SceneOccurrence.CandidateRevisionID,
			command.Candidate.UpstreamCandidates.SceneOccurrence.CandidateRevisionHash},
		{"reconcile_interaction_continuity", command.Candidate.UpstreamCandidates.InteractionContinuity.CandidateRevisionID,
			command.Candidate.UpstreamCandidates.InteractionContinuity.CandidateRevisionHash},
	}
	for _, value := range values {
		id, _ := uuid.Parse(value.id)
		var revision model.SceneAnalysisCandidateRevision
		if err := transaction.database.WithContext(ctx).Clauses(clause.Locking{Strength: "SHARE"}).
			First(&revision, "id = ?", id).Error; err != nil {
			return err
		}
		var head model.SceneAnalysisCandidateHead
		if err := transaction.database.WithContext(ctx).Clauses(clause.Locking{Strength: "SHARE"}).
			First(&head, "stage_instance_key = ?", revision.StageInstanceKey).Error; err != nil {
			return err
		}
		var invocation model.SceneAnalysisInvocationRecord
		if err := transaction.database.WithContext(ctx).Clauses(clause.Locking{Strength: "SHARE"}).
			First(&invocation, "id = ?", revision.SourceInvocationID).Error; err != nil {
			return err
		}
		if revision.WorkspaceID != workspaceID || revision.ProjectID != projectID ||
			revision.CandidateRevisionHash != value.hash || head.CurrentRevisionID != revision.ID ||
			head.CurrentCandidateRevisionHash != value.hash || head.Revision != revision.RevisionNo ||
			invocation.StageKey != value.stage || invocation.ShardKey != "script:full" || invocation.Status != "accepted" {
			return application.ErrProductionWorldConfirmationConflict
		}
	}
	return nil
}

func (transaction *confirmationTransaction) EnsureEmptyPlanningRebaseHead(
	ctx context.Context,
	workspaceID, projectID string,
	expected application.ExpectedHead,
	now time.Time,
) error {
	workspace, _ := uuid.Parse(workspaceID)
	project, _ := uuid.Parse(projectID)
	record, err := emptyPlanningRebaseHead(workspaceID, projectID, now)
	if err != nil {
		return err
	}
	var current model.ProductionWorldPlanningRebaseHead
	loadErr := transaction.database.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		First(&current, "project_id = ?", project).Error
	if expected.Revision == 0 {
		if loadErr == nil {
			return application.ErrProductionWorldConfirmationConflict
		}
		if !errors.Is(loadErr, gorm.ErrRecordNotFound) {
			return loadErr
		}
		return transaction.database.WithContext(ctx).Omit(clause.Associations).Create(&record).Error
	}
	if loadErr != nil || current.WorkspaceID != workspace || current.HeadRevision != expected.Revision ||
		current.HeadContentHash != expected.ContentHash || current.MemberCount != 0 ||
		current.ScopeContentHash != record.ScopeContentHash || current.MembersHash != record.MembersHash ||
		current.CollectionRootHash != record.CollectionRootHash || string(current.CurrentRootRefs) != "[]" {
		return application.ErrProductionWorldConfirmationConflict
	}
	return nil
}

func (transaction *confirmationTransaction) CreateCollectionReceipt(
	ctx context.Context,
	value domain.CollectionCommitReceipt,
) error {
	rebuilt, err := domain.NewCollectionCommitReceipt(domain.CollectionCommitReceiptInput{
		ID: value.ID, CommandID: value.CommandID, IdempotencyKey: value.IdempotencyKey,
		WorkspaceID: value.WorkspaceID, ProjectID: value.ProjectID,
		DecisionCheckpointID: value.DecisionCheckpointID, OwnerKind: value.OwnerKind,
		VersionFamily: value.VersionFamily, ScopeKind: value.ScopeKind, ScopeKey: value.ScopeKey,
		ScopeRevision: value.ScopeRevision, ScopeContentHash: value.ScopeContentHash,
		MembersHash: value.MembersHash, CollectionRootHash: value.CollectionRootHash,
		Members: value.Members, CoveredScopeKeys: value.CoveredScopeKeys,
		CommittedOwnerVersionRefs: value.CommittedOwnerVersionRefs,
		ReviewDecisionAuditRef:    value.ReviewDecisionAuditRef,
		CommittedAt:               value.CommittedAt, CommittedBy: value.CommittedBy,
	})
	if err != nil || !reflect.DeepEqual(rebuilt, value) {
		return errors.New("Production World collection receipt has drifted")
	}
	ids, err := parseUUIDs(value.ID, value.CommandID, value.WorkspaceID, value.ProjectID,
		value.DecisionCheckpointID, value.ReviewDecisionAuditRef.ID, value.CommittedBy)
	if err != nil {
		return err
	}
	members, _ := json.Marshal(value.Members)
	scopes, _ := json.Marshal(value.CoveredScopeKeys)
	ownerRefs, _ := json.Marshal(value.CommittedOwnerVersionRefs)
	record := model.ProductionWorldCollectionReceipt{
		ID: ids[0], CommandID: ids[1], WorkspaceID: ids[2], ProjectID: ids[3],
		DecisionCheckpointID: ids[4], ReviewDecisionID: ids[5], CommittedBy: ids[6],
		IdempotencyKey: value.IdempotencyKey, OwnerKind: value.OwnerKind, VersionFamily: value.VersionFamily,
		ScopeKind: value.ScopeKind, ScopeKey: value.ScopeKey, ScopeRevision: value.ScopeRevision,
		ScopeContentHash: value.ScopeContentHash, Members: datatypes.JSON(members), MemberCount: value.MemberCount,
		MembersHash: value.MembersHash, CollectionRootHash: value.CollectionRootHash,
		CoveredScopeKeys: datatypes.JSON(scopes), CommittedOwnerVersionRefs: datatypes.JSON(ownerRefs),
		ReviewDecisionRevision:    value.ReviewDecisionAuditRef.Revision,
		ReviewDecisionContentHash: value.ReviewDecisionAuditRef.ContentHash,
		ReceiptContentHash:        value.ReceiptContentHash, CommittedAt: value.CommittedAt,
	}
	return transaction.database.WithContext(ctx).Omit(clause.Associations).Create(&record).Error
}

func (transaction *confirmationTransaction) CreateCommandReceipt(
	ctx context.Context,
	receipt platformcommand.Receipt,
) error {
	return commandgorm.Create(ctx, transaction.database, receipt)
}

func (transaction *confirmationTransaction) AppendOutbox(
	ctx context.Context,
	value application.ConfirmationOutbox,
) error {
	ids, err := parseUUIDs(value.ID, value.WorkspaceID, value.ProjectID, value.AggregateID, value.SourceReceiptID)
	if err != nil {
		return err
	}
	record := model.OutboxEvent{
		ID: ids[0], EventType: domain.ProductionWorldConfirmedEvent, EventVersion: 1,
		WorkspaceID: ids[1], ProjectID: ids[2], AggregateKind: "production_world",
		AggregateID: ids[3].String(), AggregateRevision: value.AggregateRevision,
		SourceReceiptID: ids[4], Payload: datatypes.JSON(value.Payload), PayloadHash: value.PayloadHash,
		Status: "pending", OccurredAt: value.OccurredAt, CreatedAt: value.OccurredAt,
	}
	return transaction.database.WithContext(ctx).Omit(clause.Associations).Create(&record).Error
}

func emptyPlanningRebaseHead(workspaceID, projectID string, now time.Time) (model.ProductionWorldPlanningRebaseHead, error) {
	scopeKey := "project:" + projectID
	scopeHash, err := platformcommand.InputHash(struct {
		Schema, OwnerKind, Family, ScopeKey string
		ScopeRevision                       int64
		RootRefs                            []domain.CollectionMemberRef
	}{"production-world-planning-rebase-scope", "production/planning", domain.PlanningStructureRebaseFamily,
		scopeKey, 1, []domain.CollectionMemberRef{}})
	if err != nil {
		return model.ProductionWorldPlanningRebaseHead{}, err
	}
	membersHash, err := platformcommand.InputHash(struct {
		Schema  string
		Members []domain.CollectionMemberRef
	}{"production-world-planning-rebase-members", []domain.CollectionMemberRef{}})
	if err != nil {
		return model.ProductionWorldPlanningRebaseHead{}, err
	}
	rootHash, err := platformcommand.InputHash(struct {
		Schema, Family, ScopeKey, ScopeContentHash, MembersHash string
		ScopeRevision                                           int64
		MemberCount                                             int
	}{"production-world-planning-rebase-collection", domain.PlanningStructureRebaseFamily,
		scopeKey, scopeHash, membersHash, 1, 0})
	if err != nil {
		return model.ProductionWorldPlanningRebaseHead{}, err
	}
	headHash, err := platformcommand.InputHash(struct {
		Schema, WorkspaceID, ProjectID, ScopeKey, ScopeContentHash, MembersHash, CollectionRootHash string
		ScopeRevision, HeadRevision                                                                 int64
		MemberCount                                                                                 int
	}{"production-world-planning-rebase-head", workspaceID, projectID, scopeKey, scopeHash,
		membersHash, rootHash, 1, 1, 0})
	if err != nil {
		return model.ProductionWorldPlanningRebaseHead{}, err
	}
	workspace, _ := uuid.Parse(workspaceID)
	project, _ := uuid.Parse(projectID)
	return model.ProductionWorldPlanningRebaseHead{
		ProjectID: project, WorkspaceID: workspace, ScopeKey: scopeKey, ScopeRevision: 1,
		ScopeContentHash: scopeHash, MemberCount: 0, MembersHash: membersHash,
		CollectionRootHash: rootHash, CurrentRootRefs: datatypes.JSON([]byte("[]")),
		HeadRevision: 1, HeadContentHash: headHash, UpdatedAt: now.UTC(),
	}, nil
}

func sameExpectedHeads(left []workflowdomain.ProductionWorldExpectedHead, right []application.ExpectedHead) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index].OwnerKind != right[index].OwnerKind || left[index].VersionFamily != right[index].VersionFamily ||
			left[index].ScopeKind != right[index].ScopeKind || left[index].ScopeKey != right[index].ScopeKey ||
			left[index].Revision != right[index].Revision || left[index].ContentHash != right[index].ContentHash {
			return false
		}
	}
	return true
}

func parseUUIDs(values ...string) ([]uuid.UUID, error) {
	result := make([]uuid.UUID, len(values))
	for index, value := range values {
		parsed, err := uuid.Parse(value)
		if err != nil {
			return nil, err
		}
		result[index] = parsed
	}
	return result, nil
}

var _ application.ConfirmationTransactionManager = (*Store)(nil)
