package application

import (
	"context"
	"encoding/json"
	"slices"
	"strings"

	contract "github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	"github.com/StephenQiu30/lanverse/backend/internal/production/creation/domain"
)

type TraceAccess interface {
	AuthorizedRun(context.Context, Actor, string, bool) (domain.Run, error)
}
type TraceReader interface {
	Manifest(context.Context, domain.Run) (domain.ManifestSnapshot, error)
	Attempts(context.Context, domain.Run, string) (domain.AttemptHistory, error)
}
type TraceService struct {
	access TraceAccess
	reader TraceReader
}

func NewTraceService(access TraceAccess, reader TraceReader) *TraceService {
	return &TraceService{access: access, reader: reader}
}
func (s *TraceService) Manifest(ctx context.Context, actor Actor, runID string) (domain.ManifestSnapshot, error) {
	if !validID(runID) {
		return domain.ManifestSnapshot{}, Problem("validation_failed", 422)
	}
	run, e := s.access.AuthorizedRun(ctx, actor, runID, false)
	if e != nil {
		return domain.ManifestSnapshot{}, e
	}
	if s.reader == nil {
		return domain.ManifestSnapshot{}, Problem("creation_trace_unavailable", 503)
	}
	value, e := s.reader.Manifest(ctx, run)
	if e != nil {
		return domain.ManifestSnapshot{}, e
	}
	if e = ValidateManifest(run, value); e != nil {
		return domain.ManifestSnapshot{}, e
	}
	return value, nil
}
func (s *TraceService) Attempts(ctx context.Context, actor Actor, runID, stepID string) (domain.AttemptHistory, error) {
	if !validID(runID) || !validID(stepID) {
		return domain.AttemptHistory{}, Problem("validation_failed", 422)
	}
	run, e := s.access.AuthorizedRun(ctx, actor, runID, false)
	if e != nil {
		return domain.AttemptHistory{}, e
	}
	if s.reader == nil {
		return domain.AttemptHistory{}, Problem("creation_trace_unavailable", 503)
	}
	value, e := s.reader.Attempts(ctx, run, stepID)
	if e != nil {
		return domain.AttemptHistory{}, e
	}
	if e = ValidateAttempts(run, stepID, value); e != nil {
		return domain.AttemptHistory{}, e
	}
	return value, nil
}
func ValidateManifest(run domain.Run, v domain.ManifestSnapshot) error {
	fail := Problem("agent_manifest_invalid", 502)
	if v.Schema != "creation-manifest-production" || v.RunID != run.Command.RunID || v.CommandID != run.Command.CommandID {
		return fail
	}
	if v.Availability == "unavailable" || v.Availability == "not_frozen" {
		if v.Manifest != nil || v.ManifestHash != nil {
			return fail
		}
		return nil
	}
	m := v.Manifest
	if v.Availability != "recorded" || m == nil || m.Version != 1 || m.CommandID != v.CommandID || m.RunID != v.RunID || m.PayloadHash != run.PayloadHash || m.SourceRevisionID != run.Command.Source.RevisionID || m.SourceContentHash != run.Command.Source.ContentHash || !validHash(m.ReleaseHash) || m.CallLimit < 1 || m.CallLimit > 1000 {
		return fail
	}
	raw, e := json.Marshal(m)
	if e != nil {
		return fail
	}
	hash, e := contract.CanonicalHash(raw)
	if e != nil || v.ManifestHash == nil || hash != *v.ManifestHash {
		return fail
	}
	raw, e = json.Marshal(m.Template)
	if e != nil {
		return fail
	}
	hash, e = contract.CanonicalHash(raw)
	if e != nil || hash != m.TemplateHash {
		return fail
	}
	t := m.Template
	if t.Version != 1 || t.FlowType != run.Command.FlowType || len(t.Stages) != 4 {
		return fail
	}
	keys := []string{"map_manuscript", "analyze_episode", "build_world", "direct_scene"}
	scopes := []string{"run", "episode_collection", "run", "scene_collection"}
	gates := map[string]bool{}
	for i, s := range t.Stages {
		if s.StepKey != keys[i] || s.Scope != scopes[i] || !s.ReviewRequired || s.InstanceCount != nil || strings.TrimSpace(s.Title) == "" || len(s.Title) > 200 || !contract.ValidTextKey(s.ReviewKey) || gates[s.ReviewKey] {
			return fail
		}
		gates[s.ReviewKey] = true
	}
	return nil
}
func ValidateAttempts(run domain.Run, stepID string, v domain.AttemptHistory) error {
	fail := Problem("agent_attempt_history_invalid", 502)
	if v.Schema != "creation-attempt-history-production" || v.RunID != run.Command.RunID || v.CommandID != run.Command.CommandID || v.StepID != stepID || len(v.Attempts) > 1000 || v.Attempts == nil {
		return fail
	}
	parts := strings.Split(v.StepKey, "/")
	sizes := map[string]int{"map_manuscript": 1, "analyze_episode": 2, "build_world": 1, "direct_scene": 3}
	if sizes[parts[0]] != len(parts) {
		return fail
	}
	for _, p := range parts[1:] {
		if !contract.ValidTextKey(p) {
			return fail
		}
	}
	if v.HistoryOrigin == "unavailable" {
		if len(v.Attempts) != 0 || v.CurrentAttemptID != nil {
			return fail
		}
		return nil
	}
	if v.HistoryOrigin != "recorded" || len(v.Attempts) == 0 || v.CurrentAttemptID == nil || *v.CurrentAttemptID != v.Attempts[len(v.Attempts)-1].AttemptID {
		return fail
	}
	seen := map[string]bool{}
	lastNo := 0
	var lastFence int64
	for _, a := range v.Attempts {
		if !validID(a.AttemptID) || seen[a.AttemptID] || a.AttemptNo <= lastNo || a.Fence <= lastFence || !validHash(a.InputHash) || a.UsageStatus != "unknown" || a.StartedAt.IsZero() || !a.ExecutionDeadline.After(a.StartedAt) || !a.LeaseExpiresAt.After(a.ExecutionDeadline) {
			return fail
		}
		seen[a.AttemptID] = true
		lastNo = a.AttemptNo
		lastFence = a.Fence
		switch a.State {
		case "running":
			if a.FinishedAt != nil || a.ResultHash != nil || a.LastError != nil || a.AttemptID != *v.CurrentAttemptID {
				return fail
			}
		case "succeeded":
			if a.FinishedAt == nil || a.FinishedAt.Before(a.StartedAt) || a.ResultHash == nil || !validHash(*a.ResultHash) || a.LastError != nil || a.LeaseExpired {
				return fail
			}
		case "failed":
			if a.FinishedAt == nil || a.FinishedAt.Before(a.StartedAt) || a.ResultHash != nil || a.LastError == nil || !slices.Contains([]string{"context_insufficient", "skill_release_unavailable", "input_contract_invalid", "candidate_contract_invalid", "execution_output_budget_exceeded", "execution_deadline_exceeded", "structured_output_invalid"}, *a.LastError) || a.LeaseExpired {
				return fail
			}
		case "unknown":
			if a.FinishedAt == nil || a.FinishedAt.Before(a.StartedAt) || a.ResultHash != nil || a.LastError == nil || !slices.Contains([]string{"harness_response_unknown", "harness_result_invalid", "attempt_cancelled", "attempt_expired"}, *a.LastError) || a.LeaseExpired {
				return fail
			}
		default:
			return fail
		}
	}
	return nil
}
