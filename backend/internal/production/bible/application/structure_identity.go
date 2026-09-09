package application

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	eventingdomain "github.com/StephenQiu30/lanverse/backend/internal/eventing/domain"
	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
	"github.com/StephenQiu30/lanverse/backend/internal/production/bible/domain"
)

type ConfirmStructureIdentitySetCommand struct {
	WorkspaceID, ProjectID                       string
	GateInputID, GateInputHash, ReviewDecisionID string
	ProjectEpisodeReceiptID                      string
	DocumentRevisionID, DocumentRevisionHash     string
	SpanIndexID, SpanIndexHash                   string
	ExpectedHeadRevision                         int64
	ExpectedHeadHash, IdempotencyKey             string
	CandidateRefs                                []domain.StructureIdentityCandidateRef
	SceneRefs                                    []domain.StructureIdentitySceneRef
	Identities                                   []domain.StructureIdentity
	MentionMappings                              []domain.StructureIdentityMentionMapping
	Coverage                                     domain.StructureIdentityCoverage
}

type StructureIdentitySource struct {
	WorkspaceID, ProjectID, DocumentRevisionID, DocumentRevisionHash string
	SpanIndexID, SpanIndexHash                                       string
	CodepointCount                                                   int
}

type StructureIdentityHead struct {
	CurrentVersionID, HeadHash string
	HeadRevision               int64
}

type StructureIdentityEpisodeCheckpoint struct {
	GateInputID, GateInputHash, ReviewDecisionID string
	SourceVersionID, SourceHash                  string
	CollectionRootHash                           string
	Episodes                                     []domain.EpisodeLifecycleRef
}

type StructureIdentityAudit struct {
	WorkspaceID, ActorID, ProjectID, VersionID, ReviewDecisionID string
	Version                                                      int
	CollectionRootHash                                           string
	OccurredAt                                                   time.Time
}

type StructureIdentityOutbox struct {
	ID, WorkspaceID, ProjectID, VersionID, SourceReceiptID string
	Version, EventVersion                                  int
	Payload                                                json.RawMessage
	PayloadHash                                            string
	OccurredAt                                             time.Time
}

type StructureIdentityTransactionManager interface {
	WithinStructureIdentityTransaction(context.Context, func(StructureIdentityRepository) error) error
}

type StructureIdentityRepository interface {
	AuthorizeStructureIdentity(context.Context, Actor, string) error
	FindStructureIdentityCommandReceipt(context.Context, string, string) (platformcommand.Receipt, error)
	GetStructureIdentitySource(context.Context, string, string, bool) (StructureIdentitySource, error)
	GetEpisodeLifecycleReceipt(context.Context, string, string, string) (StructureIdentityEpisodeCheckpoint, error)
	GetStructureIdentityHead(context.Context, string, bool) (StructureIdentityHead, bool, error)
	GetStructureIdentityVersion(context.Context, string) (domain.StructureIdentitySetVersion, error)
	CreateStructureIdentityVersion(context.Context, domain.StructureIdentitySetVersion) error
	SaveStructureIdentityHead(context.Context, string, string, StructureIdentityHead, time.Time) error
	CreateStructureIdentityCollectionReceipt(context.Context, domain.StructureIdentityCollectionReceipt, string, string, time.Time) error
	CreateStructureIdentityCommandReceipt(context.Context, platformcommand.Receipt) error
	AppendStructureIdentityAudit(context.Context, StructureIdentityAudit) error
	AppendStructureIdentityOutbox(context.Context, StructureIdentityOutbox) error
}

