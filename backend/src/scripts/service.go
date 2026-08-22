package scripts

import (
	"context"

	"github.com/google/uuid"

	"github.com/stephenqiu30/lanverse/backend/src/platform/httpapi"
)

type ScriptAnalysisService struct {
	repository *ScriptRepository
}

func NewScriptAnalysisService(repository *ScriptRepository) *ScriptAnalysisService {
	return &ScriptAnalysisService{repository: repository}
}

func (s *ScriptAnalysisService) CreateWorkspace(ctx context.Context, name string) (Workspace, error) {
	if name == "" {
		return Workspace{}, httpapi.Validation("Workspace 名称不能为空", "提供 1—120 个字符的 Workspace 名称")
	}
	return s.repository.CreateWorkspace(ctx, name)
}

func (s *ScriptAnalysisService) CreateProject(ctx context.Context, workspaceID uuid.UUID, name string) (Project, error) {
	if name == "" {
		return Project{}, httpapi.Validation("项目名称不能为空", "提供项目名称后重试")
	}
	return s.repository.CreateProject(ctx, workspaceID, name)
}

func (s *ScriptAnalysisService) CreateScriptRevision(ctx context.Context, projectID uuid.UUID, name, content string) (ScriptRevision, error) {
	if name == "" {
		return ScriptRevision{}, httpapi.Validation("剧本名称不能为空", "提供剧本名称后重试")
	}
	return s.repository.CreateScriptRevision(ctx, projectID, name, content)
}

func (s *ScriptAnalysisService) QueueAnalysis(ctx context.Context, revisionID uuid.UUID) (Operation, error) {
	return s.repository.QueueAnalysis(ctx, revisionID)
}

func (s *ScriptAnalysisService) GetOperation(ctx context.Context, operationID uuid.UUID) (Operation, error) {
	return s.repository.GetOperation(ctx, operationID)
}

func (s *ScriptAnalysisService) ApproveAnalysis(ctx context.Context, revisionID uuid.UUID) (Analysis, error) {
	return s.repository.ApproveAnalysis(ctx, revisionID)
}

func (s *ScriptAnalysisService) GetProjectAnalysis(ctx context.Context, projectID uuid.UUID) (Analysis, error) {
	return s.repository.GetProjectAnalysis(ctx, projectID)
}

func (s *ScriptAnalysisService) GetAnalysisDraft(ctx context.Context, revisionID uuid.UUID) (Analysis, error) {
	return s.repository.GetAnalysisDraft(ctx, revisionID)
}

func (s *ScriptAnalysisService) CreateShots(ctx context.Context, projectID, contentUnitID uuid.UUID, count int) ([]Shot, error) {
	if count < 1 || count > 100 {
		return nil, httpapi.Validation("镜头数量必须在 1 到 100 之间", "调整镜头数量后重试")
	}
	return s.repository.CreateShots(ctx, projectID, contentUnitID, count)
}

func (s *ScriptAnalysisService) ListShots(ctx context.Context, projectID, contentUnitID uuid.UUID) ([]Shot, error) {
	return s.repository.ListShots(ctx, projectID, contentUnitID)
}

func (s *ScriptAnalysisService) CreateFixtureCandidate(ctx context.Context, shotID uuid.UUID, purpose string) (Candidate, error) {
	if purpose == "" {
		return Candidate{}, httpapi.Validation("候选用途不能为空", "提供候选用途后重试")
	}
	return s.repository.CreateFixtureCandidate(ctx, shotID, purpose)
}

func (s *ScriptAnalysisService) SelectCandidate(ctx context.Context, candidateID uuid.UUID, purpose string) (Selection, error) {
	if purpose == "" {
		return Selection{}, httpapi.Validation("选择用途不能为空", "提供选择用途后重试")
	}
	return s.repository.SelectCandidate(ctx, candidateID, purpose)
}
