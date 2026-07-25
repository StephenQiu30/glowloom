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
    /** Parent Id */
    parent_id: string | null | null;
    /** Rights Basis */
    rights_basis: "original" | "licensed";
  };

  type CreativeAssetContentV1 = {
    /** Asset Id */
    asset_id: string;
    /** Asset Type */
    asset_type: "character" | "scene" | "visual_style";
    /** Description */
    description: string;
    /** Name */
    name: string;
    /** Schema Version */
    schema_version: string | null;
  };

  type CreativeAssetListResponse = {
    /** Items */
    items: CreativeAssetVersionResponse[];
  };

  type CreativeAssetVersionResponse = {
    /** Asset Id */
    asset_id: string;
    /** Confirmed At */
    confirmed_at: string | null;
    content: CreativeAssetContentV1;
    /** Content Hash */
    content_hash: string;
    /** Created At */
    created_at: string;
    /** Episode Id */
    episode_id: string;
    /** Id */
    id: string;
    /** Input Outdated */
    input_outdated: boolean;
    /** Origin Task Id */
    origin_task_id: string | null;
    /** Parent Id */
    parent_id: string | null;
    /** Resource Version */
    resource_version: number;
    /** Source Script Version Id */
    source_script_version_id: string;
    /** Status */
    status: "draft" | "confirmed" | "superseded";
    /** Updated At */
    updated_at: string;
    /** Version */
    version: number;
  };

  type deriveScriptDraftParams = {
    version_id: string;
  };

  type deriveStoryboardDraftParams = {
    version_id: string;
  };

  type EpisodeResponse = {
    /** Created At */
    created_at: string;
    /** Current Source Revision Id */
    current_source_revision_id: string | null;
    /** Id */
    id: string;
    /** Project Id */
    project_id: string;
    /** Target Max Ticks */
    target_max_ticks: number;
    /** Target Min Ticks */
    target_min_ticks: number;
    /** Updated At */
    updated_at: string;
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
    /** Code */
    code: string;
    /** Detail */
    detail: string | null | null;
    /** Errors */
    errors: ProblemFieldError[] | null | null;
    /** Metadata */
    metadata: Record<string, any> | null | null;
    /** Request Id */
    request_id: string;
    /** Retryable */
    retryable: boolean;
    /** Status */
    status: number;
    /** Title */
    title: string;
    /** Type */
    type: string;
  };

  type ProblemFieldError = {
    /** Code */
    code: string;
    /** Field */
    field: string;
    /** Message */
    message: string;
  };

  type ProductionSpecResponse = {
    /** Aspect Ratio */
    aspect_ratio: string;
    /** Fps */
    fps: number;
    /** Height */
    height: number;
    /** Target Max Ticks */
    target_max_ticks: number;
    /** Target Min Ticks */
    target_min_ticks: number;
    /** Timebase */
    timebase: number;
    /** Width */
    width: number;
  };

  type ProjectDetailResponse = {
    episode: EpisodeResponse;
    project: ProjectResponse;
  };

  type ProjectListResponse = {
    /** Items */
    items: ProjectDetailResponse[];
  };

  type ProjectResponse = {
    /** Created At */
    created_at: string;
    /** Id */
    id: string;
    production_spec: ProductionSpecResponse;
    /** Status */
    status: string;
    /** Title */
    title: string;
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
    /** Action */
    action: string;
    /** Location */
    location: string;
    /** Ordinal */
    ordinal: number;
    /** Scene Id */
    scene_id: string;
    /** Speech Lines */
    speech_lines: SpeechLineV1[];
    /** Time Of Day */
    time_of_day: "dawn" | "day" | "dusk" | "night" | "interior";
  };

  type ScriptContentV1 = {
    /** Scenes */
    scenes: SceneV1[];
    /** Schema Version */
    schema_version: string | null;
    /** Title */
    title: string;
  };

  type ScriptVersionListResponse = {
    /** Items */
    items: ScriptVersionResponse[];
  };

  type ScriptVersionResponse = {
    /** Confirmed At */
    confirmed_at: string | null;
    content: ScriptContentV1;
    /** Content Hash */
    content_hash: string;
    /** Created At */
    created_at: string;
    /** Episode Id */
    episode_id: string;
    /** Id */
    id: string;
    /** Input Outdated */
    input_outdated: boolean;
    /** Origin Task Id */
    origin_task_id: string | null;
    /** Parent Id */
    parent_id: string | null;
    /** Resource Version */
    resource_version: number;
    /** Schema Version */
    schema_version: string | null;
    /** Source Revision Id */
    source_revision_id: string;
    /** Status */
    status: "draft" | "confirmed" | "superseded";
    /** Updated At */
    updated_at: string;
    /** Version */
    version: number;
  };

  type ShotSpecCollectionV1 = {
    /** Asset Version Ids */
    asset_version_ids: string[];
    /** Schema Version */
    schema_version: string | null;
    /** Script Version Id */
    script_version_id: string;
    /** Shots */
    shots: ShotV1[];
    /** Speech Line Ids */
    speech_line_ids: string[];
  };

  type ShotV1 = {
    /** Action */
    action: string;
    /** Asset Version Ids */
    asset_version_ids: string[];
    /** Content Hash */
    content_hash: string;
    /** Duration Ticks */
    duration_ticks: number;
    /** Narrative Purpose */
    narrative_purpose: string;
    /** Ordinal */
    ordinal: number;
    /** Shot Id */
    shot_id: string;
    /** Speech Line Ids */
    speech_line_ids: string[];
    /** Visual Prompt */
    visual_prompt: string;
  };

  type SourceRevisionListResponse = {
    /** Items */
    items: SourceRevisionResponse[];
  };

  type SourceRevisionResponse = {
    /** Codepoint Count */
    codepoint_count: number;
    /** Confirmed At */
    confirmed_at: string | null;
    /** Content */
    content: string;
    /** Created At */
    created_at: string;
    /** Episode Id */
    episode_id: string;
    /** Id */
    id: string;
    /** Normalization Version */
    normalization_version: string;
    /** Parent Id */
    parent_id: string | null;
    /** Resource Version */
    resource_version: number;
    /** Rights Basis */
    rights_basis: "original" | "licensed";
    /** Rights Declared At */
    rights_declared_at: string;
    /** Sha256 */
    sha256: string;
    /** Status */
    status: "draft" | "confirmed" | "superseded";
    /** Updated At */
    updated_at: string;
    /** Version */
    version: number;
  };

  type SpeechLineV1 = {
    /** Kind */
    kind: "dialogue" | "narration";
    /** Ordinal */
    ordinal: number;
    /** Speaker */
    speaker: string | null | null;
    /** Speech Line Id */
    speech_line_id: string;
    /** Text */
    text: string;
    /** Voice Id */
    voice_id:
      | "narrator_female"
      | "narrator_male"
      | "character_young_female"
      | "character_young_male";
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
    /** Confirmed At */
    confirmed_at: string | null;
    content: ShotSpecCollectionV1;
    /** Content Hash */
    content_hash: string;
    /** Created At */
    created_at: string;
    /** Episode Id */
    episode_id: string;
    /** Id */
    id: string;
    /** Input Outdated */
    input_outdated: boolean;
    /** Origin Task Id */
    origin_task_id: string | null;
    /** Parent Id */
    parent_id: string | null;
    /** Resource Version */
    resource_version: number;
    /** Status */
    status: "draft" | "confirmed" | "superseded";
    /** Updated At */
    updated_at: string;
    /** Version */
    version: number;
  };

  type TaskAccepted = {
    /** Resource Version */
    resource_version: number;
    /** Status */
    status: string;
    /** Status Url */
    status_url: string;
    /** Task Id */
    task_id: string;
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
    /** Completed */
    completed: number;
    /** Message */
    message: string | null | null;
    /** Phase */
    phase: string;
    /** Total */
    total: number;
  };

  type TaskResponse = {
    /** Created At */
    created_at: string;
    /** Current Attempt Id */
    current_attempt_id: string | null | null;
    error: TaskError | null | null;
    /** Finished At */
    finished_at: string | null | null;
    /** Id */
    id: string;
    /** Input Outdated */
    input_outdated: boolean;
    /** Poll After Ms */
    poll_after_ms: number | null;
    progress: TaskProgress;
    /** Resource Version */
    resource_version: number;
    /** Result Refs */
    result_refs: TaskResultRef[];
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
    /** Type */
    type:
      | "generate_script"
      | "generate_storyboard"
      | "generate_media"
      | "render_episode";
    /** Updated At */
    updated_at: string;
  };

  type TaskResultRef = {
    /** Output Id */
    output_id: string;
    /** Output Type */
    output_type:
      | "script_version"
      | "creative_asset_version"
      | "shot_spec_version"
      | "generation_candidate"
      | "delivery_version";
  };

  type TaskScope = true;

  type ValidationError = {
    /** Context */
    ctx: Record<string, any> | null;
    /** Input */
    input: any | null;
    /** Location */
    loc: (string | number)[];
    /** Message */
    msg: string;
    /** Error Type */
    type: string;
  };
}