func (service *Service) ConfirmStructureIdentitySet(
	ctx context.Context,
	actor Actor,
	command ConfirmStructureIdentitySetCommand,
) (domain.ConfirmStructureIdentitySetResult, error) {
	command = normalizeStructureIdentityCommand(command)
	if err := validateStructureIdentityCommand(command); err != nil {
		return domain.ConfirmStructureIdentitySetResult{}, err
	}
	manager, ok := service.transactions.(StructureIdentityTransactionManager)
	if !ok || service.config.Now == nil || service.config.NewID == nil {
		return domain.ConfirmStructureIdentitySetResult{}, errors.New("Production Bible structure identity owner is unavailable")
	}
	inputHash, err := platformcommand.InputHash(command)
	if err != nil {
		return domain.ConfirmStructureIdentitySetResult{}, err
	}
	var result domain.ConfirmStructureIdentitySetResult
	err = manager.WithinStructureIdentityTransaction(ctx, func(repository StructureIdentityRepository) error {
		if authorizeErr := repository.AuthorizeStructureIdentity(ctx, actor, command.ProjectID); authorizeErr != nil {
			return authorizeErr
		}
		receipt, receiptErr := repository.FindStructureIdentityCommandReceipt(ctx, command.WorkspaceID, command.IdempotencyKey)
		if receiptErr == nil {
			result, receiptErr = platformcommand.Replay[domain.ConfirmStructureIdentitySetResult](receipt, inputHash)
			if errors.Is(receiptErr, platformcommand.ErrInputMismatch) {
				return conflict("Idempotency key was already used with different input")
			}
			return receiptErr
		}
		if !errors.Is(receiptErr, platformcommand.ErrReceiptNotFound) {
			return receiptErr
		}

		source, loadErr := repository.GetStructureIdentitySource(ctx, command.ProjectID, command.DocumentRevisionID, true)
		if loadErr != nil {
			return loadErr
		}
		if source.WorkspaceID != command.WorkspaceID || source.ProjectID != command.ProjectID ||
			source.DocumentRevisionID != command.DocumentRevisionID || source.DocumentRevisionHash != command.DocumentRevisionHash ||
			source.SpanIndexID != command.SpanIndexID || source.SpanIndexHash != command.SpanIndexHash ||
			source.CodepointCount != command.SceneRefs[len(command.SceneRefs)-1].SourceEnd {
			return conflict("Production Bible structure identity source has changed")
		}
		episodeCheckpoint, loadErr := repository.GetEpisodeLifecycleReceipt(
			ctx, command.ProjectEpisodeReceiptID, command.WorkspaceID, command.ProjectID,
		)
		if loadErr != nil {
			return loadErr
		}
		if episodeCheckpoint.GateInputID != command.GateInputID || episodeCheckpoint.GateInputHash != command.GateInputHash ||
			episodeCheckpoint.ReviewDecisionID != command.ReviewDecisionID ||
			episodeCheckpoint.SourceVersionID != command.DocumentRevisionID || episodeCheckpoint.SourceHash != command.DocumentRevisionHash {
			return conflict("Project Episode lifecycle receipt does not match the Gate 1 decision")
		}
		episodes, projectRootHash := episodeCheckpoint.Episodes, episodeCheckpoint.CollectionRootHash
		if err = validateStructureIdentityScenes(command.ProjectID, command.SceneRefs, episodes, source.CodepointCount); err != nil {
			return err
		}

		head, found, loadErr := repository.GetStructureIdentityHead(ctx, command.ProjectID, true)
		if loadErr != nil {
			return loadErr
		}
		if (!found && (command.ExpectedHeadRevision != 0 || command.ExpectedHeadHash != "")) ||
			(found && (head.HeadRevision != command.ExpectedHeadRevision || head.HeadHash != command.ExpectedHeadHash)) {
			return conflict("Production Bible structure identity Head has changed")
		}
		var parentVersionID *string
		if found {
			parent, parentErr := repository.GetStructureIdentityVersion(ctx, head.CurrentVersionID)
			if parentErr != nil {
				return parentErr
			}
			parentVersionID = &parent.ID
			if err = validateStructureIdentityReuse(command.Identities, parent.Identities); err != nil {
				return err
			}
		} else if err = validateStructureIdentityReuse(command.Identities, nil); err != nil {
			return err
		}

		now := service.config.Now().UTC()
		versionID := service.config.NewID()
		versionNumber := int(command.ExpectedHeadRevision) + 1
		contentHash, hashErr := structureIdentityContentHash(command, episodes, parentVersionID, projectRootHash, versionNumber)
		if hashErr != nil {
			return hashErr
		}
		result.Version = domain.StructureIdentitySetVersion{
			SchemaVersion: domain.StructureIdentitySetSchemaVersion, ID: versionID,
			WorkspaceID: command.WorkspaceID, ProjectID: command.ProjectID, Version: versionNumber,
			ParentVersionID: parentVersionID, GateInputID: command.GateInputID, GateInputHash: command.GateInputHash,
			ReviewDecisionID: command.ReviewDecisionID, ProjectEpisodeReceiptID: command.ProjectEpisodeReceiptID,
			DocumentRevisionID: command.DocumentRevisionID, SpanIndexID: command.SpanIndexID,
			CandidateRefs: command.CandidateRefs, EpisodeRefs: episodes, SceneRefs: command.SceneRefs,
			Identities: command.Identities, MentionMappings: command.MentionMappings, Coverage: command.Coverage,
			ContentHash: contentHash, CreatedBy: actor.UserID, CreatedAt: now,
		}
		if createErr := repository.CreateStructureIdentityVersion(ctx, result.Version); createErr != nil {
			return createErr
		}
		newHead := StructureIdentityHead{CurrentVersionID: versionID, HeadRevision: int64(versionNumber), HeadHash: contentHash}
		if saveErr := repository.SaveStructureIdentityHead(ctx, command.WorkspaceID, command.ProjectID, newHead, now); saveErr != nil {
			return saveErr
		}

		scopes := make([]string, len(command.SceneRefs))
		for index, scene := range command.SceneRefs {
			scopes[index] = scene.ScopeKey
		}
		collectionRootHash, hashErr := platformcommand.InputHash(struct {
			Family, VersionID, VersionHash string
			CoveredScopeKeys               []string
		}{domain.StructureIdentityCollectionFamily, versionID, contentHash, scopes})
		if hashErr != nil {
			return hashErr
		}
		collectionReceiptID := service.config.NewID()
		receiptContentHash, hashErr := platformcommand.InputHash(struct {
			CheckpointKey, Family, VersionID, VersionHash, ReviewDecisionID, CollectionRootHash string
			CoveredScopeKeys                                                                    []string
		}{domain.StructureIdentityCheckpointKey, domain.StructureIdentityCollectionFamily, versionID, contentHash, command.ReviewDecisionID, collectionRootHash, scopes})
		if hashErr != nil {
			return hashErr
		}
		result.Receipt = domain.StructureIdentityCollectionReceipt{
			ID: collectionReceiptID, CheckpointKey: domain.StructureIdentityCheckpointKey,
			CollectionFamily: domain.StructureIdentityCollectionFamily, VersionID: versionID,
			VersionContentHash: contentHash, ReviewDecisionID: command.ReviewDecisionID,
			CoveredScopeKeys: scopes, CollectionRootHash: collectionRootHash, ReceiptContentHash: receiptContentHash,
		}
		if createErr := repository.CreateStructureIdentityCollectionReceipt(
			ctx, result.Receipt, command.WorkspaceID, command.ProjectID, now,
		); createErr != nil {
			return createErr
		}
		commandReceiptID := service.config.NewID()
		result.CommandReceiptID, result.CommandOperation = commandReceiptID, domain.StructureIdentityCommandOperation
		encodedResult, encodeErr := platformcommand.Result(result)
		if encodeErr != nil {
			return encodeErr
		}
		if createErr := repository.CreateStructureIdentityCommandReceipt(ctx, platformcommand.Receipt{
			ID: commandReceiptID, WorkspaceID: command.WorkspaceID, Operation: domain.StructureIdentityCommandOperation,
			IdempotencyKey: command.IdempotencyKey, InputHash: inputHash, ResourceID: versionID,
			Result: encodedResult, CreatedBy: actor.UserID, CreatedAt: now,
		}); createErr != nil {
			return createErr
		}
		if auditErr := repository.AppendStructureIdentityAudit(ctx, StructureIdentityAudit{
			WorkspaceID: command.WorkspaceID, ActorID: actor.UserID, ProjectID: command.ProjectID,
			VersionID: versionID, ReviewDecisionID: command.ReviewDecisionID, Version: versionNumber,
			CollectionRootHash: collectionRootHash, OccurredAt: now,
		}); auditErr != nil {
			return auditErr
		}
		payload, encodeErr := json.Marshal(map[string]any{
			"schema": domain.StructureIdentitySetSchemaVersion, "version_id": versionID,
			"version": versionNumber, "content_hash": contentHash, "collection_receipt_id": collectionReceiptID,
			"collection_root_hash": collectionRootHash,
		})
		if encodeErr != nil {
			return encodeErr
		}
		payloadHash, hashErr := eventingdomain.HashPayload(payload)
		if hashErr != nil {
			return hashErr
		}
		return repository.AppendStructureIdentityOutbox(ctx, StructureIdentityOutbox{
			ID: service.config.NewID(), WorkspaceID: command.WorkspaceID, ProjectID: command.ProjectID,
			VersionID: versionID, SourceReceiptID: collectionReceiptID, Version: versionNumber,
			EventVersion: 1, Payload: payload, PayloadHash: payloadHash, OccurredAt: now,
		})
	})
	return result, normalizeError(err)
}

