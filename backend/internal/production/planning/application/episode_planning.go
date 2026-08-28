package application

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
	bibledomain "github.com/StephenQiu30/lanverse/backend/internal/production/bible/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/production/planning/domain"
)

const applyEpisodePlanningOperation = "episode_planning.apply"

type PlanningEpisodeSource struct {
	EpisodeID, ScriptVersionID, DocumentRevisionID string
	EpisodePosition, ScriptVersion                 int
	SourceStart, SourceEnd                         int
	Content, ContentHash                           string
}

type PlanningIdentitySource struct {
	EntityKey     string
	Asset         bibledomain.MaterializedAsset
	Specification bibledomain.MaterializedSpecification
	States        []bibledomain.MaterializedState
}

type EpisodePlanningCandidateSource struct {
	CandidateRevisionID, CandidateRevisionHash string
	CandidateRevision                          int64
	WorkspaceID, ProjectID                     string
	BibleVersionID, BibleContentHash           string
	BibleVersion                               int
	MaterializationHash                        string
	Candidate                                  domain.EpisodePlanningCandidateSet
	Episodes                                   []PlanningEpisodeSource
	Identities                                 []PlanningIdentitySource
}

type PlanningFragmentReference struct {
	TemporaryKey string `json:"temporary_key"`
	Kind         string `json:"kind"`
	FragmentID   string `json:"fragment_id"`
}

type PlanningStructureReference struct {
	StructureID     string                      `json:"structure_id"`
	EpisodeID       string                      `json:"episode_id"`
	ScriptVersionID string                      `json:"script_version_id"`
	ResultHash      string                      `json:"result_hash"`
	Revision        int                         `json:"revision"`
	Fragments       []PlanningFragmentReference `json:"fragments"`
}

type PlanningOwnerSetReference struct {
	ID                    string                       `json:"id"`
	WorkspaceID           string                       `json:"workspace_id"`
	ProjectID             string                       `json:"project_id"`
	CandidateRevisionID   string                       `json:"candidate_revision_id"`
	CandidateRevisionHash string                       `json:"candidate_revision_hash"`
	CandidateRevision     int64                        `json:"candidate_revision"`
	ReviewDecisionID      string                       `json:"review_decision_id"`
	BibleVersionID        string                       `json:"bible_version_id"`
	BibleVersion          int                          `json:"bible_version"`
	BibleContentHash      string                       `json:"bible_content_hash"`
	MaterializationHash   string                       `json:"materialization_hash"`
	ContentHash           string                       `json:"content_hash"`
	Structures            []PlanningStructureReference `json:"structures"`
}

type ApplyEpisodePlanningCandidateCommand struct {
	WorkspaceID, ProjectID                     string
	CandidateRevisionID, CandidateRevisionHash string
	ExpectedCandidateRevision                  int64
	ReviewDecisionID, IdempotencyKey           string
}

type ApplyEpisodePlanningCandidateResult struct {
	Set     PlanningOwnerSetReference
	Receipt platformcommand.Receipt
}

type EpisodePlanningRepository interface {
	GetEpisodePlanningCandidate(context.Context, Actor, string, bool) (EpisodePlanningCandidateSource, error)
	FindReceipt(context.Context, string, string, string) (platformcommand.Receipt, error)
	CreateReceipt(context.Context, platformcommand.Receipt) error
	GetPlanningOwnerSet(context.Context, Actor, PlanningOwnerSetReference) ([]domain.Structure, error)
	CreatePlanningOwnerSet(context.Context, []domain.Structure) error
}

type EpisodePlanningTransactionManager interface {
	WithinEpisodePlanningTransaction(context.Context, func(EpisodePlanningRepository) error) error
}

type EpisodePlanningService struct {
	transactions EpisodePlanningTransactionManager
	config       Config
}

func NewEpisodePlanningService(
	transactions EpisodePlanningTransactionManager,
	config Config,
) *EpisodePlanningService {
	return &EpisodePlanningService{transactions: transactions, config: config}
}

