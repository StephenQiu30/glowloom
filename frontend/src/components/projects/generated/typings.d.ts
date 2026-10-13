declare namespace ProjectAPI {
  type applyCanvasCommandsParams = {
    /** 画布UUID */
    id: string;
  };

  type BatchReferenceColumn = {
    id?: string;
    label?: string;
  };

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

  type CreateProjectRequest = {
    aspect_ratio: "9:16" | "16:9" | "1:1";
    description?: string;
    name: string;
    style_key?: string;
    style_preset_id?: string;
    style_subtype?: "anime_jp" | "guofeng_xianxia" | "cartoon_3d" | "manhwa";
    style_type?: "realistic" | "stylized";
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

  type Document = {
    edges?: Edge[];
    id?: string;
    name?: string;
    nodes?: Node[];
    project_id?: string;
    revision?: number;
    scope?: Record<string, any>;
    viewport?: Viewport;
  };

  type Draft = {
    aspect_ratio?: string;
    candidate_count?: number;
    negative_prompt?: string;
    prompt?: string;
  };

  type Edge = {
    binding?: Record<string, any>;
    edge_type?: string;
    id?: string;
    role?: string;
    source_node_id?: string;
    target_node_id?: string;
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

  type getCanvasParams = {
    /** 画布UUID */
    id: string;
  };

  type getProjectParams = {
    /** 项目UUID */
    pid: string;
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

  type ListResponse = {
    items?: ProjectResponse[];
    next_cursor?: string;
  };

  type Move = {
    id?: string;
    x?: number;
    y?: number;
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
    draft?: Draft;
    generation?: GenerationConfig;
    text?: string;
    timeline?: TimelineConfig;
  };

  type Parent = {
    id?: string;
    parent_id?: string;
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

  type Size = {
    height?: number;
    id?: string;
    width?: number;
  };

  type SubtitleStyle = {
    color?: string;
    font_size?: number;
    position?: string;
  };

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

  type Viewport = {
    x?: number;
    y?: number;
    zoom?: number;
  };

  type ZIndex = {
    id?: string;
    z_index?: number;
  };
}