func normalizeStructureIdentityCommand(command ConfirmStructureIdentitySetCommand) ConfirmStructureIdentitySetCommand {
	command.WorkspaceID, command.ProjectID = strings.TrimSpace(command.WorkspaceID), strings.TrimSpace(command.ProjectID)
	command.GateInputID, command.ReviewDecisionID = strings.TrimSpace(command.GateInputID), strings.TrimSpace(command.ReviewDecisionID)
	command.ProjectEpisodeReceiptID = strings.TrimSpace(command.ProjectEpisodeReceiptID)
	command.DocumentRevisionID, command.SpanIndexID = strings.TrimSpace(command.DocumentRevisionID), strings.TrimSpace(command.SpanIndexID)
	command.GateInputHash = strings.ToLower(strings.TrimSpace(command.GateInputHash))
	command.DocumentRevisionHash = strings.ToLower(strings.TrimSpace(command.DocumentRevisionHash))
	command.SpanIndexHash = strings.ToLower(strings.TrimSpace(command.SpanIndexHash))
	command.ExpectedHeadHash = strings.ToLower(strings.TrimSpace(command.ExpectedHeadHash))
	command.IdempotencyKey = strings.TrimSpace(command.IdempotencyKey)
	command.CandidateRefs = append([]domain.StructureIdentityCandidateRef(nil), command.CandidateRefs...)
	command.SceneRefs = append([]domain.StructureIdentitySceneRef(nil), command.SceneRefs...)
	command.Identities = append([]domain.StructureIdentity(nil), command.Identities...)
	command.MentionMappings = append([]domain.StructureIdentityMentionMapping(nil), command.MentionMappings...)
	for index := range command.Identities {
		command.Identities[index].Aliases = append([]string(nil), command.Identities[index].Aliases...)
		slices.Sort(command.Identities[index].Aliases)
	}
	slices.SortFunc(command.CandidateRefs, func(left, right domain.StructureIdentityCandidateRef) int {
		return strings.Compare(left.StageKey, right.StageKey)
	})
	slices.SortFunc(command.SceneRefs, compareStructureIdentityScene)
	slices.SortFunc(command.Identities, func(left, right domain.StructureIdentity) int {
		return strings.Compare(left.TemporaryIdentityKey, right.TemporaryIdentityKey)
	})
	slices.SortFunc(command.MentionMappings, compareStructureIdentityMention)
	return command
}

