package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
	"github.com/StephenQiu30/lanverse/backend/internal/production/project/domain"
)

const confirmEpisodeLifecycleOperation = "project.confirm_episode_lifecycle"

type ConfirmEpisodeLifecycleCommand struct {
	WorkspaceID, ProjectID                       string
	GateInputID, GateInputHash, ReviewDecisionID string
	SourceVersionID, SourceHash                  string
	ExpectedProjectRevision                      int
	ExpectedActiveOrderHash, IdempotencyKey      string
	EpisodeSpans                                 []domain.EpisodeLifecycleSpan
}

type EpisodeLifecycleTransactionManager interface {
	WithinEpisodeLifecycleTransaction(context.Context, func(EpisodeLifecycleRepository) error) error
}

type EpisodeLifecycleRepository interface {
	Authorize(context.Context, Actor, string, Capability) error
	GetEpisodeLifecycleProject(context.Context, string, bool) (domain.Project, error)
	GetEpisodeLifecycleSource(context.Context, string) (domain.EpisodeLifecycleSource, error)
	ListEpisodeLifecycleEpisodes(context.Context, string) ([]domain.EpisodeLifecycleEpisode, error)
	FindEpisodeLifecycleScriptVersion(
		context.Context, string, string, int, int, string,
	) (domain.EpisodeLifecycleScriptVersion, bool, error)
	NextEpisodeLifecycleScriptVersion(context.Context, string) (int, error)
	CreateEpisodeLifecycleEpisode(context.Context, domain.EpisodeLifecycleEpisode) error
	SaveEpisodeLifecycleEpisode(context.Context, domain.EpisodeLifecycleEpisode) error
	CreateEpisodeLifecycleScriptVersion(context.Context, domain.EpisodeLifecycleScriptVersion) error
	SaveEpisodeLifecycleProject(context.Context, domain.Project) error
	FindReceipt(context.Context, string, string, string) (platformcommand.Receipt, error)
	CreateReceipt(context.Context, platformcommand.Receipt) error
	AppendAudit(context.Context, AuditEvent) error
}

