declare namespace API {
  type activateModelReleaseParams = {
    /** 发布ID */
    id: string;
  };

  type ActivateReleaseInput = {
    enable_provider?: boolean;
    expected_active_release_id?: string;
    expected_default_revision?: number;
    expected_model_revision?: number;
    expected_revision?: number;
    request_id?: string;
    set_default?: boolean;
  };

  type Actor = {
    credential_revision: number;
    display_name: string;
    id: string;
    login_name: string;
    must_change_password: boolean;
    platform_role?: "creator" | "platform_admin";
    role: "creator" | "admin";
  };

  type AdminCreateRequest = {
    display_name?: string;
    initial_password?: string;
    login_name?: string;
    role?: "creator" | "admin";
  };

  type AdminModelDetail = {
    model?: CreatedModel;
    prices?: AdminPrice[];
    versions?: AdminModelVersion[];
  };

  type AdminModelPage = {
    items?: CreatedModel[];
    next_cursor?: string;
  };

  type AdminModelVersion = {
    create_time?: string;
    expected_max_ms?: number;
    id?: string;
    limits?: Record<string, any>;
    moderation?: string;
    modes?: string[];
    param_schema?: Record<string, any>[];
    provider_model_id?: string;
    queue?: string;
    supports_callback?: boolean;
    supports_cancel?: boolean;
    supports_query?: boolean;
    version_no?: number;
  };

  type AdminMutationReceipt = {
    display_name?: string;
    id?: string;
    login_unlocked?: boolean;
    must_change_password?: boolean;
    revision?: number;
    role?: "creator" | "admin";
    status?: "active" | "disabled";
  };

  type AdminPasswordRequest = {
    expected_revision?: number;
    new_password?: string;
  };

  type AdminPrice = {
    create_time?: string;
    currency?: string;
    effective_from?: string;
    fx_rate_to_cny?: string;
    id?: string;
    rule?: Record<string, any>;
    unit?: string;
    version_no?: number;
  };

  type AdminRevisionRequest = {
    expected_revision?: number;
  };

  type AdminUpdateRequest = {
    display_name?: string;
    expected_revision?: number;
    role?: "creator" | "admin";
  };

  type AdminUser = {
    create_time?: string;
    display_name?: string;
    id?: string;
    last_login_at?: string;
    login_locked?: boolean;
    login_name?: string;
    revision?: number;
    role?: "creator" | "admin";
    status?: "active" | "disabled";
  };

  type AdminUserPage = {
    items?: AdminUser[];
    next_cursor?: string;
  };

  type adoptBibleResultParams = {
    /** Project UUID */
    pid: string;
    /** Identity kind */
    kind: "character" | "location" | "prop";
    /** Identity UUID */
    id: string;
  };

  type AdoptReceipt = {
    changed?: boolean;
    duplicate?: boolean;
    episode_mappings?: EpisodeMapping[];
    previous_version_id?: string;
    project_revision?: number;
    script_revision?: number;
    split_set_id?: string;
    version_id?: string;
  };

  type adoptScriptVersionParams = {
    /** 项目UUID */
    pid: string;
    /** 不可变剧本版本UUID */
    vid: string;
  };

  type AdoptVersionRequest = {
    ack_invalidate?: boolean;
    expected_revision?: number;
    expected_split_revision?: number;
  };

  type applyCanvasCommandsParams = {
    /** 画布UUID */
    id: string;
  };

  type ApplyConfigInput = {
    preview_hash?: string;
    request_id?: string;
    resolution?: ProviderConfig;
    upgrade?: ConfigUpgradeInput;
  };

  type applyProviderPresetParams = {
    /** 连接ID */
    id: string;
  };

  type archiveProjectParams = {
    /** 项目UUID */
    pid: string;
  };

  type AssetPage = {
    items?: AssetSummary[];
    next_cursor?: string;
  };

  type AssetSummary = {
    byte_size?: number;
    duration_ms?: number;
    file_name?: string;
    height?: number;
    id?: string;
    kind?: string;
    mime_type?: string;
    project_id?: string;
    revision?: number;
    width?: number;
  };

  type AvailabilityView = {
    available: boolean;
  };

  type BatchDetail = {
    cancel_requested_at?: string;
    failed_count?: number;
    id?: string;
    items?: TaskSummary[];
    kind?: string;
    paused_reason?: string;
    project_id?: string;
    quote_total_micros?: number;
    status?: BatchStatus;
    succeeded_count?: number;
    total_count?: number;
    unknown_count?: number;
  };

  type BatchFreeQuoteItemResult = {
    error_code?: string;
    final_prompt?: string;
    mode?: string;
    model_key?: string;
    operation_id?: string;
    prompt_preparation?: PromptPreparation;
    quote_detail?: number[];
    quote_micros?: number;
    region?: string;
    reused_from_id?: string;
  };

  type BatchReferenceColumn = {
    id?: string;
    label?: string;
  };

  type BatchStatus =
    | "quoted"
    | "confirmed"
    | "running"
    | "finished"
    | "cancelled"
    | "expired";

  type BatchTableConfig = {
    concurrency?: number;
    global_prompt?: string;
    mode?: string;
    model_profile_id?: string;
    operation?: string;
    output_count?: number;
    params?: Record<string, any>;
    reference_columns?: BatchReferenceColumn[];
    rows?: BatchTableRow[];
    version?: number;
  };

  type BatchTableRow = {
    enabled?: boolean;
    id?: string;
    input_node_ids?: string[];
    prompt?: string;
  };

  type bindBibleVoiceParams = {
    /** Project UUID */
    pid: string;
    /** Identity kind */
    kind: "character" | "location" | "prop";
    /** Identity UUID */
    id: string;
  };

  type cancelMediaDepthParams = {
    /** Depth UUID */
    job_id: string;
  };

  type cancelMediaExportParams = {
    /** Export UUID */
    job_id: string;
  };

  type cancelMediaLibraryPackageParams = {
    /** 批次UUID */
    job_id: string;
  };

  type cancelMediaPurgeParams = {
    /** 清理UUID */
    job_id: string;
  };

  type cancelMediaTranscriptionParams = {
    /** Transcription UUID */
    job_id: string;
  };

  type cancelMediaTransferParams = {
    /** 转移UUID */
    job_id: string;
  };

  type cancelOperationBatchParams = {
    /** Batch UUID */
    id: string;
  };

  type cancelOperationParams = {
    /** Operation UUID */
    id: string;
  };

  type cancelProjectCopyParams = {
    /** 复制任务UUID */
    id: string;
  };

  type cancelScriptFileImportParams = {
    /** Project UUID */
    pid: string;
    /** Import UUID */
    id: string;
  };

  type cancelScriptSourceWriteParams = {
    /** 项目UUID */
    pid: string;
    /** 保存意图UUID */
    wid: string;
  };

  type CanvasCreateInput = {
    name?: string;
    scope?: Record<string, any>;
  };

  type CanvasDocument = {
    edges?: Edge[];
    id?: string;
    name?: string;
    nodes?: Node[];
    project_id?: string;
    revision?: number;
    scope?: Record<string, any>;
    viewport?: Viewport;
  };

  type CanvasDraft = {
    aspect_ratio?: string;
    candidate_count?: number;
    composer?: PromptDraft;
    negative_prompt?: string;
    prompt?: string;
  };

  type CanvasListResponse = {
    items?: CanvasDocument[];
    next_cursor?: string;
  };

  type CanvasSource = {
    canvas_id?: string;
    node_id?: string;
    revision?: number;
    row_id?: string;
  };

  type CapabilityDefault = {
    capability?: string;
    release_id?: string;
    revision?: number;
  };

  type CapabilitySummary = {
    id?: string;
    input_roles?: string[];
    key?: string;
    modes?: string[];
    output_type?: string;
  };

  type CatalogVoice = {
    model_key?: string;
    model_version?: number;
    model_version_id?: string;
    param_schema_sha256?: string;
    params?: VoiceParams;
    voice_key?: string;
  };

  type ChangedModelStatus = {
    id?: string;
    revision?: number;
    status?: ModelStatus;
    update_time?: string;
  };

  type CharacterContent = {
    aliases?: string[];
    definition?: CharacterDefinition;
    description?: string;
    looks?: LookContent[];
    name?: string;
    voice?: VoiceContent;
  };

  type CharacterDefinition = {
    appearance?: string;
    clothing?: string;
    consistency_prompt?: string;
    multi_view_prompt?: string;
    personality?: string;
    physique?: string;
    props?: string;
    role?: string;
    voice_age?: string;
    voice_language?: string;
    voice_timbre?: string;
  };

  type CharacterInput = {
    aliases?: string[];
    definition?: CharacterDefinition;
    description?: string;
    name?: string;
  };

  type Command = {
    config?: NodeConfig;
    edges?: Edge[];
    id?: string;
    ids?: string[];
    moves?: Move[];
    names?: Name[];
    nodes?: Node[];
    parents?: Parent[];
    sizes?: Size[];
    type?: string;
    viewport?: Viewport;
    z_indices?: ZIndex[];
  };

  type CommandResult = {
    ok?: boolean;
  };

  type CommandsInput = {
    commands?: Command[];
    expected_revision?: number;
  };

  type ConfigReceipt = {
    action?: string;
    request_id?: string;
    result?: Record<string, any>;
    target_id?: string;
  };

  type ConfigUpgradeInput = {
    expected_config_id?: string;
    expected_revision?: number;
    preset_version_id?: string;
  };

  type ConfigUpgradePreview = {
    conflicts?: string[];
    current?: ConnectionConfig;
    hash?: string;
    proposed?: ProviderConfig;
    revision?: number;
  };

  type ConfirmationPage = {
    items?: ConfirmationSummary[];
    next_revision?: number;
  };

  type ConfirmationSummary = {
    candidate_set_id?: string;
    created_at?: string;
    episode_count?: number;
    formal_set_id?: string;
    id?: string;
    revision?: number;
  };

  type ConfirmBatchItem = {
    operation_id?: string;
    reasons?: string[];
    status?: OperationStatus;
  };

  type confirmBatchQuoteParams = {
    /** Batch UUID */
    id: string;
  };

  type ConfirmBatchRequest = {
    exclude_operation_ids?: string[];
    project_id?: string;
  };

  type ConfirmBatchResponse = {
    available_micros?: number;
    budget_revision?: number;
    confirmed_count?: number;
    items?: ConfirmBatchItem[];
    quote_total_micros?: number;
  };

  type confirmBibleEntryParams = {
    /** Project UUID */
    pid: string;
    /** Identity kind */
    kind: "character" | "location" | "prop";
    /** Identity UUID */
    id: string;
  };

  type confirmOperationQuoteParams = {
    /** Operation UUID */
    id: string;
  };

  type ConfirmResponse = {
    available_micros?: number;
    budget_revision?: number;
    reservation_id?: string;
  };

  type confirmScriptEpisodeSplitParams = {
    /** 项目UUID */
    pid: string;
  };

  type confirmScriptEpisodeStructureParams = {
    /** 正式分集UUID */
    eid: string;
  };

  type ConnectionConfig = {
    config?: ProviderConfig;
    id?: string;
    preset_version_id?: string;
    provider_id?: string;
    version?: number;
  };

  type ConnectPresetInput = {
    key?: string;
    name?: string;
    request_id?: string;
    version_id?: string;
  };

  type connectProviderPresetParams = {
    /** 预设ID */
    id: string;
  };

  type ContentRequest = {
    character?: CharacterInput;
    expected_revision?: number;
    location?: LocationContent;
    prop?: PropContent;
  };

