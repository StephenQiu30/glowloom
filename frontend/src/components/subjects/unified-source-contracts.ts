import { isProjectID } from "@/components/projects/project";
import { RequestError } from "@/lib/request";
export type SubjectSourceItem = Required<
  Omit<API.SubjectSourceItem, "project_id" | "media" | "source" | "kind">
> & {
  source: "personal_asset" | "generated_output";
  project_id?: string;
  kind: "image" | "video" | "audio";
  media: Required<
    Pick<
      API.SubjectSourceMedia,
      | "id"
      | "revision"
      | "kind"
      | "file_name"
      | "mime_type"
      | "byte_size"
      | "origin"
    >
  >;
};
export type SubjectSourcePage = {
  current_actor_id: string;
  current_scope_id: string;
  items: SubjectSourceItem[];
  page: number;
  page_size: number;
  total: number;
  capabilities: { upscaled_filter: boolean; upscaled_reason?: string };
};
export function requireSubjectSourceItem(value: unknown): SubjectSourceItem {
  const item = value as SubjectSourceItem | null;
  if (
    !item ||
    !isProjectID(item.id) ||
    !Number.isSafeInteger(item.revision) ||
    item.revision < 0 ||
    typeof item.favorite !== "boolean" ||
    typeof item.upscaled !== "boolean" ||
    typeof item.title !== "string" ||
    !item.title.trim() ||
    !["image", "video", "audio"].includes(item.kind) ||
    ![item.created_at, item.updated_at].every(
      (date) => typeof date === "string" && Number.isFinite(Date.parse(date)),
    ) ||
    !isProjectID(item.media?.id) ||
    !Number.isSafeInteger(item.media.revision) ||
    item.media.revision < 1 ||
    item.media.kind !== item.kind ||
    typeof item.media.mime_type !== "string" ||
    !item.media.mime_type ||
    typeof item.media.file_name !== "string" ||
    !Number.isSafeInteger(item.media.byte_size) ||
    item.media.byte_size < 1 ||
    !["upload", "generated", "system"].includes(item.media.origin)
  )
    throw new RequestError("invalid_receipt");
  if (
    item.source === "personal_asset" &&
    item.project_id == null &&
    item.id === item.media.id &&
    !item.upscaled
  )
    return item;
  if (item.source === "generated_output" && isProjectID(item.project_id))
    return item;
  throw new RequestError("invalid_receipt");
}
export function requireSubjectSourcePage(
  value: unknown,
  actorID: string,
  scopeID: string,
  kind?: string,
): SubjectSourcePage {
  const p = value as SubjectSourcePage;
  if (
    !p ||
    p.current_actor_id !== actorID ||
    p.current_scope_id !== scopeID ||
    !Array.isArray(p.items) ||
    !Number.isSafeInteger(p.page) ||
    p.page < 1 ||
    !Number.isSafeInteger(p.page_size) ||
    p.page_size < 1 ||
    p.page_size > 120 ||
    !Number.isSafeInteger(p.total) ||
    p.total < 0 ||
    p.items.length > p.page_size ||
    p.items.length > p.total ||
    !p.capabilities ||
    typeof p.capabilities.upscaled_filter !== "boolean"
  )
    throw new RequestError("invalid_receipt");
  p.items.forEach(requireSubjectSourceItem);
  if (
    new Set(p.items.map((item) => item.id)).size !== p.items.length ||
    p.items.some((item) => !!kind && item.kind !== kind)
  )
    throw new RequestError("invalid_receipt");
  return p;
}
export function subjectSourceKind(file: File) {
  const extension = file.name.split(".").at(-1)?.toLowerCase() ?? "";
  if (["png", "jpg", "jpeg", "webp"].includes(extension)) return "image";
  if (["mp4", "webm", "mov"].includes(extension)) return "video";
  if (["mp3", "wav", "m4a", "ogg", "flac"].includes(extension)) return "audio";
  return null;
}
