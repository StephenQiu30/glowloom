export type FieldPolicy = {
  kind: string;
  group?: string[];
  cas?: boolean;
  context?: string;
  optional?: boolean;
};
export const fieldPolicies: Record<string, FieldPolicy> = {
  "config.text_source": { kind: "set", group: ["text"], cas: true },
  title: {
    kind: "set",
  },
  z_index: {
    kind: "set",
  },
  parent_id: {
    kind: "set",
  },
  position: {
    kind: "set",
    group: ["x", "y"],
    cas: false,
  },
  size: {
    kind: "set",
    group: ["width", "height"],
    cas: false,
  },
  "config.text": {
    kind: "text",
  },
  "config.collapsed": {
    kind: "set",
  },
  "config.draft.prompt": {
    kind: "text",
  },
  "config.draft.negative_prompt": {
    kind: "text",
  },
  "config.draft.aspect_ratio": {
    kind: "set",
  },
  "config.draft.candidate_count": {
    kind: "set",
  },
  "config.draft.composer.document": {
    kind: "text",
  },
  "config.draft.composer.model": {
    kind: "set",
    group: ["modelVersion", "mode", "params"],
    cas: true,
  },
  "config.draft.composer.params.*": {
    kind: "set",
    context: "config.draft.composer.model",
  },
  "config.draft.composer.references": {
    kind: "list",
  },
  "config.draft.composer.references.@.binding": {
    kind: "set",
    group: ["source", "mediaKind", "role", "bindingGeneration"],
    cas: true,
  },
  "config.generation.model": {
    kind: "set",
    group: [
      "capability",
      "model_profile_id",
      "mode",
      "params",
      "output_count",
      "inputs",
    ],
    cas: true,
  },
  "config.generation.prompt": {
    kind: "text",
    context: "config.generation.model",
  },
  "config.generation.output_count": {
    kind: "set",
    context: "config.generation.model",
  },
  "config.generation.inputs": {
    kind: "set",
    group: ["inputs"],
    cas: true,
  },
  "config.generation.params.*": {
    kind: "set",
    context: "config.generation.model",
  },
  "config.batch_table.model": {
    kind: "set",
    group: ["operation", "model_profile_id", "mode", "params", "output_count"],
    cas: true,
  },
  "config.batch_table.global_prompt": {
    kind: "text",
    context: "config.batch_table.model",
  },
  "config.batch_table.concurrency": {
    kind: "set",
    context: "config.batch_table.model",
  },
  "config.batch_table.output_count": {
    kind: "set",
    context: "config.batch_table.model",
  },
  "config.batch_table.params.*": {
    kind: "set",
    context: "config.batch_table.model",
  },
  "config.batch_table.reference_columns": {
    kind: "list",
  },
  "config.batch_table.rows": {
    kind: "list",
  },
  "config.batch_table.reference_columns.@.label": {
    kind: "set",
  },
  "config.batch_table.rows.@.enabled": {
    kind: "set",
  },
  "config.batch_table.rows.@.prompt": {
    kind: "text",
  },
  "config.batch_table.rows.@.input_node_ids.@": {
    kind: "set",
  },
  "config.timeline.aspect_ratio": {
    kind: "set",
  },
  "config.timeline.fps": {
    kind: "set",
  },
  "config.timeline.snapping": {
    kind: "set",
  },
  "config.timeline.tracks": {
    kind: "list",
  },
  "config.timeline.clips": {
    kind: "list",
  },
  "config.timeline.tracks.@.label": {
    kind: "set",
  },
  "config.timeline.tracks.@.locked": {
    kind: "set",
  },
  "config.timeline.tracks.@.visible": {
    kind: "set",
  },
  "config.timeline.tracks.@.muted": {
    kind: "set",
  },
  "config.timeline.clips.@.title": {
    kind: "set",
  },
  "config.timeline.clips.@.volume": {
    kind: "set",
  },
  "config.timeline.clips.@.text": {
    kind: "text",
  },
  "config.timeline.clips.@.time": {
    kind: "set",
    group: [
      "track_id",
      "start_ms",
      "duration_ms",
      "source_start_ms",
      "source_duration_ms",
      "fade_in_ms",
      "fade_out_ms",
    ],
    cas: false,
    context: "config.timeline.clips.@.source",
  },
  "config.timeline.clips.@.source": {
    kind: "set",
    group: ["node_id", "asset_id"],
    cas: true,
  },
  "config.timeline.clips.@.crop": {
    kind: "set",
    group: ["crop"],
    cas: false,
    optional: true,
    context: "config.timeline.clips.@.source",
  },
  "config.timeline.subtitle_style.font_size": {
    kind: "set",
  },
  "config.timeline.subtitle_style.color": {
    kind: "set",
  },
  "config.timeline.subtitle_style.position": {
    kind: "set",
  },
  "config.director.title": {
    kind: "set",
  },
  "config.director.background": {
    kind: "set",
  },
  "config.director.environment_intensity": {
    kind: "set",
  },
  "config.director.grid_visible": {
    kind: "set",
  },
  "config.director.grid_snap": {
    kind: "set",
  },
  "config.director.labels_visible": {
    kind: "set",
  },
  "config.director.aspect_ratio": {
    kind: "set",
  },
  "config.director.panorama_rotation": {
    kind: "set",
  },
  "config.director.panorama_radius": {
    kind: "set",
  },
  "config.director.active_shot_id": {
    kind: "set",
  },
  "config.director.objects": {
    kind: "list",
  },
  "config.director.cameras": {
    kind: "list",
  },
  "config.director.lights": {
    kind: "list",
  },
  "config.director.shots": {
    kind: "list",
  },
  "config.director.ground": {
    kind: "set",
    group: ["ground"],
    cas: true,
    optional: true,
  },
  "config.director.ground.visible": {
    kind: "set",
  },
  "config.director.ground.opacity": {
    kind: "set",
  },
  "config.director.ground.height": {
    kind: "set",
  },
  "config.director.stage_transform": {
    kind: "set",
    group: ["stage_transform"],
    cas: false,
    optional: true,
  },
  "config.director.panorama": {
    kind: "set",
    group: ["panorama"],
    cas: true,
    optional: true,
  },
  "config.director.cover": {
    kind: "set",
    group: ["cover"],
    cas: false,
    optional: true,
  },
  "config.director.panorama.source": {
    kind: "set",
    group: ["asset_id", "name"],
    cas: true,
  },
  "config.director.panorama.rotation": {
    kind: "set",
    context: "config.director.panorama.source",
  },
  "config.director.objects.@.name": {
    kind: "set",
  },
  "config.director.objects.@.color": {
    kind: "set",
  },
  "config.director.objects.@.visible": {
    kind: "set",
  },
  "config.director.objects.@.cast_shadow": {
    kind: "set",
  },
  "config.director.objects.@.receive_shadow": {
    kind: "set",
  },
  "config.director.objects.@.pose": {
    kind: "set",
  },
  "config.director.objects.@.active_motion_clip_id": {
    kind: "set",
  },
  "config.director.objects.@.transform": {
    kind: "set",
    group: ["transform", "uniform_scale"],
    cas: false,
  },
  "config.director.objects.@.source": {
    kind: "set",
    group: [
      "primitive",
      "source_node_id",
      "asset_id",
      "builtin_actor",
      "rig",
      "motion_clips",
      "active_motion_clip_id",
      "bone_overrides",
      "bone_tracks",
      "keyframes",
    ],
    cas: true,
  },
  "config.director.objects.@.rig": {
    kind: "set",
    group: ["rig"],
    cas: true,
    context: "config.director.objects.@.source",
    optional: true,
  },
  "config.director.objects.@.motion_clips": {
    kind: "list",
    context: "config.director.objects.@.source",
  },
  "config.director.objects.@.keyframes": {
    kind: "list",
    context: "config.director.objects.@.source",
  },
  "config.director.objects.@.bone_tracks": {
    kind: "list",
    context: "config.director.objects.@.source",
  },
  "config.director.objects.@.bone_overrides.*": {
    kind: "set",
    context: "config.director.objects.@.source",
  },
  "config.director.objects.@.motion_clips.@.name": {
    kind: "set",
  },
  "config.director.objects.@.motion_clips.@.source_animation": {
    kind: "set",
    context: "config.director.objects.@.source",
  },
  "config.director.objects.@.motion_clips.@.playback": {
    kind: "set",
    group: ["start", "duration", "playback_rate", "loop"],
    cas: false,
    context: "config.director.objects.@.source",
  },
  "config.director.objects.@.keyframes.@.time": {
    kind: "set",
  },
  "config.director.objects.@.keyframes.@.easing": {
    kind: "set",
  },
  "config.director.objects.@.keyframes.@.transform": {
    kind: "set",
    group: ["transform"],
    cas: false,
    context: "config.director.objects.@.source",
  },
  "config.director.objects.@.bone_tracks.@.keyframes": {
    kind: "list",
    context: "config.director.objects.@.source",
  },
  "config.director.objects.@.bone_tracks.@.keyframes.@.time": {
    kind: "set",
  },
  "config.director.objects.@.bone_tracks.@.keyframes.@.easing": {
    kind: "set",
  },
  "config.director.objects.@.bone_tracks.@.keyframes.@.rotation": {
    kind: "set",
    group: ["rotation"],
    cas: false,
    context: "config.director.objects.@.source",
  },
  "config.director.cameras.@.name": {
    kind: "set",
  },
  "config.director.cameras.@.aperture": {
    kind: "set",
  },
  "config.director.cameras.@.focus_distance": {
    kind: "set",
  },
  "config.director.cameras.@.transform": {
    kind: "set",
    group: ["transform"],
    cas: false,
  },
  "config.director.cameras.@.follow": {
    kind: "set",
    group: [
      "target",
      "follow_object_id",
      "follow_anchor",
      "look_at_mode",
      "look_at_object_id",
    ],
    cas: false,
  },
  "config.director.cameras.@.lens": {
    kind: "set",
    group: ["focal_length", "fov"],
    cas: false,
  },
  "config.director.cameras.@.clipping": {
    kind: "set",
    group: ["near", "far"],
    cas: false,
  },
  "config.director.cameras.@.keyframes": {
    kind: "list",
  },
  "config.director.cameras.@.keyframes.@.time": {
    kind: "set",
  },
  "config.director.cameras.@.keyframes.@.easing": {
    kind: "set",
  },
  "config.director.cameras.@.keyframes.@.transform": {
    kind: "set",
    group: ["transform"],
    cas: false,
  },
  "config.director.lights.@.name": {
    kind: "set",
  },
  "config.director.lights.@.type": {
    kind: "set",
  },
  "config.director.lights.@.color": {
    kind: "set",
  },
  "config.director.lights.@.intensity": {
    kind: "set",
  },
  "config.director.lights.@.cast_shadow": {
    kind: "set",
  },
  "config.director.lights.@.transform": {
    kind: "set",
    group: ["transform"],
    cas: false,
  },
  "config.director.lights.@.beam": {
    kind: "set",
    group: ["angle", "penumbra"],
    cas: false,
  },
  "config.director.shots.@.name": {
    kind: "set",
  },
  "config.director.shots.@.camera_id": {
    kind: "set",
  },
  "config.director.shots.@.shot_size": {
    kind: "set",
  },
  "config.director.shots.@.camera_move": {
    kind: "set",
  },
  "config.director.shots.@.time": {
    kind: "set",
    group: ["duration", "fps"],
    cas: false,
  },
  "config.director.shots.@.prompt": {
    kind: "text",
  },
  "config.director.shots.@.screenshots": {
    kind: "list",
  },
  "config.director.shots.@.screenshots.@.name": {
    kind: "set",
  },
  "config.draft.composer.params.negative_prompt": {
    kind: "text",
    context: "config.draft.composer.model",
  },
};