func (service *Service) ConfirmEpisodeLifecycle(
	ctx context.Context,
	actor Actor,
	command ConfirmEpisodeLifecycleCommand,
) (domain.EpisodeLifecycleSet, error) {
	command = normalizeEpisodeLifecycleCommand(command)
	if err := validateEpisodeLifecycleCommand(command); err != nil {
		return domain.EpisodeLifecycleSet{}, err
	}
	manager, ok := service.transactions.(EpisodeLifecycleTransactionManager)
	if !ok || service.now == nil || service.newID == nil {
		return domain.EpisodeLifecycleSet{}, errors.New("Project Episode lifecycle owner is unavailable")
	}
	inputHash, err := platformcommand.InputHash(command)
	if err != nil {
		return domain.EpisodeLifecycleSet{}, err
	}
	var result domain.EpisodeLifecycleSet
	err = manager.WithinEpisodeLifecycleTransaction(ctx, func(repository EpisodeLifecycleRepository) error {
		if authorizeErr := repository.Authorize(ctx, actor, command.WorkspaceID, ContentWrite); authorizeErr != nil {
			return authorizeErr
		}
		receipt, receiptErr := repository.FindReceipt(
			ctx, command.WorkspaceID, confirmEpisodeLifecycleOperation, command.IdempotencyKey,
		)
		if receiptErr == nil {
			result, receiptErr = platformcommand.Replay[domain.EpisodeLifecycleSet](receipt, inputHash)
			return normalizeReceiptError(receiptErr)
		}
		if !errors.Is(receiptErr, platformcommand.ErrReceiptNotFound) {
			return receiptErr
		}

		project, loadErr := repository.GetEpisodeLifecycleProject(ctx, command.ProjectID, true)
		if loadErr != nil {
			return normalizeNotFound(loadErr)
		}
		if project.WorkspaceID != command.WorkspaceID || project.Status != domain.StatusActive {
			return state("Project Episode lifecycle scope has drifted", "review_structure_identity")
		}
		source, loadErr := repository.GetEpisodeLifecycleSource(ctx, command.SourceVersionID)
		if loadErr != nil {
			return loadErr
		}
		if source.WorkspaceID != command.WorkspaceID || source.ProjectID != command.ProjectID ||
			source.VersionID != command.SourceVersionID || source.ContentHash != command.SourceHash ||
			source.Revision < 1 || source.HeadRevision < 1 || !validEpisodeLifecycleHash(source.HeadHash) ||
			utf8.RuneCountInString(source.NormalizedText) != command.EpisodeSpans[len(command.EpisodeSpans)-1].SourceEnd {
			return state("Project Episode lifecycle source has drifted", "review_structure_identity")
		}
		episodes, loadErr := repository.ListEpisodeLifecycleEpisodes(ctx, command.ProjectID)
		if loadErr != nil {
			return loadErr
		}
		activeOrderHash, hashErr := episodeLifecycleActiveOrderHash(episodes)
		if hashErr != nil {
			return hashErr
		}
		if project.Revision != command.ExpectedProjectRevision || activeOrderHash != command.ExpectedActiveOrderHash {
			return &Error{
				Code: CodeVersionConflict, Message: "Project Episode lifecycle Head has changed", Status: 409,
				NextAction: "review_structure_identity",
			}
		}

		byPosition := make(map[int]domain.EpisodeLifecycleEpisode, len(episodes))
		for _, episode := range episodes {
			if episode.ProjectID != command.ProjectID || episode.WorkspaceID != command.WorkspaceID ||
				episode.Position < 1 || episode.Revision < 1 || episode.TargetDurationMS < 1 ||
				(episode.Status != "active" && episode.Status != "archived") {
				return errors.New("persisted Project Episode lifecycle has drifted")
			}
			if _, parseErr := uuid.Parse(episode.ID); parseErr != nil {
				return errors.New("persisted Project Episode identity has drifted")
			}
			if _, duplicate := byPosition[episode.Position]; duplicate {
				return errors.New("persisted Project Episode position is duplicated")
			}
			byPosition[episode.Position] = episode
		}
		now := service.now().UTC()
		changed := false
		sourceRunes := []rune(source.NormalizedText)
		refs := make([]domain.EpisodeLifecycleEpisodeRef, len(command.EpisodeSpans))
		for index, span := range command.EpisodeSpans {
			episode, exists := byPosition[span.Position]
			name := strings.TrimSpace(span.Heading)
			if name == "" {
				name = fmt.Sprintf("第 %d 集", span.Position)
			}
			if !exists {
				episode = domain.EpisodeLifecycleEpisode{
					ID:          uuid.NewSHA1(uuid.MustParse(command.ProjectID), []byte("lanverse:episode-position:"+strconv.Itoa(span.Position))).String(),
					WorkspaceID: command.WorkspaceID, ProjectID: command.ProjectID,
					Name:     name,
					Position: span.Position, TargetDurationMS: project.TargetDurationMS,
					Status: "active", Revision: 1, CreatedAt: now, UpdatedAt: now,
				}
				if createErr := repository.CreateEpisodeLifecycleEpisode(ctx, episode); createErr != nil {
					return createErr
				}
				changed = true
			}
			content := string(sourceRunes[span.SourceStart:span.SourceEnd])
			contentHash := hashEpisodeLifecycleContent(content)
			scriptVersion, found, versionErr := repository.FindEpisodeLifecycleScriptVersion(
				ctx, episode.ID, source.VersionID, span.SourceStart, span.SourceEnd, contentHash,
			)
			if versionErr != nil {
				return versionErr
			}
			if found && !validPersistedEpisodeLifecycleScript(
				scriptVersion, command, episode.ID, span, content, contentHash,
			) {
				return errors.New("persisted Project Episode Script Version has drifted")
			}
			if !found {
				versionNo, nextErr := repository.NextEpisodeLifecycleScriptVersion(ctx, episode.ID)
				if nextErr != nil {
					return nextErr
				}
				scriptVersion = domain.EpisodeLifecycleScriptVersion{
					ID:          uuid.NewSHA1(uuid.MustParse(command.GateInputID), []byte("lanverse:episode-script:"+episode.ID+":"+contentHash)).String(),
					WorkspaceID: command.WorkspaceID, ProjectID: command.ProjectID, EpisodeID: episode.ID,
					DocumentRevisionID: source.VersionID, VersionNo: versionNo,
					SourceStart: span.SourceStart, SourceEnd: span.SourceEnd,
					Content: content, ContentHash: contentHash, Status: "published", CreatedBy: actor.UserID,
					CreatedAt: now, UpdatedAt: now,
				}
				if createErr := repository.CreateEpisodeLifecycleScriptVersion(ctx, scriptVersion); createErr != nil {
					return createErr
				}
				changed = true
			}
			before := episode
			episode.Name, episode.Status, episode.CurrentScriptVersionID = name, "active", &scriptVersion.ID
			episode.UpdatedAt = now
			if !exists {
				if saveErr := repository.SaveEpisodeLifecycleEpisode(ctx, episode); saveErr != nil {
					return saveErr
				}
			} else if episodeLifecycleEpisodeChanged(before, episode) {
				episode.Revision++
				if saveErr := repository.SaveEpisodeLifecycleEpisode(ctx, episode); saveErr != nil {
					return saveErr
				}
				changed = true
			}
			refs[index] = domain.EpisodeLifecycleEpisodeRef{
				TemporaryEpisodeID: span.TemporaryEpisodeID, EpisodeID: episode.ID,
				EpisodeRevision: episode.Revision, Position: episode.Position,
				ScriptVersionID: scriptVersion.ID, ScriptVersion: scriptVersion.VersionNo,
				SourceStart: span.SourceStart, SourceEnd: span.SourceEnd, ContentHash: scriptVersion.ContentHash,
			}
		}
		for _, episode := range episodes {
			if episode.Position <= len(command.EpisodeSpans) || episode.Status != "active" {
				continue
			}
			episode.Status, episode.Revision, episode.UpdatedAt = "archived", episode.Revision+1, now
			if saveErr := repository.SaveEpisodeLifecycleEpisode(ctx, episode); saveErr != nil {
				return saveErr
			}
			changed = true
		}
		if changed {
			project.Revision++
			project.UpdatedAt = now
			if saveErr := repository.SaveEpisodeLifecycleProject(ctx, project); saveErr != nil {
				return saveErr
			}
		}
		resultOrderHash, hashErr := episodeLifecycleRefOrderHash(refs)
		if hashErr != nil {
			return hashErr
		}
		collectionRootHash, hashErr := platformcommand.InputHash(struct {
			SchemaVersion   string                              `json:"schema_version"`
			SourceVersionID string                              `json:"source_version_id"`
			SourceHash      string                              `json:"source_hash"`
			Episodes        []domain.EpisodeLifecycleEpisodeRef `json:"episodes"`
		}{domain.EpisodeLifecycleSetSchemaVersion, source.VersionID, source.ContentHash, refs})
		if hashErr != nil {
			return hashErr
		}
		result = domain.EpisodeLifecycleSet{
			SchemaVersion: domain.EpisodeLifecycleSetSchemaVersion, ID: service.newID(),
			WorkspaceID: command.WorkspaceID, ProjectID: command.ProjectID,
			GateInputID: command.GateInputID, GateInputHash: command.GateInputHash,
			ReviewDecisionID: command.ReviewDecisionID, SourceVersionID: source.VersionID, SourceHash: source.ContentHash,
			ProjectRevision: project.Revision, ActiveOrderHash: resultOrderHash,
			Episodes: refs, CollectionRootHash: collectionRootHash, CreatedAt: now,
		}
		encoded, encodeErr := platformcommand.Result(result)
		if encodeErr != nil {
			return encodeErr
		}
		if auditErr := repository.AppendAudit(ctx, AuditEvent{
			WorkspaceID: command.WorkspaceID, ActorID: actor.UserID,
			Action: "project.episode_lifecycle_confirmed", TargetID: command.ProjectID,
			Revision: project.Revision, OccurredAt: now,
			Metadata: map[string]any{
				"review_decision_id":   command.ReviewDecisionID,
				"collection_root_hash": collectionRootHash,
			},
		}); auditErr != nil {
			return auditErr
		}
		return repository.CreateReceipt(ctx, platformcommand.Receipt{
			ID: result.ID, WorkspaceID: command.WorkspaceID, Operation: confirmEpisodeLifecycleOperation,
			IdempotencyKey: command.IdempotencyKey, InputHash: inputHash, ResourceID: command.ProjectID,
			Result: encoded, CreatedBy: actor.UserID, CreatedAt: now,
		})
	})
	return result, err
}