  type ControlInput = {
    project_id?: string;
    revision?: number;
  };

  type ControlRequest = {
    acknowledged_impact?: ImpactProof;
    expected_revision?: number;
  };

  type CreateBatchFreeQuoteResult = {
    available_micros?: number;
    batch_id?: string;
    confirmable?: boolean;
    expires_at?: string;
    items?: BatchFreeQuoteItemResult[];
    total_micros?: number;
  };

  type createBibleEntryFromResultParams = {
    /** Project UUID */
    pid: string;
    /** Identity kind */
    kind: "character" | "location" | "prop";
  };

  type createBibleEntryParams = {
    /** Project UUID */
    pid: string;
    /** Identity kind */
    kind: "character" | "location" | "prop";
  };

  type createBibleLookParams = {
    /** Project UUID */
    pid: string;
    /** Identity kind */
    kind: "character" | "location" | "prop";
    /** Identity UUID */
    id: string;
  };

  type createCanvasParams = {
    /** 项目UUID */
    pid: string;
  };

  type CreatedModel = {
    capability?: string;
    create_time?: string;
    current_version_id?: string;
    display_name?: string;
    id?: string;
    model_key?: string;
    provider_id?: string;
    revision?: number;
    status?: ModelStatus;
    update_time?: string;
  };

  type CreatedProject = {
    allow_overseas_models?: boolean;
    aspect_ratio?: string;
    canvas_id: string;
    cover_asset_id?: string;
    create_time?: string;
    description?: string;
    id?: string;
    name?: string;
    org_id?: string;
    resolution?: string;
    revision?: number;
    status?: string;
    style_key?: string;
    style_preset_id?: string;
    style_subtype?: string;
    style_type?: string;
    update_time?: string;
  };

  type CreatedProvider = {
    adapter_key?: string;
    concurrency_limit?: number;
    create_time?: string;
    id?: string;
    key?: string;
    name?: string;
    rate_limit_per_min?: number;
    region?: Region;
    revision?: number;
    status?: ProviderStatus;
    update_time?: string;
  };

  type createFreeQuoteParams = {
    /** 项目UUID */
    pid: string;
  };

  type createGenerationQuotesParams = {
    /** 项目UUID */
    pid: string;
  };

  type createMediaDepthParams = {
    /** Project UUID */
    pid: string;
  };

  type createMediaExportParams = {
    /** Project UUID */
    pid: string;
  };

  type createMediaTranscriptionParams = {
    /** Project UUID */
    pid: string;
  };

  type createModelReleaseParams = {
    /** 模型ID */
    id: string;
  };

  type CreateModelRequest = {
    capability?: string;
    display_name?: string;
    model_key?: string;
    provider_id?: string;
  };

  type createProjectCopyParams = {
    /** 源项目UUID */
    pid: string;
  };

  type CreateProjectRequest = {
    aspect_ratio: "9:16" | "16:9" | "1:1";
    description?: string;
    name: string;
    style_key?: string;
    style_preset_id?: string;
    style_subtype?: "anime_jp" | "guofeng_xianxia" | "cartoon_3d" | "manhwa";
    style_type?: "realistic" | "stylized";
  };

  type createPromptDraftParams = {
    /** 项目 */
    pid: string;
  };

  type CreateProviderRequest = {
    adapter_key?: string;
    concurrency_limit?: number;
    key?: string;
    name?: string;
    rate_limit_per_min?: number;
    region?: Region;
  };

  type CreateReleaseInput = {
    config_version_id?: string;
    model_version_id?: string;
    price_version_id?: string;
    request_id?: string;
    upstream_cost?: UpstreamCost;
  };

  type createScriptFileImportParams = {
    /** Project UUID */
    pid: string;
  };

  type createScriptSourceParams = {
    /** 项目UUID */
    pid: string;
  };

  type CredentialField = {
    name?: string;
    required?: boolean;
    type?: string;
  };

  type CredentialStatus = "active" | "draining" | "disabled";

  type CredentialTestAccepted = {
    accepted?: boolean;
    check_id?: string;
    credential_id?: string;
    event_id?: string;
    provider_id?: string;
    test_id?: string;
  };

  type currentPromptSubmissionParams = {
    /** 项目 */
    pid: string;
    /** prompt_draft 或 canvas_input */
    owner_kind: string;
    /** 草稿 */
    draft_id?: string;
    /** 画布 */
    canvas_id?: string;
    /** 节点 */
    node_id?: string;
  };

  type Customization = {
    base_template_id?: string;
    content?: string;
    id?: string;
    mode?: Mode;
    operation?: string;
    revision?: number;
    update_time?: string;
  };

  type DefaultPage = {
    items?: CapabilityDefault[];
  };

  type Definition = {
    category?: string;
    content?: string;
    description?: string;
    label?: string;
    operation?: string;
    output_contract?: string;
    output_type?: string;
    schema_key?: string;
    template_id?: string;
    template_version?: number;
    variables?: Variable[];
  };

  type deleteBibleEntryParams = {
    /** Project UUID */
    pid: string;
    /** Identity kind */
    kind: "character" | "location" | "prop";
    /** Identity UUID */
    id: string;
  };

  type deleteBibleLookParams = {
    /** Project UUID */
    pid: string;
    /** Identity kind */
    kind: "character" | "location" | "prop";
    /** Identity UUID */
    id: string;
    /** Appearance UUID */
    look: string;
  };

  type deleteCanvasParams = {
    /** 画布UUID */
    id: string;
  };

  type DeleteInput = {
    expected_revision?: number;
  };

  type deleteProjectParams = {
    /** 项目UUID */
    pid: string;
  };

  type DeleteResult = {
    deleted?: boolean;
    id?: string;
    revision?: number;
  };

  type deleteScriptSourceParams = {
    /** 项目UUID */
    pid: string;
    /** 稳定来源UUID */
    lineage: string;
  };

  type deleteSessionParams = {
    /** Current session UUID */
    session_id: string;
  };

  type DepthCreateInput = {
    canvas_id?: string;
    node_id?: string;
    revision?: number;
  };

  type DepthJob = {
    asset_id?: string;
    attempt?: number;
    cancellation_requested?: boolean;
    created_at?: string;
    execution_unconfirmed?: boolean;
    failure_code?: string;
    id?: string;
    needs_reconciliation?: boolean;
    profile_id?: string;
    project_id?: string;
    reconciliation_requested?: boolean;
    retryable?: boolean;
    revision?: number;
    sha256?: string;
    source?: Source;
    source_asset_id?: string;
    source_asset_revision?: number;
    source_sha256?: string;
    stage?: string;
    status?: DepthStatus;
    updated_at?: string;
  };

  type DepthJobResponse = {
    asset_id?: string;
    attempt?: number;
    cancellation_requested?: boolean;
    created_at?: string;
    execution_unconfirmed?: boolean;
    failure_code?: string;
    id?: string;
    needs_reconciliation?: boolean;
    profile_id?: string;
    project_id?: string;
    reconciliation_requested?: boolean;
    retryable?: boolean;
    revision?: number;
    sha256?: string;
    source?: Source;
    source_asset_id?: string;
    source_asset_revision?: number;
    source_sha256?: string;
    stage?: string;
    status?: DepthStatus;
    updated_at?: string;
  };

  type DepthPage = {
    current_actor_id?: string;
    current_org_id?: string;
    items?: DepthJob[];
    next_cursor?: string;
  };

  type DepthPreview = {
    asset?: AssetSummary;
    expires_at?: string;
    job_id?: string;
    revision?: number;
    sha256?: string;
    url?: string;
  };

  type DepthStatus =
    | "queued"
    | "running"
    | "review_required"
    | "succeeded"
    | "failed"
    | "cancel_requested"
    | "cancelled";

  type Detail = {
    current?: Version;
    head?: Head;
    resolved_id?: string;
  };

  type DirectorBoneFrame = {
    easing?: string;
    id?: string;
    rotation?: number[];
    time?: number;
  };

  type DirectorBoneTrack = {
    bone?: string;
    keyframes?: DirectorBoneFrame[];
  };

  type DirectorCamera = {
    aperture?: number;
    far?: number;
    focal_length?: number;
    focus_distance?: number;
    follow_anchor?: number[];
    follow_object_id?: string;
    fov?: number;
    id?: string;
    keyframes?: DirectorKeyframe[];
    look_at_mode?: string;
    look_at_object_id?: string;
    name?: string;
    near?: number;
    target?: number[];
    transform?: DirectorTransform;
  };

  type DirectorConfig = {
    active_shot_id?: string;
    aspect_ratio?: string;
    background?: string;
    cameras?: DirectorCamera[];
    cover?: DirectorCover;
    environment_intensity?: number;
    grid_snap?: boolean;
    grid_visible?: boolean;
    ground?: DirectorGround;
    id?: string;
    labels_visible?: boolean;
    lights?: DirectorLight[];
    objects?: DirectorObject[];
    panorama?: DirectorPanorama;
    panorama_radius?: number;
    panorama_rotation?: number;
    shots?: DirectorShot[];
    stage_transform?: DirectorStageTransform;
    title?: string;
    version?: number;
  };

  type DirectorCover = {
    asset_id?: string;
    shot_id?: string;
  };

  type DirectorGround = {
    height?: number;
    opacity?: number;
    visible?: boolean;
  };

  type DirectorKeyframe = {
    easing?: string;
    id?: string;
    time?: number;
    transform?: DirectorTransform;
  };

  type DirectorLight = {
    angle?: number;
    cast_shadow?: boolean;
    color?: string;
    id?: string;
    intensity?: number;
    name?: string;
    penumbra?: number;
    transform?: DirectorTransform;
    type?: string;
  };

  type DirectorMotionClip = {
    duration?: number;
    id?: string;
    loop?: boolean;
    name?: string;
    playback_rate?: number;
    source_animation?: string;
    start?: number;
  };

  type DirectorObject = {
    active_motion_clip_id?: string;
    asset_id?: string;
    bone_overrides?: Record<string, any>;
    bone_tracks?: DirectorBoneTrack[];
    builtin_actor?: string;
    cast_shadow?: boolean;
    color?: string;
    id?: string;
    keyframes?: DirectorKeyframe[];
    kind?: string;
    motion_clips?: DirectorMotionClip[];
    name?: string;
    pose?: string;
    primitive?: string;
    receive_shadow?: boolean;
    rig?: DirectorRig;
    source_node_id?: string;
    transform?: DirectorTransform;
    uniform_scale?: number;
    visible?: boolean;
  };

  type DirectorPanorama = {
    asset_id?: string;
    name?: string;
    rotation?: number;
  };

  type DirectorRig = {
    animation_names?: string[];
    bone_map?: Record<string, any>;
  };

  type DirectorScreenshot = {
    asset_id?: string;
    created_at?: string;
    id?: string;
    name?: string;
  };

  type DirectorShot = {
    camera_id?: string;
    camera_move?: string;
    duration?: number;
    fps?: number;
    id?: string;
    name?: string;
    prompt?: string;
    screenshots?: DirectorScreenshot[];
    shot_size?: string;
  };

  type DirectorStageTransform = {
    position?: number[];
    rotation?: number[];
    scale?: number;
  };

  type DirectorTransform = {
    position?: number[];
    rotation?: number[];
    scale?: number[];
  };

  type disableAdminCredentialParams = {
    /** 渠道UUID */
    id: string;
    /** 凭据UUID */
    credential_id: string;
  };

  type disableAdminUserParams = {
    /** User UUID */
    id: string;
  };