func (service *EpisodePlanningService) ApplyEpisodePlanningCandidate(
	ctx context.Context,
	actor Actor,
	command ApplyEpisodePlanningCandidateCommand,
) (ApplyEpisodePlanningCandidateResult, error) {
	trimEpisodePlanningCommand(&command)
	if service == nil || service.transactions == nil || service.config.Now == nil || service.config.NewID == nil {
		return ApplyEpisodePlanningCandidateResult{}, errors.New("Episode Planning owner service is unavailable")
	}
	for _, identifier := range []string{
		command.WorkspaceID, command.ProjectID, command.CandidateRevisionID,
		command.ReviewDecisionID, actor.UserID,
	} {
		if _, err := uuid.Parse(identifier); err != nil {
			return ApplyEpisodePlanningCandidateResult{}, invalid("Invalid Episode Planning application identity")
		}
	}
	if len(command.CandidateRevisionHash) != 64 || command.ExpectedCandidateRevision < 1 ||
		actor.TokenVersion < 1 || command.IdempotencyKey == "" || len(command.IdempotencyKey) > 200 {
		return ApplyEpisodePlanningCandidateResult{}, invalid("Invalid Episode Planning application")
	}
	inputHash, err := platformcommand.InputHash(command)
	if err != nil {
		return ApplyEpisodePlanningCandidateResult{}, err
	}
	var result ApplyEpisodePlanningCandidateResult
	err = service.transactions.WithinEpisodePlanningTransaction(ctx, func(repo EpisodePlanningRepository) error {
		if receipt, receiptErr := repo.FindReceipt(ctx, command.WorkspaceID, applyEpisodePlanningOperation, command.IdempotencyKey); receiptErr == nil {
			return service.replayEpisodePlanning(ctx, repo, actor, command, inputHash, receipt, &result)
		} else if !errors.Is(receiptErr, platformcommand.ErrReceiptNotFound) {
			return receiptErr
		}
		source, loadErr := repo.GetEpisodePlanningCandidate(ctx, actor, command.CandidateRevisionID, true)
		if loadErr != nil {
			return loadErr
		}
		if source.WorkspaceID != command.WorkspaceID || source.ProjectID != command.ProjectID ||
			source.CandidateRevisionID != command.CandidateRevisionID ||
			source.CandidateRevisionHash != command.CandidateRevisionHash ||
			source.CandidateRevision != command.ExpectedCandidateRevision {
			return conflict("Episode Planning Candidate changed before approval")
		}
		if validationErr := validateEpisodePlanningSource(source); validationErr != nil {
			return validationErr
		}
		now, receiptID := service.config.Now().UTC(), service.config.NewID()
		structures, references, buildErr := service.buildPlanningOwnerStructures(source, actor, now)
		if buildErr != nil {
			return buildErr
		}
		set := PlanningOwnerSetReference{
			ID: receiptID, WorkspaceID: source.WorkspaceID, ProjectID: source.ProjectID,
			CandidateRevisionID:   source.CandidateRevisionID,
			CandidateRevisionHash: source.CandidateRevisionHash,
			CandidateRevision:     source.CandidateRevision, ReviewDecisionID: command.ReviewDecisionID,
			BibleVersionID: source.BibleVersionID, BibleVersion: source.BibleVersion,
			BibleContentHash: source.BibleContentHash, MaterializationHash: source.MaterializationHash,
			Structures: references,
		}
		set.ContentHash, buildErr = planningOwnerSetHash(set.Structures)
		if buildErr != nil {
			return buildErr
		}
		encoded, buildErr := platformcommand.Result(set)
		if buildErr != nil {
			return buildErr
		}
		receipt := platformcommand.Receipt{
			ID: receiptID, WorkspaceID: source.WorkspaceID, Operation: applyEpisodePlanningOperation,
			IdempotencyKey: command.IdempotencyKey, InputHash: inputHash,
			ResourceID: source.CandidateRevisionID, Result: encoded,
			CreatedBy: actor.UserID, CreatedAt: now,
		}
		if buildErr = repo.CreatePlanningOwnerSet(ctx, structures); buildErr != nil {
			return buildErr
		}
		if buildErr = repo.CreateReceipt(ctx, receipt); buildErr != nil {
			return buildErr
		}
		result = ApplyEpisodePlanningCandidateResult{Set: set, Receipt: receipt}
		return nil
	})
	return result, normalizeError(err)
}

