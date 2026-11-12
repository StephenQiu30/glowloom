declare namespace AgentBusinessApi {
  type acknowledgeReceiptParams = {
    /** rid */
    rid: string;
  };

  type AppliedReceipt = {
    authorization_revision?: number;
    decision_id?: string;
    execution_revision?: number;
    receipt_id?: string;
    status?: string;
  };

  type Authority = {
    authorization_revision?: number;
    barrier?: boolean;
    budget_micros?: number;
    content_hash?: string;
    current?: Decision;
    goal_contract?: GoalContract;
    model_selection?: Record<string, any>;
    owner_epoch?: number;
    request_id?: string;
    reserved_micros?: number;
    root_run_id?: string;
    scope?: Scope;
    session_id?: string;
    skill_pins?: SkillPin[];
    submission_id?: string;
    terminal?: boolean;
  };

  type businessAccepted = {
    accepted?: boolean;
  };

  type businessInputSources = {
    draft?: Record<string, any>;
    grant?: Grant;
  };

  type businessProposal = {
    authorization_revision?: number;
    budget_micros?: number;
    candidate?: Record<string, any>;
    candidate_hash?: string;
    canvas_revision?: number;
    control?: { terminate?: boolean };
    price_version?: string;
    proposal_id?: string;
    root_run_id?: string;
  };

  type businessSkillDiscover = {
    grant?: Grant;
  };

  type businessSkillLoad = {
    grant?: Grant;
    resource_hash?: string;
    resource_path?: string;
    version?: string;
  };

  type businessValid = {
    valid?: boolean;
  };

  type CallIdentity = {
    attempt?: number;
    call_id?: string;
    checkpoint_id?: string;
    contract_version?: string;
    input_hash?: string;
    task_id?: string;
  };

  type CallInput = {
    call?: CallIdentity;
    grant?: Grant;
    input?: Record<string, any>;
    max_micros?: number;
  };

  type CallReceipt = {
    call?: CallIdentity;
    cost_micros?: number;
    kind?: string;
    name?: string;
    receipt_id?: string;
    result?: Record<string, any>;
    root_run_id?: string;
    status?: string;
    submission_id?: string;
  };

  type CanvasSource = {
    canvas_id?: string;
    node_id?: string;
    revision?: number;
    row_id?: string;
  };

  type Decision = {
    action?: string;
    actor_id?: string;
    authorization_revision?: number;
    candidate_hash?: string;
    canvas_id?: string;
    content_hash?: string;
    decision_id?: string;
    org_id?: string;
    project_id?: string;
    proposal_id?: string;
    root_run_id?: string;
    scope_id?: string;
    session_id?: string;
  };

  type executeToolParams = {
    /** name */
    name: string;
  };

  type getCallParams = {
    /** rid */
    rid: string;
    /** call_id */
    call_id: string;
    /** submission_id */
    submission_id: string;
    /** task_id */
    task_id: string;
    /** checkpoint_id */
    checkpoint_id: string;
    /** attempt */
    attempt: number;
    /** contract_version */
    contract_version: string;
    /** input_hash */
    input_hash: string;
  };

  type getDecisionsParams = {
    /** rid */
    rid: string;
  };

  type GoalContract = {
    delivery?: GoalDeliverySpec;
    kind?: GoalKind;
    required_deliveries?: number;
    source?: CanvasSource;
  };

  type GoalDeliverySpec = {
    aspect_ratio?: string;
    duration_ms?: number;
    fps?: number;
    project_format?: OutputKind;
    source_start_ms?: number;
  };

  type GoalKind = 'chat' | 'production';

  type Grant = {
    authorization_revision?: number;
    /** AvailableMicros is a current ledger observation, not part of grant identity. */
    available_micros?: number;
    expires_at?: string;
    grant_id?: string;
    owner_epoch?: number;
    root_run_id?: string;
    scope?: Scope;
    submission_id?: string;
  };

  type GrantRequest = {
    authorization_revision?: number;
    lease_until?: string;
    owner_epoch?: number;
    root_run_id?: string;
    submission_id?: string;
  };

  type loadSkillParams = {
    /** id */
    id: string;
  };

  type OutputKind =
    'video' | 'audio' | 'media_bundle' | 'fcpxml' | 'xmeml' | 'jianying';

  type ProposalInput = {
    budget_micros?: number;
    candidate?: Record<string, any>;
    candidate_hash?: string;
    canvas_revision?: number;
    grant?: Grant;
    price_version?: string;
    request_id?: string;
  };

  type QuiescedReceipt = {
    authorization_revision?: number;
    continuation_id?: string;
    decision_id?: string;
    lease_until?: string;
    owner_epoch?: number;
    receipt_id?: string;
  };

  type quiesceRunParams = {
    /** rid */
    rid: string;
  };

  type Scope = {
    actor_id?: string;
    canvas_id?: string;
    org_id?: string;
    project_id?: string;
    scope_id?: string;
  };

  type SkillContent = {
    content?: string;
    description?: string;
    enabled?: boolean;
    name?: string;
    resource_hash?: string;
    resource_path?: string;
    skill_id?: string;
    version?: string;
  };

  type SkillPin = {
    resource_hash?: string;
    skill_id?: string;
    version?: string;
  };

  type SkillVersion = {
    description?: string;
    enabled?: boolean;
    name?: string;
    resource_hash?: string;
    skill_id?: string;
    version?: string;
  };

  type ToolDescriptor = {
    contract_version?: string;
    description?: string;
    effect?: string;
    name?: string;
    parameters?: Record<string, any>;
    replay?: string;
    roles?: string[];
  };
}