  type downloadDocumentAssetParams = {
    /** 项目UUID */
    pid: string;
    /** 文档素材UUID */
    asset_id: string;
  };

  type downloadLibraryMediaAssetParams = {
    /** 二进制素材条目UUID */
    item_id: string;
    /** personal/project；默认personal */
    scope?: string;
    /** project scope必要UUID */
    project_id?: string;
  };

  type downloadMediaDepthParams = {
    /** Depth UUID */
    job_id: string;
    /** Project UUID */
    project_id: string;
  };

  type downloadMediaExportParams = {
    /** Export UUID */
    job_id: string;
    /** Project UUID */
    project_id: string;
  };

  type downloadMediaExportSubtitlesParams = {
    /** Export UUID */
    job_id: string;
    /** Project UUID */
    project_id: string;
  };

  type downloadMediaTranscriptionSubtitlesParams = {
    /** Transcription UUID */
    job_id: string;
    /** Project UUID */
    project_id: string;
  };

  type DraftPage = {
    items?: DraftSnapshot[];
    next_cursor?: string;
  };

  type DraftRequest = {
    draft?: PromptDraft;
    expected_revision?: number;
  };

  type DraftSnapshot = {
    draft?: PromptDraft;
    draft_hash?: string;
    draft_id?: string;
    input_hash?: string;
    revision?: number;
    updated_at?: string;
  };

  type Edge = {
    binding?: Record<string, any>;
    edge_type?: string;
    id?: string;
    role?: string;
    source_node_id?: string;
    target_node_id?: string;
  };

  type EditorQuote = {
    available_micros?: number;
    confirmable?: boolean;
    expires_at?: string;
    final_prompt?: string;
    quote_micros?: number;
  };

  type enableAdminUserParams = {
    /** User UUID */
    id: string;
  };

  type Episode = {
    confirmed_structure_id?: string;
    current_structure_id?: string;
    id?: string;
    inherit_status?: string;
    is_delete?: boolean;
    org_id?: string;
    previous_episode_id?: string;
    project_id?: string;
    revision?: number;
    script_version_id?: string;
    seq_no?: number;
    span_end?: number;
    span_start?: number;
    split_set_id?: string;
    title?: string;
  };

  type EpisodeBoundary = {
    seq_no?: number;
    source_lineage_id?: string;
    span_end?: number;
    span_start?: number;
    title?: string;
  };

  type EpisodeMapping = {
    episode_id?: string;
    inherit_status?: string;
    previous_episode_id?: string;
  };

  type EpisodeStructure = {
    actor_id?: string;
    created_at?: string;
    document?: StructureDocument;
    episode_id?: string;
    id?: string;
    org_id?: string;
    project_id?: string;
    source_hash?: string;
    version_no?: number;
  };

  type EpisodeText = {
    content_hash?: string;
    episode_id?: string;
    script_version_id?: string;
    span_end?: number;
    span_start?: number;
    text?: string;
  };

  type EpisodeView = {
    candidate?: SplitSet;
    episodes?: Episode[];
    head?: VersionHead;
    version_id?: string;
  };

  type ExportJob = {
    asset_id?: string;
    attempt?: number;
    created_at?: string;
    failure_code?: string;
    id?: string;
    output_kind?: "video" | "audio";
    progress?: number;
    project_id?: string;
    revision?: number;
    sha256?: string;
    source?: Source;
    stage?: string;
    status?: MediatoolStatus;
    updated_at?: string;
  };

  type ExportJobResponse = {
    asset_id?: string;
    attempt?: number;
    created_at?: string;
    failure_code?: string;
    id?: string;
    output_kind?: "video" | "audio";
    progress?: number;
    project_id?: string;
    revision?: number;
    sha256?: string;
    source?: Source;
    stage?: string;
    status?: MediatoolStatus;
    updated_at?: string;
  };

  type exportMediaLibraryPackageParams = {
    /** personal/project */
    scope?: string;
    /** 项目UUID */
    project_id?: string;
  };

  type ExportPage = {
    items?: ExportJob[];
    next_cursor?: string;
  };

  type ExportPreview = {
    asset?: AssetSummary;
    expires_at?: string;
    job_id?: string;
    revision?: number;
    sha256?: string;
    url?: string;
    waveform?: ExportWaveformPreview;
  };

  type exportProviderPresetParams = {
    /** 预设ID */
    id: string;
  };

  type ExportWaveformPreview = {
    expires_at?: string;
    height?: number;
    url?: string;
    width?: number;
  };

  type ExtractionWarning = {
    code?: string;
    count?: number;
    paragraph?: number;
  };

  type FileImportControlRequest = {
    expected_revision?: number;
  };

  type FileImportRequest = {
    asset_ids?: string[];
    base_version_id?: string;
    expected_revision?: number;
    rights_confirmed?: boolean;
  };

  type FolderCover = {
    asset_id?: string;
    project_id?: string;
  };

  type GenerationConfig = {
    capability?: string;
    inputs?: GenerationReference[];
    mode?: string;
    model_profile_id?: string;
    output_count?: number;
    params?: Record<string, any>;
    prompt?: string;
    version?: number;
  };

  type GenerationReference = {
    media_asset_id?: string;
    role?: string;
  };

  type getAdminModelParams = {
    /** 模型UUID */
    id: string;
  };

  type getAdminProviderParams = {
    /** 渠道UUID */
    id: string;
  };

  type getBibleEntryParams = {
    /** Project UUID */
    pid: string;
    /** Identity kind */
    kind: "character" | "location" | "prop";
    /** Identity UUID */
    id: string;
  };

  type getBibleVersionParams = {
    /** Project UUID */
    pid: string;
    /** Identity kind */
    kind: "character" | "location" | "prop";
    /** Identity UUID */
    id: string;
    /** Version UUID */
    version: string;
  };

  type getCanvasParams = {
    /** 画布UUID */
    id: string;
  };

  type getConfigReceiptParams = {
    /** 请求ID */
    id: string;
    /** 动作 */
    action: string;
    /** 目标ID，创建为零UUID */
    target_id: string;
  };

  type getLoginAvailabilityParams = {
    /** ASCII login name */
    login_name: string;
  };

  type getMediaDepthParams = {
    /** Depth UUID */
    job_id: string;
    /** Project UUID */
    project_id: string;
  };

  type getMediaExportParams = {
    /** Export UUID */
    job_id: string;
    /** Project UUID */
    project_id: string;
  };

  type getMediaLibraryItemParams = {
    /** 条目UUID */
    item_id: string;
    /** personal/project */
    scope?: string;
    /** project scope必要UUID */
    project_id?: string;
  };

  type getMediaLibraryPackageParams = {
    /** 导入批次UUID */
    job_id: string;
  };

  type getMediaLibraryStorageUsageParams = {
    /** personal/project；默认personal */
    scope?: string;
    /** project scope必要UUID */
    project_id?: string;
  };

  type getMediaPreviewParams = {
    /** 项目UUID */
    pid: string;
    /** 媒体UUID */
    asset_id: string;
  };

  type getMediaPurgeParams = {
    /** 清理UUID */
    job_id: string;
  };

  type getMediaTranscriptionParams = {
    /** Transcription UUID */
    job_id: string;
    /** Project UUID */
    project_id: string;
  };

  type getMediaTranscriptionResultParams = {
    /** Transcription UUID */
    job_id: string;
    /** Project UUID */
    project_id: string;
  };

  type getMediaTransferParams = {
    /** 转移UUID */
    job_id: string;
  };

  type getOperationBatchParams = {
    /** Batch UUID */
    id: string;
    /** 项目UUID */
    project_id: string;
  };

  type getOperationParams = {
    /** Operation UUID */
    id: string;
    /** 项目UUID */
    project_id: string;
  };

  type getProjectCopyParams = {
    /** 复制任务UUID */
    id: string;
  };

  type getProjectModelDefaultsParams = {
    /** 项目UUID */
    pid: string;
  };

  type getProjectParams = {
    /** 项目UUID */
    pid: string;
  };

  type getProviderCheckParams = {
    /** 检查ID */
    id: string;
  };

  type getProviderConfigParams = {
    /** 连接ID */
    id: string;
  };

  type getScriptEpisodeSourceTextParams = {
    /** 正式分集UUID */
    eid: string;
    /** 包含起点，Unicode scalar */
    from: number;
    /** 不包含终点，最多65536scalar */
    to: number;
  };

  type getScriptEpisodeStructureParams = {
    /** 正式分集UUID */
    eid: string;
  };

  type getScriptEpisodeStructureVersionParams = {
    /** 正式分集UUID */
    eid: string;
    /** 结构版本号 */
    version: number;
  };

  type getScriptFileImportParams = {
    /** Project UUID */
    pid: string;
    /** Import UUID */
    id: string;
  };

  type getScriptSourceParams = {
    /** 项目UUID */
    pid: string;
    /** 稳定来源lineageUUID */
    lineage: string;
    /** 历史版本UUID */
    version_id?: string;
  };

  type getScriptSourceSnapshotParams = {
    /** 项目UUID */
    pid: string;
    /** 来源快照UUID */
    sid: string;
  };

  type getScriptSourceWriteParams = {
    /** 项目UUID */
    pid: string;
    /** 保存意图UUID */
    wid: string;
  };

  type getScriptSplitConfirmationParams = {
    /** 项目UUID */
    pid: string;
    /** 不可变剧本版本UUID */
    vid: string;
    /** 确认UUID */
    cid: string;
  };

  type getScriptVersionSourceTextParams = {
    /** 项目UUID */
    pid: string;
    /** 不可变剧本版本UUID */
    vid: string;
    /** 包含起点，Unicode scalar */
    from: number;
    /** 不含终点，最多65536scalar */
    to: number;
  };

  type getScriptWorkspaceParams = {
    /** 项目UUID */
    pid: string;
  };

  type Head = {
    confirmed_version_id?: string;
    created_at?: string;
    current_version_id?: string;
    deleted?: boolean;
    id?: string;
    kind?: Kind;
    org_id?: string;
    project_id?: string;
    redirect_id?: string;
    revision?: number;
    updated_at?: string;
  };

  type HistoryResponse = {
    next_cursor?: string;
    versions?: Version[];
  };

  type ImageReference = {
    media?: MediaFact;
    role?: ImageRole;
  };

  type ImageRole =
    | "primary"
    | "front"
    | "side"
    | "back"
    | "turnaround_sheet"
    | "expression_sheet";

  type ImpactProof = {
    aliases?: number;
    current_lines?: number;
    historical_lines?: number;
    revision?: number;
    sha256?: string;
  };

  type ImportFile = {
    asset_id?: string;
    attempt?: number;
    failure_code?: string;
    file_name?: string;
    position?: number;
    source_id?: string;
    source_lineage_id?: string;
    status?: string;
    warnings?: ExtractionWarning[];
  };

  type ImportJob = {
    active_io?: boolean;
    attempt?: number;
    can_control?: boolean;
    cancellation_requested?: boolean;
    created_at?: string;
    expected_script_revision?: number;
    failure_code?: string;
    files?: ImportFile[];
    id?: string;
    latest_script_revision?: number;
    latest_version_id?: string;
    needs_reconciliation?: boolean;
    project_id?: string;
    reconciliation_requested?: boolean;
    retryable?: boolean;
    revision?: number;
    stage?: string;
    status?: string;
    updated_at?: string;
  };

  type ImportPage = {
    current_actor_id?: string;
    current_org_id?: string;
    items?: ImportJob[];
    next_after?: number;
  };