func (service *EpisodePlanningService) replayEpisodePlanning(
	ctx context.Context,
	repo EpisodePlanningRepository,
	actor Actor,
	command ApplyEpisodePlanningCandidateCommand,
	inputHash string,
	receipt platformcommand.Receipt,
	result *ApplyEpisodePlanningCandidateResult,
) error {
	reference, err := platformcommand.Replay[PlanningOwnerSetReference](receipt, inputHash)
	if errors.Is(err, platformcommand.ErrInputMismatch) {
		return conflict("Idempotency key was already used with different input")
	}
	if err != nil {
		return err
	}
	if receipt.WorkspaceID != command.WorkspaceID || receipt.Operation != applyEpisodePlanningOperation ||
		receipt.ResourceID != command.CandidateRevisionID || receipt.CreatedBy != actor.UserID ||
		reference.ID != receipt.ID || reference.WorkspaceID != command.WorkspaceID ||
		reference.ProjectID != command.ProjectID || reference.CandidateRevisionID != command.CandidateRevisionID ||
		reference.CandidateRevisionHash != command.CandidateRevisionHash ||
		reference.CandidateRevision != command.ExpectedCandidateRevision ||
		reference.ReviewDecisionID != command.ReviewDecisionID {
		return conflict("Episode Planning receipt has drifted")
	}
	structures, err := repo.GetPlanningOwnerSet(ctx, actor, reference)
	if err != nil {
		return err
	}
	observed, err := planningStructureReferences(structures)
	if err != nil {
		return err
	}
	observedHash, err := planningOwnerSetHash(observed)
	if err != nil || observedHash != reference.ContentHash || !slices.EqualFunc(observed, reference.Structures, samePlanningStructureReference) {
		return conflict("Episode Planning owner set changed after approval")
	}
	*result = ApplyEpisodePlanningCandidateResult{Set: reference, Receipt: receipt}
	return nil
}

func trimEpisodePlanningCommand(command *ApplyEpisodePlanningCandidateCommand) {
	command.WorkspaceID = strings.TrimSpace(command.WorkspaceID)
	command.ProjectID = strings.TrimSpace(command.ProjectID)
	command.CandidateRevisionID = strings.TrimSpace(command.CandidateRevisionID)
	command.CandidateRevisionHash = strings.TrimSpace(command.CandidateRevisionHash)
	command.ReviewDecisionID = strings.TrimSpace(command.ReviewDecisionID)
	command.IdempotencyKey = strings.TrimSpace(command.IdempotencyKey)
}