func validPersistedEpisodeLifecycleScript(
	value domain.EpisodeLifecycleScriptVersion,
	command ConfirmEpisodeLifecycleCommand,
	episodeID string,
	span domain.EpisodeLifecycleSpan,
	content, contentHash string,
) bool {
	if _, err := uuid.Parse(value.ID); err != nil {
		return false
	}
	return value.WorkspaceID == command.WorkspaceID && value.ProjectID == command.ProjectID &&
		value.EpisodeID == episodeID && value.DocumentRevisionID == command.SourceVersionID &&
		value.VersionNo >= 1 && value.SourceStart == span.SourceStart && value.SourceEnd == span.SourceEnd &&
		value.Content == content && value.ContentHash == contentHash && value.Status == "published"
}

func normalizeEpisodeLifecycleCommand(command ConfirmEpisodeLifecycleCommand) ConfirmEpisodeLifecycleCommand {
	command.WorkspaceID, command.ProjectID = strings.TrimSpace(command.WorkspaceID), strings.TrimSpace(command.ProjectID)
	command.GateInputID, command.ReviewDecisionID = strings.TrimSpace(command.GateInputID), strings.TrimSpace(command.ReviewDecisionID)
	command.SourceVersionID, command.IdempotencyKey = strings.TrimSpace(command.SourceVersionID), strings.TrimSpace(command.IdempotencyKey)
	command.GateInputHash, command.SourceHash = strings.ToLower(strings.TrimSpace(command.GateInputHash)), strings.ToLower(strings.TrimSpace(command.SourceHash))
	command.ExpectedActiveOrderHash = strings.ToLower(strings.TrimSpace(command.ExpectedActiveOrderHash))
	command.EpisodeSpans = append([]domain.EpisodeLifecycleSpan(nil), command.EpisodeSpans...)
	for index := range command.EpisodeSpans {
		command.EpisodeSpans[index].TemporaryEpisodeID = strings.TrimSpace(command.EpisodeSpans[index].TemporaryEpisodeID)
		command.EpisodeSpans[index].Heading = strings.TrimSpace(command.EpisodeSpans[index].Heading)
	}
	return command
}