  type importScriptSourcesParams = {
    /** 项目UUID */
    pid: string;
  };

  type Inline = {
    attrs?: ReferenceAttrs;
    text?: string;
    type?: string;
  };

  type InputSource = {
    canvas_id?: string;
    canvas_revision?: number;
    draft_id?: string;
    input_revision?: number;
    kind?: string;
    node_id?: string;
    revision?: number;
  };

  type Kind = "character" | "location" | "prop";

  type LabelAllocation = {
    labelKind?: string;
    ordinal?: number;
  };

  type LabelState = {
    allocations?: Record<string, any>;
    highWater?: Record<string, any>;
  };

  type LibraryAssetSummary = {
    byte_size?: number;
    duration_ms?: number;
    file_name?: string;
    height?: number;
    id?: string;
    kind?: string;
    mime_type?: string;
    revision?: number;
    width?: number;
  };

  type LibraryCommand = {
    action?: string;
    expected_folder_revision?: number;
    expected_item_revision?: number;
    expected_revision?: number;
    folder?: LibraryFolderInput;
    folder_id?: string;
    item_id?: string;
    items?: LibraryItemRevision[];
    metadata?: LibraryMetadata;
    scope?: LibraryScope;
    target_folder_id?: string;
  };

  type LibraryFolder = {
    created_at?: string;
    id?: string;
    library_id?: string;
    library_kind?: LibraryKind;
    name?: string;
    parent_id?: string;
    position?: number;
    revision?: number;
    style?: string;
    theme?: string;
    updated_at?: string;
  };

  type LibraryFolderInput = {
    name?: string;
    parent_id?: string;
    style?: string;
    theme?: string;
  };

  type LibraryItemChange = {
    asset_id?: string;
    catalog_state?: string;
    folder_id?: string;
    id?: string;
    kind?: string;
    revision?: number;
    trashed_at?: string;
  };

  type LibraryItemDetail = {
    asset_id?: string;
    catalog_state?: string;
    category?: string;
    created_at?: string;
    favorite?: boolean;
    folder_id?: string;
    id?: string;
    kind?: string;
    media?: LibraryAssetSummary;
    note?: string;
    plain_text?: string;
    position?: number;
    revision?: number;
    source_label?: string;
    tags?: string[];
    title?: string;
    trashed_at?: string;
    updated_at?: string;
  };

  type LibraryItemRevision = {
    id?: string;
    revision?: number;
  };

  type LibraryItemSummary = {
    asset_id?: string;
    catalog_state?: string;
    category?: string;
    created_at?: string;
    favorite?: boolean;
    folder_id?: string;
    id?: string;
    kind?: string;
    media?: LibraryAssetSummary;
    note?: string;
    position?: number;
    revision?: number;
    source_label?: string;
    tags?: string[];
    title?: string;
    trashed_at?: string;
    updated_at?: string;
  };

  type LibraryKind = "project" | "personal";

  type LibraryMediaPreview = {
    asset?: LibraryAssetSummary;
    expires_at?: string;
    renditions?: LibraryRenditionPreview[];
    url?: string;
  };

  type LibraryMetadata = {
    category?: string;
    favorite?: boolean;
    folder_id?: string;
    note?: string;
    plain_text?: string;
    source_label?: string;
    tags?: string[];
    title?: string;
  };

  type LibraryPage = {
    category_counts?: Record<string, any>;
    current_actor_id?: string;
    current_org_id?: string;
    folder_counts?: Record<string, any>;
    folders?: LibraryFolder[];
    items?: LibraryItemSummary[];
    library_id?: string;
    page?: number;
    page_size?: number;
    revision?: number;
    scope?: LibraryScope;
    total?: number;
  };

  type LibraryReceipt = {
    current_actor_id?: string;
    current_org_id?: string;
    folder?: LibraryFolder;
    items?: LibraryItemChange[];
    library_id?: string;
    project_revision?: number;
    revision?: number;
    scope?: LibraryScope;
  };

  type LibraryRenditionPreview = {
    expires_at?: string;
    height?: number;
    kind?: string;
    url?: string;
    width?: number;
  };

  type LibraryScope = {
    kind?: LibraryKind;
    project_id?: string;
  };

  type listAdminModelsParams = {
    /** 1..200 */
    limit?: number;
    /** 分页游标 */
    cursor?: string;
  };

  type listAdminProvidersParams = {
    /** 1..200 */
    limit?: number;
    /** 绑定管理员与分页的不透明游标 */
    cursor?: string;
  };

  type listAdminUsersParams = {
    /** 1..200, default 50 */
    limit?: number;
    /** Actor and organization scoped cursor */
    cursor?: string;
  };

  type listBibleEntriesParams = {
    /** Return soft-deleted entries only */
    deleted?: boolean;
    /** Project UUID */
    pid: string;
    /** Identity kind */
    kind: "character" | "location" | "prop";
    /** Page size, maximum 200 */
    limit?: number;
    /** Opaque page cursor */
    cursor?: string;
  };

  type listBibleVersionsParams = {
    /** Project UUID */
    pid: string;
    /** Identity kind */
    kind: "character" | "location" | "prop";
    /** Identity UUID */
    id: string;
    /** Page size, maximum 50 */
    limit?: number;
    /** Opaque page cursor */
    cursor?: string;
  };

  type listBibleVoicesParams = {
    /** Project UUID */
    pid: string;
    /** Page size, maximum 200 */
    limit?: number;
    /** Opaque page cursor */
    cursor?: string;
  };

  type listCanvasesParams = {
    /** 项目UUID */
    pid: string;
  };

  type listMediaAssetsParams = {
    /** 项目UUID */
    pid: string;
    /** image/video/audio/model/document；默认仅可渲染媒体 */
    kind?: string;
    /** 不透明游标 */
    cursor?: string;
    /** 1..200，默认50 */
    limit?: number;
  };

  type listMediaDepthsParams = {
    /** Project UUID */
    pid: string;
    /** Canvas UUID */
    canvas_id?: string;
    /** Timeline node UUID */
    node_id?: string;
    /** 1..200, default 50 */
    limit?: number;
    /** Project and source bound cursor */
    cursor?: string;
  };

  type listMediaExportsParams = {
    /** Project UUID */
    pid: string;
    /** Canvas UUID */
    canvas_id?: string;
    /** Timeline node UUID */
    node_id?: string;
    /** 1..200, default 50 */
    limit?: number;
    /** Project and source bound cursor */
    cursor?: string;
  };

  type listMediaLibraryPackagesParams = {
    /** personal/project */
    scope?: string;
    /** 项目UUID */
    project_id?: string;
    /** 页码1..100000 */
    page?: number;
    /** 每页1..120 */
    page_size?: number;
  };

  type listMediaLibraryParams = {
    /** personal/project；默认personal */
    scope?: string;
    /** project scope必要UUID */
    project_id?: string;
    /** 页码1..100000；默认1 */
    page?: number;
    /** 每页1..120；个人默认40/项目默认20 */
    page_size?: number;
    /** text/image/video/audio/document/model */
    kind?: string;
    /** character/environment/prop/material/other */
    category?: string;
    /** 当前库目录UUID */
    folder_id?: string;
    /** 只看未分类根目录 */
    root_only?: boolean;
    /** 个人收藏 */
    favorite_only?: boolean;
    /** 最近30日 */
    recent_only?: boolean;
    /** active/trashed；默认active */
    catalog_state?: string;
    /** 完整库文字搜索 */
    search?: string;
    /** updated_desc/updated_asc/name_asc */
    order?: string;
  };

  type listMediaPurgesParams = {
    /** personal/project；默认personal */
    scope?: string;
    /** project scope必要UUID */
    project_id?: string;
    /** 1..10000；默认1 */
    page?: number;
    /** 1..100；默认20 */
    page_size?: number;
  };

  type listMediaTranscriptionsParams = {
    /** Project UUID */
    pid: string;
    /** Canvas UUID */
    canvas_id?: string;
    /** Timeline node UUID */
    node_id?: string;
    /** 1..200, default 50 */
    limit?: number;
    /** Project and source bound cursor */
    cursor?: string;
  };

  type listMediaTransfersParams = {
    /** personal/project；默认personal */
    scope?: string;
    /** project scope必要UUID */
    project_id?: string;
    /** 1..10000；默认1 */
    page?: number;
    /** 1..100；默认20 */
    page_size?: number;
  };

  type listModelReleasesParams = {
    /** 模型ID */
    id: string;
  };

  type listProjectCopiesParams = {
    /** 源项目UUID */
    pid: string;
    /** 1至100，默认50 */
    limit?: number;
    /** 上一页next_cursor */
    cursor?: string;
  };

  type listProjectFoldersParams = {
    /** 分页大小，上限200 */
    limit?: number;
    /** 当前主体目录分页游标 */
    cursor?: string;
  };

  type listProjectModelsParams = {
    /** 项目UUID */
    project_id: string;
    /** 能力 */
    capability?: string;
    /** 生成模式 */
    mode?: string;
    /** 1..200，默认50 */
    limit?: number;
    /** 绑定项目与筛选的不透明游标 */
    cursor?: string;
  };

  type listProjectOperationsParams = {
    /** 项目UUID */
    pid: string;
    /** 任务状态 */
    status?: string;
    /** 能力 */
    capability?: string;
    /** 模型 */
    model_key?: string;
    /** 来源 */
    origin?: string;
    /** 冻结来源画布UUID */
    canvas_id?: string;
    /** 冻结来源节点UUID，需canvas_id */
    node_id?: string;
    /** 冻结来源行UUID，需node_id */
    row_id?: string;
    /** 1..200，默认50 */
    limit?: number;
    /** 项目与筛选绑定游标 */
    cursor?: string;
  };

  type listProjectsParams = {
    /** 分页大小，上限200 */
    limit?: number;
    /** 不透明游标 */
    cursor?: string;
    /** 名称检索 */
    q?: string;
    /** active或archived */
    status?: string;
    /** 只查询回收中的项目 */
    deleted?: boolean;
    /** 个人目录UUID，root表示未分类；未指定列出全部 */
    folder_id?: string;
  };

  type listPromptDraftsParams = {
    /** 项目 */
    pid: string;
    /** 游标 */
    cursor?: string;
  };

  type listProviderCredentialsParams = {
    /** 连接ID */
    id: string;
  };

  type listScriptEpisodesParams = {
    /** 项目UUID */
    pid: string;
    /** 剧本版本UUID */
    version_id: string;
  };

  type listScriptEpisodeStructureVersionsParams = {
    /** 正式分集UUID */
    eid: string;
    /** 严格早于结构版本号 */
    before_version_no?: number;
    /** 1..100 */
    limit?: number;
  };

  type listScriptFileImportsParams = {
    /** Project UUID */
    pid: string;
    /** Offset */
    after?: number;
    /** Page size */
    limit?: number;
  };

  type listScriptSourceHistoryParams = {
    /** 项目UUID */
    pid: string;
    /** 稳定来源UUID */
    lineage: string;
    /** 严格早于来源修订 */
    before_revision?: number;
    /** 1..100 */
    limit?: number;
  };

  type listScriptSourcesParams = {
    /** 项目UUID */
    pid: string;
    /** 不可变历史版本UUID */
    version_id?: string;
    /** 下一个来源位置，须与返回version_id绑定 */
    after?: number;
    /** 1..100 */
    limit?: number;
  };

  type listScriptSourceWritesParams = {
    /** 项目UUID */
    pid: string;
    /** 分页位置 */
    after?: number;
    /** 1..100 */
    limit?: number;
  };