func validateEpisodePlanningSource(source EpisodePlanningCandidateSource) error {
	if source.Candidate.SchemaVersion != "episode-planning-candidate-set-v1" ||
		source.Candidate.BibleVersionID != source.BibleVersionID ||
		source.Candidate.BibleVersion != source.BibleVersion ||
		source.Candidate.BibleContentHash != source.BibleContentHash ||
		source.Candidate.MaterializationHash != source.MaterializationHash ||
		len(source.Candidate.Episodes) == 0 || len(source.Candidate.Episodes) != len(source.Episodes) ||
		len(source.Identities) == 0 {
		return conflict("Episode Planning Candidate lost its frozen sources")
	}
	identities, err := validatePlanningIdentities(source.Identities)
	if err != nil {
		return err
	}
	episodes := append([]PlanningEpisodeSource(nil), source.Episodes...)
	slices.SortFunc(episodes, func(left, right PlanningEpisodeSource) int { return left.EpisodePosition - right.EpisodePosition })
	previousEnd := -1
	for index, root := range source.Candidate.Episodes {
		episode := episodes[index]
		if root.EpisodePosition != index+1 || episode.EpisodePosition != root.EpisodePosition ||
			root.EpisodeID != episode.EpisodeID || root.ScriptVersionID != episode.ScriptVersionID ||
			root.Candidate.EpisodeID != episode.EpisodeID || root.Candidate.ScriptVersionID != episode.ScriptVersionID ||
			episode.ScriptVersion < 1 || episode.SourceStart < 0 || episode.SourceEnd <= episode.SourceStart ||
			previousEnd >= 0 && episode.SourceStart != previousEnd ||
			episode.ContentHash != bibledomain.SourceTextHash(episode.Content) ||
			len([]rune(episode.Content)) != episode.SourceEnd-episode.SourceStart {
			return conflict("Episode Planning Candidate Episode source has drifted")
		}
		scopeIdentities := make([]domain.EpisodeCandidateIdentity, len(identities))
		for identityIndex, identity := range identities {
			stateKeys := make([]string, len(identity.States))
			for stateIndex, state := range identity.States {
				stateKeys[stateIndex] = state.StateKey
			}
			scopeIdentities[identityIndex] = domain.EpisodeCandidateIdentity{EntityKey: identity.EntityKey, StateKeys: stateKeys}
		}
		if err = domain.ValidateEpisodeReconciliationCandidate(root.Candidate, domain.EpisodeCandidateScope{
			EpisodeID: episode.EpisodeID, ScriptVersionID: episode.ScriptVersionID,
			EpisodePosition: episode.EpisodePosition, SourceStart: episode.SourceStart, SourceEnd: episode.SourceEnd,
			ContextStart: episode.SourceStart, ContextText: episode.Content, KnownIdentities: scopeIdentities,
		}); err != nil {
			return invalid(err.Error())
		}
		if len(root.Candidate.Conflicts) != 0 || slices.ContainsFunc(root.Candidate.ReviewIssues, func(issue bibledomain.ReviewIssue) bool {
			return issue.Severity == "blocking"
		}) {
			return invalid("Episode Planning Candidate still contains blocking review issues")
		}
		previousEnd = episode.SourceEnd
	}
	return nil
}

func validatePlanningIdentities(values []PlanningIdentitySource) ([]PlanningIdentitySource, error) {
	values = append([]PlanningIdentitySource(nil), values...)
	slices.SortFunc(values, func(left, right PlanningIdentitySource) int { return strings.Compare(left.EntityKey, right.EntityKey) })
	previous := ""
	for index := range values {
		value := &values[index]
		if value.EntityKey == "" || value.EntityKey != value.Asset.IdentityKey ||
			value.EntityKey != value.Specification.EntityKey || value.Asset.ID != value.Specification.AssetID ||
			value.Asset.Kind != value.Specification.Kind || value.Asset.Revision != 1 ||
			value.Specification.Version < 1 || len(value.States) == 0 || previous >= value.EntityKey {
			return nil, conflict("Production Bible identity materialization has drifted")
		}
		slices.SortFunc(value.States, func(left, right bibledomain.MaterializedState) int {
			return strings.Compare(left.StateKey, right.StateKey)
		})
		previousState := ""
		for _, state := range value.States {
			if state.AssetID != value.Asset.ID || state.Revision < 1 || state.StateKey == "" || previousState >= state.StateKey {
				return nil, conflict("Production Bible identity state materialization has drifted")
			}
			previousState = state.StateKey
		}
		previous = value.EntityKey
	}
	return values, nil
}

