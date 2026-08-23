declare namespace API {
  type AccessAuditEvent = {
    action: string | null;
    actor_display_name: string | null;
    actor_email: string | null;
    actor_id: string | null;
    actor_type: string | null;
    after_hash: string | null;
    after_state: Record<string, any> | null;
    before_hash: string | null;
    before_state: Record<string, any> | null;
    id: string | null;
    object_display_name: string | null;
    object_email: string | null;
    object_id: string | null;
    object_type: string | null;
    occurred_at: string | null;
    reason: string | null;
    request_id: string | null;
    result: AccessAuditResult | null;
    workspace_id: string | null;
  };

  type AccessAuditPage = {
    items: AccessAuditEvent[] | null;
    page: number | null;
    page_size: number | null;
    total: number | null;
  };

  type AccessAuditPageEnvelope = {
    data: AccessAuditPage | null;
  };

  type AccessAuditResult = "succeeded" | "denied" | "failed";

  type Account = {
    display_name: string | null;
    email: string | null;
    id: string | null;
  };

  type AccountStatus = "active" | "suspended" | "removed";

  type adminListAccessAuditParams = {
    /** 跨主体、对象、动作、理由和 Request ID 搜索 */
    search: string | null;
    /** 按主体名称、邮箱或 ID 搜索 */
    actor: string | null;
    /** 按对象类型、名称、邮箱或 ID 搜索 */
    object: string | null;
    /** 按动作精确筛选 */
    action: string | null;
    /** 按结果筛选 */
    result: "succeeded" | "denied" | "failed" | null;
    /** 起始时间（RFC3339，含） */
    occurred_from: string | null;
    /** 结束时间（RFC3339，含） */
    occurred_to: string | null;
    /** 页码 */
    page: number | null;
    /** 每页数量 */
    page_size: number | null;
  };

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
    breakdown: EpisodeBreakdown | null;
    characters: Asset[] | null;
    costumes: Asset[] | null;
    episodes: Episode[] | null;
    locations: Asset[] | null;
    mentions: ProductionElementMention[] | null;
    narrative: NarrativeRevision | null;
    parse_report: ParseReport | null;
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

  type approveNarrativeRequest = {
    expected_narrative_hash: string | null;
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

  type BreakdownIssue = {
    anchor: Anchor | null;
    candidate_keys: string[] | null;
    code: string | null;
    message: string | null;
  };

  type BreakdownOperationType =
    | "split"
    | "merge"
    | "move_boundary"
    | "rename"
    | "reorder"
    | "ignore";

  type BreakdownStatus = "ready" | "blocked";

  type Candidate = {
    artifact_id: string | null;
    content_hash: string | null;
    fixture: boolean | null;
    id: string | null;
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
    boundary_rule: string | null;
    content_unit_id: string | null;
    decision: string | null;
    number: number | null;
    ordinal: number | null;
    scenes: Scene[] | null;
    temporary_key: string | null;
    title: string | null;
  };

  type EpisodeBreakdown = {
    coverage_hash: string | null;
    issues: BreakdownIssue[] | null;
    revision_no: number | null;
    segmentation_hash: string | null;
    status: BreakdownStatus | null;
  };

  type EpisodeBreakdownOperation = {
    boundary_offset: number | null;
    candidate_key: string | null;
    candidate_keys: string[] | null;
    left_key: string | null;
    left_title: string | null;
    ordered_candidate_keys: string[] | null;
    right_key: string | null;
    right_title: string | null;
    target_key: string | null;
    target_title: string | null;
    title: string | null;
    type: BreakdownOperationType | null;
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

  type MembershipStatus = "active" | "suspended" | "removed";

  type NarrativeIssue = {
    anchor: Anchor | null;
    code: string | null;
    mention_id: string | null;
    message: string | null;
    node_id: string | null;
    scene_id: string | null;
  };

  type NarrativeNodeKind = "beat" | "dialogue" | "action" | "narration";

  type NarrativeNodeStatus = "active" | "ignored";

  type NarrativeOperation = {
    anchor: Anchor | null;
    boundary_node_id: string | null;
    element_type: string | null;
    episode_key: string | null;
    heading: string | null;
    ignore_reason: string | null;
    left_heading: string | null;
    left_scene_id: string | null;
    mention_id: string | null;
    node_id: string | null;
    node_kind: NarrativeNodeKind | null;
    ordered_node_ids: string[] | null;
    ordered_scene_ids: string[] | null;
    right_heading: string | null;
    right_scene_id: string | null;
    scene_id: string | null;
    scene_ids: string[] | null;
    speaker: string | null;
    surface_text: string | null;
    target_scene_id: string | null;
    text: string | null;
    type: NarrativeOperationType | null;
  };

  type NarrativeOperationType =
    | "update_scene"
    | "split_scene"
    | "merge_scenes"
    | "reorder_scenes"
    | "create_node"
    | "update_node"
    | "delete_node"
    | "reorder_nodes"
    | "ignore_node"
    | "create_mention"
    | "update_mention"
    | "delete_mention";

  type NarrativeRevision = {
    completeness: string | null;
    content_hash: string | null;
    id: string | null;
    issues: NarrativeIssue[] | null;
    revision_no: number | null;
    status: NarrativeStatus | null;
  };

  type NarrativeStatus = "ready" | "blocked" | "approved";

  type NarrativeUnit = {
    anchor: Anchor | null;
    id: string | null;
    ignore_reason: string | null;
    kind: NarrativeNodeKind | null;
    speaker: string | null;
    status: NarrativeNodeStatus | null;
    text: string | null;
  };

  type Operation = {
    error: string | null;
    error_code: string | null;
    id: string | null;
    progress: number | null;
    project_id: string | null;
    source_revision_id: string | null;
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

  type ParseReport = {
    character_count: number | null;
    failed_scopes: string[] | null;
    format: string | null;
    original_hash: string | null;
    paragraph_count: number | null;
    parser_version: string | null;
    status: string | null;
    text_hash: string | null;
  };

  type ProductionElementMention = {
    anchor: Anchor | null;
    element_type: string | null;
    id: string | null;
    scene_id: string | null;
    status: string | null;
    surface_text: string | null;
  };

  type Project = {
    created_at: string | null;
    id: string | null;
    latest_workflow: ProjectWorkflow | null;
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

  type projectListParams = {
    /** Workspace UUID */
    workspaceID: string;
    /** 页码 */
    page: number | null;
    /** 每页数量 */
    page_size: number | null;
  };

  type ProjectPage = {
    items: Project[] | null;
    page: number | null;
    page_size: number | null;
    total: number | null;
  };

  type ProjectPageEnvelope = {
    data: ProjectPage | null;
  };

  type ProjectWorkflow = {
    operation_id: string | null;
    operation_status: string | null;
    progress: number | null;
    project_id: string | null;
    source_revision_id: string | null;
    source_status: string | null;
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

  type reviseAnalysisDraftRequest = {
    expected_source_hash: string | null;
    operations: EpisodeBreakdownOperation[] | null;
  };

  type reviseNarrativeDraftRequest = {
    expected_narrative_hash: string | null;
    operations: NarrativeOperation[] | null;
  };

  type RoleCode = "admin" | "user" | "ban";

  type Scene = {
    anchor: Anchor | null;
    heading: string | null;
    id: string | null;
    narratives: NarrativeUnit[] | null;
  };

  type scriptAnalysisDraftParams = {
    /** Script Revision UUID */
    revisionID: string;
  };

  type scriptAnalysisDraftReviseParams = {
    /** Script Revision UUID */
    revisionID: string;
  };

  type scriptAnalysisQueueParams = {
    /** Script Revision UUID */
    revisionID: string;
  };

  type scriptEpisodeBreakdownApproveParams = {
    /** Script Revision UUID */
    revisionID: string;
  };

  type scriptNarrativeApproveParams = {
    /** Script Revision UUID */
    revisionID: string;
  };

  type scriptNarrativeDraftReviseParams = {
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
    source_type: string | null;
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
    reason: string | null;
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