func validateStructureIdentityCommand(command ConfirmStructureIdentitySetCommand) error {
	for _, identifier := range []string{command.WorkspaceID, command.ProjectID, command.GateInputID, command.ReviewDecisionID,
		command.ProjectEpisodeReceiptID, command.DocumentRevisionID, command.SpanIndexID} {
		if _, err := uuid.Parse(identifier); err != nil {
			return invalid("Invalid Production Bible structure identity")
		}
	}
	for _, hash := range []string{command.GateInputHash, command.DocumentRevisionHash, command.SpanIndexHash} {
		if !validStructureIdentityHash(hash) {
			return invalid("Invalid Production Bible structure identity hash")
		}
	}
	if command.ExpectedHeadRevision < 0 || (command.ExpectedHeadRevision == 0) != (command.ExpectedHeadHash == "") ||
		(command.ExpectedHeadRevision > 0 && !validStructureIdentityHash(command.ExpectedHeadHash)) ||
		command.IdempotencyKey == "" || len(command.IdempotencyKey) > 200 || len(command.SceneRefs) == 0 {
		return invalid("Invalid Production Bible structure identity command")
	}
	expectedStages := []string{"extract_scene_facts", "propose_script_spans", "resolve_identities", "review_candidate"}
	if len(command.CandidateRefs) != len(expectedStages) {
		return invalid("Incomplete Production Bible Candidate lineage")
	}
	candidateIDs := make(map[string]struct{}, len(command.CandidateRefs))
	invocationIDs := make(map[string]struct{}, len(command.CandidateRefs))
	for index, ref := range command.CandidateRefs {
		if ref.StageKey != expectedStages[index] || ref.ShardKey != "script:full" ||
			!validStructureIdentityHash(ref.CandidateRevisionHash) || !validStructureIdentityHash(ref.SourceResultHash) ||
			!validStructureIdentityHash(ref.SkillReleaseHash) || !validStructureIdentityHash(ref.StageReleaseHash) ||
			!validStructureIdentityHash(ref.BundleContentHash) ||
			!strings.HasPrefix(ref.AgentImageDigest, "sha256:") ||
			!validStructureIdentityHash(strings.TrimPrefix(ref.AgentImageDigest, "sha256:")) {
			return invalid("Invalid Production Bible Candidate lineage")
		}
		for _, identifier := range []string{ref.CandidateRevisionID, ref.SourceInvocationID, ref.SkillReleaseID} {
			if _, err := uuid.Parse(identifier); err != nil {
				return invalid("Invalid Production Bible Candidate identity")
			}
		}
		if _, duplicate := candidateIDs[ref.CandidateRevisionID]; duplicate {
			return invalid("Duplicate Production Bible Candidate revision")
		}
		if _, duplicate := invocationIDs[ref.SourceInvocationID]; duplicate {
			return invalid("Duplicate Production Bible Candidate invocation")
		}
		candidateIDs[ref.CandidateRevisionID], invocationIDs[ref.SourceInvocationID] = struct{}{}, struct{}{}
	}
	identityKinds := make(map[string]string, len(command.Identities))
	temporaryIdentityKeys := make(map[string]struct{}, len(command.Identities))
	for _, identity := range command.Identities {
		if _, err := uuid.Parse(identity.IdentityKey); err != nil ||
			(identity.Kind != "character" && identity.Kind != "location" && identity.Kind != "prop") ||
			!strings.HasPrefix(identity.TemporaryIdentityKey, "identity_"+identity.Kind+"_") ||
			strings.TrimSpace(identity.CanonicalName) == "" || len(identity.Aliases) == 0 {
			return invalid("Invalid Production Bible identity decision")
		}
		if _, duplicate := identityKinds[identity.IdentityKey]; duplicate {
			return invalid("Duplicate Production Bible identity key")
		}
		if _, duplicate := temporaryIdentityKeys[identity.TemporaryIdentityKey]; duplicate {
			return invalid("Duplicate Production Bible temporary identity key")
		}
		identityKinds[identity.IdentityKey] = identity.Kind
		temporaryIdentityKeys[identity.TemporaryIdentityKey] = struct{}{}
		switch identity.Resolution {
		case "new":
			if identity.ReuseIdentityKey != nil || identity.IdentityKey != deterministicStructureIdentityKey(command.ProjectID, identity.TemporaryIdentityKey) {
				return invalid("Invalid new Production Bible identity key")
			}
		case "reuse":
			if identity.ReuseIdentityKey == nil || identity.IdentityKey != *identity.ReuseIdentityKey {
				return invalid("Invalid reused Production Bible identity key")
			}
		default:
			return invalid("Invalid Production Bible identity resolution")
		}
		canonicalAlias := false
		for index, alias := range identity.Aliases {
			if strings.TrimSpace(alias) == "" || (index > 0 && identity.Aliases[index-1] == alias) {
				return invalid("Invalid Production Bible identity aliases")
			}
			canonicalAlias = canonicalAlias || alias == identity.CanonicalName
		}
		if !canonicalAlias {
			return invalid("Production Bible identity aliases omit the canonical name")
		}
	}
	sceneBounds := make(map[string][2]int, len(command.SceneRefs))
	for _, scene := range command.SceneRefs {
		sceneBounds[scene.TemporarySceneID] = [2]int{scene.SourceStart, scene.SourceEnd}
	}
	resolved := 0
	for index, mention := range command.MentionMappings {
		bounds, sceneExists := sceneBounds[mention.TemporarySceneID]
		if (mention.Kind != "character" && mention.Kind != "location" && mention.Kind != "prop") ||
			(mention.OccurrenceRole != "actual" && mention.OccurrenceRole != "mentioned_only") ||
			(mention.Kind == "location" && mention.OccurrenceRole != "actual") ||
			mention.SourceStart < 0 || mention.SourceEnd <= mention.SourceStart || !validStructureIdentityHash(mention.TextHash) ||
			strings.TrimSpace(mention.ExactAnchor) == "" || strings.TrimSpace(mention.TemporarySceneID) == "" ||
			!sceneExists || mention.SourceStart < bounds[0] || mention.SourceEnd > bounds[1] ||
			(index > 0 && compareStructureIdentityMention(command.MentionMappings[index-1], mention) == 0) {
			return invalid("Invalid Production Bible mention mapping")
		}
		switch mention.Resolution {
		case "resolved":
			if mention.IdentityKey == nil || identityKinds[*mention.IdentityKey] != mention.Kind {
				return invalid("Production Bible mention maps to an invalid identity")
			}
			resolved++
		case "unresolved":
			if mention.IdentityKey != nil {
				return invalid("Unresolved Production Bible mention has an identity")
			}
		default:
			return invalid("Invalid Production Bible mention resolution")
		}
	}
	mentionHash, err := platformcommand.InputHash(command.MentionMappings)
	if err != nil {
		return err
	}
	scopes := make([]string, len(command.SceneRefs))
	for index, scene := range command.SceneRefs {
		scopes[index] = scene.ScopeKey
	}
	scopeHash, err := platformcommand.InputHash(scopes)
	if err != nil {
		return err
	}
	if command.Coverage.SceneCount != len(command.SceneRefs) || command.Coverage.IdentityCount != len(command.Identities) ||
		command.Coverage.MentionCount != len(command.MentionMappings) || command.Coverage.ResolvedCount != resolved ||
		command.Coverage.UnresolvedCount != len(command.MentionMappings)-resolved ||
		command.Coverage.MentionUniverseHash != mentionHash || command.Coverage.ScopeSetHash != scopeHash {
		return invalid("Production Bible structure identity coverage is incomplete")
	}
	return nil
}