func (service *EpisodePlanningService) buildPlanningOwnerStructures(
	source EpisodePlanningCandidateSource,
	actor Actor,
	now time.Time,
) ([]domain.Structure, []PlanningStructureReference, error) {
	identities, err := validatePlanningIdentities(source.Identities)
	if err != nil {
		return nil, nil, err
	}
	identityByKey := make(map[string]PlanningIdentitySource, len(identities))
	for _, identity := range identities {
		identityByKey[identity.EntityKey] = identity
	}
	structures := make([]domain.Structure, len(source.Candidate.Episodes))
	for index, root := range source.Candidate.Episodes {
		structureID := service.config.NewID()
		scenes, err := buildPlanningScenes(structureID, root.Candidate, identityByKey)
		if err != nil {
			return nil, nil, err
		}
		resultHash, err := bibledomain.CanonicalStoryHash(struct {
			Schema string         `json:"schema"`
			Scenes []domain.Scene `json:"scenes"`
		}{"episode-planning-owner-v1", scenes})
		if err != nil {
			return nil, nil, err
		}
		confirmedBy := actor.UserID
		structures[index] = domain.Structure{
			ID: structureID, WorkspaceID: source.WorkspaceID, ProjectID: source.ProjectID,
			EpisodeID: root.EpisodeID, ScriptVersionID: root.ScriptVersionID,
			Status: "confirmed", ResultHash: resultHash, Revision: 1,
			ConfirmedBy: &confirmedBy, ConfirmedAt: &now,
			CreatedBy: actor.UserID, CreatedAt: now, UpdatedAt: now, Scenes: scenes,
		}
	}
	references, err := planningStructureReferences(structures)
	return structures, references, err
}

