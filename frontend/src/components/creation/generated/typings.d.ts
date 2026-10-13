declare namespace CreationAPI {
  type AdminCreateRequest = {
    display_name?: string;
    initial_password?: string;
    login_name?: string;
    role?: "creator" | "admin";
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

  type cancelOperationParams = {
    /** Operation UUID */
    id: string;
  };

  type CanvasSource = {
    canvas_id?: string;
    node_id?: string;
    revision?: number;
    row_id?: string;
  };

  type CatalogVoice = {
    model_key?: string;
    model_version?: number;
    model_version_id?: string;
    param_schema_sha256?: string;
    params?: VoiceParams;
    voice_key?: string;
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

  type ContentRequest = {
    character?: CharacterInput;
    expected_revision?: number;
    location?: LocationContent;
    prop?: PropContent;
  };

  type ControlRequest = {
    acknowledged_impact?: ImpactProof;
    expected_revision?: number;
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

  type createFreeQuoteParams = {
    /** 项目UUID */
    pid: string;
  };

  type deleteBibleEntryParams = {
    /** Project UUID */
    pid: string;
    /** Identity kind */
    kind: "character" | "location" | "prop";
    /** Identity UUID */
    id: string;
  };

  type Detail = {
    current?: Version;
    head?: Head;
    resolved_id?: string;
  };

  type disableAdminUserParams = {
    /** User UUID */
    id: string;
  };

  type enableAdminUserParams = {
    /** User UUID */
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

  type getMediaPreviewParams = {
    /** 项目UUID */
    pid: string;
    /** 媒体UUID */
    asset_id: string;
  };

  type getOperationParams = {
    /** Operation UUID */
    id: string;
    /** 项目UUID */
    project_id: string;
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

  type Kind = "character" | "location" | "prop";

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

  type LocationContent = {
    aliases?: string[];
    description?: string;
    name?: string;
    prompt?: string;
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

  type MediaFact = {
    asset_id?: string;
    byte_size?: number;
    kind?: string;
    rendition_id?: string;
    rendition_sha256?: string;
    revision?: number;
    sha256?: string;
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

  type ModelVersion = {
    id?: string;
    limits?: Record<string, any>;
    modes?: string[];
    param_schema?: Record<string, any>[];
    version_no?: number;
  };

  type OutlineOptions = {
    chapter_count: "3" | "5" | "8" | "10";
    chapter_length: "短" | "中" | "长";
    character_scale: "2 个" | "3-4 个" | "5-6 个";
    perspective: "第三人称" | "第一人称" | "多视角";
    structure: "单线推进" | "双线并行" | "群像多线" | "反转嵌套";
    tone: "平稳叙事" | "轻松喜剧" | "紧张悬疑" | "热血成长" | "甜宠治愈";
    word_count: "500" | "800" | "1200" | "2000";
  };

  type PageResponse = {
    current_actor_id?: string;
    current_org_id?: string;
    entries?: Summary[];
    next_cursor?: string;
  };

  type Preview = {
    asset?: AssetSummary;
    expires_at?: string;
    url?: string;
  };

  type Problem = {
    code?: string;
    detail?: string;
    meta?: Record<string, any>;
    request_id?: string;
    status?: number;
    title?: string;
    type?: string;
  };

  type ProjectRequest = {
    project_id?: string;
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

  type QuoteItemRequest = {
    capability?: string;
    force_regenerate?: boolean;
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
    confirmable?: boolean;
    expires_at?: string;
    final_prompt?: string;
    operation_id?: string;
    prompt_preparation?: PromptPreparation;
    quote_detail?: Record<string, any>;
    quote_micros?: number;
    reused_from_id?: string;
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

  type ReferenceInput = {
    asset_id?: string;
    role?: ImageRole;
  };

  type ReferencesRequest = {
    expected_revision?: number;
    references?: ReferenceInput[];
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

  type restoreBibleEntryParams = {
    /** Project UUID */
    pid: string;
    /** Identity kind */
    kind: "character" | "location" | "prop";
    /** Identity UUID */
    id: string;
  };

  type ResultSource = {
    input_sha256?: string;
    operation_id?: string;
    output_id?: string;
    output_sha256?: string;
  };

  type SampleVoice = {
    media?: MediaFact;
    name?: string;
  };

  type Status =
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
    status?: Status;
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
    status?: Status;
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

  type unlockAdminUserParams = {
    /** User UUID */
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

  type uploadMediaAssetParams = {
    /** 项目UUID */
    pid: string;
  };

  type UploadResult = {
    asset?: AssetSummary;
    duplicate_of?: string;
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

  type VersionOrigin = "manual" | "ai";

  type VoiceContent = {
    catalog?: CatalogVoice;
    instructions?: string;
    kind?: VoiceKind;
    sample?: SampleVoice;
  };

  type VoiceKind = "catalog" | "sample";

  type VoiceParams = {
    emotion?: string;
    language?: string;
    pitch?: number;
    speed?: number;
    volume?: number;
  };

  type WorkflowControlResult = {
    accepted?: boolean;
    action?: string;
    event_id?: string;
    request_id?: string;
    target_id?: string;
    target_type?: string;
  };
}
