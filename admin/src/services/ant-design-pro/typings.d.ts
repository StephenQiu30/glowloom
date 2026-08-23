declare namespace API {
  type Account = {
    display_name?: string;
    email?: string;
    id?: string;
  };

  type AccountStatus = "active" | "suspended" | "removed";

  type adminListMembersParams = {
    /** 按邮箱或显示名搜索 */
    search?: string;
    /** 页码 */
    page?: number;
    /** 每页数量 */
    page_size?: number;
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
    characters?: Asset[];
    costumes?: Asset[];
    episodes?: Episode[];
    locations?: Asset[];
    parse_report?: ParseReport;
    props?: Asset[];
    source_hash?: string;
  };

  type AnalysisEnvelope = {
    data?: Analysis;
  };

  type Anchor = {
    end_offset?: number;
    line?: number;
    start_offset?: number;
  };

  type APIError = {
    code?: ErrorCode;
    details?: any;
    message?: string;
    next_action?: string;
    recovery_actions?: RecoveryAction[];
    request_id?: string;
    retry_after_seconds?: number;
  };

  type approveRequest = {
    execution_disposition?: string;
    selected_item_ids?: string[];
  };

  type Asset = {
    episode_numbers?: number[];
    evidence?: Anchor[];
    kind?: string;
    name?: string;
  };

  type AuthResponse = {
    access_token?: string;
    expires_at?: string;
    role?: RoleCode;
    token_type?: string;
    user?: Account;
    workspace?: Workspace;
  };

  type AuthResponseEnvelope = {
    data?: AuthResponse;
  };

  type Candidate = {
    artifact_id?: string;
    content_hash?: string;
    fixture?: boolean;
    id?: string;
    project_id?: string;
    status?: string;
    target_id?: string;
    target_type?: string;
  };

  type candidateCreateFixtureParams = {
    /** Shot UUID */
    shotID: string;
  };

  type CandidateEnvelope = {
    data?: Candidate;
  };

  type candidateSelectParams = {
    /** Candidate UUID */
    candidateID: string;
  };

  type createFixtureCandidateRequest = {
    purpose?: string;
  };

  type createProjectRequest = {
    name?: string;
  };

  type createRequest = {
    capability_key?: string;
    count?: number;
    project_id?: string;
    prompt?: string;
    target_id?: string;
    target_type?: string;
  };

  type createShotsRequest = {
    count?: number;
  };

  type createWorkspaceRequest = {
    name?: string;
  };

  type CurrentIdentity = {
    membership_id?: string;
    role?: RoleCode;
    session_id?: string;
    user_id?: string;
    workspace_id?: string;
  };

  type CurrentIdentityEnvelope = {
    data?: CurrentIdentity;
  };

  type Episode = {
    anchor?: Anchor;
    content_unit_id?: string;
    number?: number;
    scenes?: Scene[];
    title?: string;
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
    error?: APIError;
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
    email?: string;
    password?: string;
  };

  type MembershipStatus = "invited" | "active" | "suspended" | "removed";

  type NarrativeUnit = {
    anchor?: Anchor;
    id?: string;
    kind?: string;
    speaker?: string;
    text?: string;
  };

  type Operation = {
    error?: string;
    error_code?: string;
    id?: string;
    progress?: number;
    project_id?: string;
    source_revision_id?: string;
    status?: string;
    type?: string;
  };

  type OperationEnvelope = {
    data?: Operation;
  };

  type operationGetParams = {
    /** Operation UUID */
    operationID: string;
  };

  type ParseReport = {
    character_count?: number;
    failed_scopes?: string[];
    format?: string;
    original_hash?: string;
    paragraph_count?: number;
    parser_version?: string;
    status?: string;
    text_hash?: string;
  };

  type Project = {
    created_at?: string;
    id?: string;
    latest_workflow?: ProjectWorkflow;
    name?: string;
    workspace_id?: string;
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
    data?: Project;
  };

  type projectListParams = {
    /** Workspace UUID */
    workspaceID: string;
    /** 页码 */
    page?: number;
    /** 每页数量 */
    page_size?: number;
  };

  type ProjectPage = {
    items?: Project[];
    page?: number;
    page_size?: number;
    total?: number;
  };

  type ProjectPageEnvelope = {
    data?: ProjectPage;
  };

  type ProjectWorkflow = {
    operation_id?: string;
    operation_status?: string;
    progress?: number;
    project_id?: string;
    source_revision_id?: string;
    source_status?: string;
  };

  type RecoveryAction = {
    code?: string;
    label?: string;
  };

  type registerRequest = {
    display_name?: string;
    email?: string;
    password?: string;
    workspace_name?: string;
  };

  type RoleCode = "admin" | "user" | "ban";

  type Scene = {
    anchor?: Anchor;
    heading?: string;
    id?: string;
    narratives?: NarrativeUnit[];
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
    content_hash?: string;
    content_length?: number;
    created_at?: string;
    id?: string;
    name?: string;
    project_id?: string;
    source_type?: string;
    status?: string;
  };

  type scriptRevisionCreateParams = {
    /** Project UUID */
    projectID: string;
  };

  type ScriptRevisionEnvelope = {
    data?: ScriptRevision;
  };

  type selectCandidateRequest = {
    purpose?: string;
  };

  type Selection = {
    candidate_id?: string;
    id?: string;
    project_id?: string;
    selection_purpose?: string;
    status?: string;
    target_id?: string;
    target_type?: string;
  };

  type SelectionEnvelope = {
    data?: Selection;
  };

  type Shot = {
    content_unit_id?: string;
    id?: string;
    ordinal?: number;
    project_id?: string;
    shot_key?: string;
    source_beat_id?: string;
    status?: string;
  };

  type shotCreateParams = {
    /** Project UUID */
    projectID: string;
    /** Content Unit UUID */
    contentUnitID: string;
  };

  type ShotList = {
    items?: Shot[];
  };

  type ShotListEnvelope = {
    data?: ShotList;
  };

  type shotListParams = {
    /** Project UUID */
    projectID: string;
    /** Content Unit UUID */
    contentUnitID: string;
  };

  type startRequest = {
    operation_id?: string;
    project_id?: string;
    request_hash?: string;
    skill?: string;
    snapshot_ref?: string;
    stage?: string;
  };

  type updateMemberRequest = {
    reason?: string;
    role?: string;
    status?: string;
  };

  type Workspace = {
    id?: string;
    name?: string;
  };

  type Workspace = {
    created_at?: string;
    id?: string;
    name?: string;
  };

  type WorkspaceEnvelope = {
    data?: Workspace;
  };

  type WorkspaceMember = {
    account_status?: AccountStatus;
    created_at?: string;
    display_name?: string;
    email?: string;
    membership_id?: string;
    membership_status?: MembershipStatus;
    role?: RoleCode;
    user_id?: string;
  };

  type WorkspaceMemberEnvelope = {
    data?: WorkspaceMember;
  };

  type WorkspaceMemberPage = {
    items?: WorkspaceMember[];
    page?: number;
    page_size?: number;
    total?: number;
  };

  type WorkspaceMemberPageEnvelope = {
    data?: WorkspaceMemberPage;
  };
}
