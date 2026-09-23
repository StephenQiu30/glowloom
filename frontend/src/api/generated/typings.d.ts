declare namespace GeneratedAPI {
  type AcceptedScriptSourceResponse = {
    identity: ScriptSourceIdentity;
    span_index_id: string;
    span_index_hash: string;
    codepoint_count: number;
    utf8_byte_count: number;
    newline_normalization: string;
    codepoint_index_rule: string;
    head_revision: number;
    head_hash: string;
    collection_root_hash: string;
    collection_receipt_id: string;
    command_receipt_id: string;
  };

  type acceptProductionTaskParams = {
    structure_id: string;
    task_id: string;
  };

  type acceptScriptSourceParams = {
    project_id: string;
  };

  type AcceptScriptSourceRequest = {
    document_revision_id: string;
    expected_head_revision: number;
    expected_head_hash?: string | null;
    idempotency_key: string;
  };

  type acceptStoryboardDraftShotParams = {
    batch_id: string;
  };

  type adoptCreationProposalParams = {
    run_id: string;
    proposal_id: string;
  };

  type AdoptCreationProposalRequest = {
    expected_revision: number;
    decision_id: string;
    idempotency_key: string;
    risk_resolutions: CreationRiskResolution[];
  };

  type applyProjectCanvasOperationsParams = {
    project_id: string;
  };

  type applyStoryboardDraftParams = {
    batch_id: string;
  };

  type approveStoryboardDraftParams = {
    batch_id: string;
  };

  type archiveProjectParams = {
    project_id: string;
  };

  type archiveWorkspaceParams = {
    workspace_id: string;
  };

  type authorizeInitialReferenceExecutionParams = {
    project_id: string;
    generation_target_id: string;
  };

  type authorizeInitialReferenceGenerationParams = {
    project_id: string;
    target_version_id: string;
  };

  type AuthResponse = {
    /** Access Token */
    access_token: string;
    /** Expires In */
    expires_in: number;
    /** Token Type */
    token_type?: string;
    user: UserResponse;
    workspace: WorkspaceResponse;
  };

  type BibleEvidence = {
    episode_number: number | null;
    exact_anchor: string;
    source_end: number;
    source_start: number;
    text_hash: string;
  };

  type BibleReviewIssue = {
    /** Code */
    code: string;
    /** Evidence */
    evidence?: BibleEvidence[];
    /** Issue Key */
    issue_key: string;
    /** Repair Hint */
    repair_hint?: string | null;
    /** Scope */
    scope: "global" | "entity" | "entity_state" | "world_entry";
    /** Severity */
    severity: "warning" | "blocking";
    /** Subject Key */
    subject_key?: string | null;
    /** Summary */
    summary: string;
  };

  type BibleTextWorldAssetNeed = {
    description: string;
    entity_id: string;
    evidence: any;
  };

  type BibleTextWorldEntity = {
    evidence: any;
    id: string;
    identity_basis: string;
    key: string;
    kind: string;
    label: string;
    mention_ids: any;
    uncertainty: string | null;
  };

  type BibleTextWorldIssue = {
    code: string;
    scope: string;
    severity: string;
    summary: string;
  };

  type BibleTextWorldRelation = {
    basis: string;
    evidence: any;
    origin: string;
    predicate: string;
    subject_id: string;
    target_id: string;
  };

  type BibleTextWorldRiskResolution = {
    code: string;
    reason: string;
    scope: string;
  };

  type BibleTextWorldState = {
    after: string | null;
    basis: string;
    before: string | null;
    entity_id: string;
    episode_id: string;
    evidence: any;
    knowledge: string;
    property: string;
    scene_id: string;
    story_time: string;
    time_branch: string;
  };

  type BibleTextWorldVersion = {
    asset_needs: any;
    content_hash: string;
    created_at: string;
    created_by: string;
    decision_id: string;
    entities: any;
    id: string;
    id_mapping: Record<string, any>;
    issues: any;
    project_id: string;
    proposal_id: string;
    relations: any;
    revision: number;
    risk_resolutions: any;
    run_id: string;
    source_hash: string;
    source_revision_id: string;
    state_events: any;
    unresolved_mention_ids: any;
    workspace_id: string;
  };

  type buildInitialReferenceGenerationTargetParams = {
    project_id: string;
  };

  type CanvasApplyResponse = {
    document: CanvasDocument;
    applied_revision: number;
    replayed: boolean;
  };

  type CanvasConnection = {
    id: string;
    from_node_id: string;
    to_node_id: string;
    purpose: string;
    revision: number;
  };

  type CanvasDocument = {
    schema_version: number;
    project_id: string;
    revision: number;
    nodes: CanvasNode[];
    connections: CanvasConnection[];
    tombstones: string[];
  };

  type CanvasNode = {
    id: string;
    kind: "text" | "image" | "video" | "audio" | "group" | "note";
    title: string;
    content?: string;
    prompt?: string;
    media_version_id?: string;
    group_id?: string;
    x: number;
    y: number;
    width: number;
    height: number;
    revision: number;
  };

  type CanvasOperation = {
    kind: "create_node" | "update_node" | "delete_node" | "create_edge" | "delete_edge";
    node?: CanvasNode;
    node_id?: string;
    connection?: CanvasConnection;
    connection_id?: string;
    expected_revision: number;
  };

  type CanvasOperationRequest = {
    operations: CanvasOperation[];
    idempotency_key: string;
  };

  type ChangePasswordRequest = {
    /** Current Password */
    current_password: string;
    /** New Password */
    new_password: string;
  };

  type claimHumanTaskParams = {
    human_task_id: string;
  };

  type commitScriptImportParams = {
    project_id: string;
  };

  type completeMediaUploadParams = {
    upload_session_id: string;
  };

  type confirmEpisodePlanParams = {
    plan_id: string;
  };

  type ConfirmEpisodePlanRequest = {
    /** Expected Revision */
    expected_revision: number;
    /** Idempotency Key */
    idempotency_key: string;
  };

  type confirmEpisodeStructureParams = {
    structure_id: string;
  };

  type controlWorkflowRunParams = {
    workflow_run_id: string;
  };

  type CostBudgetResponse = {
    id: string;
    workspace_id: string;
    project_id: string;
    limit_amount: string;
    currency: string;
    revision: number;
    created_by: string;
    updated_by: string;
    created_at: string;
    updated_at: string;
  };

  type CostBudgetSetRequest = {
    limit_amount: string;
    currency: string;
    expected_revision: number;
    idempotency_key: string;
  };

  type CostPriceQuoteResponse = {
    id: string;
    workspace_id: string;
    project_id: string;
    model_profile_version_id: string;
    billing_metric: "generation.image.call" | "generation.video.call";
    reservation_unit_amount: string;
    currency: string;
    revision: number;
    content_hash: string;
    created_by: string;
    created_at: string;
  };

  type CostPriceQuoteSetRequest = {
    reservation_unit_amount: string;
    currency: string;
    expected_revision: number;
    idempotency_key: string;
  };

  type createCreationRunParams = {
    project_id: string;
  };

  type CreateCreationRunRequest = {
    document_revision_id: string;
    source_hash: string;
    idempotency_key: string;
  };

  type createEpisodePlanParams = {
    revision_id: string;
  };

  type createProductionBibleParams = {
    revision_id: string;
  };

  type createStoryboardDraftParams = {
    episode_id: string;
  };

  type createStoryboardExportParams = {
    episode_id: string;
  };

  type CreationAcceptanceReceipt = {
    schema: string;
    command_id: string;
    run_id: string;
    payload_hash: string;
    flow_type: string;
    workflow_id: string;
    receipt_id: string;
    accepted_at: string;
  };

  type CreationAdoptionReceipt = {
    schema: string;
    submission_id: string;
    run_id: string;
    proposal_id: string;
    step_id: string;
    gate: "map_manuscript" | "analyze_episode" | "build_world" | "direct_scene";
    proposal_revision: number;
    proposal_hash: string;
    candidate_hash: string;
    decision_id: string;
    owner_receipts: CreationOwnerReceipt[];
    formal_refs: CreationFormalRef[];
    id_mapping: Record<string, any>;
    risk_resolutions: CreationRiskResolution[];
    accepted_at: string;
  };

  type CreationAttempt = {
    attempt_id: string;
    attempt_no: number;
    execution_deadline: string;
    fence: number;
    finished_at: string | null;
    input_hash: string;
    last_error: string | null;
    lease_expired: boolean;
    lease_expires_at: string;
    result_hash: string | null;
    started_at: string;
    state: string;
    usage_status: string;
  };

  type CreationAttemptHistory = {
    attempts: any;
    command_id: string;
    current_attempt_id: string | null;
    history_origin: string;
    run_id: string;
    schema: string;
    step_id: string;
    step_key: string;
  };

  type CreationDraftRef = {
    step_id: string;
    draft_id: string;
    result_hash: string;
    candidate_hash: string;
    step_key?: string;
    output_role?: string;
    item_key?: string;
  };

  type CreationExecution = {
    schema: string;
    command_id: string;
    run_id: string;
    payload_hash: string;
    source_revision_id: string;
    release_hash: string | string;
    call_limit: number;
    reserved_calls: number;
    status: "queued" | "running" | "waiting_review" | "blocked" | "rejected" | "completed";
    stage: "" | "map_manuscript" | "analyze_episode" | "build_world" | "direct_scene";
    last_error: string;
    steps: CreationExecutionStep[];
    outputs: CreationDraftRef[];
    /** 只有可安全恢复的blocked为true，unknown不能通过此操作重复推理。 */
    can_resume: boolean;
  };

  type CreationExecutionStep = {
    id: string;
    step_key: string;
    input_hash: string;
    state: "running" | "needs_review" | "unknown";
    fence: number;
    usage_status: string;
    last_error: any;
  };

  type CreationFormalRef = {
    owner: string;
    type: string;
    id: string;
    revision: number;
    content_hash: string;
  };

  type CreationManifest = {
    call_limit: number;
    command_id: string;
    payload_hash: string;
    release_hash: string;
    run_id: string;
    source_content_hash: string;
    source_revision_id: string;
    template: CreationPlanTemplate;
    template_hash: string;
    version: number;
  };

  type CreationManifestSnapshot = {
    availability: string;
    command_id: string;
    manifest: CreationManifest | null;
    manifest_hash: string | null;
    run_id: string;
    schema: string;
  };

  type CreationOwnerReceipt = {
    id: string;
    owner: string;
    operation: string;
  };

  type CreationPlanTemplate = {
    flow_type: string;
    stages: any;
    version: number;
  };

  type CreationProposal = {
    id: string;
    run_id: string;
    step_id: string;
    stage: "map_manuscript" | "analyze_episode" | "build_world" | "direct_scene";
    step_key: string;
    revision: number;
    result_hash: string;
    candidate_hash: string;
    source_revision_id: string;
    source_hash: string;
    invocation_id: string;
    input_hash: string;
    release_hash: string;
    candidate:
      | CreationTextEpisodeMap
      | CreationTextEpisodeAnalysis
      | CreationTextWorldBook
      | CreationTextSceneDirection;
    evidence: CreationTextResolvedEvidence[];
    issues: CreationTextIssue[];
    human_task_id: string;
    status: "needs_review" | "accepted";
    acceptance: CreationAdoptionReceipt | null;
    created_at: string;
  };

  type CreationResumeResponse = {
    run_id: string;
    status: string;
    schema: string;
    command_id: string;
  };

  type CreationRiskResolution = {
    code: string;
    scope: string;
    reason: string;
  };

  type CreationRunResponse = {
    id: string;
    project_id: string;
    workspace_id: string;
    source: CreationSourceSnapshot;
    flow_type: string;
    workflow_id: string;
    status: "queued" | "delivery_unknown" | "delivery_blocked" | "accepted";
    revision: number;
    last_error: string;
    acceptance: CreationAcceptanceReceipt | null;
    created_at: string;
    updated_at: string;
  };

  type CreationSourceSnapshot = {
    document_id: string;
    revision_id: string;
    revision: number;
    content_hash: string;
    span_index_id: string;
  };

  type CreationStageDescriptor = {
    instance_count: number | null;
    review_key: string;
    review_required: boolean;
    scope: string;
    step_key: string;
    title: string;
  };

  type CreationSyncResult = {
    execution: CreationExecution;
    proposals: CreationProposal[];
  };

  type CreationTextAssetNeed = {
    /** Entity Key */
    entity_key: string;
    /** Description */
    description: string;
    /** Evidence */
    evidence: CreationTextEvidence[];
  };

  type CreationTextAudioCue = {
    /** Dialogue Key */
    dialogue_key: string;
    /** Channel */
    channel: "onscreen" | "offscreen" | "phone" | "inner" | "group" | "unknown";
  };

  type CreationTextBeat = {
    /** Key */
    key: string;
    /** Action */
    action: string;
    /** Evidence */
    evidence: CreationTextEvidence[];
    /** Required */
    required: boolean;
    /** Origin */
    origin: "extracted" | "inferred" | "proposed";
  };

  type CreationTextBlocking = {
    /** Mention Key */
    mention_key: string;
    /** Position */
    position: string;
    /** Facing */
    facing: string;
    /** Action */
    action: string;
  };

  type CreationTextDialogue = {
    /** Key */
    key: string;
    /** Speaker Mention */
    speaker_mention: string | null;
    /** Text */
    text: string;
    evidence: CreationTextEvidence;
    /** Channel */
    channel: "onscreen" | "offscreen" | "phone" | "inner" | "group" | "unknown";
  };

  type CreationTextEntityProposal = {
    /** Key */
    key: string;
    /** Kind */
    kind: "cast" | "place" | "prop";
    /** Label */
    label: string;
    /** Mentions */
    mentions: CreationTextMentionRef[];
    /** Identity Basis */
    identity_basis: "explicit" | "inferred" | "uncertain";
    /** Evidence */
    evidence: CreationTextEvidence[];
    /** Uncertainty */
    uncertainty: string | null;
  };

  type CreationTextEpisode = {
    /** First Block */
    first_block: number;
    /** Last Block */
    last_block: number;
    /** Key */
    key: string;
    /** Number */
    number: number | null;
    /** Title */
    title: string;
    /** Rationale */
    rationale: string;
  };

  type CreationTextEpisodeAnalysis = {
    /** Episode Key */
    episode_key: string;
    /** Summary */
    summary: string;
    /** Conflict */
    conflict: string;
    /** Turning Point */
    turning_point: string;
    /** Ending Hook */
    ending_hook: string;
    /** Scenes */
    scenes: CreationTextScene[];
    /** Excluded */
    excluded: CreationTextExcludedBlock[];
    /** Issues */
    issues: CreationTextIssue[];
  };

  type CreationTextEpisodeMap = {
    /** Mode */
    mode: "preserve" | "propose";
    /** Episodes */
    episodes: CreationTextEpisode[];
    /** Excluded */
    excluded: CreationTextExcludedBlock[];
    /** Issues */
    issues: CreationTextIssue[];
  };

  type CreationTextEvidence = {
    /** Block */
    block: number;
    /** Quote */
    quote: string;
    /** Occurrence */
    occurrence?: number | null;
  };

  type CreationTextExcludedBlock = {
    /** First Block */
    first_block: number;
    /** Last Block */
    last_block: number;
    /** Kind */
    kind: "heading" | "author_note" | "non_story" | "unresolved";
    /** Reason */
    reason: string;
  };

  type CreationTextIssue = {
    /** Code */
    code: string;
    /** Scope */
    scope: string;
    /** Severity */
    severity: "warning" | "blocker";
    /** Summary */
    summary: string;
  };

  type CreationTextMention = {
    /** Key */
    key: string;
    /** Kind */
    kind: "cast" | "place" | "prop";
    /** Name */
    name: string;
    /** Presence */
    presence?: "onscreen" | "offscreen" | "mentioned" | "unknown";
    evidence: CreationTextEvidence;
    /** Visual Details */
    visual_details: CreationTextEvidence[];
  };

  type CreationTextMentionRef = {
    /** Episode Key */
    episode_key: string;
    /** Scene Key */
    scene_key: string;
    /** Mention Key */
    mention_key: string;
  };

  type CreationTextRelation = {
    /** Subject */
    subject: string;
    /** Predicate */
    predicate: string;
    /** Target */
    target: string;
    /** Origin */
    origin: "extracted" | "inferred" | "proposed";
    /** Basis */
    basis: "narration" | "claim" | "unknown";
    /** Evidence */
    evidence: CreationTextEvidence[];
  };

  type CreationTextResolvedEvidence = {
    /** Revision Id */
    revision_id: string;
    /** Source Hash */
    source_hash: string;
    /** Block */
    block: number;
    /** Start */
    start: number;
    /** End */
    end: number;
    /** Text Hash */
    text_hash: string;
    /** Quote */
    quote: string;
  };

  type CreationTextScene = {
    /** First Block */
    first_block: number;
    /** Last Block */
    last_block: number;
    /** Key */
    key: string;
    /** Title */
    title: string;
    /** Summary */
    summary: string;
    /** Time Label */
    time_label: string;
    /** Time Branch */
    time_branch: string;
    /** Presentation */
    presentation: "present" | "flashback" | "dream" | "intercut" | "montage" | "unknown";
    /** Beats */
    beats: CreationTextBeat[];
    /** Dialogues */
    dialogues: CreationTextDialogue[];
    /** Mentions */
    mentions: CreationTextMention[];
    /** Issues */
    issues: CreationTextIssue[];
  };

  type CreationTextSceneDirection = {
    /** Episode Key */
    episode_key: string;
    /** Scene Key */
    scene_key: string;
    /** Dramatic Intent */
    dramatic_intent: string;
    /** Audience Knows */
    audience_knows: string[];
    /** Withhold */
    withhold: string[];
    /** Blocking */
    blocking: CreationTextBlocking[];
    /** Shots */
    shots: CreationTextShot[];
    /** Issues */
    issues: CreationTextIssue[];
  };

  type CreationTextShot = {
    /** Key */
    key: string;
    /** Purpose */
    purpose: string;
    /** Framing */
    framing: string;
    /** Camera Movement */
    camera_movement: string;
    /** Action */
    action: string;
    /** Beat Keys */
    beat_keys: string[];
    /** Audio */
    audio: CreationTextAudioCue[];
    /** Visible Mentions */
    visible_mentions: string[];
    /** Detail Evidence */
    detail_evidence: CreationTextEvidence[];
    /** Duration Min Ms */
    duration_min_ms: number;
    /** Duration Max Ms */
    duration_max_ms: number;
    /** Timing Basis */
    timing_basis: string;
    /** Screen Direction */
    screen_direction: string;
    /** Entry State */
    entry_state: string;
    /** Exit State */
    exit_state: string;
    /** Panel Caption */
    panel_caption: string;
  };

  type CreationTextStateEvent = {
    /** Entity Key */
    entity_key: string;
    /** Episode Key */
    episode_key: string;
    /** Scene Key */
    scene_key: string;
    /** Time Branch */
    time_branch: string;
    /** Story Time */
    story_time: string;
    /** Property */
    property: string;
    /** Before */
    before: string | null;
    /** After */
    after: string | null;
    /** Knowledge */
    knowledge: "known" | "unknown" | "conflicting";
    /** Basis */
    basis: "narration" | "claim" | "unknown";
    /** Evidence */
    evidence: CreationTextEvidence[];
  };

  type CreationTextWorldBook = {
    /** Entities */
    entities: CreationTextEntityProposal[];
    /** Unresolved Mentions */
    unresolved_mentions: CreationTextMentionRef[];
    /** Relations */
    relations: CreationTextRelation[];
    /** State Events */
    state_events: CreationTextStateEvent[];
    /** Asset Needs */
    asset_needs: CreationTextAssetNeed[];
    /** Issues */
    issues: CreationTextIssue[];
  };

  type DeactivateAccountRequest = {
    /** Confirmation */
    confirmation: string;
  };

  type decideHumanTaskParams = {
    human_task_id: string;
  };

  type decideProductionBibleReviewIssueParams = {
    bible_id: string;
  };

  type deleteProjectParams = {
    project_id: string;
  };

  type diffStoryGraphVersionsParams = {
    project_id: string;
    base_version_id: string;
    target_version_id: string;
    limit: number;
    cursor?: string;
  };

  type DocumentRevisionResponse = {
    /** Analysis Status */
    analysis_status: "deterministic" | "ai_candidate_required" | "rejected";
    /** Analyzer Version */
    analyzer_version: string;
    /** Codepoint Count */
    codepoint_count: number;
    /** Created At */
    created_at: string;
    /** Created By */
    created_by: string;
    /** Document Id */
    document_id: string;
    /** Id */
    id: string;
    /** Normalization Map */
    normalization_map: Record<string, any>;
    /** Normalized Hash */
    normalized_hash: string;
    /** Normalized Text */
    normalized_text: string;
    /** Normalizer Version */
    normalizer_version: string;
    /** Raw Hash */
    raw_hash: string;
    /** Raw Text */
    raw_text: string;
    /** Source Media Version Id */
    source_media_version_id: string | null;
    /** Source Type */
    source_type: "text" | "media";
    /** Version No */
    version_no: number;
    /** Workspace Id */
    workspace_id: string;
  };

  type downloadStoryboardExportParams = {
    export_id: string;
  };

  type EpisodePlanCreateRequest = {
    /** Idempotency Key */
    idempotency_key: string;
    /** Requested Episode Count */
    requested_episode_count?: number | null;
    /** Strategy */
    strategy: "explicit_markers" | "target_duration_ai";
    /** Target Duration Ms */
    target_duration_ms: number;
  };

  type EpisodePlanDetailResponse = {
    impact: EpisodePlanImpactResponse;
    plan: EpisodePlanResponse;
    /** Proposals */
    proposals: EpisodeProposalResponse[];
    source: EpisodePlanSourceResponse;
  };

  type EpisodePlanImpactBlocker = {
    /** Code */
    code: string;
    /** Next Action */
    next_action: string;
    /** Summary */
    summary: string;
  };

  type EpisodePlanImpactResponse = {
    /** Active Episode Count */
    active_episode_count: number;
    /** Active Order Hash */
    active_order_hash: string;
    /** Allowed */
    allowed: boolean;
    /** Blockers */
    blockers: EpisodePlanImpactBlocker[];
    /** Project Revision */
    project_revision: number;
    /** Projected Episode Count */
    projected_episode_count: number;
  };

  type EpisodePlanResponse = {
    /** Confirmed At */
    confirmed_at: string | null;
    /** Confirmed By */
    confirmed_by: string | null;
    /** Created At */
    created_at: string;
    /** Created By */
    created_by: string;
    /** Document Revision Id */
    document_revision_id: string;
    /** Id */
    id: string;
    /** Input Hash */
    input_hash: string;
    /** Model Name */
    model_name: string | null;
    /** Planning Engine Version */
    planning_engine_version: string;
    /** Planning Error Code */
    planning_error_code: string | null;
    /** Planning Task Id */
    planning_task_id: string | null;
    /** Project Id */
    project_id: string;
    /** Prompt Version */
    prompt_version: string | null;
    /** Requested Episode Count */
    requested_episode_count: number | null;
    /** Revision */
    revision: number;
    /** Schema Version */
    schema_version: string;
    /** Status */
    status: "draft" | "review_ready" | "confirmed" | "materialized" | "superseded";
    /** Strategy */
    strategy: "explicit_markers" | "target_duration_ai";
    /** Target Duration Ms */
    target_duration_ms: number;
    /** Total Estimated Duration Ms */
    total_estimated_duration_ms: number;
    /** Updated At */
    updated_at: string;
    /** Workspace Id */
    workspace_id: string;
  };

  type EpisodePlanSourceResponse = {
    /** Blocks */
    blocks: NarrativeBlockResponse[];
    /** Codepoint Count */
    codepoint_count: number;
    /** Document Revision Id */
    document_revision_id: string;
    /** Normalized Hash */
    normalized_hash: string;
    /** Normalized Text */
    normalized_text: string;
  };

  type EpisodeProposalResponse = {
    /** Boundary Evidence */
    boundary_evidence: Record<string, any>;
    /** Confidence */
    confidence: number;
    /** Content Hash */
    content_hash: string;
    /** End Block Id */
    end_block_id: string;
    /** End Block Position */
    end_block_position: number;
    /** Estimated Duration Ms */
    estimated_duration_ms: number;
    /** Id */
    id: string;
    /** Is Locked */
    is_locked: boolean;
    /** Plan Id */
    plan_id: string;
    /** Position */
    position: number;
    /** Reason */
    reason: string;
    /** Source End */
    source_end: number;
    /** Source Start */
    source_start: number;
    /** Start Block Id */
    start_block_id: string;
    /** Start Block Position */
    start_block_position: number;
    /** Title */
    title: string;
  };

  type EpisodeResponse = {
    /** Current Script Version Id */
    current_script_version_id: string | null;
    /** Current Timeline Version Id */
    current_timeline_version_id: string | null;
    /** Id */
    id: string;
    /** Name */
    name: string;
    /** Position */
    position: number;
    /** Project Id */
    project_id: string;
    /** Revision */
    revision: number;
    /** Status */
    status: "active" | "archived";
    /** Target Duration Ms */
    target_duration_ms: number;
    /** Workspace Id */
    workspace_id: string;
  };

  type EpisodeSegmentOriginResponse = {
    /** Document Revision Id */
    document_revision_id: string;
    /** Draft Version Id */
    draft_version_id: string;
    /** Episode Id */
    episode_id: string;
    /** Id */
    id: string;
    /** Import Commit Id */
    import_commit_id: string;
    /** Position */
    position: number;
    /** Proposal Id */
    proposal_id: string;
    /** Published Version Id */
    published_version_id: string | null;
    /** Source End */
    source_end: number;
    /** Source Hash */
    source_hash: string;
    /** Source Id */
    source_id: string;
    /** Source Start */
    source_start: number;
  };

  type FormatIssueResponse = {
    /** Code */
    code: string;
    /** Column Number */
    column_number: number;
    /** Details */
    details: Record<string, any>;
    /** Document Revision Id */
    document_revision_id: string;
    /** Id */
    id: string;
    /** Line Number */
    line_number: number;
    /** Next Action */
    next_action: string;
    /** Position */
    position: number;
    /** Severity */
    severity: "warning" | "blocking";
    /** Source End */
    source_end: number;
    /** Source Start */
    source_start: number;
  };

  type freezeStoryboardIntentsParams = {
    project_id: string;
  };

  type FreezeStoryboardIntentsRequest = {
    workspace_id: string;
    candidate_revision_id: string;
    candidate_revision_hash: string;
    expected_candidate_revision: number;
    review_decision_id: string;
    idempotency_key: string;
  };

  type getAcceptedScriptSourceParams = {
    project_id: string;
    revision_id: string;
  };

  type getApprovedStoryboardIntentsParams = {
    set_id: string;
  };

  type getCostBudgetParams = {
    project_id: string;
  };

  type getCreationAttemptHistoryParams = {
    run_id: string;
    step_id: string;
  };

  type getCreationExecutionParams = {
    run_id: string;
  };

  type getCreationManifestParams = {
    run_id: string;
  };

  type getCreationProposalParams = {
    run_id: string;
    proposal_id: string;
  };

  type getCreationRunParams = {
    run_id: string;
  };

  type getCurrentCostPriceQuoteParams = {
    project_id: string;
    profile_version_id: string;
  };

  type getCurrentProductionBibleParams = {
    project_id: string;
  };

  type getCurrentScriptDocumentParams = {
    project_id: string;
  };

  type getCurrentScriptSourceParams = {
    project_id: string;
  };

  type getCurrentStoryGraphVersionParams = {
    project_id: string;
  };

  type getCurrentStructureIdentityParams = {
    project_id: string;
  };

  type getDocumentRevisionParams = {
    revision_id: string;
  };

  type getEpisodeParams = {
    episode_id: string;
  };

  type getEpisodePlanParams = {
    plan_id: string;
  };

  type getEpisodeStructureParams = {
    episode_id: string;
  };

  type getHumanTaskParams = {
    human_task_id: string;
  };

  type getLatestStoryboardDraftParams = {
    episode_id: string;
  };

  type getLatestStoryboardExportParams = {
    episode_id: string;
  };

  type getMediaVersionParams = {
    version_id: string;
  };

  type getProductionBibleParams = {
    bible_id: string;
  };

  type getProjectCanvasParams = {
    project_id: string;
  };

  type getProjectParams = {
    project_id: string;
  };

  type getProjectPresetSelectionParams = {
    project_id: string;
  };

  type getReferenceBundleInputsParams = {
    project_id: string;
    execution_id: string;
  };

  type getReferenceCandidateBundleParams = {
    project_id: string;
    bundle_id: string;
  };

  type getReferenceCandidateSetParams = {
    project_id: string;
    set_id: string;
  };

  type getReferenceCoverageMatrixParams = {
    project_id: string;
  };

  type getReferenceExecutionProgressParams = {
    project_id: string;
    execution_id: string;
  };

  type getReferenceGenerationProgressParams = {
    project_id: string;
    generation_target_id: string;
  };

  type getReferenceTargetDetailParams = {
    project_id: string;
    target_version_id: string;
  };

  type getSceneAnalysisCandidateParams = {
    project_id: string;
    candidate_id: string;
  };

  type getScriptSourceSpanParams = {
    project_id: string;
    revision_id: string;
    start: number;
    end: number;
    expected_hash: string;
  };

  type getStoryboardDraftParams = {
    batch_id: string;
  };

  type getStoryboardDraftSetParams = {
    set_id: string;
  };

  type getStoryboardExportParams = {
    export_id: string;
  };

  type getStoryGraphVersionParams = {
    project_id: string;
    version_id: string;
  };

  type getTextIntentVersionParams = {
    project_id: string;
    version_id: string;
  };

  type getTextWorldVersionParams = {
    project_id: string;
    version_id: string;
  };

  type getWorkflowRunParams = {
    workflow_run_id: string;
  };

  type getWorkspaceParams = {
    workspace_id: string;
  };

  type HumanGateChangeEvidenceRef = {
    source_version_id: string;
    source_start: number;
    source_end: number;
    text_hash: string;
  };

  type HumanGateChangeRequest = {
    issue_refs: string[];
    evidence_refs: HumanGateChangeEvidenceRef[];
    change_spec: HumanGateChangeSpec;
    reason_code:
      | "source_interpretation_incorrect"
      | "insufficient_evidence"
      | "structure_boundary_incorrect"
      | "identity_resolution_incorrect";
    user_note?: string;
  };

  type HumanGateChangeSpec = {
    operation:
      | "inspect_source"
      | "adjust_episode_boundary"
      | "adjust_scene_boundary"
      | "separate_identity"
      | "merge_identity"
      | "resolve_mention"
      | "reject_mention";
    target_keys: string[];
    affected_scope_keys: string[];
  };

  type HumanGateCoordinationResponse = {
    review_decision_id: string;
    decision_status: string;
    owner_apply_status: "pending" | "not_required" | "completed" | "conflict";
    owner_receipt_id: any;
    workflow_resume_status: "pending" | "unknown" | "completed" | "conflict";
    workflow_signal_receipt_id: any;
    conflict_code: any;
    repair_workflow_run_id: any;
  };

  type HumanGateDecisionEnvelope = {
    data: {
      task: HumanTaskResponse;
      decision: ReviewDecisionResponse;
      coordination: HumanGateCoordinationResponse | null;
    };
  };

  type HumanGateResumeEnvelope = {
    data: { coordination: HumanGateCoordinationResponse };
  };

  type HumanTaskBaseResponse = {
    id: string;
    workspace_id: string;
    project_id: string;
    workflow_run_id: string;
    node_run_id: string;
    subject_type: string;
    subject_id: string;
    subject_revision: number;
    subject_hash: string;
    candidate_ids: string[];
    rubric_version: string;
    allowed_decisions: ("approved" | "rejected" | "changes_requested" | "selected")[];
    status: "OPEN" | "CLAIMED" | "COMPLETED" | "CANCELLED" | "STALE";
    revision: number;
    created_at: string;
    updated_at: string;
  };

  type HumanTaskClaimDetail = {
    claimed_by: string;
    expires_at: string;
    claim_token?: string;
  };

  type HumanTaskClaimRequest = {
    expected_revision: number;
    idempotency_key: string;
  };

  type HumanTaskClaimSummary = {
    claimed_by: string;
    expires_at: string;
  };

  type HumanTaskClaimTokenRequest = {
    claim_token: string;
    expected_revision: number;
    idempotency_key: string;
  };

  type HumanTaskCommandEnvelope = {
    data: { task: HumanTaskResponse };
  };

  type HumanTaskDecisionRequest = {
    claim_token: string;
    expected_task_revision: number;
    expected_subject_revision: number;
    expected_subject_hash: string;
    decision: "approved" | "rejected" | "changes_requested" | "selected";
    selected_candidate_id: any;
    change_request?: HumanGateChangeRequest | null;
    idempotency_key: string;
  };

  type HumanTaskDetailEnvelope = {
    data: {
      task: HumanTaskResponse;
      subject: StructureIdentityReviewSubjectResponse | ProductionWorldReviewSubjectResponse | null;
      decision: ReviewDecisionResponse | null;
      coordination: HumanGateCoordinationResponse | null;
    };
  };

  type HumanTaskListEnvelope = {
    data: { items: HumanTaskListItemResponse[]; next_after: any };
  };

  type HumanTaskListItemResponse =
    // #/components/schemas/HumanTaskBaseResponse
    HumanTaskBaseResponse & {
      claim: HumanTaskClaimSummary | null;
    };

  type HumanTaskResponse =
    // #/components/schemas/HumanTaskBaseResponse
    HumanTaskBaseResponse & {
      claim: HumanTaskClaimDetail | null;
    };

  type ImportCommitDetailResponse = {
    commit: ImportCommitResponse;
    /** Segments */
    segments: EpisodeSegmentOriginResponse[];
  };

  type ImportCommitResponse = {
    /** Created At */
    created_at: string;
    /** Created By */
    created_by: string;
    /** Error Code */
    error_code: string | null;
    /** Expected Active Order Hash */
    expected_active_order_hash: string;
    /** Expected Project Revision */
    expected_project_revision: number;
    /** Id */
    id: string;
    /** Input Hash */
    input_hash: string;
    /** Mode */
    mode: string;
    /** Plan Id */
    plan_id: string;
    /** Project Id */
    project_id: string;
    /** Revision */
    revision: number;
    /** Status */
    status:
      | "pending"
      | "materializing"
      | "materialized"
      | "publishing"
      | "published"
      | "conflict"
      | "failed";
    /** Updated At */
    updated_at: string;
    /** Workspace Id */
    workspace_id: string;
  };

  type listCreationProposalsParams = {
    run_id: string;
  };

  type listCreationRunsParams = {
    project_id: string;
    limit?: number;
  };

  type listEpisodesParams = {
    project_id: string;
  };

  type listHumanTasksParams = {
    project_id: string;
    status?: "active" | "OPEN" | "CLAIMED" | "COMPLETED" | "CANCELLED" | "STALE";
    subject_type?: string;
    limit?: number;
    after?: string;
  };

  type listScriptDocumentsParams = {
    project_id: string;
  };

  type listStoryboardShotsParams = {
    episode_id: string;
  };

  type LoginRequest = {
    /** Email */
    email: string;
    /** Password */
    password: string;
  };

  type materializeEpisodePlanParams = {
    plan_id: string;
  };

  type MaterializeEpisodePlanRequest = {
    /** Expected Active Order Hash */
    expected_active_order_hash: string;
    /** Expected Plan Revision */
    expected_plan_revision: number;
    /** Expected Project Revision */
    expected_project_revision: number;
    /** Idempotency Key */
    idempotency_key: string;
    /** Mode */
    mode: string;
  };

  type materializeReferenceCandidateSetParams = {
    project_id: string;
    execution_id: string;
  };

  type MediaObjectResponse = {
    /** Current Version Id */
    current_version_id: string | null;
    /** Id */
    id: string;
    /** Kind */
    kind: "image" | "video" | "audio" | "subtitle" | "delivery" | "document";
    /** Revision */
    revision: number;
    /** Source Type */
    source_type: "upload" | "generated" | "rendered";
    /** Status */
    status: "active" | "archived";
    /** Workspace Id */
    workspace_id: string;
  };

  type MediaVersionResponse = {
    /** Codec */
    codec: string | null;
    /** Container */
    container: string | null;
    /** Created At */
    created_at: string;
    /** Duration Ms */
    duration_ms: number | null;
    /** Filename */
    filename: string;
    /** Height */
    height: number | null;
    /** Id */
    id: string;
    /** Media Object Current Version Id */
    media_object_current_version_id: string | null;
    /** Media Object Id */
    media_object_id: string;
    /** Media Object Kind */
    media_object_kind: "image" | "video" | "audio" | "subtitle" | "delivery" | "document";
    /** Media Object Revision */
    media_object_revision: number;
    /** Media Object Source Type */
    media_object_source_type: "upload" | "generated" | "rendered";
    /** Media Object Status */
    media_object_status: "active" | "archived";
    /** Mime Type */
    mime_type: string;
    /** Probe Attempt */
    probe_attempt: number;
    /** Probe Error Code */
    probe_error_code: string | null;
    /** Probe Error Summary */
    probe_error_summary: string | null;
    /** Probe Next Action */
    probe_next_action: string | null;
    /** Probe Status */
    probe_status: "pending" | "ready" | "failed" | "quarantined";
    /** Sha256 */
    sha256: string;
    /** Size Bytes */
    size_bytes: number;
    /** Version No */
    version_no: number;
    /** Width */
    width: number | null;
    /** Workspace Id */
    workspace_id: string;
  };

  type MeResponse = {
    user: UserResponse;
    workspace: WorkspaceResponse;
  };

  type NarrativeBlockResponse = {
    /** Document Revision Id */
    document_revision_id: string;
    /** Id */
    id: string;
    /** Kind */
    kind:
      | "preamble"
      | "episode_marker"
      | "scene_heading"
      | "dialogue"
      | "narration"
      | "action"
      | "separator";
    /** Metadata */
    metadata: Record<string, any>;
    /** Position */
    position: number;
    /** Source End */
    source_end: number;
    /** Source Start */
    source_start: number;
    /** Text Hash */
    text_hash: string;
  };

  type PaginatedProjects = {
    /** Items */
    items: ProjectResponse[];
    /** Limit */
    limit: number;
    /** Offset */
    offset: number;
    /** Total */
    total: number;
  };

  type prepareInitialReferenceExecutionParams = {
    project_id: string;
    generation_target_id: string;
  };

  type PresetCapability = {
    target_kind: string;
    view_roles: string[];
  };

  type PresetContentRef = {
    owner: string;
    key: string;
    content_hash: string;
  };

  type PresetProvenance = {
    origin: "first_party" | "open_source";
    source_url: string;
    license_spdx: string;
    notice_path: string;
  };

  type PresetPurposeProfile = {
    target_kind: string;
    design_focus: string[];
    forbidden_changes: string[];
  };

  type PresetRelease = {
    contract_id: string;
    key: string;
    release: string;
    label: string;
    category: string;
    description: string;
    default_mode: ProjectPresetApplicationMode;
    provenance: PresetProvenance;
    capability_manifest: PresetCapability[];
    world_design_basis: PresetWorldDesignBasis;
    visual_grammar: PresetVisualGrammar;
    fidelity_invariants: string[];
    world_adaptation_rules: PresetWorldAdaptationRule[];
    purpose_profiles: PresetPurposeProfile[];
    skill_release_refs: PresetContentRef[];
    qc_policy_ref: PresetContentRef;
    model_capability_policy_ref: PresetContentRef;
    content_hash: string;
  };

  type PresetVisualGrammar = {
    medium: string;
    realism: string;
    shape_language: string;
    proportion: string;
    palette: string;
    linework: string;
    texture: string;
    material_rendering: string;
    lighting: string;
    contrast: string;
    composition: string;
    camera: string;
    negative_constraints: string[];
  };

  type PresetWorldAdaptationRule = {
    rule_key: string;
    source_fact_kind: string;
    design_domain: string;
    directive: string;
    preserved_invariant_keys: string[];
    impact_scope_kinds: string[];
    requires_human_decision: boolean;
  };

  type PresetWorldDesignBasis = {
    era: string;
    region: string;
    civilization_language: string;
    technology_or_magic_language: string;
    architecture_language: string;
    wardrobe_language: string;
    prop_language: string;
    material_system: string;
    motifs: string[];
    anachronism_constraints: string[];
  };

  type previewScriptImportParams = {
    project_id: string;
  };

  type ProductionBibleConfirmRequest = {
    /** Expected Result Hash */
    expected_result_hash: string;
    /** Expected Revision */
    expected_revision: number;
    /** Idempotency Key */
    idempotency_key: string;
  };

  type ProductionBibleCreateRequest = {
    /** Idempotency Key */
    idempotency_key: string;
  };

  type ProductionBibleEntityResponse = {
    /** Aliases */
    aliases: string[];
    /** Asset Id */
    asset_id: string | null;
    /** Canonical Name */
    canonical_name: string;
    /** Created At */
    created_at: string;
    /** Entity Key */
    entity_key: string;
    /** Episode Numbers */
    episode_numbers: number[];
    /** Evidence */
    evidence: BibleEvidence[];
    /** Id */
    id: string;
    /** Kind */
    kind: "character" | "location" | "prop" | "costume" | "visual_style" | "voice";
    /** Normalized Name */
    normalized_name: string;
    /** Stable Spec */
    stable_spec: Record<string, any>;
    /** States */
    states?: ProductionBibleEntityStateResponse[];
    /** Updated At */
    updated_at: string;
  };

  type ProductionBibleEntityStateResponse = {
    /** Asset State Id */
    asset_state_id: string | null;
    /** Asset Version Id */
    asset_version_id: string | null;
    /** Created At */
    created_at: string;
    /** Entity Id */
    entity_id: string;
    /** Episode Numbers */
    episode_numbers: number[];
    /** Evidence */
    evidence: BibleEvidence[];
    /** Id */
    id: string;
    /** Label */
    label: string;
    /** State Key */
    state_key: string;
    /** State Spec */
    state_spec: Record<string, any>;
    /** Updated At */
    updated_at: string;
  };

  type ProductionBibleEnvelope = {
    data: ProductionBibleResponse;
  };

  type ProductionBibleResponse = {
    /** Checkpoint Revision */
    checkpoint_revision: number;
    /** Checkpoint Stage */
    checkpoint_stage: string | null;
    /** Checkpoint Updated At */
    checkpoint_updated_at: string | null;
    /** Confirmed At */
    confirmed_at: string | null;
    /** Confirmed By */
    confirmed_by: string | null;
    /** Created At */
    created_at: string;
    /** Document Revision Id */
    document_revision_id: string;
    /** Engine Version */
    engine_version: string;
    /** Entities */
    entities?: ProductionBibleEntityResponse[];
    /** Harness Version */
    harness_version: string;
    /** Id */
    id: string;
    /** Input Hash */
    input_hash: string;
    /** Model Name */
    model_name: string;
    /** Project Id */
    project_id: string;
    /** Prompt Version */
    prompt_version: string;
    /** Result Hash */
    result_hash?: string | null;
    /** Review Issues */
    review_issues: BibleReviewIssue[];
    /** Revision */
    revision: number;
    /** Schema Version */
    schema_version: string;
    /** Status */
    status:
      | "queued"
      | "running"
      | "needs_review"
      | "confirmed"
      | "failed"
      | "unknown"
      | "superseded"
      | "cancelled";
    /** Task Id */
    task_id: string | null;
    /** Updated At */
    updated_at: string;
    /** Workspace Id */
    workspace_id: string;
    /** World Entries */
    world_entries?: ProductionBibleWorldEntryResponse[];
    review_decisions: Record<string, any>;
    /** Generation Error */
    generation_error: TaskErrorResponse | null;
  };

  type ProductionBibleResumeRequest = {
    /** Expected Revision */
    expected_revision: number;
    /** Idempotency Key */
    idempotency_key: string;
  };

  type ProductionBibleReviewDecisionRequest = {
    issue_key: string;
    action: "accepted" | "rejected";
    expected_revision: number;
    idempotency_key: string;
  };

  type ProductionBibleWorldEntryResponse = {
    /** Category */
    category: string;
    /** Created At */
    created_at: string;
    /** Entity Keys */
    entity_keys: string[];
    /** Entry Key */
    entry_key: string;
    /** Episode Numbers */
    episode_numbers: number[];
    /** Evidence */
    evidence: BibleEvidence[];
    /** Facts */
    facts: string[];
    /** Id */
    id: string;
    /** Rules */
    rules: string[];
    /** Title */
    title: string;
    /** Updated At */
    updated_at: string;
  };

  type ProductionWorldCandidateReviewIssueResponse = {
    issue_key: string;
    code: string;
    severity: "warning" | "blocking";
    scope: string;
    summary: string;
    evidence: ProductionWorldEvidenceSpanResponse[];
  };

  type ProductionWorldCandidateRevisionResponse = {
    candidate_revision_id: string;
    candidate_revision: number;
    candidate_revision_hash: string;
    candidate_content_hash: string;
  };

  type ProductionWorldClaimAnchorResponse = {
    role: "episode" | "scene" | "beat";
    target_key: string;
  };

  type ProductionWorldClaimParticipantResponse = {
    role: "subject" | "object" | "participant";
    identity_key: string;
  };

  type ProductionWorldClaimResponse = {
    claim_key: string;
    claim_type:
      | "world_rule"
      | "relationship"
      | "foreshadowing"
      | "payoff"
      | "story_arc"
      | "plot_thread";
    participants: ProductionWorldClaimParticipantResponse[];
    statement: string;
    narrative: ProductionWorldNarrativeClaimResponse | null;
    basis: ProductionWorldSourceBasisResponse;
  };

  type ProductionWorldClaimScopeResponse = {
    kind: "project" | "episode" | "scene" | "beat";
    owner_logical_id: string;
  };

  type ProductionWorldContinuityClaimResponse = {
    continuity_key: string;
    claim_series_key: string;
    claim_revision: number;
    supersedes_continuity_key: any;
    subject_kind: string;
    identity_key: string;
    from_scene_scope_key: string;
    to_scene_scope_key: string;
    story_time_start: string;
    story_time_end: string;
    before_state_key: string;
    after_state_key: string;
    transition: string;
    delta: any;
    evidence: ProductionWorldEvidenceSpanResponse[];
  };

  type ProductionWorldContinuityLedgerResponse = {
    ledger_key: string;
    subject_kind: string;
    identity_key: string;
    scene_scope_key: string;
    story_time_key: string;
    state_key: string;
    holder_identity_key: any;
    location_identity_key: any;
    transition_interaction_key: any;
    evidence: ProductionWorldEvidenceSpanResponse[];
  };

  type ProductionWorldContinuityReviewResponse = {
    claims: ProductionWorldContinuityClaimResponse[];
    ledger: ProductionWorldContinuityLedgerResponse[];
  };

  type ProductionWorldCreatorDecisionProposalResponse = {
    decision_key: string;
    rationale: string;
  };

  type ProductionWorldDesignGapResponse = {
    gap_key: string;
    subject_key: string;
    field_key: string;
    missing_reason: string;
    source_constraints: ProductionWorldEvidenceSpanResponse[];
    mutually_exclusive_options: string[];
    impacted_scene_scope_keys: string[];
    allowed_resolution_sources: string[];
  };

  type ProductionWorldEntityReviewItemResponse = {
    identity_key: string;
    kind: "character" | "location" | "prop";
    specification_key: string;
    specification_slots: ProductionWorldSemanticSlotResponse[];
    states: ProductionWorldStateResponse[];
    basis: ProductionWorldSourceBasisResponse;
  };

  type ProductionWorldEvidenceSpanResponse = {
    source_start: number;
    source_end: number;
    text_hash: string;
    exact_anchor: string;
  };

  type ProductionWorldInteractionGeometryEvidenceResponse = {
    hand: ProductionWorldEvidenceSpanResponse | null;
    grip_type: ProductionWorldEvidenceSpanResponse | null;
    contact_point: ProductionWorldEvidenceSpanResponse | null;
    direction: ProductionWorldEvidenceSpanResponse | null;
    relative_scale: ProductionWorldEvidenceSpanResponse | null;
  };

  type ProductionWorldInteractionResponse = {
    interaction_key: string;
    claim_series_key: string;
    claim_revision: number;
    supersedes_interaction_key: any;
    scene_scope_key: string;
    beat_key: any;
    story_time_key: string;
    predicate: string;
    actor_occurrence_key: string;
    prop_occurrence_key: string;
    counterparty_occurrence_key: any;
    holder_before_identity_key: any;
    holder_after_identity_key: any;
    prop_state_before_key: string;
    prop_state_after_key: string;
    state_delta: any;
    hand: string;
    grip_type: any;
    contact_point: any;
    direction: any;
    relative_scale: ProductionWorldPositiveRationalResponse | null;
    geometry_evidence: ProductionWorldInteractionGeometryEvidenceResponse;
    evidence: ProductionWorldEvidenceSpanResponse;
  };

  type ProductionWorldNarrativeClaimResponse = {
    claim_series_key: string;
    predicate: string;
    anchors: ProductionWorldClaimAnchorResponse[];
    valid_scope: ProductionWorldClaimScopeResponse;
    story_time_range: ProductionWorldStoryTimeRangeResponse | null;
    polarity: "positive" | "negative" | "neutral";
    status: "asserted" | "negated";
  };

  type ProductionWorldOccurrenceResponse = {
    occurrence_key: string;
    order: number;
    subject_kind: "character" | "location" | "prop";
    identity_key: string;
    state_key: string;
    occurrence_role: string;
    evidence: ProductionWorldEvidenceSpanResponse;
  };

  type ProductionWorldPartitionRootsResponse = {
    bible: string;
    planning: string;
    asset: string;
    proof: string;
  };

  type ProductionWorldPositiveRationalResponse = {
    numerator: number;
    denominator: number;
  };

  type ProductionWorldRepairTargetSetResponse = {
    operation:
      | "revise_production_entity"
      | "rebind_scene_occurrence"
      | "revise_interaction"
      | "revise_continuity";
    target_keys: string[];
  };

  type ProductionWorldReviewIssueResponse = {
    source_stage: string;
    issue: ProductionWorldCandidateReviewIssueResponse;
  };

  type ProductionWorldReviewSubjectResponse = {
    schema_version: string;
    gate_key: string;
    input_hash: string;
    candidate_revision: ProductionWorldCandidateRevisionResponse;
    partition_roots: ProductionWorldPartitionRootsResponse;
    allowed_decisions: any[];
    repair_targets: ProductionWorldRepairTargetSetResponse[];
    views: ProductionWorldReviewViewsResponse;
    world_claims: ProductionWorldClaimResponse[];
    design_gaps: ProductionWorldDesignGapResponse[];
    review_issues: ProductionWorldReviewIssueResponse[];
  };

  type ProductionWorldReviewViewsResponse = {
    character_appearances: ProductionWorldEntityReviewItemResponse[];
    locations: ProductionWorldEntityReviewItemResponse[];
    prop_states: ProductionWorldEntityReviewItemResponse[];
    scene_occurrences: ProductionWorldSceneOccurrenceReviewItemResponse[];
    interactions: ProductionWorldInteractionResponse[];
    continuity: ProductionWorldContinuityReviewResponse;
  };

  type ProductionWorldSceneOccurrenceReviewItemResponse = {
    scene_scope_key: string;
    scene_owner_logical_id: string;
    temporary_scene_id: string;
    source_start: number;
    source_end: number;
    story_time_key: string;
    occurrences: ProductionWorldOccurrenceResponse[];
  };

  type ProductionWorldSemanticSlotResponse = {
    slot_key: string;
    resolution: string;
    value: any;
    design_gap_key: any;
  };

  type ProductionWorldSourceBasisResponse = {
    provenance: string;
    evidence: ProductionWorldEvidenceSpanResponse[];
    creator_decision_proposal: ProductionWorldCreatorDecisionProposalResponse | null;
  };

  type ProductionWorldStateResponse = {
    state_key: string;
    state_kind: string;
    complete_slots: ProductionWorldSemanticSlotResponse[];
    applicable_scene_scope_keys: string[];
    entry_reason: string;
    exit_reason: string;
    previous_state_key: any;
    next_state_key: any;
    basis: ProductionWorldSourceBasisResponse;
  };

  type ProductionWorldStoryTimeRangeResponse = {
    start_key: string;
    end_key: string;
  };

  type ProfileUpdateRequest = {
    /** Avatar Url */
    avatar_url?: string | null;
    /** Display Name */
    display_name?: string | null;
  };

  type ProjectCreateRequest = {
    /** Aspect Ratio */
    aspect_ratio?: "9:16" | "16:9" | "1:1";
    /** Description */
    description?: string | null;
    /** Language */
    language?: string;
    /** Name */
    name: string;
    /** Target Duration Ms */
    target_duration_ms?: number;
    /** Visual Style */
    visual_style?: string | null;
    /** Workspace Id */
    workspace_id: string;
    idempotency_key: string;
  };

  type projectDeletePreflightParams = {
    project_id: string;
  };

  type ProjectPresetApplicationMode = "faithful" | "world_adaptation";

  type ProjectPresetReleaseRef = {
    key: string;
    release: string;
    content_hash: string;
  };

  type ProjectPresetSelection = {
    id: string;
    contract_id: string;
    workspace_id: string;
    project_id: string;
    revision: number;
    parent_selection_id: string | null;
    parent_content_hash: string | null;
    preset_release: ProjectPresetReleaseRef;
    application_mode: ProjectPresetApplicationMode;
    selected_by: string;
    selected_at: string;
    content_hash: string;
  };

  type ProjectPresetSelectionRequest = {
    preset_key: string;
    preset_release: string;
    application_mode: ProjectPresetApplicationMode;
    expected_revision: number;
    idempotency_key: string;
  };

  type ProjectResponse = {
    /** Aspect Ratio */
    aspect_ratio: "9:16" | "16:9" | "1:1";
    /** Description */
    description: string | null;
    /** Id */
    id: string;
    /** Language */
    language: string;
    /** Name */
    name: string;
    /** Revision */
    revision: number;
    /** Status */
    status: "active" | "archived";
    /** Target Duration Ms */
    target_duration_ms: number;
    /** Visual Style */
    visual_style: string | null;
    /** Workspace Id */
    workspace_id: string;
  };

  type publishEpisodeScriptsParams = {
    commit_id: string;
  };

  type PublishImportCommitRequest = {
    /** Expected Revision */
    expected_revision: number;
    /** Idempotency Key */
    idempotency_key: string;
  };

  type queryStoryGraphLensParams = {
    project_id: string;
    version_ref: string | string;
    lens: "outline" | "narrative" | "entity" | "production" | "impact";
    scope_kind: "project" | "story_node";
    scope_id: string;
    depth: number;
    limit: number;
    cursor?: string;
  };

  type recoverStoryAnalysisShardParams = {
    workflow_run_id: string;
  };

  type ReferenceBriefCandidateRefResponse = {
    revision_id: string;
    revision: number;
    revision_hash: string;
    content_hash: string;
  };

  type ReferenceBundleAdmissionResponse = {
    policy_ref: { contract_id: string; content_hash: string };
    internal_review_ready: boolean;
    selection_ready: boolean;
    publication_ready: boolean;
    internal_review_blockers: string[];
    formal_use_blockers: string[];
  };

  type ReferenceBundleEvaluationResponse = {
    input: ReferenceCandidateBundleInputResponse;
    slot_qc_results: ReferenceDeterministicQCResponse[];
    bundle_qc_result: ReferenceDeterministicQCResponse;
    admission: ReferenceBundleAdmissionResponse;
  };

  type ReferenceBundleInputsResponse = {
    contract_id: string;
    generation_target_ref: { id: string; revision: number; content_hash: string };
    execution_ref: { id: string; revision: number; content_hash: string };
    job_hash: string;
    bundles: ReferenceBundleEvaluationResponse[];
    content_hash: string;
  };

  type ReferenceBundleSlotResponse = {
    slot_key: string;
    view_role: string;
    provider_call_ref: {
      contract_id: string;
      execution_ref: { id: string; revision: number; content_hash: string };
      bundle_index: number;
      slot_key: string;
      compiled_request_hash: string;
      call_key: string;
    };
    call_state_hash: string;
    call_status: "SUCCEEDED" | "FAILED";
    staged_media_ref?: { id: string; revision: number; content_hash: string };
    media_sha256: string;
    deterministic_qc_result_ref: { id: string; content_hash: string };
  };

  type ReferenceCandidateBundleInputResponse = {
    bundle_input_id: string;
    generation_target_ref: { id: string; revision: number; content_hash: string };
    execution_ref: { id: string; revision: number; content_hash: string };
    generation_round: number;
    candidate_bundle_index: number;
    output_contract_ref: { contract_id: string; content_hash: string };
    slots: ReferenceBundleSlotResponse[];
    slot_set_root: string;
    bundle_completeness: "complete" | "partial_explicit_failure";
    bundle_deterministic_qc_result_ref: { id: string; content_hash: string };
    dependency_root_hash: string;
    created_at: string;
    content_hash: string;
  };

  type ReferenceCandidateBundleResponse = {
    contract_id: string;
    candidate_bundle_id: string;
    workspace_id: string;
    project_id: string;
    generation_target_ref: { id: string; revision: number; content_hash: string };
    execution_ref: { id: string; revision: number; content_hash: string };
    generation_round: number;
    candidate_bundle_index: number;
    bundle_input_ref: ReferenceCommandActionRef;
    bundle_vision_review_candidate_revision_ref: {
      id: string;
      revision: number;
      content_hash: string;
    };
    dependency_root_hash: string;
    created_at: string;
    content_hash: string;
  };

  type ReferenceCandidateSetRequest = {
    execution_hash: string;
    expected_progress_hash: string;
    candidate_bundle_refs: { id: string; revision: number; content_hash: string }[];
  };

  type ReferenceCandidateSetResponse = {
    contract_id: string;
    candidate_set_id: string;
    workspace_id: string;
    project_id: string;
    generation_target_ref: { id: string; revision: number; content_hash: string };
    execution_ref: { id: string; revision: number; content_hash: string };
    generation_round: number;
    expected_bundle_count: number;
    execution_progress_hash: string;
    ordered_candidate_bundle_refs: {
      candidate_bundle_index: number;
      candidate_bundle_ref: { id: string; revision: number; content_hash: string };
    }[];
    failed_or_unknown_slot_refs: {
      provider_call_ref: {
        contract_id: string;
        execution_ref: { id: string; revision: number; content_hash: string };
        bundle_index: number;
        slot_key: string;
        compiled_request_hash: string;
        call_key: string;
      };
      call_state_hash: string;
      deterministic_qc_ref: ReferenceCommandActionRef | null;
      issues: (
        | "provider_explicit_failure"
        | "media_rejected"
        | "media_policy_failed"
        | "duplicate_image"
        | "outcome_unknown"
      )[];
    }[];
    generation_completion_state: "complete" | "partial_explicit_failure" | "outcome_unknown";
    dependency_root_hash: string;
    created_at: string;
    content_hash: string;
  };

  type ReferenceCommandActionRef = {
    id: string;
    content_hash: string;
  };

  type ReferenceCoverageBlockerResponse = {
    code:
      | "dependency_asset_version_unavailable"
      | "reference_brief_rejected"
      | "reference_brief_outcome_unknown";
    target_business_key?: string;
  };

  type ReferenceCoverageMatrixResponse = {
    schema_version: string;
    plan_version_id: string;
    plan_revision: number;
    plan_content_hash: string;
    rows: ReferenceCoverageRowResponse[];
    summary: ReferenceCoverageSummaryResponse;
    content_hash: string;
  };

  type ReferenceCoverageRowResponse = {
    target_version_id: string;
    target_content_hash: string;
    target_business_key: string;
    target_kind:
      | "character_appearance"
      | "character_identity_anchor"
      | "interaction_composition"
      | "location_board"
      | "prop_sheet"
      | "scene_composition";
    fulfillment: "required" | "optional" | "not_generated";
    wave_key: "base" | "appearance" | "composition" | "none";
    coverage_scope_keys: string[];
    depends_on_target_business_keys: string[];
    status: "not_generated" | "planned" | "generating" | "blocked";
    brief_status:
      | "not_required"
      | "dependency_blocked"
      | "not_started"
      | "queued"
      | "running"
      | "accepted"
      | "rejected"
      | "outcome_unknown";
    brief_invocation_id?: string;
    brief_input_hash?: string;
    brief_candidate?: ReferenceBriefCandidateRefResponse;
    blockers: ReferenceCoverageBlockerResponse[];
    allowed_actions: "view_target"[];
  };

  type ReferenceCoverageSummaryResponse = {
    required_total: number;
    optional_total: number;
    not_generated: number;
    planned: number;
    generating: number;
    blocked: number;
    selected_total: number;
    reference_ready: boolean;
  };

  type ReferenceDeterministicQCResponse = {
    qc_result_id: string;
    scope: "slot" | "bundle";
    input_root: string;
    policy_ref: { contract_id: string; content_hash: string };
    status: "passed" | "blocked" | "failed";
    issues: (
      | "rights_not_assessed"
      | "provider_explicit_failure"
      | "media_rejected"
      | "media_policy_failed"
      | "duplicate_image"
    )[];
    content_hash: string;
  };

  type ReferenceExecutionAuthorizationRequest = {
    workspace_id: string;
    target_hash: string;
    selected_provider_binding_ref: executionRef;
    idempotency_key: string;
  };

  type ReferenceExecutionAuthorizationResponse = {
    execution_authorization_ref: ReferenceCommandActionRef;
  };

  type ReferenceExecutionPreparationRequest = {
    workspace_id: string;
    target_hash: string;
    execution_authorization_ref: ReferenceCommandActionRef;
    idempotency_key: string;
  };

  type ReferenceExecutionPreparationResponse = {
    execution_ref: executionRef;
  };

  type ReferenceExecutionProgressResponse = {
    execution_ref: { id: string; revision: number; content_hash: string };
    job_hash: string;
    call_set_root: string;
    status:
      | "PENDING"
      | "RUNNING"
      | "OUTCOME_UNKNOWN"
      | "SUCCEEDED"
      | "PARTIAL_SUCCEEDED"
      | "FAILED";
    terminal: boolean;
    total: number;
    pending: number;
    dispatching: number;
    succeeded: number;
    failed: number;
    outcome_unknown: number;
    calls: {
      call_key: string;
      bundle_index: number;
      slot_key: string;
      status: "PENDING" | "DISPATCHING" | "OUTCOME_UNKNOWN" | "SUCCEEDED" | "FAILED";
      revision: number;
      state_hash: string;
    }[];
    content_hash: string;
  };

  type ReferenceExecutionWorkflowStartRequest = {
    execution_hash: string;
    idempotency_key: string;
  };

  type ReferenceExecutionWorkflowStartResponse = {
    workflow_run_id: string;
    status: status;
  };

  type ReferenceGenerationAuthorizationRequest = {
    workspace_id: string;
    plan_version_id: string;
    plan_content_hash: string;
    target_content_hash: string;
    brief_revision_id: string;
    brief_revision_hash: string;
    candidate_bundle_count: number;
    idempotency_key: string;
  };

  type ReferenceGenerationAuthorizationResponse = {
    generation_authorization_ref: ReferenceCommandActionRef;
  };

  type ReferenceGenerationProgressResponse = {
    workspace_id: string;
    project_id: string;
    generation_target_ref: executionRef;
    plan_ref: { id: string; revision: number; content_hash: string };
    reference_target_ref: planRef;
    generation_round: number;
    execution: ReferenceExecutionProgressResponse | null;
    content_hash: string;
  };

  type ReferenceGenerationTargetBuildRequest = {
    workspace_id: string;
    generation_authorization_ref: ReferenceCommandActionRef;
    brief_revision_id: string;
    brief_revision_hash: string;
    slot_policies: ReferenceOutputSlotPolicyRequest[];
    idempotency_key: string;
  };

  type ReferenceGenerationTargetBuildResponse = {
    generation_target_ref: executionRef;
  };

  type ReferenceOutputSlotPolicyRequest = {
    view_role: string;
    allowed_media_types: ("image/png" | "image/jpeg")[];
    aspect_ratio: string;
    min_width: number;
    min_height: number;
    max_bytes: number;
  };

  type ReferencePlanOwnerRefResponse = {
    workspace_id: string;
    project_id: string;
    owner_kind: string;
    version_family: string;
    owner_logical_id: string;
    owner_version_id: string;
    owner_revision: number;
    owner_content_hash: string;
    fragment_key: any;
    fragment_content_hash: any;
  };

  type ReferencePlanTargetOwnerRefsResponse = {
    identity: ReferencePlanOwnerRefResponse[];
    specification: ReferencePlanOwnerRefResponse[];
    state: ReferencePlanOwnerRefResponse[];
    scene: ReferencePlanOwnerRefResponse[];
    occurrence: ReferencePlanOwnerRefResponse[];
    interaction: ReferencePlanOwnerRefResponse[];
  };

  type ReferencePlanTargetVersionResponse = {
    id: string;
    contract_id: string;
    workspace_id: string;
    project_id: string;
    plan_logical_id: string;
    plan_version_id: string;
    revision: number;
    target_business_key: string;
    target_kind: targetKind;
    fulfillment: fulfillment;
    owner_refs: ReferencePlanTargetOwnerRefsResponse;
    coverage_scope_keys: string[];
    depends_on_target_business_keys: string[];
    constraints: ReferenceTargetConstraintsResponse;
    effective_style_snapshot: ReferenceVersionRefResponse;
    effective_policy_snapshot: ReferenceVersionRefResponse;
    review_decision_id: string;
    created_by: string;
    created_at: string;
    content_hash: string;
  };

  type ReferenceTargetConstraintsResponse = {
    production_world_owner_set_hash: string;
    reference_target_seed_root: string;
    visual_foundation_candidate_revision_id: string;
    visual_foundation_candidate_revision_hash: string;
    preset_release_content_hash: string;
    design_focus: string[];
    forbidden_changes: string[];
  };

  type ReferenceTargetDetailResponse = {
    schema_version: string;
    plan_version_id: string;
    plan_revision: number;
    plan_content_hash: string;
    target: ReferencePlanTargetVersionResponse;
    row: ReferenceCoverageRowResponse;
    content_hash: string;
  };

  type ReferenceVersionRefResponse = {
    workspace_id: string;
    project_id: string;
    owner_kind: string;
    version_family: string;
    owner_logical_id: string;
    owner_version_id: string;
    owner_revision: number;
    owner_content_hash: string;
  };

  type RegisterRequest = {
    /** Display Name */
    display_name: string;
    /** Password */
    password: string;
    /** Registration Ticket */
    registration_ticket: string;
  };

  type RegistrationVerificationAccepted = {
    /** Accepted */
    accepted?: boolean;
    /** Email Sent */
    email_sent: boolean;
    /** Retry After Seconds */
    retry_after_seconds: number;
  };

  type RegistrationVerificationConfirmed = {
    /** Expires In */
    expires_in: number;
    /** Registration Ticket */
    registration_ticket: string;
  };

  type RegistrationVerificationConfirmRequest = {
    /** Code */
    code: string;
    /** Email */
    email: string;
  };

  type RegistrationVerificationRequest = {
    /** Email */
    email: string;
  };

  type releaseHumanTaskClaimParams = {
    human_task_id: string;
  };

  type renewHumanTaskClaimParams = {
    human_task_id: string;
  };

  type rerunWorkflowFromNodeParams = {
    workflow_run_id: string;
  };

  type restoreProjectParams = {
    project_id: string;
  };

  type restoreWorkspaceParams = {
    workspace_id: string;
  };

  type resumeCreationRunParams = {
    run_id: string;
  };

  type ResumeCreationRunRequest = {
    expected_revision: number;
  };

  type resumeHumanGateFromReviewDecisionParams = {
    review_decision_id: string;
  };

  type resumeProductionBibleParams = {
    bible_id: string;
  };

  type retryCreationDeliveryParams = {
    run_id: string;
  };

  type RetryCreationDeliveryRequest = {
    expected_revision: number;
  };

  type ReviewDecisionResponse = {
    id: string;
    human_task_id: string;
    decision: "approved" | "rejected" | "changes_requested" | "selected";
    subject_revision: number;
    subject_hash: string;
    selected_candidate_id: any;
    change_request: HumanGateChangeRequest | null;
    decision_payload_hash: string;
    created_by: string;
    created_at: string;
  };

  type RevocationResponse = {
    /** Revoked */
    revoked?: boolean;
  };

  type SceneAnalysisCandidateResponse = {
    id: string;
    workspace_id: string;
    project_id: string;
    stage_key: "propose_script_spans" | "extract_scene_facts";
    profile_key: string;
    stage_instance_key: string;
    revision: number;
    candidate_type: "script_span_candidate" | "scene_fact_candidate";
    candidate: ScriptSpanCandidate | SceneFactCandidate;
    candidate_content_hash: string;
    candidate_revision_hash: string;
    source_invocation_id: string;
    source_result_id: string;
    source_result_hash: string;
    created_at: string;
  };

  type SceneAnalysisDialogue = {
    speaker_mention: string;
    text: string;
    evidence: SceneAnalysisEvidence;
  };

  type SceneAnalysisEvidence = {
    source_start: number;
    source_end: number;
    text_hash: string;
    exact_anchor: string;
  };

  type SceneAnalysisGroundedText = {
    text: string;
    evidence: SceneAnalysisEvidence;
  };

  type SceneAnalysisReviewIssue = {
    issue_key: string;
    code: string;
    severity: "warning" | "blocking";
    scope: string;
    summary: string;
    evidence: SceneAnalysisEvidence[];
  };

  type SceneFact = {
    temporary_scene_id: string;
    span_id: string;
    source_start: number;
    source_end: number;
    location: SceneAnalysisGroundedText | null;
    time: SceneAnalysisGroundedText | null;
    actions: SceneAnalysisGroundedText[];
    dialogues: SceneAnalysisDialogue[];
    raw_character_mentions: SceneAnalysisGroundedText[];
    raw_prop_mentions: SceneAnalysisGroundedText[];
  };

  type SceneFactCandidate = {
    source_version_id: string;
    source_hash: string;
    span_candidate_revision_id: string;
    span_candidate_revision_hash: string;
    scenes: SceneFact[];
    review_issues: SceneAnalysisReviewIssue[];
  };

  type ScriptDocumentAnalysisResponse = {
    /** Blocks */
    blocks: NarrativeBlockResponse[];
    document: ScriptDocumentResponse;
    /** Issues */
    issues: FormatIssueResponse[];
    revision: DocumentRevisionResponse;
  };

  type ScriptDocumentImportRequest = {
    /** Idempotency Key */
    idempotency_key: string;
    /** Input Type */
    input_type: "text" | "media";
    /** Language */
    language: string;
    /** Media Version Id */
    media_version_id?: string | null;
    /** Rights Declaration */
    rights_declaration: string;
    /** Text */
    text?: string | null;
    /** Title */
    title: string;
  };

  type ScriptDocumentPreviewRequest = {
    /** Media Version Id */
    media_version_id: string;
  };

  type ScriptDocumentPreviewResponse = {
    /** Codepoint Count */
    codepoint_count: number;
    /** Media Version Id */
    media_version_id: string;
    /** Raw Hash */
    raw_hash: string;
    /** Raw Text */
    raw_text: string;
  };

  type ScriptDocumentResponse = {
    /** Created At */
    created_at: string;
    /** Created By */
    created_by: string;
    /** Id */
    id: string;
    /** Language */
    language: string;
    /** Project Id */
    project_id: string;
    /** Revision */
    revision: number;
    /** Rights Declaration */
    rights_declaration: string;
    /** Source Media Version Id */
    source_media_version_id: string | null;
    /** Source Type */
    source_type: "text" | "media";
    /** Status */
    status: "active" | "archived";
    /** Title */
    title: string;
    /** Workspace Id */
    workspace_id: string;
  };

  type ScriptSceneSpan = {
    temporary_span_id: string;
    kind: string;
    codepoint_start: number;
    codepoint_end: number;
    heading: string;
    evidence: SceneAnalysisEvidence;
  };

  type ScriptSourceIdentity = {
    owner_kind: string;
    logical_id: string;
    version_id: string;
    revision: number;
    content_hash: string;
    created_at: string;
  };

  type ScriptSourceSpanResponse = {
    identity: ScriptSourceIdentity;
    span_index_id: string;
    start: number;
    end: number;
    text: string;
    text_hash: string;
    codepoint_index_rule: string;
  };

  type ScriptSpanCandidate = {
    source_version_id: string;
    source_hash: string;
    codepoint_count: number;
    coverage: {
      source_hash: string;
      codepoint_start: number;
      codepoint_end: number;
      covered_codepoints: number;
    };
    spans: ScriptSceneSpan[];
    review_issues: SceneAnalysisReviewIssue[];
  };

  type SearchEnvelope = {
    data: SearchResult;
  };

  type SearchEvidence = {
    document_revision_id: string;
    start: number;
    end: number;
    text_hash: string;
    href: string;
  };

  type SearchHit = {
    score: number;
    snippet: string;
    owner_kind: string;
    owner_logical_id: string;
    owner_version_id: string;
    owner_revision: number;
    owner_content_hash: string;
    story_node_key?: string;
    node_type?: string;
    owner_href: string;
    version_href: string;
    evidence: SearchEvidence[];
  };

  type SearchProjectionSource = {
    kind: "event" | "reindex";
    id: string;
  };

  type SearchResult = {
    kind: "script" | "storygraph";
    status: "fresh" | "stale" | "degraded";
    stale: boolean;
    error_code?: "search_unavailable";
    expected_snapshot_hash: string;
    indexed_snapshot_hash: string;
    index_version: string;
    source?: SearchProjectionSource;
    indexed_at?: string;
    hits: SearchHit[];
  };

  type searchScriptsParams = {
    project_id: string;
    q: string;
    limit: number;
  };

  type searchStoryGraphParams = {
    project_id: string;
    q: string;
    limit: number;
  };

  type selectProjectPresetParams = {
    project_id: string;
  };

  type setCostBudgetParams = {
    project_id: string;
  };

  type setCostPriceQuoteParams = {
    project_id: string;
    profile_version_id: string;
  };

  type startReferenceExecutionWorkflowParams = {
    project_id: string;
    execution_id: string;
  };

  type StoryAnalysisRecoveryEnvelope = {
    data: StoryAnalysisRecoveryResponse;
  };

  type StoryAnalysisRecoveryRequest = {
    node_run_id: string;
    idempotency_key: string;
  };

  type StoryAnalysisRecoveryResponse = {
    receipt_id: string;
    workflow_run_id: string;
    node_run_id: string;
    invocation_id: string;
    stage: "analyze_story" | "reconcile_story";
    shard_key: string;
    status: string;
    failure_code: string;
    previous_claim_version: number;
  };

  type storyboardApplyPreflightParams = {
    batch_id: string;
  };

  type StoryboardApprovedIntentScene = {
    scene_story_node_key: string;
    batch_id: string;
    episode_id: string;
    structure_id: string;
    script_version_id: string;
    candidate_revision_id: string;
    candidate_revision_hash: string;
    asset_readiness: string;
    shot_intents: any;
  };

  type StoryboardApprovedIntentSet = {
    schema_version: string;
    id: string;
    workspace_id: string;
    project_id: string;
    draft_set_id: string;
    draft_set_revision: number;
    candidate_revision_id: string;
    candidate_revision_hash: string;
    candidate_revision: number;
    graph_version_id: string;
    graph_version_no: number;
    graph_content_hash: string;
    manifest_id: string;
    manifest_version: number;
    manifest_hash: string;
    review_decision_id: string;
    scenes: any;
    visual_requirements_hash: string;
    content_hash: string;
  };

  type StoryboardAssetVersionRef = {
    asset_version_id: string;
    revision: number;
    content_hash: string;
    lineage_hash: string;
  };

  type StoryboardCameraIntent = {
    scale: string;
    angle: string;
    movement: string;
    composition: string;
  };

  type StoryboardDraftSetBatch = {
    batch_id: string;
    episode_id: string;
    structure_id: string;
    script_version_id: string;
    scene_story_node_key: string;
    input_hash: string;
    baseline_order_hash: string;
    result_hash: string | null;
    candidate_revision_id: string | null;
    candidate_revision_hash: string | null;
  };

  type StoryboardDraftSetResponse = {
    id: string;
    workspace_id: string;
    project_id: string;
    workflow_run_id: string;
    node_run_id: string;
    graph_version_id: string;
    manifest_id: string;
    graph_version_no: number;
    manifest_version: number;
    revision: number;
    graph_content_hash: string;
    manifest_hash: string;
    input_hash: string;
    result_hash: string | null;
    candidate_revision_hash: string | null;
    candidate_revision_id: string | null;
    status: "queued" | "needs_asset" | "intent_frozen" | "failed" | "unknown" | "cancelled";
    batches: StoryboardDraftSetBatch[];
    created_at: string;
    updated_at: string;
  };

  type StoryboardEvidenceRef = {
    absolute_end: number;
    absolute_start: number;
    document_revision_id: string;
    text_hash: string;
  };

  type storyboardExportPreflightParams = {
    episode_id: string;
  };

  type StoryboardFrameIntent = {
    first: string;
    key: string;
    last: string;
  };

  type StoryboardIntentAcceptanceResponse = {
    set: StoryboardDraftSetResponse;
    approved: StoryboardApprovedIntentSet;
    receipt: { id: string; operation: string; resource_id: string; created_at: string };
  };

  type StoryboardReviewIssue = {
    code: string;
    severity: string;
    summary: string;
    evidence: any;
  };

  type StoryboardShotIntent = {
    shot_key: string;
    intent_order: number;
    source_beat_story_node_keys: any;
    source_evidence: any;
    purpose: string;
    proposed_duration_ms: number;
    camera: StoryboardCameraIntent;
    action_intent: string;
    dialogue_intent: string | null;
    sound_intent: string;
    performance_intent: string;
    continuity_in: string;
    continuity_out: string;
    frame_intent: StoryboardFrameIntent;
    visual_requirements: any;
    risk_codes: any;
    review_issues: any;
  };

  type StoryboardTextIntentAudio = {
    channel: string;
    dialogue_id: string;
    speaker_mention_id?: string;
    text: string;
  };

  type StoryboardTextIntentBlocking = {
    action: string;
    facing: string;
    mention_id: string;
    position: string;
  };

  type StoryboardTextIntentIssue = {
    code: string;
    scope: string;
    severity: string;
    summary: string;
  };

  type StoryboardTextIntentRiskResolution = {
    code: string;
    reason: string;
    scope: string;
  };

  type StoryboardTextIntentShot = {
    action: string;
    asset_readiness: string;
    audio: any;
    camera_movement: string;
    detail_evidence: any;
    duration_max_ms: number;
    duration_min_ms: number;
    entry_state: string;
    exit_state: string;
    framing: string;
    id: string;
    key: string;
    narrative_unit_ids: any;
    panel_caption: string;
    position: number;
    purpose: string;
    screen_direction: string;
    timing_basis: string;
    visual_requirements: any;
  };

  type StoryboardTextIntentVersion = {
    asset_readiness: string;
    audience_knows: any;
    blocking: any;
    content_hash: string;
    created_at: string;
    created_by: string;
    decision_id: string;
    dramatic_intent: string;
    episode_id: string;
    id: string;
    id_mapping: Record<string, any>;
    issues: any;
    project_id: string;
    proposal_id: string;
    revision: number;
    risk_resolutions: any;
    run_id: string;
    scene_id: string;
    shots: any;
    source_hash: string;
    source_revision_id: string;
    structure_id: string;
    withhold: any;
    workspace_id: string;
    world_version_id: string;
  };

  type StoryboardTextIntentVisual = {
    asset_readiness: string;
    entity_id?: string;
    mention_id: string;
  };

  type StoryboardVisualRequirement = {
    occurrence_story_node_key: string;
    identity_story_node_key: string;
    specification_story_node_key: string;
    asset_state_story_node_key: string;
    asset_id: string;
    specification_version_id: string;
    asset_state_id: string;
    asset_role: string;
    required_view_roles: any;
    asset_readiness: string;
    asset_version_ref: StoryboardAssetVersionRef | null;
  };

  type StoryGraphDiffEnvelope = {
    data: StoryGraphDiffResponse;
  };

  type StoryGraphDiffResponse = {
    base_version_id: string;
    base_content_hash: string;
    target_version_id: string;
    target_content_hash: string;
    node_changes: StoryGraphNodeChange[];
    edge_changes: StoryGraphEdgeChange[];
    truncated: boolean;
    next_cursor: any;
    result_hash: string;
  };

  type StoryGraphEdge = {
    edge_key: string;
    edge_type: string;
    from_node_key: string;
    to_node_key: string;
    qualifier: Record<string, any>;
    content_hash: string;
  };

  type StoryGraphEdgeChange = {
    edge_key: string;
    change_type: "added" | "removed" | "changed";
    before_content_hash: any;
    after_content_hash: any;
  };

  type StoryGraphNode = {
    story_node_key: string;
    node_type: string;
    owner_ref: StoryGraphOwnerRef;
    label?: string;
    business_position?: Record<string, any>;
    evidence_refs: Record<string, any>[];
    payload: Record<string, any>;
    content_hash: string;
  };

  type StoryGraphNodeChange = {
    story_node_key: string;
    change_type: "added" | "removed" | "changed";
    before_content_hash: any;
    after_content_hash: any;
  };

  type StoryGraphOwnerRef = {
    owner_kind: string;
    owner_logical_id: string;
    fragment_key?: string;
    owner_version_id: string;
    owner_revision: number;
    content_hash: string;
  };

  type StoryGraphSubgraphEnvelope = {
    data: StoryGraphSubgraphResponse;
  };

  type StoryGraphSubgraphResponse = {
    version_id: string;
    version_no: number;
    content_hash: string;
    lens: "outline" | "narrative" | "entity" | "production" | "impact" | any;
    scope: { kind: "project" | "story_node"; id: string };
    direction: "upstream" | "downstream" | any;
    depth: number;
    nodes: StoryGraphNode[];
    edges: StoryGraphEdge[];
    truncated: boolean;
    next_cursor: any;
    result_hash: string;
  };

  type StoryGraphVersionEnvelope = {
    data: StoryGraphVersionResponse;
  };

  type StoryGraphVersionResponse = {
    id: string;
    workspace_id: string;
    project_id: string;
    version_no: number;
    parent_version_id: any;
    parent_content_hash: any;
    source_revision_id: string;
    source_revision_hash: string;
    owner_set_hash: string;
    schema_version: string;
    topology_hash: string;
    content_hash: string;
    status: string;
    published_at: string;
    created_by: string;
    created_at: string;
    node_count: number;
    edge_count: number;
    compiled_from: StoryGraphOwnerRef[];
    stale: boolean;
  };

  type StructureIdentityCandidateRefResponse = {
    stage_key: string;
    shard_key: string;
    candidate_revision_id: string;
    candidate_revision_hash: string;
    source_invocation_id: string;
    source_result_hash: string;
    skill_release_id: string;
    skill_release_hash: string;
    stage_release_hash: string;
    bundle_content_hash: string;
    agent_image_digest: string;
  };

  type StructureIdentityCoverageResponse = {
    scene_count: number;
    identity_count: number;
    mention_count: number;
    resolved_count: number;
    unresolved_count: number;
    mention_universe_hash: string;
    scope_set_hash: string;
  };

  type StructureIdentityEpisodeRefResponse = {
    temporary_episode_id: string;
    episode_id: string;
    episode_revision: number;
    position: number;
    script_version_id: string;
    script_version: number;
    source_start: number;
    source_end: number;
    content_hash: string;
  };

  type StructureIdentityImpactSummaryResponse = {
    affected_scope_keys: string[];
    preserved_families: string[];
    invalidated_families: string[];
  };

  type StructureIdentityMentionMappingResponse = {
    kind: "character" | "location" | "prop";
    temporary_scene_id: string;
    source_start: number;
    source_end: number;
    text_hash: string;
    exact_anchor: string;
    resolution: "resolved" | "unresolved";
    identity_key: any;
  };

  type StructureIdentityReceiptResponse = {
    id: string;
    checkpoint_key: string;
    collection_family: string;
    version_id: string;
    version_content_hash: string;
    review_decision_id: string;
    covered_scope_keys: string[];
    collection_root_hash: string;
    receipt_content_hash: string;
  };

  type StructureIdentityRepairOptionResponse = {
    issue_key: string;
    code: string;
    severity: "warning" | "blocking";
    scope: string;
    summary: string;
    evidence_refs: HumanGateChangeEvidenceRef[];
    allowed_changes: HumanGateChangeSpec[];
  };

  type StructureIdentityResponse = {
    temporary_identity_key: string;
    identity_key: string;
    kind: "character" | "location" | "prop";
    resolution: "new" | "reuse";
    reuse_identity_key?: any;
    canonical_name: string;
    aliases: string[];
  };

  type StructureIdentityReviewSubjectResponse = {
    schema_version: string;
    gate_key: string;
    input_hash: string;
    evidence_refs: HumanGateChangeEvidenceRef[];
    impact_summary: StructureIdentityImpactSummaryResponse;
    repair_options: StructureIdentityRepairOptionResponse[];
  };

  type StructureIdentitySceneRefResponse = {
    temporary_episode_id: string;
    episode_id: string;
    temporary_span_id: string;
    temporary_scene_id: string;
    scene_owner_logical_id: string;
    scope_key: string;
    source_start: number;
    source_end: number;
    evidence_hash: string;
  };

  type StructureIdentitySetVersionResponse = {
    schema_version: string;
    id: string;
    workspace_id: string;
    project_id: string;
    version: number;
    parent_version_id?: any;
    gate_input_id: string;
    gate_input_hash: string;
    review_decision_id: string;
    project_episode_receipt_id: string;
    document_revision_id: string;
    span_index_id: string;
    candidate_refs: StructureIdentityCandidateRefResponse[];
    episode_refs: StructureIdentityEpisodeRefResponse[];
    scene_refs: StructureIdentitySceneRefResponse[];
    identities: StructureIdentityResponse[];
    mention_mappings: StructureIdentityMentionMappingResponse[];
    coverage: StructureIdentityCoverageResponse;
    content_hash: string;
    created_by: string;
    created_at: string;
  };

  type StructureIdentitySnapshotResponse = {
    version: StructureIdentitySetVersionResponse;
    receipt: StructureIdentityReceiptResponse;
    command_receipt_id: string;
  };

  type syncCreationProposalsParams = {
    run_id: string;
  };

  type TaskErrorResponse = {
    /** Code */
    code: string;
    /** Retryable */
    retryable: boolean;
    /** Summary */
    summary: string;
  };

  type TaskResponse = {
    /** Cancel Status */
    cancel_status: "none" | "requested" | "accepted" | "rejected";
    error: TaskErrorResponse | null;
    /** Id */
    id: string;
    /** Next Action */
    next_action: string | null;
    /** Progress Stage */
    progress_stage: string;
    /** Request Id */
    request_id: string;
    /** Request Type */
    request_type:
      | "extraction_batch"
      | "episode_plan"
      | "production_bible"
      | "adaptation_run"
      | "storyboard_draft_batch"
      | "storyboard_export_job"
      | "generation_request"
      | "media_version"
      | "upload_session"
      | "workspace"
      | "media_location";
    /** Revision */
    revision: number;
    scope: TaskScopeResponse;
    /** Status */
    status:
      | "queued"
      | "running"
      | "waiting_provider"
      | "succeeded"
      | "failed"
      | "cancelled"
      | "unknown";
    /** Task Type */
    task_type: "production_bible" | "storyboard_draft" | "storyboard_export" | "media_probe";
    /** Workspace Id */
    workspace_id: string;
  };

  type TaskScopeResponse = {
    /** Episode Id */
    episode_id: string | null;
    /** Input Hash */
    input_hash: string | null;
    /** Input Version Id */
    input_version_id: string | null;
    /** Render Snapshot Id */
    render_snapshot_id: string | null;
    /** Usage Id */
    usage_id: string | null;
    /** Usage Type */
    usage_type: string | null;
  };

  type traceStoryGraphNodeParams = {
    project_id: string;
    version_ref: string | string;
    story_node_key: string;
    direction: "upstream" | "downstream";
    depth: number;
    limit: number;
    cursor?: string;
  };

  type updateProjectParams = {
    project_id: string;
  };

  type updateWorkspaceParams = {
    workspace_id: string;
  };

  type UploadCapabilityResponse = {
    /** Expires At */
    expires_at: string;
    /** Headers */
    headers: Record<string, any>;
    /** Method */
    method?: string;
    /** Url */
    url: string;
  };

  type UploadCompletionResponse = {
    media_object: MediaObjectResponse;
    probe_task: TaskResponse;
    version: MediaVersionResponse;
  };

  type UploadDeclaration = {
    /** Filename */
    filename: string;
    /** Idempotency Key */
    idempotency_key: string;
    /** Kind */
    kind: "image" | "video" | "audio" | "subtitle" | "delivery" | "document";
    /** Mime Type */
    mime_type: string;
    /** Sha256 */
    sha256: string;
    /** Size Bytes */
    size_bytes: number;
    /** Workspace Id */
    workspace_id: string;
  };

  type UploadInitializationResponse = {
    upload: UploadCapabilityResponse;
    upload_session: UploadSessionResponse;
  };

  type UploadSessionResponse = {
    /** Expires At */
    expires_at: string;
    /** Filename */
    filename: string;
    /** Id */
    id: string;
    /** Kind */
    kind: "image" | "video" | "audio" | "subtitle" | "delivery" | "document";
    /** Media Object Id */
    media_object_id: string | null;
    /** Mime Type */
    mime_type: string;
    /** Sha256 */
    sha256: string;
    /** Size Bytes */
    size_bytes: number;
    /** Status */
    status: "pending" | "completed" | "expired" | "failed";
    /** Workspace Id */
    workspace_id: string;
  };

  type UserResponse = {
    /** Avatar Url */
    avatar_url: string | null;
    /** Display Name */
    display_name: string;
    /** Email */
    email: string;
    /** Id */
    id: string;
  };

  type WorkflowControlEnvelope = {
    data: WorkflowRunViewResponse & { control: WorkflowControlIntentResponse };
  };

  type WorkflowControlIntentResponse = {
    id: string;
    workflow_run_id: string;
    action: "pause" | "resume" | "cancel";
    expected_run_revision: number;
    status: "pending" | "completed" | "unknown" | "conflict";
    attempt_no: number;
    revision: number;
    created_at: string;
    updated_at: string;
  };

  type WorkflowControlRequest = {
    action: "pause" | "resume" | "cancel";
    expected_revision: number;
    idempotency_key: string;
  };

  type WorkflowNodeRunResponse = {
    id: string;
    workspace_id: string;
    workflow_run_id: string;
    node_id: string;
    definition_key: string;
    definition_version: string;
    executor: string;
    risk_level: string;
    status:
      | "QUEUED"
      | "RUNNING"
      | "WAITING_HUMAN"
      | "RETRYING"
      | "SKIPPED"
      | "SUCCEEDED"
      | "FAILED"
      | "CANCELLED";
    attempt: number;
    reused_from_node_run_id: any;
    input_hash: string;
    cache_key: string;
    output_hash: string;
    revision: number;
    created_at: string;
    updated_at: string;
  };

  type WorkflowRerunRequest = {
    root_node_id: string;
    idempotency_key: string;
  };

  type WorkflowRunEnvelope = {
    data: WorkflowRunViewResponse;
  };

  type WorkflowRunResponse = {
    id: string;
    workspace_id: string;
    project_id: string;
    authoring_revision_id: string;
    definition_version_id: string;
    run_input_snapshot_id: string;
    temporal_workflow_id: string;
    start_input_hash: string;
    source_workflow_run_id: any;
    rerun_root_node_id: any;
    repair_decision_id: any;
    repair_decision_hash: any;
    status:
      | "RUNNING"
      | "RETRYING"
      | "WAITING_HUMAN"
      | "PAUSED"
      | "NEEDS_ATTENTION"
      | "SUCCEEDED"
      | "FAILED"
      | "CANCELLED";
    progress_stage: string;
    next_action: any;
    error: any;
    paused_from_status: any;
    paused_from_progress_stage: any;
    revision: number;
    created_by: string;
    created_at: string;
    updated_at: string;
  };

  type WorkflowRunViewResponse = {
    run: WorkflowRunResponse;
    nodes: WorkflowNodeRunResponse[];
  };

  type WorkflowStartRequest = {
    authoring_revision_id: string;
    idempotency_key: string;
  };

  type WorkspaceCreateRequest = {
    /** Name */
    name: string;
  };

  type WorkspaceResponse = {
    /** Id */
    id: string;
    /** Name */
    name: string;
    /** Revision */
    revision: number;
    /** Role */
    role: "owner" | "editor" | "viewer";
    /** Status */
    status: "active" | "archived";
  };

  type WorkspaceStateRequest = {
    /** Expected Revision */
    expected_revision: number;
  };

  type WorkspaceUpdateRequest = {
    /** Expected Revision */
    expected_revision: number;
    /** Name */
    name: string;
  };
}