  type listScriptSplitConfirmationsParams = {
    /** 项目UUID */
    pid: string;
    /** 不可变剧本版本UUID */
    vid: string;
    /** 严格早于确认修订 */
    before_revision?: number;
    /** 1..100 */
    limit?: number;
  };

  type listScriptVersionsParams = {
    /** 项目UUID */
    pid: string;
    /** 严格早于版本号 */
    before_version_no?: number;
    /** 1..100 */
    limit?: number;
  };

  type listStylePresetsParams = {
    /** 风格类型 */
    style_type?: "realistic" | "stylized";
    /** 子风格 */
    style_subtype?: "anime_jp" | "guofeng_xianxia" | "cartoon_3d" | "manhwa";
    /** 每页数量（1～200，未传或0默认50） */
    limit?: number;
    /** 与账号、组织及筛选绑定的游标 */
    cursor?: string;
  };

  type LocationContent = {
    aliases?: string[];
    description?: string;
    name?: string;
    prompt?: string;
  };

  type LoginRequest = {
    login_name: string;
    password: string;
    persistent: boolean;
  };

  type LookContent = {
    applies_to?: LookScope[];
    default?: boolean;
    description?: string;
    id?: string;
    name?: string;
    references?: ImageReference[];
  };

  type LookInput = {
    applies_to?: LookScope[];
    default?: boolean;
    description?: string;
    name?: string;
  };

  type LookRequest = {
    expected_revision?: number;
    look?: LookInput;
  };

  type LookScope = {
    episode_id?: string;
    scene_key?: string;
  };

  type MarkAttrs = {
    color?: string;
    href?: string;
  };

  type MediaFact = {
    asset_id?: string;
    byte_size?: number;
    kind?: string;
    rendition_id?: string;
    rendition_sha256?: string;
    revision?: number;
    sha256?: string;
  };

  type MediatoolCreateInput = {
    canvas_id?: string;
    node_id?: string;
    output_kind?: "video" | "audio";
    revision?: number;
  };

  type MediatoolStatus =
    | "queued"
    | "running"
    | "review_required"
    | "succeeded"
    | "failed"
    | "cancel_requested"
    | "cancelled";

  type mergeBibleCharacterParams = {
    /** Project UUID */
    pid: string;
    /** Character kind */
    kind: "character";
    /** Source character UUID */
    id: string;
  };

  type MergeRequest = {
    acknowledged_impact?: ImpactProof;
    expected_revision?: number;
    expected_target_revision?: number;
    target_id?: string;
  };

  type Mode = "inherit" | "append" | "rewrite";

  type ModelCandidate = {
    capability?: string;
    display_name?: string;
    metadata?: Record<string, any>;
    provider_model_id?: string;
  };

  type ModelDefaults = {
    default_models?: Record<string, any>;
    project_id?: string;
    revision?: number;
  };

  type ModelDefaultsRequest = {
    default_models: Record<string, any>;
    expected_revision: number;
  };

  type ModelPage = {
    items?: ModelResponse[];
    next_cursor?: string;
  };

  type ModelPrice = {
    currency?: string;
    effective_from?: string;
    fx_rate_to_cny?: string;
    id?: string;
    rule?: Record<string, any>;
    unit?: string;
    version_no?: number;
  };

  type ModelRelease = {
    config_version_id?: string;
    created_at?: string;
    id?: string;
    model_id?: string;
    model_version_id?: string;
    price_version_id?: string;
    required_modes?: string[];
    revision?: number;
    status?: string;
    upstream_cost?: UpstreamCost;
    validated_modes?: string[];
  };

  type ModelResponse = {
    capability?: string;
    credential_present?: boolean;
    credential_test_result?: string;
    current_price?: ModelPrice;
    current_version?: ModelVersion;
    display_name?: string;
    id?: string;
    input_roles?: string[];
    key?: string;
    provider_id?: string;
    provider_name?: string;
    provider_status?: string;
    region?: string;
    status?: string;
  };

  type ModelStatus = "active" | "disabled";

  type ModelVersion = {
    id?: string;
    limits?: Record<string, any>;
    modes?: string[];
    param_schema?: Record<string, any>[];
    version_no?: number;
  };

  type Moderation = "provider" | "platform" | "both";

  type Move = {
    id?: string;
    x?: number;
    y?: number;
  };

  type moveProjectToFolderParams = {
    /** 项目UUID */
    pid: string;
  };

  type Name = {
    id?: string;
    title?: string;
  };

  type Node = {
    config?: NodeConfig;
    height?: number;
    id?: string;
    input_revision?: number;
    last_operation_id?: string;
    node_action?: string;
    node_type?: string;
    parent_id?: string;
    ref_id?: string;
    ref_type?: string;
    title?: string;
    width?: number;
    x?: number;
    y?: number;
    z_index?: number;
  };

  type NodeConfig = {
    batch_table?: BatchTableConfig;
    collapsed?: boolean;
    director?: DirectorConfig;
    draft?: CanvasDraft;
    generation?: GenerationConfig;
    text?: string;
    timeline?: TimelineConfig;
  };

  type OperationStatus =
    | "draft"
    | "quoted"
    | "confirmed"
    | "submitting"
    | "submitted"
    | "succeeded"
    | "ingesting"
    | "completed"
    | "failed"
    | "unknown"
    | "reconciling"
    | "manual"
    | "cancelling"
    | "cancelled"
    | "expired";

  type OutlineOptions = {
    chapter_count: "3" | "5" | "8" | "10";
    chapter_length: "短" | "中" | "长";
    character_scale: "2 个" | "3-4 个" | "5-6 个";
    perspective: "第三人称" | "第一人称" | "多视角";
    structure: "单线推进" | "双线并行" | "群像多线" | "反转嵌套";
    tone: "平稳叙事" | "轻松喜剧" | "紧张悬疑" | "热血成长" | "甜宠治愈";
    word_count: "500" | "800" | "1200" | "2000";
  };

  type OutputKind = "video" | "audio";

  type PackageControl = {
    expected_revision?: number;
  };

  type PackageJob = {
    created_at?: string;
    folder_count?: number;
    id?: string;
    item_count?: number;
    library_revision?: number;
    project_revision?: number;
    revision?: number;
    scope?: LibraryScope;
    status?: string;
    updated_at?: string;
    warnings?: PackageWarning[];
  };

  type PackagePage = {
    current_actor_id?: string;
    current_org_id?: string;
    jobs?: PackageJob[];
    page?: number;
    page_size?: number;
    scope?: LibraryScope;
    total?: number;
  };

  type PackageWarning = {
    code?: string;
    item_id?: string;
  };

  type PageResponse = {
    current_actor_id?: string;
    current_org_id?: string;
    entries?: Summary[];
    next_cursor?: string;
  };

  type Paragraph = {
    content?: Inline[];
    type?: string;
  };

  type Parent = {
    id?: string;
    parent_id?: string;
  };

  type PasswordChange = {
    confirm_password: string;
    current_password: string;
    expected_credential_revision: number;
    new_password: string;
  };

  type PersonalUploadResult = {
    asset?: LibraryAssetSummary;
    duplicate_of?: string;
  };

  type Preference = {
    customization?: Customization;
    definition?: Definition;
    outdated?: boolean;
  };

  type PreferencesResponse = {
    items?: Preference[];
  };

  type Preset = {
    archived?: boolean;
    document?: PresetDocument;
    emergency_disabled?: boolean;
    id?: string;
    origin?: string;
    parent_id?: string;
    revision?: number;
    slug?: string;
    updated_at?: string;
    version?: number;
    version_id?: string;
  };

  type PresetConnection = {
    config?: ConnectionConfig;
    provider_id?: string;
  };

  type PresetDocument = {
    config?: ProviderConfig;
    name?: string;
    schema_version?: number;
  };

  type PresetImportRequest = {
    document_json?: string;
  };

  type PresetPage = {
    items?: Preset[];
  };

  type PresetPreview = {
    document?: PresetDocument;
    hash?: string;
  };

  type PresetStateInput = {
    archived?: boolean;
    emergency_disabled?: boolean;
    expected_revision?: number;
    request_id?: string;
  };

  type Preview = {
    asset?: AssetSummary;
    expires_at?: string;
    url?: string;
  };

  type previewLibraryMediaAssetParams = {
    /** 二进制素材条目UUID */
    item_id: string;
    /** personal/project；默认personal */
    scope?: string;
    /** project scope必要UUID */
    project_id?: string;
  };

  type previewMediaDepthParams = {
    /** Depth UUID */
    job_id: string;
    /** Project UUID */
    project_id: string;
  };

  type previewMediaExportParams = {
    /** Export UUID */
    job_id: string;
    /** Project UUID */
    project_id: string;
  };

  type previewProviderUpgradeParams = {
    /** 连接ID */
    id: string;
  };

  type PriceUnit =
    | "per_image"
    | "per_second"
    | "per_request"
    | "per_1k_tokens"
    | "per_1k_chars";

  type Problem = {
    code?: string;
    detail?: string;
    meta?: Record<string, any>;
    request_id?: string;
    status?: number;
    title?: string;
    type?: string;
  };

  type ProjectCopyBibleCounts = {
    character_confirmations?: number;
    character_versions?: number;
    characters?: number;
    location_confirmations?: number;
    location_versions?: number;
    locations?: number;
    look_versions?: number;
    looks?: number;
    prop_confirmations?: number;
    prop_versions?: number;
    props?: number;
    redirects?: number;
    references?: number;
    splits?: number;
    voices?: number;
  };

  type ProjectCopyBibleProgress = {
    completed_counts?: ProjectCopyBibleCounts;
    counts?: ProjectCopyBibleCounts;
  };

  type ProjectCopyListResponse = {
    copies?: ProjectCopyResponse[];
    current_actor_id?: string;
    current_org_id?: string;
    next_cursor?: string;
  };

  type ProjectCopyPlacementRequest = {
    expected_folder_revision: number;
    expected_placement_revision: number;
    folder_id?: string;
  };

  type ProjectCopyRequest = {
    expected_revision: number;
    placement?: ProjectCopyPlacementRequest;
    target_name?: string;
  };

  type ProjectCopyResponse = {
    assets?: number;
    attempt?: number;
    bible?: ProjectCopyBibleProgress;
    cancellation_requested?: boolean;
    completed_assets?: number;
    completed_documents?: number;
    completed_renditions?: number;
    documents?: number;
    execution_unconfirmed?: boolean;
    failure_code?: string;
    id?: string;
    needs_reconciliation?: boolean;
    reconciliation_requested?: boolean;
    renditions?: number;
    retryable?: boolean;
    revision?: number;
    script?: ProjectCopyScriptProgress;
    source_project_id?: string;
    source_revision?: number;
    stage?:
      | "media"
      | "bible"
      | "script"
      | "canvases"
      | "finalizing"
      | "cleanup"
      | "complete";
    status?:
      | "queued"
      | "running"
      | "failed"
      | "cancel_requested"
      | "cancelled"
      | "succeeded";
    target_name?: string;
    target_project_id?: string;
  };

  type ProjectCopyScriptCounts = {
    action_lines?: number;
    dialogue_lines?: number;
    episodes?: number;
    objects?: number;
    project_states?: number;
    scenes?: number;
    sources?: number;
    split_confirmations?: number;
    split_sets?: number;
    structures?: number;
    version_heads?: number;
    version_sources?: number;
    versions?: number;
  };

  type ProjectCopyScriptProgress = {
    completed_counts?: ProjectCopyScriptCounts;
    counts?: ProjectCopyScriptCounts;
  };