func validateStructureIdentityScenes(projectID string, scenes []domain.StructureIdentitySceneRef, episodes []domain.EpisodeLifecycleRef, codepointCount int) error {
	byTemporaryID := make(map[string]domain.EpisodeLifecycleRef, len(episodes))
	for _, episode := range episodes {
		byTemporaryID[episode.TemporaryEpisodeID] = episode
	}
	previousEnd := 0
	seenIDs := make(map[string]struct{}, len(scenes))
	seenTemps := make(map[string]struct{}, len(scenes))
	seenSpans := make(map[string]struct{}, len(scenes))
	for index, scene := range scenes {
		episode, exists := byTemporaryID[scene.TemporaryEpisodeID]
		if !exists || scene.EpisodeID != episode.EpisodeID || scene.SourceStart != previousEnd || scene.SourceEnd <= scene.SourceStart ||
			scene.SourceStart < episode.SourceStart || scene.SourceEnd > episode.SourceEnd ||
			scene.SceneOwnerLogicalID != deterministicStructureSceneID(projectID, scene.TemporarySpanID) ||
			scene.ScopeKey != "scene:"+scene.SceneOwnerLogicalID || !validStructureIdentityHash(scene.EvidenceHash) ||
			strings.TrimSpace(scene.TemporarySceneID) == "" || strings.TrimSpace(scene.TemporarySpanID) == "" ||
			(index > 0 && compareStructureIdentityScene(scenes[index-1], scene) >= 0) {
			return invalid("Invalid Production Bible Scene coverage")
		}
		if _, duplicate := seenIDs[scene.SceneOwnerLogicalID]; duplicate {
			return invalid("Duplicate Production Bible Scene identity")
		}
		if _, duplicate := seenTemps[scene.TemporarySceneID]; duplicate {
			return invalid("Duplicate Production Bible temporary Scene")
		}
		if _, duplicate := seenSpans[scene.TemporarySpanID]; duplicate {
			return invalid("Duplicate Production Bible temporary Scene span")
		}
		seenIDs[scene.SceneOwnerLogicalID], seenTemps[scene.TemporarySceneID], seenSpans[scene.TemporarySpanID] = struct{}{}, struct{}{}, struct{}{}
		previousEnd = scene.SourceEnd
	}
	if previousEnd != codepointCount {
		return invalid("Incomplete Production Bible Scene coverage")
	}
	return nil
}

