import type { MediaKind } from "./prompt-document";

/** Canvas geometry stays with its owner; only non-secret source identities cross this boundary. */
export type CanvasReferenceSource = {
  nodeID: string;
  title: string;
  kind: MediaKind;
  assetID?: string;
};

/** A subject keeps its own immutable version and expands into its authorized media. */
export type CanvasSubjectReferenceSource = { nodeID: string; title: string } & (
  | { source: "subject"; subjectID: string; versionID: string }
  | { source: "canvas_subject"; canvasID: string; inputRevision: number }
);

export function referenceRoleLabel(role: string) {
  return (
    (
      {
        reference_image: "参考图片",
        character_image: "角色图片",
        driving_audio: "讲话音频",
        driving_video: "动作视频",
        reference_video: "参考视频",
        reference_audio: "参考音频",
        first_frame: "首帧",
        last_frame: "尾帧",
        subject: "主体",
      } as Record<string, string>
    )[role] ?? role
  );
}