func validateEpisodeLifecycleCommand(command ConfirmEpisodeLifecycleCommand) error {
	for _, identifier := range []string{
		command.WorkspaceID, command.ProjectID, command.GateInputID,
		command.ReviewDecisionID, command.SourceVersionID,
	} {
		if _, err := uuid.Parse(identifier); err != nil {
			return invalid("Invalid Project Episode lifecycle identity")
		}
	}
	if !validEpisodeLifecycleHash(command.GateInputHash) || !validEpisodeLifecycleHash(command.SourceHash) ||
		!validEpisodeLifecycleHash(command.ExpectedActiveOrderHash) || command.ExpectedProjectRevision < 1 ||
		len(command.EpisodeSpans) == 0 || validateIdempotencyKey(command.IdempotencyKey) != nil {
		return invalid("Invalid Project Episode lifecycle command")
	}
	previousEnd := 0
	seen := make(map[string]struct{}, len(command.EpisodeSpans))
	for index, span := range command.EpisodeSpans {
		if !strings.HasPrefix(span.TemporaryEpisodeID, "episode_") || span.Position != index+1 ||
			span.SourceStart != previousEnd || span.SourceEnd <= span.SourceStart {
			return invalid("Invalid Project Episode lifecycle coverage")
		}
		if _, duplicate := seen[span.TemporaryEpisodeID]; duplicate {
			return invalid("Duplicate Project Episode lifecycle key")
		}
		seen[span.TemporaryEpisodeID] = struct{}{}
		previousEnd = span.SourceEnd
	}
	return nil
}

func validEpisodeLifecycleHash(value string) bool {
	if len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func hashEpisodeLifecycleContent(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

func episodeLifecycleActiveOrderHash(episodes []domain.EpisodeLifecycleEpisode) (string, error) {
	active := make([]domain.EpisodeLifecycleEpisode, 0, len(episodes))
	for _, episode := range episodes {
		if episode.Status == "active" {
			active = append(active, episode)
		}
	}
	slices.SortFunc(active, func(left, right domain.EpisodeLifecycleEpisode) int {
		if left.Position != right.Position {
			return left.Position - right.Position
		}
		return strings.Compare(left.ID, right.ID)
	})
	order := make([]struct {
		ID       string `json:"id"`
		Position int    `json:"position"`
	}, len(active))
	for index, episode := range active {
		order[index] = struct {
			ID       string `json:"id"`
			Position int    `json:"position"`
		}{episode.ID, episode.Position}
	}
	return platformcommand.InputHash(order)
}

func episodeLifecycleRefOrderHash(refs []domain.EpisodeLifecycleEpisodeRef) (string, error) {
	order := make([]struct {
		ID       string `json:"id"`
		Position int    `json:"position"`
	}, len(refs))
	for index, ref := range refs {
		order[index] = struct {
			ID       string `json:"id"`
			Position int    `json:"position"`
		}{ref.EpisodeID, ref.Position}
	}
	return platformcommand.InputHash(order)
}

func episodeLifecycleEpisodeChanged(
	before, after domain.EpisodeLifecycleEpisode,
) bool {
	return before.Name != after.Name || before.Status != after.Status ||
		before.CurrentScriptVersionID == nil || after.CurrentScriptVersionID == nil ||
		*before.CurrentScriptVersionID != *after.CurrentScriptVersionID
}