func buildPlanningScenes(
	structureID string,
	candidate domain.EpisodeReconciliationCandidate,
	identities map[string]PlanningIdentitySource,
) ([]domain.Scene, error) {
	fragmentIDs := make(map[string]string, len(candidate.OrderedFragments)+len(candidate.Claims))
	fragmentKinds := make(map[string]string, len(candidate.OrderedFragments))
	fragmentSceneKeys := make(map[string]string, len(candidate.OrderedFragments))
	for _, fragment := range candidate.OrderedFragments {
		fragmentIDs[fragment.TemporaryKey] = planningFragmentID(structureID, fragment.Kind, fragment.TemporaryKey)
		fragmentKinds[fragment.TemporaryKey] = fragment.Kind
		if fragment.Kind == "scene" {
			fragmentSceneKeys[fragment.TemporaryKey] = fragment.TemporaryKey
		} else if fragment.Attributes.SceneKey != nil {
			fragmentSceneKeys[fragment.TemporaryKey] = *fragment.Attributes.SceneKey
		}
	}
	for _, claim := range candidate.Claims {
		fragmentIDs[claim.ClaimKey] = planningFragmentID(structureID, "claim", claim.ClaimKey)
	}
	scenes := make([]domain.Scene, 0)
	sceneIndexes := map[string]int{}
	for _, fragment := range candidate.OrderedFragments {
		if fragment.Kind != "scene" {
			continue
		}
		if fragment.Attributes.SceneKey != nil || fragment.Attributes.SpeakerKey != nil ||
			fragment.Attributes.DialogueText != nil || fragment.Attributes.OccurrenceEntityKey != nil {
			return nil, invalid("Scene Candidate contains fields owned by another fragment kind")
		}
		scene := domain.Scene{
			ID: fragmentIDs[fragment.TemporaryKey], TemporaryKey: fragment.TemporaryKey,
			Heading: fragment.Summary, Position: len(scenes) + 1,
			SourceStart: fragment.SourceStart, SourceEnd: fragment.SourceEnd,
			Evidence:  append([]bibledomain.Evidence(nil), fragment.Evidence...),
			Dialogues: []domain.Dialogue{}, NarrativeUnits: []domain.NarrativeUnit{},
			Occurrences: []domain.Occurrence{}, Claims: []domain.PlanningClaim{}, Tasks: []domain.ProductionTask{},
		}
		if fragment.Attributes.LocationKey != nil {
			identity, exists := identities[*fragment.Attributes.LocationKey]
			if !exists || identity.Asset.Kind != "location" {
				return nil, invalid("Scene Candidate references an unknown location")
			}
			value := planningIdentityReference(identity)
			scene.LocationIdentity = &value
			if fragment.Attributes.StateKey != nil {
				state, stateErr := planningStateReference(identity, *fragment.Attributes.StateKey)
				if stateErr != nil {
					return nil, stateErr
				}
				scene.LocationState = &state
			}
		}
		sceneIndexes[fragment.TemporaryKey] = len(scenes)
		scenes = append(scenes, scene)
	}
	if len(scenes) == 0 {
		return nil, invalid("Episode Planning Candidate must contain at least one Scene")
	}
	for _, fragment := range candidate.OrderedFragments {
		if fragment.Kind == "scene" {
			continue
		}
		sceneKey := fragmentSceneKeys[fragment.TemporaryKey]
		sceneIndex, exists := sceneIndexes[sceneKey]
		if !exists {
			return nil, invalid("Episode Planning fragment has no exact Scene")
		}
		scene := &scenes[sceneIndex]
		switch fragment.Kind {
		case "dialogue":
			if fragment.Attributes.SpeakerKey == nil || fragment.Attributes.DialogueText == nil {
				return nil, invalid("Dialogue Candidate has no exact speaker or text")
			}
			identity, exists := identities[*fragment.Attributes.SpeakerKey]
			if !exists || identity.Asset.Kind != "character" {
				return nil, invalid("Dialogue Candidate references an unknown character")
			}
			identityRef := planningIdentityReference(identity)
			scene.Dialogues = append(scene.Dialogues, domain.Dialogue{
				ID: fragmentIDs[fragment.TemporaryKey], TemporaryKey: fragment.TemporaryKey,
				Speaker: identity.EntityKey, SpeakerIdentity: &identityRef, Text: strings.TrimSpace(*fragment.Attributes.DialogueText),
				SourceStart: fragment.SourceStart, SourceEnd: fragment.SourceEnd,
				Evidence: append([]bibledomain.Evidence(nil), fragment.Evidence...),
			})
		case "beat":
			participants, participantErr := planningParticipants(fragment.Attributes.ParticipantKeys, identities)
			if participantErr != nil {
				return nil, participantErr
			}
			text := fragment.Summary
			if fragment.Attributes.Action != nil {
				text = strings.TrimSpace(*fragment.Attributes.Action)
			}
			if text == "" {
				return nil, invalid("Beat Candidate has no action")
			}
			scene.NarrativeUnits = append(scene.NarrativeUnits, domain.NarrativeUnit{
				ID: fragmentIDs[fragment.TemporaryKey], TemporaryKey: fragment.TemporaryKey,
				Kind: "beat", Text: text, SourceStart: fragment.SourceStart, SourceEnd: fragment.SourceEnd,
				Participants: participants, Evidence: append([]bibledomain.Evidence(nil), fragment.Evidence...),
			})
		case "occurrence":
			if fragment.Attributes.OccurrenceEntityKey == nil || fragment.Attributes.StateKey == nil {
				return nil, invalid("Occurrence Candidate has no exact Identity and State")
			}
			identity, exists := identities[*fragment.Attributes.OccurrenceEntityKey]
			if !exists {
				return nil, invalid("Occurrence Candidate references an unknown Identity")
			}
			state, stateErr := planningStateReference(identity, *fragment.Attributes.StateKey)
			if stateErr != nil {
				return nil, stateErr
			}
			scene.Occurrences = append(scene.Occurrences, domain.Occurrence{
				ID: fragmentIDs[fragment.TemporaryKey], TemporaryKey: fragment.TemporaryKey, SceneID: scene.ID,
				Summary: fragment.Summary, SourceStart: fragment.SourceStart, SourceEnd: fragment.SourceEnd,
				Identity: planningIdentityReference(identity), State: state,
				Evidence: append([]bibledomain.Evidence(nil), fragment.Evidence...),
			})
		default:
			return nil, invalid("Episode Planning Candidate contains an unsupported fragment kind")
		}
	}
	for _, candidateClaim := range candidate.Claims {
		if candidateClaim.Status != "proposed" || candidateClaim.Polarity == "unknown" ||
			candidateClaim.Scope != "episode:"+candidate.EpisodeID {
			return nil, invalid("Claim Candidate is not eligible for formal application")
		}
		participants := make([]domain.PlanningClaimParticipant, len(candidateClaim.ParticipantKeys))
		seenParticipants := map[string]struct{}{}
		for index, key := range candidateClaim.ParticipantKeys {
			identity, exists := identities[key]
			if !exists {
				return nil, invalid("Claim Candidate references an unknown Identity")
			}
			if _, duplicate := seenParticipants[key]; duplicate {
				return nil, invalid("Claim Candidate repeats an Identity")
			}
			seenParticipants[key] = struct{}{}
			role := "participant"
			if index == 0 {
				role = "subject"
			} else if index == 1 {
				role = "object"
			}
			participants[index] = domain.PlanningClaimParticipant{Role: role, Identity: planningIdentityReference(identity)}
		}
		anchors := make([]domain.PlanningClaimAnchor, len(candidateClaim.AnchorKeys))
		claimSceneKey := ""
		seenAnchors := map[string]struct{}{}
		for index, key := range candidateClaim.AnchorKeys {
			fragmentID, exists := fragmentIDs[key]
			kind, kindExists := fragmentKinds[key]
			sceneKey, sceneExists := fragmentSceneKeys[key]
			if !exists || !kindExists || !sceneExists {
				return nil, invalid("Claim Candidate must anchor an exact Planning fragment")
			}
			if _, duplicate := seenAnchors[key]; duplicate {
				return nil, invalid("Claim Candidate repeats an anchor")
			}
			seenAnchors[key] = struct{}{}
			if claimSceneKey == "" {
				claimSceneKey = sceneKey
			} else if claimSceneKey != sceneKey {
				return nil, invalid("Claim Candidate anchors must belong to one Scene")
			}
			role := "context"
			if index == 0 {
				role = "primary"
			}
			anchors[index] = domain.PlanningClaimAnchor{Role: role, Kind: kind, FragmentID: fragmentID, TemporaryKey: key}
		}
		sceneIndex, exists := sceneIndexes[claimSceneKey]
		if !exists {
			return nil, invalid("Claim Candidate has no exact Scene")
		}
		scenes[sceneIndex].Claims = append(scenes[sceneIndex].Claims, domain.PlanningClaim{
			ID: fragmentIDs[candidateClaim.ClaimKey], TemporaryKey: candidateClaim.ClaimKey,
			ClaimType: candidateClaim.ClaimType, Scope: candidateClaim.Scope,
			Polarity: candidateClaim.Polarity, Status: "confirmed",
			Participants: participants, Anchors: anchors,
			Evidence: append([]bibledomain.Evidence(nil), candidateClaim.Evidence...),
		})
	}
	return scenes, nil
}

