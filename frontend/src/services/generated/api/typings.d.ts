declare namespace API {
  type cancelTaskParams = {
    task_id: string;
  };

  type confirmScriptParams = {
    version_id: string;
  };

  type confirmSourceParams = {
    version_id: string;
  };

  type confirmStoryboardParams = {
    version_id: string;
  };

  type CreateProjectRequest = {
    /** Title */
    title: string;
  };

  type createSourceRevisionParams = {
    episode_id: string;
  };

  type CreateSourceRevisionRequest = {
    /** Content */
    content: string;
    /** Rights Basis */
    rights_basis: "original" | "licensed";
    /** Parent Id */
    parent_id: string | null | null;
  };

  type CreativeAssetContentV1 = {
    /** Schema Version */
    schema_version: string | null;
    /** Asset Id */
    asset_id: string;
    /** Asset Type */
    asset_type: "character" | "scene" | "visual_style";
    /** Name */
    name: string;
    /** Description */
    description: string;
  };

  type CreativeAssetListResponse = {
    /** Items */
    items: CreativeAssetVersionResponse[];
  };

  type CreativeAssetVersionResponse = {
    /** Id */
    id: string;
    /** Asset Id */
    asset_id: string;
    /** Episode Id */
    episode_id: string;
    /** Version */
    version: number;
    /** Parent Id */
    parent_id: string | null;
    /** Source Script Version Id */
    source_script_version_id: string;
    content: CreativeAssetContentV1;
    /** Content Hash */
    content_hash: string;
    /** Origin Task Id */
    origin_task_id: string | null;
    /** Status */
    status: "draft" | "confirmed" | "superseded";
    /** Resource Version */
    resource_version: number;
    /** Input Outdated */
    input_outdated: boolean;
    /** Created At */
    created_at: string;
    /** Updated At */
    updated_at: string;
    /** Confirmed At */
    confirmed_at: string | null;
  };

  type deriveScriptDraftParams = {
    version_id: string;
  };

  type deriveStoryboardDraftParams = {
    version_id: string;
  };

  type EpisodeResponse = {
    /** Id */
    id: string;
    /** Project Id */
    project_id: string;
    /** Target Min Ticks */
    target_min_ticks: number;
    /** Target Max Ticks */
    target_max_ticks: number;
    /** Current Source Revision Id */
    current_source_revision_id: string | null;
    /** Created At */
    created_at: string;
    /** Updated At */
    updated_at: string;
  };

  type generateMediaParams = {
    episode_id: string;
  };

  type GenerateMediaRequest = {
    /** Usage Type */
    usage_type: "asset_image" | "shot_image" | "shot_video" | "speech_audio";
    /** Usage Id */
    usage_id: string;
    /** Input Version Id */
    input_version_id: string;
    /** Model Profile Id */
    model_profile_id: string | null | null;
  };

  type generateScriptParams = {
    episode_id: string;
  };

  type generateStoryboardParams = {
    episode_id: string;
  };

  type getCreativeAssetVersionParams = {
    version_id: string;
  };

  type getCurrentScriptParams = {
    episode_id: string;
  };

  type getEpisodeParams = {
    episode_id: string;
  };

  type getProjectParams = {
    project_id: string;
  };

  type getScriptVersionParams = {
    version_id: string;
  };

  type getSourceRevisionParams = {
    version_id: string;
  };

  type getStoryboardParams = {
    episode_id: string;
  };

  type getStoryboardVersionParams = {
    version_id: string;
  };

  type getTaskParams = {
    task_id: string;
  };

  type HTTPValidationError = {
    /** Detail */
    detail: ValidationError[] | null;
  };

  type listCreativeAssetsParams = {
    episode_id: string;
    include_versions: boolean;
  };

  type listScriptVersionsParams = {
    episode_id: string;
  };

  type listSourceRevisionsParams = {
    episode_id: string;
  };

  type listStoryboardVersionsParams = {
    episode_id: string;
  };

  type listTasksParams = {
    episode_id: string;
  };

  type Problem = {
    /** Type */
    type: string;
    /** Title */
    title: string;
    /** Status */
    status: number;
    /** Code */
    code: string;
    /** Retryable */
    retryable: boolean;
    /** Request Id */
    request_id: string;
    /** Detail */
    detail: string | null | null;
    /** Errors */
    errors: ProblemFieldError[] | null | null;
    /** Metadata */
    metadata: Record<string, any> | null | null;
  };

  type ProblemFieldError = {
    /** Field */
    field: string;
    /** Code */
    code: string;
    /** Message */
    message: string;
  };

  type ProductionSpecResponse = {
    /** Aspect Ratio */
    aspect_ratio: string;
    /** Width */
    width: number;
    /** Height */
    height: number;
    /** Fps */
    fps: number;
    /** Timebase */
    timebase: number;
    /** Target Min Ticks */
    target_min_ticks: number;
    /** Target Max Ticks */
    target_max_ticks: number;
  };

  type ProjectDetailResponse = {
    project: ProjectResponse;
    episode: EpisodeResponse;
  };

  type ProjectListResponse = {
    /** Items */
    items: ProjectDetailResponse[];
  };

  type ProjectResponse = {
    /** Id */
    id: string;
    /** Title */
    title: string;
    /** Status */
    status: string;
    production_spec: ProductionSpecResponse;
    /** Created At */
    created_at: string;
    /** Updated At */
    updated_at: string;
  };

  type retryTaskParams = {
    task_id: string;
  };

  type saveCreativeAssetParams = {
    version_id: string;
  };

  type SaveCreativeAssetRequest = {
    content: CreativeAssetContentV1;
  };

  type saveScriptParams = {
    version_id: string;
  };

  type SaveScriptRequest = {
    content: ScriptContentV1;
  };

  type saveStoryboardParams = {
    version_id: string;
  };

  type SaveStoryboardRequest = {
    content: ShotSpecCollectionV1;
  };

  type SceneV1 = {
    /** Scene Id */
    scene_id: string;
    /** Ordinal */
    ordinal: number;
    /** Location */
    location: string;
    /** Time Of Day */
    time_of_day: "dawn" | "day" | "dusk" | "night" | "interior";
    /** Action */
    action: string;
    /** Speech Lines */
    speech_lines: SpeechLineV1[];
  };

  type ScriptContentV1 = {
    /** Schema Version */
    schema_version: string | null;
    /** Title */
    title: string;
    /** Scenes */
    scenes: SceneV1[];
  };

  type ScriptVersionListResponse = {
    /** Items */
    items: ScriptVersionResponse[];
  };

  type ScriptVersionResponse = {
    /** Id */
    id: string;
    /** Episode Id */
    episode_id: string;
    /** Version */
    version: number;
    /** Parent Id */
    parent_id: string | null;
    /** Source Revision Id */
    source_revision_id: string;
    /** Schema Version */
    schema_version: string | null;
    content: ScriptContentV1;
    /** Content Hash */
    content_hash: string;
    /** Origin Task Id */
    origin_task_id: string | null;
    /** Status */
    status: "draft" | "confirmed" | "superseded";
    /** Resource Version */
    resource_version: number;
    /** Input Outdated */
    input_outdated: boolean;
    /** Created At */
    created_at: string;
    /** Updated At */
    updated_at: string;
    /** Confirmed At */
    confirmed_at: string | null;
  };

  type ShotSpecCollectionV1 = {
    /** Schema Version */
    schema_version: string | null;
    /** Script Version Id */
    script_version_id: string;
    /** Asset Version Ids */
    asset_version_ids: string[];
    /** Speech Line Ids */
    speech_line_ids: string[];
    /** Shots */
    shots: ShotV1[];
  };

  type ShotV1 = {
    /** Shot Id */
    shot_id: string;
    /** Ordinal */
    ordinal: number;
    /** Narrative Purpose */
    narrative_purpose: string;
    /** Visual Prompt */
    visual_prompt: string;
    /** Action */
    action: string;
    /** Duration Ticks */
    duration_ticks: number;
    /** Asset Version Ids */
    asset_version_ids: string[];
    /** Speech Line Ids */
    speech_line_ids: string[];
    /** Content Hash */
    content_hash: string;
  };

  type SourceRevisionListResponse = {
    /** Items */
    items: SourceRevisionResponse[];
  };

  type SourceRevisionResponse = {
    /** Id */
    id: string;
    /** Episode Id */
    episode_id: string;
    /** Version */
    version: number;
    /** Parent Id */
    parent_id: string | null;
    /** Content */
    content: string;
    /** Normalization Version */
    normalization_version: string;
    /** Codepoint Count */
    codepoint_count: number;
    /** Sha256 */
    sha256: string;
    /** Rights Basis */
    rights_basis: "original" | "licensed";
    /** Rights Declared At */
    rights_declared_at: string;
    /** Status */
    status: "draft" | "confirmed" | "superseded";
    /** Resource Version */
    resource_version: number;
    /** Created At */
    created_at: string;
    /** Updated At */
    updated_at: string;
    /** Confirmed At */
    confirmed_at: string | null;
  };

  type SpeechLineV1 = {
    /** Speech Line Id */
    speech_line_id: string;
    /** Ordinal */
    ordinal: number;
    /** Kind */
    kind: "dialogue" | "narration";
    /** Text */
    text: string;
    /** Voice Id */
    voice_id:
      | "narrator_female"
      | "narrator_male"
      | "character_young_female"
      | "character_young_male";
    /** Speaker */
    speaker: string | null | null;
  };

  type StoryboardGenerationResponse = {
    /** Assets */
    assets: CreativeAssetVersionResponse[];
    storyboard: StoryboardVersionResponse;
  };

  type StoryboardVersionListResponse = {
    /** Items */
    items: StoryboardVersionResponse[];
  };

  type StoryboardVersionResponse = {
    /** Id */
    id: string;
    /** Episode Id */
    episode_id: string;
    /** Version */
    version: number;
    /** Parent Id */
    parent_id: string | null;
    content: ShotSpecCollectionV1;
    /** Content Hash */
    content_hash: string;
    /** Origin Task Id */
    origin_task_id: string | null;
    /** Status */
    status: "draft" | "confirmed" | "superseded";
    /** Resource Version */
    resource_version: number;
    /** Input Outdated */
    input_outdated: boolean;
    /** Created At */
    created_at: string;
    /** Updated At */
    updated_at: string;
    /** Confirmed At */
    confirmed_at: string | null;
  };

  type TaskAccepted = {
    /** Task Id */
    task_id: string;
    /** Status */
    status: string;
    /** Resource Version */
    resource_version: number;
    /** Status Url */
    status_url: string;
  };

  type TaskError = {
    /** Code */
    code: string;
    /** Retryable */
    retryable: boolean;
    /** Summary */
    summary: string;
  };

  type TaskListResponse = {
    /** Items */
    items: TaskResponse[];
  };

  type TaskProgress = {
    /** Phase */
    phase: string;
    /** Completed */
    completed: number;
    /** Total */
    total: number;
    /** Message */
    message: string | null | null;
  };

  type TaskResponse = {
    /** Id */
    id: string;
    /** Type */
    type:
      | "generate_script"
      | "generate_storyboard"
      | "generate_media"
      | "render_episode";
    scope: TaskScope;
    /** Status */
    status:
      | "queued"
      | "running"
      | "cancelling"
      | "cancelled"
      | "succeeded"
      | "failed"
      | "unknown";
    progress: TaskProgress;
    /** Input Outdated */
    input_outdated: boolean;
    /** Current Attempt Id */
    current_attempt_id: string | null | null;
    /** Result Refs */
    result_refs: TaskResultRef[];
    error: TaskError | null | null;
    /** Resource Version */
    resource_version: number;
    /** Poll After Ms */
    poll_after_ms: number | null;
    /** Created At */
    created_at: string;
    /** Updated At */
    updated_at: string;
    /** Finished At */
    finished_at: string | null | null;
  };

  type TaskResultRef = {
    /** Output Type */
    output_type:
      | "script_version"
      | "creative_asset_version"
      | "shot_spec_version"
      | "generation_candidate"
      | "delivery_version";
    /** Output Id */
    output_id: string;
  };

  type TaskScope = true;

  type ValidationError = {
    /** Location */
    loc: (string | number)[];
    /** Message */
    msg: string;
    /** Error Type */
    type: string;
    /** Input */
    input: any | null;
    /** Context */
    ctx: Record<string, any> | null;
  };
}
