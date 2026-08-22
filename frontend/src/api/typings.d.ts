declare namespace API {
  type Analysis = {
    source_hash: string;
    episodes: Episode[];
    characters: Asset[];
    locations: Asset[];
    props: Asset[];
    costumes: Asset[];
  };

  type AnalysisResponse = {
    data: Analysis;
  };

  type Anchor = {
    line: number;
    start_offset: number;
    end_offset: number;
  };

  type ApiError = {
    code: string;
    message: string;
    next_action: string;
  };

  type approveScriptAnalysisParams = {
    revision_id: string;
  };

  type Asset = {
    kind: "character" | "location" | "prop" | "costume";
    name: string;
    episode_numbers: number[];
    evidence: Anchor[];
  };

  type createProjectParams = {
    workspace_id: string;
  };

  type CreateProjectRequest = {
    name: string;
  };

  type createScriptRevisionParams = {
    project_id: string;
  };

  type CreateScriptRevisionRequest = {
    name: string;
    content: string;
  };

  type CreateWorkspaceRequest = {
    name: string;
  };

  type Episode = {
    number: number;
    title: string;
    anchor: Anchor;
    scenes: Scene[];
  };

  type ErrorResponse = {
    error: ApiError;
  };

  type getAnalysisDraftParams = {
    revision_id: string;
  };

  type getOperationParams = {
    operation_id: string;
  };

  type getProjectAnalysisParams = {
    project_id: string;
  };

  type NarrativeUnit = {
    id: string;
    kind: "scene" | "dialogue" | "action";
    text: string;
    anchor: Anchor;
    speaker: string | null;
  };

  type Operation = {
    id: string;
    project_id: string;
    type: "script_analysis";
    status: "queued" | "running" | "succeeded" | "failed";
    progress: number;
    error_code: string | null;
    error: string | null;
  };

  type OperationResponse = {
    data: Operation;
  };

  type Project = {
    id: string;
    workspace_id: string;
    name: string;
    created_at: string;
  };

  type ProjectResponse = {
    data: Project;
  };

  type queueScriptAnalysisParams = {
    revision_id: string;
  };

  type Readiness = {
    status: "ready";
  };

  type ReadinessResponse = {
    data: Readiness;
  };

  type Scene = {
    id: string;
    heading: string;
    anchor: Anchor;
    narratives: NarrativeUnit[];
  };

  type ScriptRevision = {
    id: string;
    project_id: string;
    name: string;
    content_hash: string;
    content_length: number;
    status: "uploaded" | "analyzing" | "approved" | "failed";
    created_at: string;
  };

  type ScriptRevisionResponse = {
    data: ScriptRevision;
  };

  type Workspace = {
    id: string;
    name: string;
    created_at: string;
  };

  type WorkspaceResponse = {
    data: Workspace;
  };
}