func planningParticipants(
	keys []string,
	identities map[string]PlanningIdentitySource,
) ([]domain.PlanningIdentityReference, error) {
	result := make([]domain.PlanningIdentityReference, len(keys))
	seen := map[string]struct{}{}
	for index, key := range keys {
		identity, exists := identities[key]
		if !exists {
			return nil, invalid("Beat Candidate references an unknown Identity")
		}
		if _, duplicate := seen[key]; duplicate {
			return nil, invalid("Beat Candidate repeats an Identity")
		}
		seen[key] = struct{}{}
		result[index] = planningIdentityReference(identity)
	}
	return result, nil
}

func planningIdentityReference(value PlanningIdentitySource) domain.PlanningIdentityReference {
	return domain.PlanningIdentityReference{
		EntityKey: value.EntityKey, Kind: value.Asset.Kind,
		AssetID: value.Asset.ID, AssetRevision: value.Asset.Revision, AssetContentHash: value.Asset.ContentHash,
		SpecificationID: value.Specification.ID, SpecificationVersion: value.Specification.Version,
		SpecificationHash: value.Specification.ContentHash,
	}
}

func planningStateReference(
	identity PlanningIdentitySource,
	stateKey string,
) (domain.PlanningStateReference, error) {
	for _, state := range identity.States {
		if state.StateKey == stateKey {
			if state.AssetID != identity.Asset.ID {
				break
			}
			return domain.PlanningStateReference{
				ID: state.ID, StateKey: state.StateKey, Revision: state.Revision, ContentHash: state.ContentHash,
			}, nil
		}
	}
	return domain.PlanningStateReference{}, invalid(fmt.Sprintf("State %q does not belong to Identity %q", stateKey, identity.EntityKey))
}

