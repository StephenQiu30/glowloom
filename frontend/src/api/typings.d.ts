declare namespace API {
  type AdoptCandidateRequest = {
    /** Usage Type */
    usage_type: "asset_image" | "shot_image" | "shot_video" | "speech_audio";
    /** Usage Id */
    usage_id: string;
    /** Input Version Id */
    input_version_id: string;
    /** Input Hash */
    input_hash: string;
    /** Candidate Id */
    candidate_id: string;
  };

  type AdoptionResponse = {
    /** Id */
    id: string;
    /** Episode Id */
    episode_id: string;
    /** Usage Type */
    usage_type: "asset_image" | "shot_image" | "shot_video" | "speech_audio";
    /** Usage Id */
    usage_id: string;
    /** Input Version Id */
    input_version_id: string;
    /** Input Hash */
    input_hash: string;
    /** Version */
    version: number;
    /** Candidate Id */
    candidate_id: string;
    /** Supersedes Id */
    supersedes_id: string | null;
    /** Status */
    status: "active" | "superseded";
    /** Created At */
    created_at: string;
    /** Superseded At */
    superseded_at: string | null;
  };

  type authorizeCandidatePreviewParams = {
    media_version_id: string;
  };

  type authorizeDownloadParams = {
    delivery_id: string;
  };

  type cancelTaskParams = {
    task_id: string;
  };

  type CandidateListResponse = {
    /** Items */
    items: CandidateResponse[];
  };

  type CandidateResponse = {
    /** Id */
    id: string;
    /** Episode Id */
    episode_id: string;
    /** Task Id */
    task_id: string;
    /** Attempt Id */
    attempt_id: string;
    /** Output Slot */
    output_slot: string;
    /** Usage Type */
    usage_type: "asset_image" | "shot_image" | "shot_video" | "speech_audio";
    /** Usage Id */
    usage_id: string;
    /** Input Version Id */
    input_version_id: string;
    /** Input Hash */
    input_hash: string;
    /** Media Version Id */
    media_version_id: string;
    /** Status */
    status: "pending_media" | "ready" | "blocked";
    /** Blocked Reason */
    blocked_reason: string | null;
    /** Model Profile Id */
    model_profile_id: string;
    /** Provider Id */
    provider_id: string;
    /** Model Id */
    model_id: string;
    /** Route Version */
    route_version: string;
    /** Schema Version */
    schema_version: string;
    /** Active Adoption Id */
    active_adoption_id: string | null;
    technical_summary: CandidateTechnicalSummary;
    /** Created At */
    created_at: string;
    /** Finalized At */
    finalized_at: string | null;
  };

  type CandidateTechnicalSummary = {
    /** Mime Type */
    mime_type: string;
    /** Byte Size */
    byte_size: number | null;
    /** Sha256 */
    sha256: string | null;
    /** Width */
    width: number | null;
    /** Height */
    height: number | null;
    /** Duration Ticks */
    duration_ticks: number | null;
    /** Timebase */
    timebase: number | null;
    /** Codec */
    codec: string | null | null;
    /** Pixel Format */
    pixel_format: string | null | null;
    /** Frame Rate */
    frame_rate: string | null | null;
    /** Audio Stream Count */
    audio_stream_count: number | null | null;
    /** Sample Rate */
    sample_rate: number | null | null;
    /** Channels */
    channels: number | null | null;
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

  type confirmSubtitlesParams = {
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

  type createSubtitlesParams = {
    episode_id: string;
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

  type DeliveryArtifactResponse = {
    /** Artifact Type */
    artifact_type: "mp4" | "srt" | "manifest";
    /** Media Version Id */
    media_version_id: string;
    /** Source Kind */
    source_kind: "ffmpeg" | "application";
    /** Mime Type */
    mime_type: string;
    /** Byte Size */
    byte_size: number;
    /** Sha256 */
    sha256: string;
    /** Width */
    width: number | null;
    /** Height */
    height: number | null;
    /** Duration Ticks */
    duration_ticks: number | null;
    /** Timebase */
    timebase: number | null;
  };

  type DeliveryDetailResponse = {
    /** Id */
    id: string;
    /** Episode Id */
    episode_id: string;
    /** Version */
    version: number;
    /** Render Task Id */
    render_task_id: string;
    /** Final Attempt Id */
    final_attempt_id: string | null;
    /** Retry Of Delivery Id */
    retry_of_delivery_id: string | null;
    /** Render Snapshot Id */
    render_snapshot_id: string;
    /** Status */
    status: "rendering" | "ready" | "failed" | "cancelled";
    /** Artifacts */
    artifacts: DeliveryArtifactResponse[];
    /** Ffmpeg Version */
    ffmpeg_version: string | null;
    ffprobe_summary: DeliveryProbeSummaryV1 | null;
    /** Error Code */
    error_code: string | null;
    /** Created At */
    created_at: string;
    /** Updated At */
    updated_at: string;
    /** Finished At */
    finished_at: string | null;
    lineage: DeliveryLineageResponse;
  };

  type DeliveryLineageResponse = {
    source_revision: SourceRevisionResponse;
    script_version: ScriptVersionResponse;
    /** Creative Asset Versions */
    creative_asset_versions: CreativeAssetVersionResponse[];
    shot_spec_version: StoryboardVersionResponse;
    subtitle_version: SubtitleVersionResponse;
    render_snapshot: RenderSnapshotLineageResponse;
    render_task: RenderTaskLineageResponse;
    /** Render Attempts */
    render_attempts: RenderAttemptResponse[];
    /** Input Media */
    input_media: DeliveryMediaLineageV1[];
    /** Delivery Media */
    delivery_media: DeliveryArtifactResponse[];
  };

  type DeliveryListResponse = {
    /** Items */
    items: DeliverySummaryResponse[];
  };

  type DeliveryMediaLineageV1 = {
    /** Usage Type */
    usage_type: "shot_video" | "speech_audio";
    /** Usage Id */
    usage_id: string;
    /** Input Version Id */
    input_version_id: string;
    /** Input Hash */
    input_hash: string;
    /** Adoption Id */
    adoption_id: string;
    /** Candidate Id */
    candidate_id: string;
    /** Media Version Id */
    media_version_id: string;
    /** Media Sha256 */
    media_sha256: string;
    /** Media Kind */
    media_kind: "video" | "audio";
    /** Source Kind */
    source_kind: string;
    /** Mime Type */
    mime_type: string;
    /** Byte Size */
    byte_size: number;
    /** Duration Ticks */
    duration_ticks: number;
    /** Timebase */
    timebase: number;
    /** Probe Summary */
    probe_summary: Record<string, any>;
    /** Origin Attempt Id */
    origin_attempt_id: string;
    /** Origin Task Id */
    origin_task_id: string;
    /** Origin Submission Snapshot Id */
    origin_submission_snapshot_id: string;
    /** Capability */
    capability: "video" | "tts";
    /** Model Profile Id */
    model_profile_id: string;
    /** Provider Id */
    provider_id: string;
    /** Model Id */
    model_id: string;
    /** Route Version */
    route_version: string;
    /** Provider Schema Version */
    provider_schema_version: string;
  };

  type DeliveryProbeSummaryV1 = {
    /** Schema Version */
    schema_version: string | null;
    /** Video Codec */
    video_codec: string;
    /** Pixel Format */
    pixel_format: string;
    /** Width */
    width: number;
    /** Height */
    height: number;
    /** Frame Rate */
    frame_rate: string;
    /** Audio Codec */
    audio_codec: string;
    /** Audio Sample Rate */
    audio_sample_rate: number;
    /** Audio Channels */
    audio_channels: number;
    /** Duration Ticks */
    duration_ticks: number;
    /** Video Start Ticks */
    video_start_ticks: number;
    /** Video Duration Ticks */
    video_duration_ticks: number;
    /** Audio Start Ticks */
    audio_start_ticks: number;
    /** Audio Duration Ticks */
    audio_duration_ticks: number;
    /** Timebase */
    timebase: number | null;
  };

  type DeliverySummaryResponse = {
    /** Id */
    id: string;
    /** Episode Id */
    episode_id: string;
    /** Version */
    version: number;
    /** Render Task Id */
    render_task_id: string;
    /** Final Attempt Id */
    final_attempt_id: string | null;
    /** Retry Of Delivery Id */
    retry_of_delivery_id: string | null;
    /** Render Snapshot Id */
    render_snapshot_id: string;
    /** Status */
    status: "rendering" | "ready" | "failed" | "cancelled";
    /** Artifacts */
    artifacts: DeliveryArtifactResponse[];
    /** Ffmpeg Version */
    ffmpeg_version: string | null;
    ffprobe_summary: DeliveryProbeSummaryV1 | null;
    /** Error Code */
    error_code: string | null;
    /** Created At */
    created_at: string;
    /** Updated At */
    updated_at: string;
    /** Finished At */
    finished_at: string | null;
  };

  type deriveScriptDraftParams = {
    version_id: string;
  };

  type deriveStoryboardDraftParams = {
    version_id: string;
  };

  type deriveSubtitleDraftParams = {
    version_id: string;
  };

  type DownloadAuthorizationItem = {
    /** Artifact Type */
    artifact_type: "mp4" | "srt" | "manifest";
    /** Media Version Id */
    media_version_id: string;
    /** Url */
    url: string;
    /** Expires In Seconds */
    expires_in_seconds: number;
    /** Expires At */
    expires_at: string;
  };

  type DownloadAuthorizationRequest = {
    /** Episode Id */
    episode_id: string;
    /** Artifact Types */
    artifact_types: ("mp4" | "srt" | "manifest")[] | null;
  };

  type DownloadAuthorizationResponse = {
    /** Items */
    items: DownloadAuthorizationItem[];
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

  type getDeliveryParams = {
    delivery_id: string;
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

  type getSubtitlesParams = {
    episode_id: string;
  };

  type getSubtitleVersionParams = {
    version_id: string;
  };

  type getTaskParams = {
    task_id: string;
  };

  type HTTPValidationError = {
    /** Detail */
    detail: ValidationError[] | null;
  };

  type listCandidatesParams = {
    episode_id: string;
    usage_type: "asset_image" | "shot_image" | "shot_video" | "speech_audio";
    usage_id: string;
    input_version_id: string;
    input_hash: string;
  };

  type listCreativeAssetsParams = {
    episode_id: string;
    include_versions: boolean;
  };

  type listDeliveriesParams = {
    episode_id: string;
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

  type listSubtitleVersionsParams = {
    episode_id: string;
  };

  type listTasksParams = {
    episode_id: string;
  };

  type PreviewAuthorizationRequest = {
    /** Episode Id */
    episode_id: string;
  };

  type PreviewAuthorizationResponse = {
    /** Media Version Id */
    media_version_id: string;
    /** Url */
    url: string;
    /** Expires In Seconds */
    expires_in_seconds: number;
    /** Expires At */
    expires_at: string;
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

  type RenderAttemptResponse = {
    /** Id */
    id: string;
    /** Task Id */
    task_id: string;
    /** Submission Snapshot Id */
    submission_snapshot_id: string;
    /** Attempt No */
    attempt_no: number;
    /** Parent Attempt Id */
    parent_attempt_id: string | null;
    /** Status */
    status: string;
    /** Execution Metadata */
    execution_metadata: Record<string, any>;
    /** Error Code */
    error_code: string | null;
    /** Error Summary */
    error_summary: string | null;
    /** Created At */
    created_at: string;
    /** Submitted At */
    submitted_at: string | null;
    /** Started At */
    started_at: string | null;
    /** Finished At */
    finished_at: string | null;
  };

  type RenderInputRefsV1 = {
    /** Schema Version */
    schema_version: string | null;
    /** Shot Spec Version Id */
    shot_spec_version_id: string;
    /** Subtitle Version Id */
    subtitle_version_id: string;
    /** Subtitle Content Hash */
    subtitle_content_hash: string;
    /** Video Adoptions */
    video_adoptions: RenderMediaRefV1[];
    /** Tts Adoptions */
    tts_adoptions: RenderMediaRefV1[];
  };

  type RenderMediaRefV1 = {
    /** Usage Type */
    usage_type: "shot_video" | "speech_audio";
    /** Usage Id */
    usage_id: string;
    /** Input Version Id */
    input_version_id: string;
    /** Input Hash */
    input_hash: string;
    /** Adoption Id */
    adoption_id: string;
    /** Candidate Id */
    candidate_id: string;
    /** Media Version Id */
    media_version_id: string;
    /** Sha256 */
    sha256: string;
    /** Duration Ticks */
    duration_ticks: number;
    /** Timebase */
    timebase: number | null;
  };

  type RenderRecipeV1 = {
    /** Schema Version */
    schema_version: string | null;
    /** Runtime Image */
    runtime_image: string;
    /** Ffmpeg Version */
    ffmpeg_version: string;
    /** Ffprobe Version */
    ffprobe_version: string;
    /** Font Name */
    font_name: string;
    /** Font File */
    font_file: string;
    /** Font Sha256 */
    font_sha256: string;
    /** Font License */
    font_license: "OFL-1.1" | "Bitstream-Vera";
    /** Timebase */
    timebase: number | null;
    /** Width */
    width: number | null;
    /** Height */
    height: number | null;
    /** Fps */
    fps: number | null;
    /** Audio Rate */
    audio_rate: number | null;
    /** Audio Channels */
    audio_channels: number | null;
    /** Video Codec */
    video_codec: string | null;
    /** Video Preset */
    video_preset: string | null;
    /** Pixel Format */
    pixel_format: string | null;
    /** Audio Codec */
    audio_codec: string | null;
    /** Audio Bitrate */
    audio_bitrate: string | null;
    /** Scale Mode */
    scale_mode: string | null;
    /** Padding Color */
    padding_color: string | null;
    /** Video Tolerance Ticks */
    video_tolerance_ticks: number | null;
    /** Remove Source Audio */
    remove_source_audio: boolean | null;
    /** Preserve Tts Speed */
    preserve_tts_speed: boolean | null;
    /** Burn Subtitles */
    burn_subtitles: boolean | null;
  };

  type RenderSegmentV1 = {
    /** Shot Id */
    shot_id: string;
    /** Ordinal */
    ordinal: number;
    /** Start Ticks */
    start_ticks: number;
    /** End Ticks */
    end_ticks: number;
    /** Duration Ticks */
    duration_ticks: number;
    /** Video Adoption Id */
    video_adoption_id: string;
    /** Tts Adoption Ids */
    tts_adoption_ids: string[];
  };

  type RenderSnapshotLineageResponse = {
    /** Id */
    id: string;
    /** Episode Id */
    episode_id: string;
    /** Initial Task Id */
    initial_task_id: string | null;
    input_refs: RenderInputRefsV1;
    /** Segments */
    segments: RenderSegmentV1[];
    recipe: RenderRecipeV1;
    /** Recipe Hash */
    recipe_hash: string;
    /** Content Hash */
    content_hash: string;
    /** Created At */
    created_at: string;
  };

  type RenderTaskLineageResponse = {
    /** Id */
    id: string;
    /** Submission Snapshot Id */
    submission_snapshot_id: string;
    /** Status */
    status: string;
    /** Resource Version */
    resource_version: number;
    /** Retry Of Task Id */
    retry_of_task_id: string | null;
    /** Created At */
    created_at: string;
    /** Updated At */
    updated_at: string;
    /** Finished At */
    finished_at: string | null;
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

  type SaveSubtitleRequest = {
    content: SubtitleContentV1;
  };

  type saveSubtitlesParams = {
    version_id: string;
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

  type SubtitleContentV1 = {
    /** Schema Version */
    schema_version: string | null;
    /** Language */
    language: string;
    /** Timebase */
    timebase: number | null;
    /** Cues */
    cues: SubtitleCueV1[];
  };

  type SubtitleCueV1 = {
    /** Cue Id */
    cue_id: string;
    /** Ordinal */
    ordinal: number;
    /** Speech Line Id */
    speech_line_id: string;
    /** Shot Id */
    shot_id: string;
    /** Text */
    text: string;
    /** Voice Id */
    voice_id:
      | "narrator_female"
      | "narrator_male"
      | "character_young_female"
      | "character_young_male";
    /** Source Text Hash */
    source_text_hash: string;
    /** Start Ticks */
    start_ticks: number;
    /** End Ticks */
    end_ticks: number;
    /** Tts Duration Ticks */
    tts_duration_ticks: number;
    /** Shot Start Ticks */
    shot_start_ticks: number;
    /** Shot End Ticks */
    shot_end_ticks: number;
  };

  type SubtitleVersionListResponse = {
    /** Items */
    items: SubtitleVersionResponse[];
  };

  type SubtitleVersionResponse = {
    /** Id */
    id: string;
    /** Episode Id */
    episode_id: string;
    /** Version */
    version: number;
    /** Parent Id */
    parent_id: string | null;
    /** Script Version Id */
    script_version_id: string;
    /** Shot Spec Version Id */
    shot_spec_version_id: string;
    content: SubtitleContentV1;
    /** Content Hash */
    content_hash: string;
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