func validateStructureIdentityReuse(current, previous []domain.StructureIdentity) error {
	previousKinds := make(map[string]string, len(previous))
	for _, identity := range previous {
		previousKinds[identity.IdentityKey] = identity.Kind
	}
	for _, identity := range current {
		previousKind, exists := previousKinds[identity.IdentityKey]
		if identity.Resolution == "reuse" && (!exists || previousKind != identity.Kind) {
			return conflict("Reused Production Bible identity is outside the current Head")
		}
		if identity.Resolution == "new" && exists {
			return conflict("Existing Production Bible identity must be explicitly reused")
		}
	}
	return nil
}

func structureIdentityContentHash(command ConfirmStructureIdentitySetCommand, episodes []domain.EpisodeLifecycleRef, parentVersionID *string, projectRootHash string, version int) (string, error) {
	value := domain.StructureIdentitySetVersion{
		SchemaVersion: domain.StructureIdentitySetSchemaVersion,
		WorkspaceID:   command.WorkspaceID, ProjectID: command.ProjectID, Version: version,
		ParentVersionID: parentVersionID, GateInputID: command.GateInputID, GateInputHash: command.GateInputHash,
		ReviewDecisionID: command.ReviewDecisionID, ProjectEpisodeReceiptID: command.ProjectEpisodeReceiptID,
		DocumentRevisionID: command.DocumentRevisionID, SpanIndexID: command.SpanIndexID,
		CandidateRefs: command.CandidateRefs, EpisodeRefs: episodes, SceneRefs: command.SceneRefs,
		Identities: command.Identities, MentionMappings: command.MentionMappings, Coverage: command.Coverage,
	}
	return HashStructureIdentityVersionContent(
		value,
		command.DocumentRevisionHash,
		command.SpanIndexHash,
		projectRootHash,
	)
}