func planningFragmentID(structureID, kind, temporaryKey string) string {
	return uuid.NewSHA1(uuid.MustParse(structureID), []byte("planning-fragment-v1\x00"+kind+"\x00"+temporaryKey)).String()
}

func planningStructureReferences(structures []domain.Structure) ([]PlanningStructureReference, error) {
	structures = append([]domain.Structure(nil), structures...)
	slices.SortFunc(structures, func(left, right domain.Structure) int {
		if left.EpisodeID != right.EpisodeID {
			return strings.Compare(left.EpisodeID, right.EpisodeID)
		}
		return strings.Compare(left.ID, right.ID)
	})
	result := make([]PlanningStructureReference, len(structures))
	for index, structure := range structures {
		if structure.Status != "confirmed" || structure.Revision < 1 || structure.ConfirmedBy == nil ||
			structure.ConfirmedAt == nil || len(structure.Scenes) == 0 {
			return nil, conflict("Episode Planning owner Structure is incomplete")
		}
		fragments := make([]PlanningFragmentReference, 0)
		for _, scene := range structure.Scenes {
			fragments = append(fragments, PlanningFragmentReference{scene.TemporaryKey, "scene", scene.ID})
			for _, dialogue := range scene.Dialogues {
				fragments = append(fragments, PlanningFragmentReference{dialogue.TemporaryKey, "dialogue", dialogue.ID})
			}
			for _, beat := range scene.NarrativeUnits {
				fragments = append(fragments, PlanningFragmentReference{beat.TemporaryKey, "beat", beat.ID})
			}
			for _, occurrence := range scene.Occurrences {
				fragments = append(fragments, PlanningFragmentReference{occurrence.TemporaryKey, "occurrence", occurrence.ID})
			}
			for _, claim := range scene.Claims {
				fragments = append(fragments, PlanningFragmentReference{claim.TemporaryKey, "claim", claim.ID})
			}
		}
		slices.SortFunc(fragments, func(left, right PlanningFragmentReference) int {
			if left.TemporaryKey != right.TemporaryKey {
				return strings.Compare(left.TemporaryKey, right.TemporaryKey)
			}
			return strings.Compare(left.Kind, right.Kind)
		})
		result[index] = PlanningStructureReference{
			StructureID: structure.ID, EpisodeID: structure.EpisodeID, ScriptVersionID: structure.ScriptVersionID,
			ResultHash: structure.ResultHash, Revision: structure.Revision, Fragments: fragments,
		}
	}
	return result, nil
}

func planningOwnerSetHash(structures []PlanningStructureReference) (string, error) {
	return platformcommand.InputHash(struct {
		Schema     string                       `json:"schema"`
		Structures []PlanningStructureReference `json:"structures"`
	}{"planning-owner-set-v1", structures})
}

func samePlanningStructureReference(left, right PlanningStructureReference) bool {
	return left.StructureID == right.StructureID && left.EpisodeID == right.EpisodeID &&
		left.ScriptVersionID == right.ScriptVersionID && left.ResultHash == right.ResultHash &&
		left.Revision == right.Revision && slices.Equal(left.Fragments, right.Fragments)
}
