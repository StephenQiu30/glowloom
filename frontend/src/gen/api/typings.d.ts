declare namespace API {
  type applyCanvasCommandsParams = {
    /** 画布UUID */
    id: string;
  };

  type archiveProjectParams = {
    /** 项目UUID */
    pid: string;
  };

  type cancelMediaExportParams = {
    /** Export UUID */
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

  type confirmBatchQuoteParams = {
    /** Batch UUID */
    id: string;
  };

  type confirmOperationQuoteParams = {
    /** Operation UUID */
    id: string;
  };

  type createCanvasParams = {
    /** 项目UUID */
    pid: string;
  };

  type createFreeQuoteParams = {
    /** 项目UUID */
    pid: string;
  };

  type createGenerationQuotesParams = {
    /** 项目UUID */
    pid: string;
  };

  type createMediaExportParams = {
    /** Project UUID */
    pid: string;
  };

  type deleteCanvasParams = {
    /** 画布UUID */
    id: string;
  };

  type deleteProjectParams = {
    /** 项目UUID */
    pid: string;
  };

  type disableAdminCredentialParams = {
    /** 渠道UUID */
    id: string;
    /** 凭据UUID */
    credential_id: string;
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

  type getAdminModelParams = {
    /** 模型UUID */
    id: string;
  };

  type getAdminProviderParams = {
    /** 渠道UUID */
    id: string;
  };

  type getCanvasParams = {
    /** 画布UUID */
    id: string;
  };

  type getMediaExportParams = {
    /** Export UUID */
    job_id: string;
    /** Project UUID */
    project_id: string;
  };

  type getMediaPreviewParams = {
    /** 项目UUID */
    pid: string;
    /** 媒体UUID */
    asset_id: string;
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

  type getProjectModelDefaultsParams = {
    /** 项目UUID */
    pid: string;
  };

  type getProjectParams = {
    /** 项目UUID */
    pid: string;
  };

  type githubComStephenQiu30LanverseBackendInternalCanvasApplicationCommandResult =
    {
      ok?: boolean;
    };

  type githubComStephenQiu30LanverseBackendInternalCanvasApplicationCommandsInput =
    {
      commands?: githubComStephenQiu30LanverseBackendInternalCanvasDomainCommand[];
      expected_revision?: number;
    };

  type githubComStephenQiu30LanverseBackendInternalCanvasApplicationCreateInput =
    {
      name?: string;
      scope?: Record<string, any>;
    };

  type githubComStephenQiu30LanverseBackendInternalCanvasApplicationDeleteInput =
    {
      expected_revision?: number;
    };

  type githubComStephenQiu30LanverseBackendInternalCanvasApplicationDeleteResult =
    {
      deleted?: boolean;
      id?: string;
      revision?: number;
    };

  type githubComStephenQiu30LanverseBackendInternalCanvasApplicationRenameInput =
    {
      expected_revision?: number;
      name?: string;
    };

  type githubComStephenQiu30LanverseBackendInternalCanvasApplicationResult = {
    edges?: githubComStephenQiu30LanverseBackendInternalCanvasDomainEdge[];
    id?: string;
    name?: string;
    nodes?: githubComStephenQiu30LanverseBackendInternalCanvasDomainNode[];
    project_id?: string;
    results?: githubComStephenQiu30LanverseBackendInternalCanvasApplicationCommandResult[];
    revision?: number;
    scope?: Record<string, any>;
    viewport?: githubComStephenQiu30LanverseBackendInternalCanvasDomainViewport;
  };

  type githubComStephenQiu30LanverseBackendInternalCanvasDomainBatchReferenceColumn =
    {
      id?: string;
      label?: string;
    };

  type githubComStephenQiu30LanverseBackendInternalCanvasDomainBatchTableConfig =
    {
      concurrency?: number;
      global_prompt?: string;
      mode?: string;
      model_profile_id?: string | null;
      operation?: string;
      output_count?: number;
      params?: Record<string, any>;
      reference_columns?: githubComStephenQiu30LanverseBackendInternalCanvasDomainBatchReferenceColumn[];
      rows?: githubComStephenQiu30LanverseBackendInternalCanvasDomainBatchTableRow[];
      version?: number;
    };

  type githubComStephenQiu30LanverseBackendInternalCanvasDomainBatchTableRow = {
    enabled?: boolean;
    id?: string;
    input_node_ids?: (string | null)[];
    prompt?: string;
  };

  type githubComStephenQiu30LanverseBackendInternalCanvasDomainCommand = {
    config?: githubComStephenQiu30LanverseBackendInternalCanvasDomainNodeConfig;
    edges?: githubComStephenQiu30LanverseBackendInternalCanvasDomainEdge[];
    id?: string;
    ids?: string[];
    moves?: githubComStephenQiu30LanverseBackendInternalCanvasDomainMove[];
    names?: githubComStephenQiu30LanverseBackendInternalCanvasDomainName[];
    nodes?: githubComStephenQiu30LanverseBackendInternalCanvasDomainNode[];
    parents?: githubComStephenQiu30LanverseBackendInternalCanvasDomainParent[];
    sizes?: githubComStephenQiu30LanverseBackendInternalCanvasDomainSize[];
    type?: string;
    viewport?: githubComStephenQiu30LanverseBackendInternalCanvasDomainViewport;
    z_indices?: githubComStephenQiu30LanverseBackendInternalCanvasDomainZIndex[];
  };

  type githubComStephenQiu30LanverseBackendInternalCanvasDomainDirectorBoneFrame =
    {
      easing?: string;
      id?: string;
      rotation?: number[];
      time?: number;
    };

  type githubComStephenQiu30LanverseBackendInternalCanvasDomainDirectorBoneTrack =
    {
      bone?: string;
      keyframes?: githubComStephenQiu30LanverseBackendInternalCanvasDomainDirectorBoneFrame[];
    };

  type githubComStephenQiu30LanverseBackendInternalCanvasDomainDirectorCamera =
    {
      aperture?: number;
      far?: number;
      focal_length?: number;
      focus_distance?: number;
      follow_anchor?: number[];
      follow_object_id?: string;
      fov?: number;
      id?: string;
      keyframes?: githubComStephenQiu30LanverseBackendInternalCanvasDomainDirectorKeyframe[];
      look_at_mode?: string;
      look_at_object_id?: string;
      name?: string;
      near?: number;
      target?: number[];
      transform?: githubComStephenQiu30LanverseBackendInternalCanvasDomainDirectorTransform;
    };

  type githubComStephenQiu30LanverseBackendInternalCanvasDomainDirectorConfig =
    {
      active_shot_id?: string;
      aspect_ratio?: string;
      background?: string;
      cameras?: githubComStephenQiu30LanverseBackendInternalCanvasDomainDirectorCamera[];
      cover?: githubComStephenQiu30LanverseBackendInternalCanvasDomainDirectorCover;
      environment_intensity?: number;
      grid_snap?: boolean;
      grid_visible?: boolean;
      ground?: githubComStephenQiu30LanverseBackendInternalCanvasDomainDirectorGround;
      id?: string;
      labels_visible?: boolean;
      lights?: githubComStephenQiu30LanverseBackendInternalCanvasDomainDirectorLight[];
      objects?: githubComStephenQiu30LanverseBackendInternalCanvasDomainDirectorObject[];
      panorama?: githubComStephenQiu30LanverseBackendInternalCanvasDomainDirectorPanorama;
      panorama_radius?: number;
      panorama_rotation?: number;
      shots?: githubComStephenQiu30LanverseBackendInternalCanvasDomainDirectorShot[];
      stage_transform?: githubComStephenQiu30LanverseBackendInternalCanvasDomainDirectorStageTransform;
      title?: string;
      version?: number;
    };

  type githubComStephenQiu30LanverseBackendInternalCanvasDomainDirectorCover = {
    asset_id?: string;
    shot_id?: string;
  };

  type githubComStephenQiu30LanverseBackendInternalCanvasDomainDirectorGround =
    {
      height?: number;
      opacity?: number;
      visible?: boolean;
    };

  type githubComStephenQiu30LanverseBackendInternalCanvasDomainDirectorKeyframe =
    {
      easing?: string;
      id?: string;
      time?: number;
      transform?: githubComStephenQiu30LanverseBackendInternalCanvasDomainDirectorTransform;
    };

  type githubComStephenQiu30LanverseBackendInternalCanvasDomainDirectorLight = {
    angle?: number;
    cast_shadow?: boolean;
    color?: string;
    id?: string;
    intensity?: number;
    name?: string;
    penumbra?: number;
    transform?: githubComStephenQiu30LanverseBackendInternalCanvasDomainDirectorTransform;
    type?: string;
  };

  type githubComStephenQiu30LanverseBackendInternalCanvasDomainDirectorMotionClip =
    {
      duration?: number;
      id?: string;
      loop?: boolean;
      name?: string;
      playback_rate?: number;
      source_animation?: string;
      start?: number;
    };

  type githubComStephenQiu30LanverseBackendInternalCanvasDomainDirectorObject =
    {
      active_motion_clip_id?: string;
      asset_id?: string;
      bone_overrides?: Record<string, any>;
      bone_tracks?: githubComStephenQiu30LanverseBackendInternalCanvasDomainDirectorBoneTrack[];
      builtin_actor?: string;
      cast_shadow?: boolean;
      color?: string;
      id?: string;
      keyframes?: githubComStephenQiu30LanverseBackendInternalCanvasDomainDirectorKeyframe[];
      kind?: string;
      motion_clips?: githubComStephenQiu30LanverseBackendInternalCanvasDomainDirectorMotionClip[];
      name?: string;
      pose?: string;
      primitive?: string;
      receive_shadow?: boolean;
      rig?: githubComStephenQiu30LanverseBackendInternalCanvasDomainDirectorRig;
      source_node_id?: string;
      transform?: githubComStephenQiu30LanverseBackendInternalCanvasDomainDirectorTransform;
      uniform_scale?: number;
      visible?: boolean;
    };

  type githubComStephenQiu30LanverseBackendInternalCanvasDomainDirectorPanorama =
    {
      asset_id?: string;
      name?: string;
      rotation?: number;
    };

  type githubComStephenQiu30LanverseBackendInternalCanvasDomainDirectorRig = {
    animation_names?: string[];
    bone_map?: Record<string, any>;
  };

  type githubComStephenQiu30LanverseBackendInternalCanvasDomainDirectorScreenshot =
    {
      asset_id?: string;
      created_at?: string;
      id?: string;
      name?: string;
    };

  type githubComStephenQiu30LanverseBackendInternalCanvasDomainDirectorShot = {
    camera_id?: string;
    camera_move?: string;
    duration?: number;
    fps?: number;
    id?: string;
    name?: string;
    prompt?: string;
    screenshots?: githubComStephenQiu30LanverseBackendInternalCanvasDomainDirectorScreenshot[];
    shot_size?: string;
  };

  type githubComStephenQiu30LanverseBackendInternalCanvasDomainDirectorStageTransform =
    {
      position?: number[];
      rotation?: number[];
      scale?: number;
    };

  type githubComStephenQiu30LanverseBackendInternalCanvasDomainDirectorTransform =
    {
      position?: number[];
      rotation?: number[];
      scale?: number[];
    };

  type githubComStephenQiu30LanverseBackendInternalCanvasDomainDocument = {
    edges?: githubComStephenQiu30LanverseBackendInternalCanvasDomainEdge[];
    id?: string;
    name?: string;
    nodes?: githubComStephenQiu30LanverseBackendInternalCanvasDomainNode[];
    project_id?: string;
    revision?: number;
    scope?: Record<string, any>;
    viewport?: githubComStephenQiu30LanverseBackendInternalCanvasDomainViewport;
  };

  type githubComStephenQiu30LanverseBackendInternalCanvasDomainEdge = {
    binding?: Record<string, any>;
    edge_type?: string;
    id?: string;
    role?: string;
    source_node_id?: string;
    target_node_id?: string;
  };

  type githubComStephenQiu30LanverseBackendInternalCanvasDomainGenerationConfig =
    {
      capability?: string;
      inputs?: githubComStephenQiu30LanverseBackendInternalCanvasDomainGenerationReference[];
      mode?: string;
      model_profile_id?: string | null;
      output_count?: number;
      params?: Record<string, any>;
      prompt?: string;
      version?: number;
    };

  type githubComStephenQiu30LanverseBackendInternalCanvasDomainGenerationReference =
    {
      media_asset_id?: string;
      role?: string;
    };

  type githubComStephenQiu30LanverseBackendInternalCanvasDomainMove = {
    id?: string;
    x?: number;
    y?: number;
  };

  type githubComStephenQiu30LanverseBackendInternalCanvasDomainName = {
    id?: string;
    title?: string;
  };

  type githubComStephenQiu30LanverseBackendInternalCanvasDomainNode = {
    config?: githubComStephenQiu30LanverseBackendInternalCanvasDomainNodeConfig;
    height?: number;
    id?: string;
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

  type githubComStephenQiu30LanverseBackendInternalCanvasDomainNodeConfig = {
    batch_table?: githubComStephenQiu30LanverseBackendInternalCanvasDomainBatchTableConfig;
    collapsed?: boolean;
    director?: githubComStephenQiu30LanverseBackendInternalCanvasDomainDirectorConfig;
    generation?: githubComStephenQiu30LanverseBackendInternalCanvasDomainGenerationConfig;
    text?: string;
    timeline?: githubComStephenQiu30LanverseBackendInternalCanvasDomainTimelineConfig;
  };

  type githubComStephenQiu30LanverseBackendInternalCanvasDomainParent = {
    id?: string;
    parent_id?: string | null;
  };

  type githubComStephenQiu30LanverseBackendInternalCanvasDomainSize = {
    height?: number;
    id?: string;
    width?: number;
  };

  type githubComStephenQiu30LanverseBackendInternalCanvasDomainSubtitleStyle = {
    color?: string;
    font_size?: number;
    position?: string;
  };

  type githubComStephenQiu30LanverseBackendInternalCanvasDomainTimelineClip = {
    asset_id?: string | null;
    crop?: githubComStephenQiu30LanverseBackendInternalCanvasDomainTimelineCrop | null;
    duration_ms?: number;
    fade_in_ms?: number;
    fade_out_ms?: number;
    id?: string;
    kind?: string;
    node_id?: string | null;
    source_duration_ms?: number | null;
    source_start_ms?: number;
    start_ms?: number;
    text?: string;
    title?: string;
    track_id?: string;
    volume?: number;
  };

  type githubComStephenQiu30LanverseBackendInternalCanvasDomainTimelineConfig =
    {
      aspect_ratio?: string;
      clips?: githubComStephenQiu30LanverseBackendInternalCanvasDomainTimelineClip[];
      fps?: number;
      snapping?: boolean;
      subtitle_style?: githubComStephenQiu30LanverseBackendInternalCanvasDomainSubtitleStyle;
      tracks?: githubComStephenQiu30LanverseBackendInternalCanvasDomainTimelineTrack[];
      version?: number;
    };

  type githubComStephenQiu30LanverseBackendInternalCanvasDomainTimelineCrop = {
    height?: number;
    width?: number;
    x?: number;
    y?: number;
  };

  type githubComStephenQiu30LanverseBackendInternalCanvasDomainTimelineTrack = {
    id?: string;
    kind?: string;
    label?: string;
    locked?: boolean;
    muted?: boolean;
    visible?: boolean;
  };

  type githubComStephenQiu30LanverseBackendInternalCanvasDomainViewport = {
    x?: number;
    y?: number;
    zoom?: number;
  };

  type githubComStephenQiu30LanverseBackendInternalCanvasDomainZIndex = {
    id?: string;
    z_index?: number;
  };

  type githubComStephenQiu30LanverseBackendInternalCatalogApplicationAdminModelDetail =
    {
      model?: githubComStephenQiu30LanverseBackendInternalCatalogApplicationCreatedModel;
      prices?: githubComStephenQiu30LanverseBackendInternalCatalogApplicationAdminPrice[];
      versions?: githubComStephenQiu30LanverseBackendInternalCatalogApplicationAdminModelVersion[];
    };

  type githubComStephenQiu30LanverseBackendInternalCatalogApplicationAdminModelVersion =
    {
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

  type githubComStephenQiu30LanverseBackendInternalCatalogApplicationAdminPrice =
    {
      create_time?: string;
      currency?: string;
      effective_from?: string;
      fx_rate_to_cny?: string | null;
      id?: string;
      rule?: Record<string, any>;
      unit?: string;
      version_no?: number;
    };

  type githubComStephenQiu30LanverseBackendInternalCatalogApplicationCapabilitySummary =
    {
      id?: string;
      input_roles?: string[];
      key?: string;
      modes?: string[];
      output_type?: string;
    };

  type githubComStephenQiu30LanverseBackendInternalCatalogApplicationChangedModelStatus =
    {
      id?: string;
      revision?: number;
      status?: githubComStephenQiu30LanverseBackendInternalCatalogDomainModelStatus;
      update_time?: string;
    };

  type githubComStephenQiu30LanverseBackendInternalCatalogApplicationCreatedModel =
    {
      capability?: string;
      create_time?: string;
      current_version_id?: string;
      display_name?: string;
      id?: string;
      model_key?: string;
      provider_id?: string;
      revision?: number;
      status?: githubComStephenQiu30LanverseBackendInternalCatalogDomainModelStatus;
      update_time?: string;
    };

  type githubComStephenQiu30LanverseBackendInternalCatalogApplicationCreatedProvider =
    {
      adapter_key?: string;
      concurrency_limit?: number;
      create_time?: string;
      id?: string;
      key?: string;
      name?: string;
      rate_limit_per_min?: number;
      region?: githubComStephenQiu30LanverseBackendInternalCatalogDomainRegion;
      revision?: number;
      status?: githubComStephenQiu30LanverseBackendInternalCatalogDomainProviderStatus;
      update_time?: string;
    };

  type githubComStephenQiu30LanverseBackendInternalCatalogApplicationCredentialField =
    {
      name?: string;
      required?: boolean;
      type?: string;
    };

  type githubComStephenQiu30LanverseBackendInternalCatalogApplicationCredentialTestAccepted =
    {
      accepted?: boolean;
      credential_id?: string;
      event_id?: string;
      provider_id?: string;
      test_id?: string;
    };

  type githubComStephenQiu30LanverseBackendInternalCatalogApplicationProviderCredentialSummary =
    {
      create_time?: string;
      id?: string;
      label?: string;
      last4?: string;
      last_test_result?: githubComStephenQiu30LanverseBackendInternalCatalogDomainTestResult;
      last_tested_at?: string;
      provider_id?: string;
      status?: githubComStephenQiu30LanverseBackendInternalCatalogDomainCredentialStatus;
      update_time?: string;
    };

  type githubComStephenQiu30LanverseBackendInternalCatalogApplicationProviderDetail =
    {
      credential?: githubComStephenQiu30LanverseBackendInternalCatalogApplicationProviderCredentialSummary;
      credential_schema?: githubComStephenQiu30LanverseBackendInternalCatalogApplicationCredentialField[];
      provider?: githubComStephenQiu30LanverseBackendInternalCatalogApplicationCreatedProvider;
    };

  type githubComStephenQiu30LanverseBackendInternalCatalogApplicationProviderListItem =
    {
      credential?: githubComStephenQiu30LanverseBackendInternalCatalogApplicationProviderCredentialSummary;
      model_count?: number;
      provider?: githubComStephenQiu30LanverseBackendInternalCatalogApplicationCreatedProvider;
    };

  type githubComStephenQiu30LanverseBackendInternalCatalogApplicationPublishedModelVersion =
    {
      create_time?: string;
      id?: string;
      model_id?: string;
      revision?: number;
      version_no?: number;
    };

  type githubComStephenQiu30LanverseBackendInternalCatalogApplicationPublishedPriceRule =
    {
      create_time?: string;
      id?: string;
      model_id?: string;
      revision?: number;
      version_no?: number;
    };

  type githubComStephenQiu30LanverseBackendInternalCatalogApplicationSavedCredential =
    {
      create_time?: string;
      id?: string;
      label?: string;
      last4?: string;
      last_test_result?: githubComStephenQiu30LanverseBackendInternalCatalogDomainTestResult;
      provider_id?: string;
      status?: githubComStephenQiu30LanverseBackendInternalCatalogDomainCredentialStatus;
      update_time?: string;
    };

  type githubComStephenQiu30LanverseBackendInternalCatalogDomainCredentialStatus =
    "active" | "disabled";

  type githubComStephenQiu30LanverseBackendInternalCatalogDomainModelStatus =
    "active" | "disabled";

  type githubComStephenQiu30LanverseBackendInternalCatalogDomainModeration =
    "provider" | "platform" | "both";

  type githubComStephenQiu30LanverseBackendInternalCatalogDomainPriceUnit =
    | "per_image"
    | "per_second"
    | "per_request"
    | "per_1k_tokens"
    | "per_1k_chars";

  type githubComStephenQiu30LanverseBackendInternalCatalogDomainProviderStatus =
    "active" | "disabled";

  type githubComStephenQiu30LanverseBackendInternalCatalogDomainRegion =
    "domestic" | "overseas";

  type githubComStephenQiu30LanverseBackendInternalCatalogDomainTestResult =
    "ok" | "auth_failed" | "unreachable" | "timeout" | "unsupported";

  type githubComStephenQiu30LanverseBackendInternalMediaApplicationAssetPage = {
    items?: githubComStephenQiu30LanverseBackendInternalMediaApplicationAssetSummary[];
    next_cursor?: string;
  };

  type githubComStephenQiu30LanverseBackendInternalMediaApplicationAssetSummary =
    {
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

  type githubComStephenQiu30LanverseBackendInternalMediaApplicationPreview = {
    asset?: githubComStephenQiu30LanverseBackendInternalMediaApplicationAssetSummary;
    expires_at?: string;
    url?: string;
  };

  type githubComStephenQiu30LanverseBackendInternalMediaApplicationUploadResult =
    {
      asset?: githubComStephenQiu30LanverseBackendInternalMediaApplicationAssetSummary;
      duplicate_of?: string;
    };

  type githubComStephenQiu30LanverseBackendInternalMediatoolApplicationControlInput =
    {
      project_id?: string;
      revision?: number;
    };

  type githubComStephenQiu30LanverseBackendInternalMediatoolApplicationCreateInput =
    {
      canvas_id?: string;
      node_id?: string;
      output_kind?: "video" | "audio";
      revision?: number;
    };

  type githubComStephenQiu30LanverseBackendInternalMediatoolApplicationExportPage =
    {
      items?: githubComStephenQiu30LanverseBackendInternalMediatoolDomainExportJob[];
      next_cursor?: string | null;
    };

  type githubComStephenQiu30LanverseBackendInternalMediatoolApplicationExportPreview =
    {
      asset?: githubComStephenQiu30LanverseBackendInternalMediaApplicationAssetSummary;
      expires_at?: string;
      job_id?: string;
      revision?: number;
      sha256?: string;
      url?: string;
      waveform?: githubComStephenQiu30LanverseBackendInternalMediatoolApplicationExportWaveformPreview;
    };

  type githubComStephenQiu30LanverseBackendInternalMediatoolApplicationExportWaveformPreview =
    {
      expires_at?: string;
      height?: number;
      url?: string;
      width?: number;
    };

  type githubComStephenQiu30LanverseBackendInternalMediatoolApplicationReviewInput =
    {
      local_review_confirmed?: boolean;
      project_id?: string;
      revision?: number;
      sha256?: string;
    };

  type githubComStephenQiu30LanverseBackendInternalMediatoolDomainExportJob = {
    asset_id?: string | null;
    attempt?: number;
    created_at?: string;
    failure_code?: string | null;
    id?: string;
    output_kind?: "video" | "audio";
    progress?: number;
    project_id?: string;
    revision?: number;
    sha256?: string | null;
    source?: githubComStephenQiu30LanverseBackendInternalMediatoolDomainSource;
    stage?: string;
    status?: githubComStephenQiu30LanverseBackendInternalMediatoolDomainStatus;
    updated_at?: string;
  };

  type githubComStephenQiu30LanverseBackendInternalMediatoolDomainOutputKind =
    "video" | "audio";

  type githubComStephenQiu30LanverseBackendInternalMediatoolDomainSource = {
    canvas_id?: string;
    node_id?: string;
    revision?: number;
  };

  type githubComStephenQiu30LanverseBackendInternalMediatoolDomainStatus =
    | "queued"
    | "running"
    | "review_required"
    | "succeeded"
    | "failed"
    | "cancel_requested"
    | "cancelled";

  type githubComStephenQiu30LanverseBackendInternalOperationApplicationBatchDetail =
    {
      cancel_requested_at?: string | null;
      failed_count?: number;
      id?: string;
      items?: githubComStephenQiu30LanverseBackendInternalOperationApplicationTaskSummary[];
      kind?: string;
      paused_reason?: string | null;
      project_id?: string;
      quote_total_micros?: number;
      status?: githubComStephenQiu30LanverseBackendInternalOperationDomainBatchStatus;
      succeeded_count?: number;
      total_count?: number;
      unknown_count?: number;
    };

  type githubComStephenQiu30LanverseBackendInternalOperationApplicationBatchFreeQuoteItemResult =
    {
      error_code?: string;
      final_prompt?: string;
      mode?: string;
      model_key?: string;
      operation_id?: string;
      prompt_preparation?: githubComStephenQiu30LanverseBackendInternalOperationDomainPromptPreparation;
      quote_detail?: number[];
      quote_micros?: number;
      region?: string;
      reused_from_id?: string;
    };

  type githubComStephenQiu30LanverseBackendInternalOperationApplicationCreateBatchFreeQuoteResult =
    {
      available_micros?: number;
      batch_id?: string;
      confirmable?: boolean;
      expires_at?: string;
      items?: githubComStephenQiu30LanverseBackendInternalOperationApplicationBatchFreeQuoteItemResult[];
      total_micros?: number;
    };

  type githubComStephenQiu30LanverseBackendInternalOperationApplicationTaskDetail =
    {
      batch_id?: string | null;
      cancel_requested?: boolean;
      capability?: string;
      create_time?: string;
      events?: githubComStephenQiu30LanverseBackendInternalOperationApplicationTaskEvent[];
      failure_code?: string | null;
      finished_at?: string | null;
      id?: string;
      inputs?: githubComStephenQiu30LanverseBackendInternalOperationApplicationTaskInput[];
      mode?: string;
      model_key?: string;
      model_name?: string;
      origin?: string;
      output_count?: number;
      outputs?: githubComStephenQiu30LanverseBackendInternalOperationApplicationTaskOutput[];
      params?: Record<string, any>;
      project_id?: string;
      prompt_preparation?: githubComStephenQiu30LanverseBackendInternalOperationDomainPromptPreparation;
      quote_detail?: Record<string, any>;
      quote_expires_at?: string | null;
      quote_micros?: number | null;
      retryable?: boolean | null;
      reused_from_id?: string | null;
      settled_micros?: number | null;
      source?: githubComStephenQiu30LanverseBackendInternalOperationDomainCanvasSource | null;
      started_at?: string | null;
      status?: githubComStephenQiu30LanverseBackendInternalOperationDomainStatus;
      target_id?: string | null;
      target_type?: string;
      update_time?: string;
    };

  type githubComStephenQiu30LanverseBackendInternalOperationApplicationTaskEvent =
    {
      create_time?: string;
      from_status?: string | null;
      id?: string;
      reason?: string | null;
      to_status?: string;
    };

  type githubComStephenQiu30LanverseBackendInternalOperationApplicationTaskInput =
    {
      mask_asset_id?: string | null;
      media_asset_id?: string | null;
      role?: string;
      sequence?: number;
      text?: string | null;
    };

  type githubComStephenQiu30LanverseBackendInternalOperationApplicationTaskOutput =
    {
      create_time?: string;
      id?: string;
      kind?: string;
      media_asset_id?: string | null;
      moderation_status?: string;
      sequence?: number;
    };

  type githubComStephenQiu30LanverseBackendInternalOperationApplicationTaskSummary =
    {
      batch_id?: string | null;
      cancel_requested?: boolean;
      capability?: string;
      create_time?: string;
      failure_code?: string | null;
      finished_at?: string | null;
      id?: string;
      mode?: string;
      model_key?: string;
      model_name?: string;
      origin?: string;
      project_id?: string;
      quote_micros?: number | null;
      retryable?: boolean | null;
      settled_micros?: number | null;
      source?: githubComStephenQiu30LanverseBackendInternalOperationDomainCanvasSource | null;
      started_at?: string | null;
      status?: githubComStephenQiu30LanverseBackendInternalOperationDomainStatus;
      target_id?: string | null;
      target_type?: string;
      update_time?: string;
    };

  type githubComStephenQiu30LanverseBackendInternalOperationApplicationWorkflowControlResult =
    {
      accepted?: boolean;
      action?: string;
      event_id?: string;
      request_id?: string;
      target_id?: string;
      target_type?: string;
    };

  type githubComStephenQiu30LanverseBackendInternalOperationDomainBatchStatus =
    "quoted" | "confirmed" | "running" | "finished" | "cancelled" | "expired";

  type githubComStephenQiu30LanverseBackendInternalOperationDomainCanvasSource =
    {
      canvas_id?: string;
      node_id?: string;
      revision?: number;
      row_id?: string | null;
    };

  type githubComStephenQiu30LanverseBackendInternalOperationDomainPromptPreparation =
    {
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

  type githubComStephenQiu30LanverseBackendInternalOperationDomainStatus =
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

  type githubComStephenQiu30LanverseBackendInternalPlatformHttpapiProblem = {
    code?: string;
    detail?: string;
    meta?: Record<string, any>;
    request_id?: string;
    status?: number;
    title?: string;
    type?: string;
  };

  type githubComStephenQiu30LanverseBackendInternalPromptApplicationOutlineOptions =
    {
      chapter_count: "3" | "5" | "8" | "10";
      chapter_length: "短" | "中" | "长";
      character_scale: "2 个" | "3-4 个" | "5-6 个";
      perspective: "第三人称" | "第一人称" | "多视角";
      structure: "单线推进" | "双线并行" | "群像多线" | "反转嵌套";
      tone: "平稳叙事" | "轻松喜剧" | "紧张悬疑" | "热血成长" | "甜宠治愈";
      word_count: "500" | "800" | "1200" | "2000";
    };

  type githubComStephenQiu30LanverseBackendInternalPromptApplicationPreference =
    {
      customization?: githubComStephenQiu30LanverseBackendInternalPromptDomainCustomization;
      definition?: githubComStephenQiu30LanverseBackendInternalPromptDomainDefinition;
      outdated?: boolean;
    };

  type githubComStephenQiu30LanverseBackendInternalPromptApplicationTemplateRequest =
    {
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
      outline?: githubComStephenQiu30LanverseBackendInternalPromptApplicationOutlineOptions;
    };

  type githubComStephenQiu30LanverseBackendInternalPromptDomainCustomization = {
    base_template_id?: string;
    content?: string;
    id?: string;
    mode?: githubComStephenQiu30LanverseBackendInternalPromptDomainMode;
    operation?: string;
    revision?: number;
    update_time?: string;
  };

  type githubComStephenQiu30LanverseBackendInternalPromptDomainDefinition = {
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
    variables?: githubComStephenQiu30LanverseBackendInternalPromptDomainVariable[];
  };

  type githubComStephenQiu30LanverseBackendInternalPromptDomainMode =
    "inherit" | "append" | "rewrite";

  type githubComStephenQiu30LanverseBackendInternalPromptDomainVariable = {
    label?: string;
    placeholder?: string;
  };

  type githubComStephenQiu30LanverseBackendInternalWorkspaceApplicationCreatedProject =
    {
      allow_overseas_models?: boolean;
      aspect_ratio?: string;
      create_time?: string;
      description?: string;
      id?: string;
      name?: string;
      org_id?: string;
      resolution?: string;
      revision?: number;
      status?: string;
      style_preset_id?: string;
      style_subtype?: string;
      style_type?: string;
      update_time?: string;
    };

  type githubComStephenQiu30LanverseBackendInternalWorkspaceApplicationModelDefaults =
    {
      default_models?: Record<string, any>;
      project_id?: string;
      revision?: number;
    };

  type githubComStephenQiu30LanverseBackendInternalWorkspaceApplicationStylePresetSummary =
    {
      id?: string;
      name?: string;
      style_subtype?: string;
      style_type?: string;
    };

  type internalCanvasAdapterHttpListResponse = {
    items?: githubComStephenQiu30LanverseBackendInternalCanvasDomainDocument[];
    next_cursor?: string;
  };

  type internalCatalogAdapterHttpAdminModelPage = {
    items?: githubComStephenQiu30LanverseBackendInternalCatalogApplicationCreatedModel[];
    next_cursor?: string | null;
  };

  type internalCatalogAdapterHttpCreateModelRequest = {
    capability?: string;
    display_name?: string;
    model_key?: string;
    provider_id?: string;
  };

  type internalCatalogAdapterHttpCreateProviderRequest = {
    adapter_key?: string;
    concurrency_limit?: number;
    key?: string;
    name?: string;
    rate_limit_per_min?: number;
    region?: githubComStephenQiu30LanverseBackendInternalCatalogDomainRegion;
  };

  type internalCatalogAdapterHttpModelPage = {
    items?: internalCatalogAdapterHttpModelResponse[];
    next_cursor?: string | null;
  };

  type internalCatalogAdapterHttpModelPrice = {
    currency?: string;
    effective_from?: string;
    fx_rate_to_cny?: string | null;
    id?: string;
    rule?: Record<string, any>;
    unit?: string;
    version_no?: number;
  };

  type internalCatalogAdapterHttpModelResponse = {
    capability?: string;
    credential_present?: boolean;
    credential_test_result?: string | null;
    current_price?: internalCatalogAdapterHttpModelPrice | null;
    current_version?: internalCatalogAdapterHttpModelVersion | null;
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

  type internalCatalogAdapterHttpModelVersion = {
    id?: string;
    limits?: Record<string, any>;
    modes?: string[];
    param_schema?: Record<string, any>[];
    version_no?: number;
  };

  type internalCatalogAdapterHttpProviderPage = {
    items?: githubComStephenQiu30LanverseBackendInternalCatalogApplicationProviderListItem[];
    next_cursor?: string | null;
  };

  type internalCatalogAdapterHttpPublishPriceRequest = {
    currency?: string;
    effective_from?: string;
    expected_revision?: number;
    fx_rate_to_cny?: string;
    rule?: Record<string, any>;
    unit?: githubComStephenQiu30LanverseBackendInternalCatalogDomainPriceUnit;
    version_no?: number;
  };

  type internalCatalogAdapterHttpPublishVersionRequest = {
    expected_max_ms?: number;
    expected_revision?: number;
    limits?: Record<string, any>;
    moderation?: githubComStephenQiu30LanverseBackendInternalCatalogDomainModeration;
    modes?: string[];
    param_schema?: Record<string, any>[];
    provider_model_id?: string;
    queue?: string;
    supports_callback?: boolean;
    supports_cancel?: boolean;
    supports_query?: boolean;
    version_no?: number;
  };

  type internalCatalogAdapterHttpSetCredentialRequest = {
    label?: string;
    secret?: Record<string, any>;
  };

  type internalCatalogAdapterHttpSetModelStatusRequest = {
    expected_revision?: number;
    status?: githubComStephenQiu30LanverseBackendInternalCatalogDomainModelStatus;
  };

  type internalCatalogAdapterHttpUpdateProviderRequest = {
    concurrency_limit?: number;
    expected_revision?: number;
    rate_limit_per_min?: number;
    status?: githubComStephenQiu30LanverseBackendInternalCatalogDomainProviderStatus;
  };

  type internalMediatoolAdapterHttpExportJobResponse = {
    asset_id?: string | null;
    attempt?: number;
    created_at?: string;
    failure_code?: string | null;
    id?: string;
    output_kind?: "video" | "audio";
    progress?: number;
    project_id?: string;
    revision?: number;
    sha256?: string | null;
    source?: githubComStephenQiu30LanverseBackendInternalMediatoolDomainSource;
    stage?: string;
    status?: githubComStephenQiu30LanverseBackendInternalMediatoolDomainStatus;
    updated_at?: string;
  };

  type internalOperationAdapterHttpConfirmBatchItem = {
    operation_id?: string;
    reasons?: string[];
    status?: githubComStephenQiu30LanverseBackendInternalOperationDomainStatus;
  };

  type internalOperationAdapterHttpConfirmBatchRequest = {
    exclude_operation_ids?: string[];
    project_id?: string;
  };

  type internalOperationAdapterHttpConfirmBatchResponse = {
    available_micros?: number;
    budget_revision?: number;
    confirmed_count?: number;
    items?: internalOperationAdapterHttpConfirmBatchItem[];
    quote_total_micros?: number;
  };

  type internalOperationAdapterHttpConfirmResponse = {
    available_micros?: number;
    budget_revision?: number;
    reservation_id?: string;
  };

  type internalOperationAdapterHttpProjectRequest = {
    project_id?: string;
  };

  type internalOperationAdapterHttpQuoteItemRequest = {
    capability?: string;
    force_regenerate?: boolean;
    media_inputs?: internalOperationAdapterHttpQuoteMediaInput[];
    mode?: string;
    model_key?: string;
    output_count?: number;
    params?: Record<string, any>;
    prompt?: string;
    prompt_template?: githubComStephenQiu30LanverseBackendInternalPromptApplicationTemplateRequest;
    source?: githubComStephenQiu30LanverseBackendInternalOperationDomainCanvasSource | null;
  };

  type internalOperationAdapterHttpQuoteMediaInput = {
    media_asset_id?: string;
    role?: string;
  };

  type internalOperationAdapterHttpQuoteResponse = {
    available_micros?: number;
    confirmable?: boolean;
    expires_at?: string;
    final_prompt?: string;
    operation_id?: string;
    prompt_preparation?: githubComStephenQiu30LanverseBackendInternalOperationDomainPromptPreparation;
    quote_detail?: Record<string, any>;
    quote_micros?: number;
    reused_from_id?: string | null;
  };

  type internalOperationAdapterHttpQuotesRequest = {
    items?: internalOperationAdapterHttpQuoteItemRequest[];
  };

  type internalOperationAdapterHttpTaskPageResponse = {
    items?: githubComStephenQiu30LanverseBackendInternalOperationApplicationTaskSummary[];
    next_cursor?: string | null;
  };

  type internalPromptAdapterHttpPreferencesResponse = {
    items?: githubComStephenQiu30LanverseBackendInternalPromptApplicationPreference[];
  };

  type internalPromptAdapterHttpSaveRequest = {
    base_template_id: string;
    content?: string;
    expected_revision: number;
    mode: githubComStephenQiu30LanverseBackendInternalPromptDomainMode;
  };

  type internalWorkspaceAdapterHttpCreateProjectRequest = {
    aspect_ratio: "9:16" | "16:9";
    description?: string;
    name: string;
    style_preset_id?: string;
    style_subtype?: "anime_jp" | "guofeng_xianxia" | "cartoon_3d" | "manhwa";
    style_type: "realistic" | "stylized";
  };

  type internalWorkspaceAdapterHttpListResponse = {
    items?: internalWorkspaceAdapterHttpProjectResponse[];
    next_cursor?: string | null;
  };

  type internalWorkspaceAdapterHttpModelDefaultsRequest = {
    default_models: Record<string, any>;
    expected_revision: number;
  };

  type internalWorkspaceAdapterHttpProjectDetailResponse = {
    allow_overseas_models?: boolean;
    archived_at?: string | null;
    aspect_ratio?: string;
    create_time?: string;
    default_models?: Record<string, any>;
    delete_time?: string | null;
    description?: string;
    id?: string;
    is_delete?: boolean;
    name?: string;
    purge_after?: string | null;
    resolution?: string;
    revision?: number;
    status?: string;
    style_preset_id?: string | null;
    style_subtype?: string;
    style_type?: string;
    update_time?: string;
  };

  type internalWorkspaceAdapterHttpProjectResponse = {
    archived_at?: string | null;
    aspect_ratio?: string;
    delete_time?: string | null;
    id?: string;
    is_delete?: boolean;
    name?: string;
    purge_after?: string | null;
    revision?: number;
    status?: string;
    style_type?: string;
  };

  type internalWorkspaceAdapterHttpProjectTransitionRequest = {
    expected_revision: number;
  };

  type internalWorkspaceAdapterHttpProjectUpdateRequest = {
    allow_overseas_models?: boolean;
    description?: string;
    expected_revision: number;
    name?: string;
    style_preset_id?: string;
  };

  type internalWorkspaceAdapterHttpStylePresetListResponse = {
    items?: githubComStephenQiu30LanverseBackendInternalWorkspaceApplicationStylePresetSummary[];
    next_cursor?: string | null;
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

  type listCanvasesParams = {
    /** 项目UUID */
    pid: string;
  };

  type listMediaAssetsParams = {
    /** 项目UUID */
    pid: string;
    /** image/video/audio/model */
    kind?: string;
    /** 不透明游标 */
    cursor?: string;
    /** 1..200，默认50 */
    limit?: number;
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

  type previewMediaExportParams = {
    /** Export UUID */
    job_id: string;
    /** Project UUID */
    project_id: string;
  };

  type publishAdminModelPriceParams = {
    /** 模型UUID */
    id: string;
  };

  type publishAdminModelVersionParams = {
    /** 模型UUID */
    id: string;
  };

  type renameCanvasParams = {
    /** 画布UUID */
    id: string;
  };

  type restoreProjectParams = {
    /** 项目UUID */
    pid: string;
  };

  type resumeOperationBatchParams = {
    /** Batch UUID */
    id: string;
  };

  type retryMediaExportParams = {
    /** Export UUID */
    job_id: string;
  };

  type reviewMediaExportParams = {
    /** Export UUID */
    job_id: string;
  };

  type saveProjectModelDefaultsParams = {
    /** 项目UUID */
    pid: string;
  };

  type savePromptPreferenceParams = {
    /** 只读目录中的操作标识 */
    operation: string;
  };

  type setAdminCredentialParams = {
    /** 渠道UUID */
    id: string;
  };

  type setAdminModelStatusParams = {
    /** 模型UUID */
    id: string;
  };

  type testAdminCredentialParams = {
    /** 渠道UUID */
    id: string;
    /** 凭据UUID */
    credential_id: string;
  };

  type unarchiveProjectParams = {
    /** 项目UUID */
    pid: string;
  };

  type updateAdminProviderParams = {
    /** 渠道UUID */
    id: string;
  };

  type updateProjectParams = {
    /** 项目UUID */
    pid: string;
  };

  type uploadMediaAssetParams = {
    /** 项目UUID */
    pid: string;
  };
}
