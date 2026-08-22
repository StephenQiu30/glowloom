declare namespace API {
  type AgentRun = {
    id: string;
    project_id: string;
    operation_id: string;
    skill: string;
    stage: string;
    stage_generation: number;
    request_hash: string;
    status: string;
    input_snapshot_hash: string;
    result_hash: string | null;
    created_at: string;
  };

  type AgentRunData = {
    run: AgentRun;
    items: ProposalItem[];
  };

  type AgentRunResponse = {
    data: AgentRunData;
  };

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
    request_id: string | null;
    details: any | null;
    recovery_actions: { code: string; label: string }[] | null;
  };

  type approveGenerationPlanParams = {
    plan_id: string;
  };

  type ApproveGenerationPlanRequest = {
    execution_disposition: "start_now" | "hold";
    selected_item_ids: string[];
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

  type cancelAgentRunParams = {
    agent_run_id: string;
  };

  type Candidate = {
    id: string;
    project_id: string;
    target_type: string;
    target_id: string;
    artifact_id: string;
    status: string;
    fixture: boolean;
    object_key: string | null;
    content_hash: string | null;
  };

  type CandidateResponse = {
    data: Candidate;
  };

  type createFixtureCandidateParams = {
    shot_id: string;
  };

  type CreateGenerationPlanRequest = {
    project_id: string;
    target_type: string;
    target_id: string;
    prompt: string;
    capability_key: string;
    count: number | null;
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

  type CreateSessionRequest = {
    identity_subject: string;
    workspace_id: string;
  };

  type createShotsParams = {
    project_id: string;
    content_unit_id: string;
  };

  type CreateShotsRequest = {
    count: number;
  };

  type CreateWorkspaceRequest = {
    name: string;
  };

  type Episode = {
    number: number;
    title: string;
    content_unit_id: string | null;
    anchor: Anchor;
    scenes: Scene[];
  };

  type ErrorResponse = {
    error: ApiError;
  };

  type FixtureCandidateRequest = {
    purpose: string;
  };

  type GenerationPlan = {
    id: string;
    project_id: string;
    target_type: string;
    target_id: string;
    status: string;
    execution_disposition: string | null;
    input_snapshot_hash: string;
    prompt_hash: string;
  };

  type GenerationPlanData = {
    plan: GenerationPlan;
    items: GenerationPlanItem[];
  };

  type GenerationPlanItem = {
    id: string;
    plan_id: string;
    ordinal: number;
    capability_key: string;
    prompt: string;
    status: string;
  };

  type GenerationPlanResponse = {
    data: GenerationPlanData;
  };

  type getAgentRunParams = {
    agent_run_id: string;
  };

  type getAnalysisDraftParams = {
    revision_id: string;
  };

  type getGenerationPlanParams = {
    plan_id: string;
  };

  type getOperationParams = {
    operation_id: string;
  };

  type getProjectAnalysisParams = {
    project_id: string;
  };

  type listShotsParams = {
    project_id: string;
    content_unit_id: string;
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

  type preflightGenerationPlanParams = {
    plan_id: string;
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

  type ProposalItem = {
    id: string;
    agent_run_id: string;
    target_module: string;
    target_command: string;
    payload: any | null;
    decision: string;
    read_set_hash: string;
    write_set_hash: string;
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

  type selectCandidateParams = {
    candidate_id: string;
  };

  type Selection = {
    id: string;
    project_id: string;
    target_type: string;
    target_id: string;
    selection_purpose: string;
    candidate_id: string;
    status: string;
  };

  type SelectionRequest = {
    purpose: string;
  };

  type SelectionResponse = {
    data: Selection;
  };

  type Session = {
    token: string;
    user_id: string;
    workspace_id: string;
    expires_at: string;
  };

  type SessionResponse = {
    data: Session;
  };

  type Shot = {
    id: string;
    project_id: string;
    content_unit_id: string;
    shot_key: string;
    ordinal: number;
    status: string;
  };

  type ShotList = {
    items: Shot[];
  };

  type ShotListResponse = {
    data: ShotList;
  };

  type StartAgentRunRequest = {
    project_id: string;
    operation_id: string;
    skill: any;
    stage: "manifest" | "narrative" | "knowledge";
    request_hash: string;
    snapshot_ref: string;
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