  type ProjectDetailResponse = {
    allow_overseas_models?: boolean;
    archived_at?: string;
    aspect_ratio?: string;
    canvas_id?: string;
    cover_asset_id?: string;
    cover_unavailable?: boolean;
    create_time?: string;
    default_models?: Record<string, any>;
    delete_time?: string;
    description?: string;
    id?: string;
    is_delete?: boolean;
    name?: string;
    purge_after?: string;
    resolution?: string;
    revision?: number;
    status?: string;
    style_key?: string;
    style_preset_id?: string;
    style_subtype?: string;
    style_type?: string;
    update_time?: string;
  };

  type ProjectFolderChangeResponse = {
    folder?: ProjectFolderResponse;
    placement?: ProjectFolderPlacementResponse;
    recycled_project_ids?: string[];
  };

  type ProjectFolderCreateRequest = {
    cover?: FolderCover;
    name?: string;
  };

  type ProjectFolderListResponse = {
    current_actor_id?: string;
    current_org_id?: string;
    items?: ProjectFolderResponse[];
    next_cursor?: string;
  };

  type ProjectFolderMoveRequest = {
    expected_folder_revision?: number;
    expected_placement_revision?: number;
    expected_project_revision?: number;
    folder_id?: string;
  };

  type ProjectFolderPlacementResponse = {
    folder_id?: string;
    project_id?: string;
    revision?: number;
  };

  type ProjectFolderResponse = {
    cover?: FolderCover;
    cover_unavailable?: boolean;
    create_time?: string;
    delete_time?: string;
    id?: string;
    is_delete?: boolean;
    name?: string;
    project_count?: number;
    revision?: number;
    update_time?: string;
  };

  type ProjectFolderUpdateRequest = {
    cover?: FolderCover;
    expected_revision?: number;
    name?: string;
  };

  type ProjectRequest = {
    project_id?: string;
  };

  type ProjectResponse = {
    archived_at?: string;
    aspect_ratio?: string;
    canvas_id?: string;
    cover_asset_id?: string;
    cover_unavailable?: boolean;
    delete_time?: string;
    folder_id?: string;
    id?: string;
    is_delete?: boolean;
    name?: string;
    placement_revision?: number;
    purge_after?: string;
    revision?: number;
    status?: string;
    style_key?: string;
    style_type?: string;
    update_time?: string;
  };

  type ProjectState = {
    adopted_version_id?: string;
    draft_version_id?: string;
    org_id?: string;
    project_id?: string;
    revision?: number;
    updated_at?: string;
  };

  type ProjectTransitionRequest = {
    expected_revision: number;
  };

  type ProjectUpdateRequest = {
    allow_overseas_models?: boolean;
    cover_asset_id?: string;
    description?: string;
    expected_revision: number;
    name?: string;
    style_preset_id?: string;
  };

  type PromptDocument = {
    content?: Paragraph[];
    type?: string;
  };

  type PromptDraft = {
    document?: PromptDocument;
    labelState?: LabelState;
    mode?: string;
    modelVersion?: string;
    params?: Record<string, any>;
    references?: Reference[];
    schemaVersion?: number;
  };

  type PromptPreparation = {
    content_sha256?: string;
    customization_id?: string;
    customization_revision?: number;
    operation?: string;
    policy?: string;
    request_sha256?: string;
    template_id?: string;
    template_version?: number;
    user_prompt_sha256?: string;
    version?: number;
  };

  type PropContent = {
    aliases?: string[];
    description?: string;
    name?: string;
    prompt?: string;
  };

  type ProviderCheck = {
    adapter_result?: string;
    auth_result?: string;
    candidates?: ModelCandidate[];
    completed_at?: string;
    config_version_id?: string;
    created_at?: string;
    credential_id?: string;
    discovery_result?: string;
    error_code?: string;
    id?: string;
    kind?: string;
    provider_id?: string;
    status?: string;
  };

  type ProviderConfig = {
    adapter_key?: string;
    base_url?: string;
    capabilities?: string[];
    concurrency_limit?: number;
    rate_limit_per_min?: number;
    region?: Region;
    timeout_seconds?: number;
  };

  type ProviderCredentialSummary = {
    create_time?: string;
    id?: string;
    label?: string;
    last4?: string;
    last_test_result?: TestResult;
    last_tested_at?: string;
    provider_id?: string;
    status?: CredentialStatus;
    update_time?: string;
  };

  type ProviderDetail = {
    credential?: ProviderCredentialSummary;
    credential_schema?: CredentialField[];
    provider?: CreatedProvider;
  };

  type ProviderListItem = {
    credential?: ProviderCredentialSummary;
    model_count?: number;
    provider?: CreatedProvider;
  };

  type ProviderPage = {
    items?: ProviderListItem[];
    next_cursor?: string;
  };

  type ProviderStatus = "active" | "disabled";

  type publishAdminModelPriceParams = {
    /** 模型UUID */
    id: string;
  };

  type publishAdminModelVersionParams = {
    /** 模型UUID */
    id: string;
  };

  type PublishedModelVersion = {
    create_time?: string;
    id?: string;
    model_id?: string;
    revision?: number;
    version_no?: number;
  };

  type PublishedPriceRule = {
    create_time?: string;
    id?: string;
    model_id?: string;
    revision?: number;
    version_no?: number;
  };

  type PublishPriceRequest = {
    currency?: string;
    effective_from?: string;
    expected_revision?: number;
    fx_rate_to_cny?: string;
    rule?: Record<string, any>;
    unit?: PriceUnit;
    version_no?: number;
  };

  type publishProviderPresetVersionParams = {
    /** 预设ID */
    id: string;
  };

  type PublishVersionRequest = {
    expected_max_ms?: number;
    expected_revision?: number;
    limits?: Record<string, any>;
    moderation?: Moderation;
    modes?: string[];
    param_schema?: Record<string, any>[];
    provider_model_id?: string;
    queue?: string;
    supports_callback?: boolean;
    supports_cancel?: boolean;
    supports_query?: boolean;
    version_no?: number;
  };

  type PurgeControlRequest = {
    revision?: number;
  };

  type PurgeInput = {
    expected_project_revision?: number;
    expected_revision?: number;
    items?: LibraryItemRevision[];
    permanent_delete_confirmed?: boolean;
    scope?: LibraryScope;
  };

  type PurgeItemResult = {
    asset_id?: string;
    failure_code?: string;
    index?: number;
    item_id?: string;
    status?: string;
  };

  type PurgeJob = {
    attempt?: number;
    cancellation_requested?: boolean;
    created_at?: string;
    current_actor_id?: string;
    current_org_id?: string;
    execution_unconfirmed?: boolean;
    id?: string;
    items?: PurgeItemResult[];
    needs_reconciliation?: boolean;
    revision?: number;
    scope?: LibraryScope;
    stage?: string;
    status?: string;
    updated_at?: string;
  };

  type PurgePage = {
    current_actor_id?: string;
    current_org_id?: string;
    items?: PurgeJob[];
    page?: number;
    page_size?: number;
  };

  type QuoteItemRequest = {
    capability?: string;
    expected_submission_epoch?: number;
    force_regenerate?: boolean;
    input_source?: InputSource;
    media_inputs?: QuoteMediaInput[];
    mode?: string;
    model_key?: string;
    output_count?: number;
    params?: Record<string, any>;
    prompt?: string;
    prompt_template?: TemplateRequest;
    source?: CanvasSource;
  };

  type QuoteMediaInput = {
    media_asset_id?: string;
    role?: string;
  };

  type QuoteResponse = {
    available_micros?: number;
    confirm_key?: string;
    confirmable?: boolean;
    editor_draft?: PromptDraft;
    expires_at?: string;
    final_prompt?: string;
    operation_id?: string;
    prompt_preparation?: PromptPreparation;
    quote_detail?: Record<string, any>;
    quote_micros?: number;
    reused_from_id?: string;
    submission_epoch?: number;
    submission_id?: string;
  };

  type QuotesRequest = {
    items?: QuoteItemRequest[];
  };

  type readPromptDraftParams = {
    /** 项目 */
    pid: string;
    /** 草稿 */
    id: string;
  };

  type Receipt = {
    confirmed_version_id?: string;
    content_sha256?: string;
    created_entry_id?: string;
    created_version_id?: string;
    entry_id?: string;
    kind?: Kind;
    project_revision?: number;
    redirect_id?: string;
    revision?: number;
    version_id?: string;
    version_number?: number;
  };

  type reconcileMediaDepthParams = {
    /** Depth UUID */
    job_id: string;
  };

  type reconcileMediaLibraryPackageParams = {
    /** 批次UUID */
    job_id: string;
  };

  type reconcileMediaPurgeParams = {
    /** 清理UUID */
    job_id: string;
  };

  type reconcileMediaTransferParams = {
    /** 转移UUID */
    job_id: string;
  };

  type reconcileProjectCopyParams = {
    /** 复制任务UUID */
    id: string;
  };

  type reconcileScriptFileImportParams = {
    /** Project UUID */
    pid: string;
    /** Import UUID */
    id: string;
  };

  type reconcileScriptSourceWriteParams = {
    /** 项目UUID */
    pid: string;
    /** 保存意图UUID */
    wid: string;
  };

  type recycleProjectFolderParams = {
    /** 目录UUID */
    folder_id: string;
  };

  type Reference = {
    bindingGeneration?: number;
    id?: string;
    mediaKind?: string;
    order?: number;
    role?: string;
    source?: ReferenceSource;
  };

  type ReferenceAttrs = {
    referenceId?: string;
  };

  type ReferenceInput = {
    asset_id?: string;
    role?: ImageRole;
  };

  type ReferenceSource = {
    clientUploadId?: string;
    description?: string;
    kind?: string;
    media?: SourceMedia[];
    sourceId?: string;
    sourceVersion?: string;
    uploadAttemptId?: string;
    uploadSessionId?: string;
  };

  type ReferencesRequest = {
    expected_revision?: number;
    references?: ReferenceInput[];
  };

  type Region = "domestic" | "overseas";

  type RegisterRequest = {
    confirm_password: string;
    display_name: string;
    login_name: string;
    password: string;
  };

  type ReleasePage = {
    items?: ModelRelease[];
  };

  type renameCanvasParams = {
    /** 画布UUID */
    id: string;
  };

  type RenameInput = {
    expected_revision?: number;
    name?: string;
  };

  type reorderScriptSourcesParams = {
    /** 项目UUID */
    pid: string;
  };

  type replaceBibleReferencesParams = {
    /** Project UUID */
    pid: string;
    /** Identity kind */
    kind: "character" | "location" | "prop";
    /** Identity UUID */
    id: string;
    /** Appearance UUID */
    look: string;
  };

  type resetAdminPasswordParams = {
    /** User UUID */
    id: string;
  };

  type resolvePromptSubmissionParams = {
    /** 项目 */
    pid: string;
    /** 提交 */
    id: string;
  };

  type ResolveSubmissionRequest = {
    action?: string;
    expected_epoch?: number;
  };

  type resplitScriptEpisodesParams = {
    /** 项目UUID */
    pid: string;
  };

  type restoreBibleEntryParams = {
    /** Project UUID */
    pid: string;
    /** Identity kind */
    kind: "character" | "location" | "prop";
    /** Identity UUID */
    id: string;
  };

  type restoreProjectParams = {
    /** 项目UUID */
    pid: string;
  };

