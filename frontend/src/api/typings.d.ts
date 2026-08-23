declare namespace API {
  type Account = {
    display_name: string | null;
    email: string | null;
    id: string | null;
  };

  type AccountStatus = "active" | "suspended" | "removed";

  type adminListMembersParams = {
    /** 按邮箱或显示名搜索 */
    search: string | null;
    /** 页码 */
    page: number | null;
    /** 每页数量 */
    page_size: number | null;
  };

  type adminUpdateMemberParams = {
    /** Membership UUID */
    membership_id: string;
  };

  type agentRunCancelParams = {
    /** AgentRun UUID */
    agentRunID: string;
  };

  type agentRunGetParams = {
    /** AgentRun UUID */
    agentRunID: string;
  };

  type Analysis = {
    characters: Asset[] | null;
    costumes: Asset[] | null;
    episodes: Episode[] | null;
    locations: Asset[] | null;
    props: Asset[] | null;
    source_hash: string | null;
  };

  type AnalysisEnvelope = {
    data: Analysis | null;
  };

  type Anchor = {
    end_offset: number | null;
    line: number | null;
    start_offset: number | null;
  };

  type APIError = {
    code: ErrorCode | null;
    details: any | null;
    message: string | null;
    next_action: string | null;
    recovery_actions: RecoveryAction[] | null;
    request_id: string | null;
    retry_after_seconds: number | null;
  };

  type approveRequest = {
    execution_disposition: string | null;
    selected_item_ids: string[] | null;
  };

  type Asset = {
    episode_numbers: number[] | null;
    evidence: Anchor[] | null;
    kind: string | null;
    name: string | null;
  };

  type AuthResponse = {
    access_token: string | null;
    expires_at: string | null;
    role: RoleCode | null;
    token_type: string | null;
    user: Account | null;
    workspace: Workspace | null;
  };

  type AuthResponseEnvelope = {
    data: AuthResponse | null;
  };

  type Candidate = {
    artifact_id: string | null;
    content_hash: string | null;
    fixture: boolean | null;
    id: string | null;
    object_key: string | null;
    project_id: string | null;
    status: string | null;
    target_id: string | null;
    target_type: string | null;
  };

  type candidateCreateFixtureParams = {
    /** Shot UUID */
    shotID: string;
  };

  type CandidateEnvelope = {
    data: Candidate | null;
  };

  type candidateSelectParams = {
    /** Candidate UUID */
    candidateID: string;
  };

  type createFixtureCandidateRequest = {
    purpose: string | null;
  };

  type createProjectRequest = {
    name: string | null;
  };

  type createRequest = {
    capability_key: string | null;
    count: number | null;
    project_id: string | null;
    prompt: string | null;
    target_id: string | null;
    target_type: string | null;
  };

  type createScriptRevisionRequest = {
    content: string | null;
    name: string | null;
  };

  type createShotsRequest = {
    count: number | null;
  };

  type createWorkspaceRequest = {
    name: string | null;
  };

  type CurrentIdentity = {
    membership_id: string | null;
    role: RoleCode | null;
    session_id: string | null;
    user_id: string | null;
    workspace_id: string | null;
  };

  type CurrentIdentityEnvelope = {
    data: CurrentIdentity | null;
  };

  type Episode = {
    anchor: Anchor | null;
    content_unit_id: string | null;
    number: number | null;
    scenes: Scene[] | null;
    title: string | null;
  };

  type ErrorCode =
    | "invalid_json"
    | "invalid_id"
    | "unauthorized"
    | "forbidden"
    | "not_found"
    | "conflict"
    | "validation_failed"
    | "rate_limited"
    | "dependency_unavailable"
    | "schema_unavailable"
    | "internal_error"
    | "request_failed"
    | "generation_plan_invalid"
    | "session_invalid"
    | "workspace_invalid"
    | "project_invalid"
    | "script_invalid";

  type ErrorEnvelope = {
    error: APIError | null;
  };

  type generationPlanApproveParams = {
    /** Generation Plan UUID */
    planID: string;
  };

  type generationPlanGetParams = {
    /** Generation Plan UUID */
    planID: string;
  };

  type generationPlanPreflightParams = {
    /** Generation Plan UUID */
    planID: string;
  };

  type loginRequest = {
    email: string | null;
    password: string | null;
  };

  type MembershipStatus = "invited" | "active" | "suspended" | "removed";

  type NarrativeUnit = {
    anchor: Anchor | null;
    id: string | null;
    kind: string | null;
    speaker: string | null;
    text: string | null;
  };

  type Operation = {
    error: string | null;
    error_code: string | null;
    id: string | null;
    progress: number | null;
    project_id: string | null;
    status: string | null;
    type: string | null;
  };

  type OperationEnvelope = {
    data: Operation | null;
  };

  type operationGetParams = {
    /** Operation UUID */
    operationID: string;
  };

  type Project = {
    created_at: string | null;
    id: string | null;
    name: string | null;
    workspace_id: string | null;
  };

  type projectAnalysisGetParams = {
    /** Project UUID */
    projectID: string;
  };

  type projectCreateParams = {
    /** Workspace UUID */
    workspaceID: string;
  };

  type ProjectEnvelope = {
    data: Project | null;
  };

  type RecoveryAction = {
    code: string | null;
    label: string | null;
  };

  type registerRequest = {
    display_name: string | null;
    email: string | null;
    password: string | null;
    workspace_name: string | null;
  };

  type RoleCode = "admin" | "user" | "ban";

  type Scene = {
    anchor: Anchor | null;
    heading: string | null;
    id: string | null;
    narratives: NarrativeUnit[] | null;
  };

  type scriptAnalysisApproveParams = {
    /** Script Revision UUID */
    revisionID: string;
  };

  type scriptAnalysisDraftParams = {
    /** Script Revision UUID */
    revisionID: string;
  };

  type scriptAnalysisQueueParams = {
    /** Script Revision UUID */
    revisionID: string;
  };

  type ScriptRevision = {
    content_hash: string | null;
    content_length: number | null;
    created_at: string | null;
    id: string | null;
    name: string | null;
    project_id: string | null;
    status: string | null;
  };

  type scriptRevisionCreateParams = {
    /** Project UUID */
    projectID: string;
  };

  type ScriptRevisionEnvelope = {
    data: ScriptRevision | null;
  };

  type selectCandidateRequest = {
    purpose: string | null;
  };

  type Selection = {
    candidate_id: string | null;
    id: string | null;
    project_id: string | null;
    selection_purpose: string | null;
    status: string | null;
    target_id: string | null;
    target_type: string | null;
  };

  type SelectionEnvelope = {
    data: Selection | null;
  };

  type Shot = {
    content_unit_id: string | null;
    id: string | null;
    ordinal: number | null;
    project_id: string | null;
    shot_key: string | null;
    source_beat_id: string | null;
    status: string | null;
  };

  type shotCreateParams = {
    /** Project UUID */
    projectID: string;
    /** Content Unit UUID */
    contentUnitID: string;
  };

  type ShotList = {
    items: Shot[] | null;
  };

  type ShotListEnvelope = {
    data: ShotList | null;
  };

  type shotListParams = {
    /** Project UUID */
    projectID: string;
    /** Content Unit UUID */
    contentUnitID: string;
  };

  type startRequest = {
    operation_id: string | null;
    project_id: string | null;
    request_hash: string | null;
    skill: string | null;
    snapshot_ref: string | null;
    stage: string | null;
  };

  type updateMemberRequest = {
    role: string | null;
    status: string | null;
  };

  type Workspace = {
    id: string | null;
    name: string | null;
  };

  type Workspace = {
    created_at: string | null;
    id: string | null;
    name: string | null;
  };

  type WorkspaceEnvelope = {
    data: Workspace | null;
  };

  type WorkspaceMember = {
    account_status: AccountStatus | null;
    created_at: string | null;
    display_name: string | null;
    email: string | null;
    membership_id: string | null;
    membership_status: MembershipStatus | null;
    role: RoleCode | null;
    user_id: string | null;
  };

  type WorkspaceMemberEnvelope = {
    data: WorkspaceMember | null;
  };

  type WorkspaceMemberPage = {
    items: WorkspaceMember[] | null;
    page: number | null;
    page_size: number | null;
    total: number | null;
  };

  type WorkspaceMemberPageEnvelope = {
    data: WorkspaceMemberPage | null;
  };
}