func HashStructureIdentityVersionContent(
	value domain.StructureIdentitySetVersion,
	documentRevisionHash, spanIndexHash, projectRootHash string,
) (string, error) {
	return platformcommand.InputHash(struct {
		SchemaVersion, WorkspaceID, ProjectID                                 string
		Version                                                               int
		ParentVersionID                                                       *string
		GateInputID, GateInputHash, ReviewDecisionID, ProjectEpisodeReceiptID string
		ProjectRootHash, DocumentRevisionID, DocumentRevisionHash             string
		SpanIndexID, SpanIndexHash                                            string
		CandidateRefs                                                         []domain.StructureIdentityCandidateRef
		EpisodeRefs                                                           []domain.EpisodeLifecycleRef
		SceneRefs                                                             []domain.StructureIdentitySceneRef
		Identities                                                            []domain.StructureIdentity
		MentionMappings                                                       []domain.StructureIdentityMentionMapping
		Coverage                                                              domain.StructureIdentityCoverage
	}{
		domain.StructureIdentitySetSchemaVersion, value.WorkspaceID, value.ProjectID,
		value.Version, value.ParentVersionID, value.GateInputID, value.GateInputHash, value.ReviewDecisionID,
		value.ProjectEpisodeReceiptID, projectRootHash, value.DocumentRevisionID, documentRevisionHash,
		value.SpanIndexID, spanIndexHash, value.CandidateRefs, value.EpisodeRefs, value.SceneRefs,
		value.Identities, value.MentionMappings, value.Coverage,
	})
}

func deterministicStructureSceneID(projectID, temporarySpanID string) string {
	return uuid.NewSHA1(uuid.MustParse(projectID), []byte("lanverse:scene:"+temporarySpanID)).String()
}

func deterministicStructureIdentityKey(projectID, temporaryIdentityKey string) string {
	return uuid.NewSHA1(uuid.MustParse(projectID), []byte("lanverse:identity:"+temporaryIdentityKey)).String()
}

func compareStructureIdentityScene(left, right domain.StructureIdentitySceneRef) int {
	if left.SourceStart != right.SourceStart {
		return left.SourceStart - right.SourceStart
	}
	return strings.Compare(left.TemporarySpanID, right.TemporarySpanID)
}

func compareStructureIdentityMention(left, right domain.StructureIdentityMentionMapping) int {
	if left.SourceStart != right.SourceStart {
		return left.SourceStart - right.SourceStart
	}
	if left.SourceEnd != right.SourceEnd {
		return left.SourceEnd - right.SourceEnd
	}
	if compared := strings.Compare(left.Kind, right.Kind); compared != 0 {
		return compared
	}
	if compared := strings.Compare(left.OccurrenceRole, right.OccurrenceRole); compared != 0 {
		return compared
	}
	return strings.Compare(left.TemporarySceneID, right.TemporarySceneID)
}

func validStructureIdentityHash(value string) bool {
	if len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