  type Result = {
    edges?: Edge[];
    id?: string;
    name?: string;
    nodes?: Node[];
    project_id?: string;
    results?: CommandResult[];
    revision?: number;
    scope?: Record<string, any>;
    viewport?: Viewport;
  };

  type ResultRequest = {
    expected_revision?: number;
    operation_id?: string;
    output_id?: string;
  };

  type ResultSource = {
    input_sha256?: string;
    operation_id?: string;
    output_id?: string;
    output_sha256?: string;
  };

  type resumeOperationBatchParams = {
    /** Batch UUID */
    id: string;
  };

  type retryMediaDepthParams = {
    /** Depth UUID */
    job_id: string;
  };

  type retryMediaExportParams = {
    /** Export UUID */
    job_id: string;
  };

  type retryMediaTranscriptionParams = {
    /** Transcription UUID */
    job_id: string;
  };

  type retryMediaTransferParams = {
    /** 转移UUID */
    job_id: string;
  };

  type retryProjectCopyParams = {
    /** 复制任务UUID */
    id: string;
  };

  type retryScriptFileImportParams = {
    /** Project UUID */
    pid: string;
    /** Import UUID */
    id: string;
  };

  type ReviewInput = {
    local_review_confirmed?: boolean;
    project_id?: string;
    revision?: number;
    sha256?: string;
  };

  type reviewMediaDepthParams = {
    /** Depth UUID */
    job_id: string;
  };

  type reviewMediaExportParams = {
    /** Export UUID */
    job_id: string;
  };

  type RichAttrs = {
    language?: string;
    level?: number;
    start?: number;
    text_align?: string;
  };

  type RichDocument = {
    attrs?: RichAttrs;
    content?: RichDocument[];
    marks?: RichMark[];
    text?: string;
    type?: string;
  };

  type RichMark = {
    attrs?: MarkAttrs;
    type?: string;
  };

  type SampleInput = {
    asset_id?: string;
    name?: string;
  };

  type SampleVoice = {
    media?: MediaFact;
    name?: string;
  };

  type SavedCredential = {
    create_time?: string;
    id?: string;
    label?: string;
    last4?: string;
    last_test_result?: TestResult;
    provider_id?: string;
    status?: CredentialStatus;
    update_time?: string;
  };

  type SavePresetInput = {
    document?: PresetDocument;
    expected_revision?: number;
    parent_id?: string;
    preview_hash?: string;
    request_id?: string;
    slug?: string;
  };

  type saveProjectModelDefaultsParams = {
    /** 项目UUID */
    pid: string;
  };

  type savePromptDraftParams = {
    /** 项目 */
    pid: string;
    /** 草稿 */
    id: string;
  };

  type savePromptPreferenceParams = {
    /** 只读目录中的操作标识 */
    operation: string;
  };

  type SaveRequest = {
    base_template_id: string;
    content?: string;
    expected_revision: number;
    mode: Mode;
  };

  type saveScriptEpisodeStructureParams = {
    /** 正式分集UUID */
    eid: string;
  };

  type ScalarSpan = {
    end?: number;
    start?: number;
  };

  type SessionView = {
    absolute_expires_at: string;
    actor: Actor;
    last_active_at: string;
    persistent: boolean;
    session_id: string;
  };

  type setAdminCredentialParams = {
    /** 渠道UUID */
    id: string;
  };

  type setAdminModelStatusParams = {
    /** 模型UUID */
    id: string;
  };

  type setBibleDefaultLookParams = {
    /** Project UUID */
    pid: string;
    /** Identity kind */
    kind: "character" | "location" | "prop";
    /** Identity UUID */
    id: string;
    /** Appearance UUID */
    look: string;
  };

  type SetCredentialRequest = {
    label?: string;
    secret?: Record<string, any>;
  };

  type SetModelStatusRequest = {
    expected_revision?: number;
    status?: ModelStatus;
  };

  type Size = {
    height?: number;
    id?: string;
    width?: number;
  };

  type Source = {
    canvas_id?: string;
    node_id?: string;
    revision?: number;
  };

  type SourceChange = {
    new_source_id?: string;
    old_source_id?: string;
    source_lineage_id?: string;
  };

  type SourceControlReceipt = {
    accepted?: boolean;
    action?: string;
    intent_id?: string;
    revision?: number;
  };

  type SourceControlRequest = {
    expected_revision?: number;
  };

  type SourceDeleteRequest = {
    base_version_id?: string;
    expected_revision?: number;
  };

  type SourceDetail = {
    char_count?: number;
    content_hash?: string;
    document?: RichDocument;
    id?: string;
    media_asset_id?: string;
    origin?: string;
    plain_text?: string;
    position?: number;
    previous_source_id?: string;
    provenance?: SourceProvenance;
    rich_sha256?: string;
    source_kind?: string;
    source_lineage_id?: string;
    source_revision?: number;
    source_span?: SourceSpan;
    status?: string;
    title?: string;
  };

  type SourceExtractionWarning = {
    code?: string;
    count?: number;
    paragraph?: number;
  };

  type SourceHistoryPage = {
    items?: SourceSummary[];
    next_revision?: number;
  };

  type SourceImportItem = {
    document?: RichDocument;
    original_html?: string;
    provenance?: SourceProvenance;
    source_kind?: string;
    status?: string;
    title?: string;
  };

  type SourceImportRequest = {
    base_version_id?: string;
    expected_revision?: number;
    rights_confirmed?: boolean;
    sources?: SourceImportItem[];
  };

  type SourceMedia = {
    mediaKind?: string;
    sourceId?: string;
    sourceVersion?: string;
  };

  type SourceOriginMapping = {
    decoded_end?: number;
    decoded_start?: number;
    paragraph?: number;
    part?: string;
    span_end?: number;
    span_start?: number;
  };

  type SourcePage = {
    items?: SourceSummary[];
    next_position?: number;
    version_id?: string;
  };

  type SourceProvenance = {
    encoding?: string;
    external_id?: string;
    external_position?: number;
    mapping?: SourceOriginMapping[];
    warnings?: SourceExtractionWarning[];
  };

  type SourceReceipt = {
    changed?: boolean;
    duplicate?: boolean;
    project_revision?: number;
    script_revision?: number;
    source_mappings?: SourceChange[];
    split_set_id?: string;
    version_id?: string;
  };

  type SourceReorderRequest = {
    base_version_id?: string;
    expected_revision?: number;
    source_lineage_ids?: string[];
  };

  type SourceSnapshotDetail = {
    char_count?: number;
    content_hash?: string;
    document?: RichDocument;
    id?: string;
    media_asset_id?: string;
    origin?: string;
    original_html?: string;
    plain_text?: string;
    position?: number;
    previous_source_id?: string;
    provenance?: SourceProvenance;
    rich_sha256?: string;
    source_kind?: string;
    source_lineage_id?: string;
    source_revision?: number;
    status?: string;
    title?: string;
  };

  type SourceSpan = {
    end?: number;
    position?: number;
    source_id?: string;
    source_lineage_id?: string;
    start?: number;
  };

  type SourceSummary = {
    char_count?: number;
    content_hash?: string;
    id?: string;
    media_asset_id?: string;
    origin?: string;
    position?: number;
    previous_source_id?: string;
    rich_sha256?: string;
    source_kind?: string;
    source_lineage_id?: string;
    source_revision?: number;
    status?: string;
    title?: string;
  };

  type SourceWriteIntent = {
    action?: string;
    active_io?: boolean;
    base_version_id?: string;
    can_control?: boolean;
    cancellation_requested?: boolean;
    confirmed_object_count?: number;
    created_at?: string;
    expected_script_revision?: number;
    id?: string;
    needs_reconciliation?: boolean;
    object_count?: number;
    revision?: number;
    status?: string;
    updated_at?: string;
  };

  type SourceWritePage = {
    current_actor_id?: string;
    current_org_id?: string;
    items?: SourceWriteIntent[];
    next_after?: number;
  };

  type SourceWriteRequest = {
    base_version_id?: string;
    document?: RichDocument;
    expected_revision?: number;
    original_html?: string;
    provenance?: SourceProvenance;
    rights_confirmed?: boolean;
    source_kind?: string;
    status?: string;
    title?: string;
  };

  type splitBibleCharacterParams = {
    /** Project UUID */
    pid: string;
    /** Character kind */
    kind: "character";
    /** Parent character UUID */
    id: string;
  };

  type SplitConfirmation = {
    actor_id?: string;
    candidate_set_id?: string;
    created_at?: string;
    episodes?: Episode[];
    formal_set_id?: string;
    id?: string;
    org_id?: string;
    preface?: ScalarSpan;
    project_id?: string;
    revision?: number;
    version_id?: string;
  };

  type SplitReceipt = {
    confirmation_id?: string;
    episode_mappings?: EpisodeMapping[];
    episodes?: Episode[];
    project_revision?: number;
    renamed_episode_ids?: string[];
    script_revision?: number;
    split_revision?: number;
    split_set_id?: string;
  };

  type SplitRequest = {
    character?: CharacterInput;
    expected_revision?: number;
  };

  type SplitReviewRequest = {
    ack_invalidate?: boolean;
    boundaries?: EpisodeBoundary[];
    candidate_set_id?: string;
    expected_revision?: number;
    expected_split_revision?: number;
    preface?: ScalarSpan;
    version_id?: string;
  };

  type SplitSet = {
    boundaries?: EpisodeBoundary[];
    created_at?: string;
    id?: string;
    kind?: string;
    org_id?: string;
    origin?: string;
    preface?: ScalarSpan;
    project_id?: string;
    version_id?: string;
    warnings?: string[];
  };

  type StorageUsage = {
    calculated_at?: string;
    current_actor_id?: string;
    current_org_id?: string;
    library_id?: string;
    limit_bytes?: number;
    object_count?: number;
    scope?: LibraryScope;
    used_bytes?: number;
  };

  type StructureConfirmRequest = {
    ack_invalidate?: boolean;
    base_structure_version_no?: number;
    expected_episode_revision?: number;
    expected_revision?: number;
  };

  type StructureDetail = {
    episode?: Episode;
    structure?: EpisodeStructure;
  };

  type StructureDocument = {
    scenes?: StructureScene[];
    unassigned_lines?: UnassignedLine[];
  };

  type StructureItem = {
    character_id?: string;
    character_version_id?: string;
    content?: string;
    emotion?: string;
    kind?: string;
    line_key?: string;
    span_end?: number;
    span_start?: number;
    speaker_text?: string;
    type?: string;
  };

  type StructurePage = {
    items?: StructureSummary[];
    next_version_no?: number;
  };

  type StructureReceipt = {
    episode_revision?: number;
    project_revision?: number;
    review_status?: string;
    script_revision?: number;
    structure_id?: string;
    version_no?: number;
  };

  type StructureSaveRequest = {
    base_structure_version_no?: number;
    document?: StructureDocument;
    expected_episode_revision?: number;
    expected_revision?: number;
  };

  type StructureScene = {
    heading?: string;
    items?: StructureItem[];
    location_text?: string;
    scene_key?: string;
    seq_no?: number;
    span_end?: number;
    span_start?: number;
    time_of_day?: string;
  };

  type StructureSummary = {
    created_at?: string;
    episode_id?: string;
    id?: string;
    source_hash?: string;
    version_no?: number;
  };

  type StylePresetListResponse = {
    items?: StylePresetSummary[];
    next_cursor?: string;
  };

  type StylePresetSummary = {
    id?: string;
    name?: string;
    style_subtype?: string;
    style_type?: string;
  };

  type Submission = {
    confirm_key?: string;
    draft?: PromptDraft;
    epoch?: number;
    id?: string;
    input_source?: InputSource;
    operation_id?: string;
    quote?: EditorQuote;
    state?: string;
  };

  type SubmissionCurrent = {
    epoch?: number;
    submission?: Submission;
  };

  type SubtitleSegment = {
    end_ms?: number;
    start_ms?: number;
    text?: string;
  };

  type SubtitleStyle = {
    color?: string;
    font_size?: number;
    position?: string;
  };

  type Summary = {
    content_sha256?: string;
    head?: Head;
    name?: string;
  };

  type TaskDetail = {
    batch_id?: string;
    cancel_requested?: boolean;
    capability?: string;
    create_time?: string;
    editor_draft?: PromptDraft;
    events?: TaskEvent[];
    failure_code?: string;
    finished_at?: string;
    id?: string;
    inputs?: TaskInput[];
    mode?: string;
    model_key?: string;
    model_name?: string;
    origin?: string;
    output_count?: number;
    outputs?: TaskOutput[];
    params?: Record<string, any>;
    project_id?: string;
    prompt_preparation?: PromptPreparation;
    quote_detail?: Record<string, any>;
    quote_expires_at?: string;
    quote_micros?: number;
    retryable?: boolean;
    reused_from_id?: string;
    settled_micros?: number;
    source?: CanvasSource;
    started_at?: string;
    status?: OperationStatus;
    target_id?: string;
    target_type?: string;
    update_time?: string;
  };

  type TaskEvent = {
    create_time?: string;
    from_status?: string;
    id?: string;
    reason?: string;
    to_status?: string;
  };

  type TaskInput = {
    mask_asset_id?: string;
    media_asset_id?: string;
    role?: string;
    sequence?: number;
    text?: string;
  };

  type TaskOutput = {
    create_time?: string;
    id?: string;
    kind?: string;
    media_asset_id?: string;
    moderation_status?: string;
    sequence?: number;
  };

  type TaskPageResponse = {
    items?: TaskSummary[];
    next_cursor?: string;
  };

  type TaskSummary = {
    batch_id?: string;
    cancel_requested?: boolean;
    capability?: string;
    create_time?: string;
    failure_code?: string;
    finished_at?: string;
    id?: string;
    mode?: string;
    model_key?: string;
    model_name?: string;
    origin?: string;
    project_id?: string;
    quote_micros?: number;
    retryable?: boolean;
    settled_micros?: number;
    source?: CanvasSource;
    started_at?: string;
    status?: OperationStatus;
    target_id?: string;
    target_type?: string;
    update_time?: string;
  };

  type TemplateRequest = {
    expected_customization_revision: number;
    expected_template_id: string;
    operation:
      | "chapter_assets_extract"
      | "character_extract"
      | "character_turnaround"
      | "storyboard_plan"
      | "storyboard_repair"
      | "storyboard_first_frame"
      | "storyboard_video"
      | "short_drama_outline"
      | "skill_draft";
    outline?: OutlineOptions;
  };

  type testAdminCredentialParams = {
    /** 渠道UUID */
    id: string;
    /** 凭据UUID */
    credential_id: string;
  };

  type TestResult =
    | "ok"
    | "auth_failed"
    | "unreachable"
    | "timeout"
    | "unsupported";

  type TimelineClip = {
    asset_id?: string;
    crop?: TimelineCrop;
    duration_ms?: number;
    fade_in_ms?: number;
    fade_out_ms?: number;
    id?: string;
    kind?: string;
    node_id?: string;
    source_duration_ms?: number;
    source_start_ms?: number;
    start_ms?: number;
    text?: string;
    title?: string;
    track_id?: string;
    volume?: number;
  };

  type TimelineConfig = {
    aspect_ratio?: string;
    clips?: TimelineClip[];
    fps?: number;
    snapping?: boolean;
    subtitle_style?: SubtitleStyle;
    tracks?: TimelineTrack[];
    version?: number;
  };

  type TimelineCrop = {
    height?: number;
    width?: number;
    x?: number;
    y?: number;
  };

  type TimelineTrack = {
    id?: string;
    kind?: string;
    label?: string;
    locked?: boolean;
    muted?: boolean;
    visible?: boolean;
  };

  type Transcript = {
    duration_ms?: number;
    language?: string;
    segments?: SubtitleSegment[];
    version?: number;
  };

  type TranscriptionCreateInput = {
    canvas_id?: string;
    language?: string;
    node_id?: string;
    revision?: number;
  };

  type TranscriptionJob = {
    attempt?: number;
    created_at?: string;
    failure_code?: string;
    id?: string;
    language?: string;
    progress?: number;
    project_id?: string;
    result_sha256?: string;
    revision?: number;
    source?: TranscriptionSource;
    stage?: string;
    status?: TranscriptionStatus;
    updated_at?: string;
  };

  type TranscriptionJobResponse = {
    attempt?: number;
    created_at?: string;
    failure_code?: string;
    id?: string;
    language?: string;
    progress?: number;
    project_id?: string;
    result_sha256?: string;
    revision?: number;
    source?: TranscriptionSource;
    stage?: string;
    status?: TranscriptionStatus;
    updated_at?: string;
  };

  type TranscriptionPage = {
    items?: TranscriptionJob[];
    next_cursor?: string;
  };

  type TranscriptionResult = {
    draft?: Transcript;
    job_id?: string;
    revision?: number;
    sha256?: string;
    source_asset_id?: string;
    source_asset_revision?: number;
    source_sha256?: string;
    srt?: string;
  };

  type TranscriptionSource = {
    canvas_id?: string;
    node_id?: string;
    revision?: number;
  };

  type TranscriptionStatus =
    | "queued"
    | "running"
    | "succeeded"
    | "failed"
    | "cancel_requested"
    | "cancelled";

  type TransferControlRequest = {
    revision?: number;
  };

  type TransferInput = {
    expected_folder_revision?: number;
    expected_project_revision?: number;
    expected_source_revision?: number;
    expected_target_revision?: number;
    items?: LibraryItemRevision[];
    source?: LibraryScope;
    target?: LibraryScope;
    target_folder_id?: string;
  };

  type TransferItemResult = {
    failure_code?: string;
    index?: number;
    source_item_id?: string;
    status?: string;
    target_asset_id?: string;
    target_item_id?: string;
  };

  type TransferJob = {
    attempt?: number;
    cancellation_requested?: boolean;
    created_at?: string;
    current_actor_id?: string;
    current_org_id?: string;
    execution_unconfirmed?: boolean;
    id?: string;
    items?: TransferItemResult[];
    needs_reconciliation?: boolean;
    revision?: number;
    source?: LibraryScope;
    stage?: string;
    status?: string;
    target?: LibraryScope;
    target_folder_id?: string;
    updated_at?: string;
  };

  type TransferPage = {
    current_actor_id?: string;
    current_org_id?: string;
    items?: TransferJob[];
    page?: number;
    page_size?: number;
  };

  type unarchiveProjectParams = {
    /** 项目UUID */
    pid: string;
  };

  type UnassignedLine = {
    content?: string;
    line_key?: string;
    span_end?: number;
    span_start?: number;
  };

  type unbindBibleVoiceParams = {
    /** Project UUID */
    pid: string;
    /** Identity kind */
    kind: "character" | "location" | "prop";
    /** Identity UUID */
    id: string;
  };

  type unlockAdminUserParams = {
    /** User UUID */
    id: string;
  };

  type updateAdminProviderParams = {
    /** 渠道UUID */
    id: string;
  };

  type updateAdminUserParams = {
    /** User UUID */
    id: string;
  };

  type updateBibleEntryParams = {
    /** Project UUID */
    pid: string;
    /** Identity kind */
    kind: "character" | "location" | "prop";
    /** Identity UUID */
    id: string;
  };

  type updateBibleLookParams = {
    /** Project UUID */
    pid: string;
    /** Identity kind */
    kind: "character" | "location" | "prop";
    /** Identity UUID */
    id: string;
    /** Appearance UUID */
    look: string;
  };

  type updateProjectFolderParams = {
    /** 目录UUID */
    folder_id: string;
  };

  type updateProjectParams = {
    /** 项目UUID */
    pid: string;
  };

  type updateProviderPresetStateParams = {
    /** 预设ID */
    id: string;
  };

  type UpdateProviderRequest = {
    concurrency_limit?: number;
    expected_revision?: number;
    rate_limit_per_min?: number;
    status?: ProviderStatus;
  };

  type updateScriptSourceParams = {
    /** 项目UUID */
    pid: string;
    /** 稳定来源UUID */
    lineage: string;
  };

  type uploadMediaAssetParams = {
    /** 项目UUID */
    pid: string;
  };

  type UploadResult = {
    asset?: AssetSummary;
    duplicate_of?: string;
  };

  type UpstreamCost = {
    currency?: string;
    max_micros?: number;
    reviewed_at?: string;
    source?: string;
  };

  type Variable = {
    label?: string;
    placeholder?: string;
  };

  type Version = {
    actor_id?: string;
    character?: CharacterContent;
    content_sha256?: string;
    created_at?: string;
    entry_id?: string;
    id?: string;
    kind?: Kind;
    location?: LocationContent;
    number?: number;
    org_id?: string;
    origin?: VersionOrigin;
    previous_id?: string;
    project_id?: string;
    prop?: PropContent;
    result?: ResultSource;
  };

  type VersionHead = {
    candidate_split_set_id?: string;
    confirmed_split_set_id?: string;
    split_revision?: number;
    version_id?: string;
  };

  type VersionOrigin = "manual" | "ai";

  type VersionPage = {
    items?: VersionSummary[];
    next_version_no?: number;
  };

  type VersionSummary = {
    char_count?: number;
    content_hash?: string;
    created_at?: string;
    document_sha256?: string;
    id?: string;
    source_count?: number;
    source_manifest_sha256?: string;
    version_no?: number;
  };

  type VersionText = {
    content_hash?: string;
    script_version_id?: string;
    span_end?: number;
    span_start?: number;
    text?: string;
  };

  type Viewport = {
    x?: number;
    y?: number;
    zoom?: number;
  };

  type VoiceChoice = {
    display_name?: string;
    model_key?: string;
    model_version?: number;
    voice_key?: string;
  };

  type VoiceContent = {
    catalog?: CatalogVoice;
    instructions?: string;
    kind?: VoiceKind;
    sample?: SampleVoice;
  };

  type VoiceInput = {
    catalog?: VoiceSelection;
    instructions?: string;
    kind?: VoiceKind;
    sample?: SampleInput;
  };

  type VoiceKind = "catalog" | "sample";

  type VoiceParams = {
    emotion?: string;
    language?: string;
    pitch?: number;
    speed?: number;
    volume?: number;
  };

  type VoiceRequest = {
    expected_revision?: number;
    voice?: VoiceInput;
  };

  type VoiceResponse = {
    next_cursor?: string;
    voices?: VoiceChoice[];
  };

  type VoiceSelection = {
    expected_model_version?: number;
    model_key?: string;
    params?: VoiceParams;
    voice_key?: string;
  };

  type WorkflowControlResult = {
    accepted?: boolean;
    action?: string;
    event_id?: string;
    request_id?: string;
    target_id?: string;
    target_type?: string;
  };

  type WorkspaceListResponse = {
    items?: ProjectResponse[];
    next_cursor?: string;
  };

  type WorkspaceView = {
    current_actor_id?: string;
    current_org_id?: string;
    state?: ProjectState;
  };

  type ZIndex = {
    id?: string;
    z_index?: number;
  };
}
