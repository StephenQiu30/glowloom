/** E0 client contract. Resource authorization and server hashes belong to E1/E4. */
export type MediaKind = "image" | "video" | "audio";
export type LabelKind = MediaKind | "subject";
export type PromptInline =
  | { type: "text"; text: string }
  | { type: "hardBreak" }
  | { type: "reference"; attrs: { referenceId: string } };
export type PromptDocument = {
  type: "doc";
  content: { type: "paragraph"; content?: PromptInline[] }[];
};
export type ReferenceSource =
  | {
      kind: "pending_upload";
      clientUploadId: string;
      uploadAttemptId: string;
      uploadSessionId?: string;
    }
  | { kind: "media"; sourceId: string; sourceVersion: string }
  | {
      kind: "subject";
      sourceId: string;
      sourceVersion: string;
      description: string;
      media: {
        sourceId: string;
        sourceVersion: string;
        mediaKind: MediaKind;
        bindingId?: string;
      }[];
    };
export type ReferenceBinding = {
  id: string;
  source: ReferenceSource;
  bindingGeneration: number;
  mediaKind: MediaKind;
  role: string;
  order: number;
};
export type LabelState = {
  highWater: Record<LabelKind, number>;
  allocations: Record<string, { labelKind: LabelKind; ordinal: number }>;
};
export type PromptDraft = {
  schemaVersion: 1;
  document: PromptDocument;
  references: ReferenceBinding[];
  modelVersion: string | null;
  mode: string | null;
  params: Record<string, string | number | boolean | null>;
  labelState: LabelState;
};
export const labelNames: Record<LabelKind, string> = {
  image: "图片",
  video: "视频",
  audio: "音频",
  subject: "主体",
};
const labelKinds = Object.keys(labelNames);
// Bounded tombstones, not a provider's reference limit. Exhaustion must be explicit.
export const MAX_LABEL_ALLOCATIONS = 10_000;
export function emptyPromptDraft(): PromptDraft {
  return {
    schemaVersion: 1,
    document: { type: "doc", content: [{ type: "paragraph" }] },
    references: [],
    modelVersion: null,
    mode: null,
    params: {},
    labelState: {
      highWater: { image: 0, video: 0, audio: 0, subject: 0 },
      allocations: {},
    },
  };
}
function requireValue(ok: unknown): asserts ok {
  if (!ok) throw new Error("invalid_prompt_draft");
}
function record(value: unknown): asserts value is Record<string, unknown> {
  requireValue(
    value !== null && typeof value === "object" && !Array.isArray(value),
  );
  requireValue(
    Object.getPrototypeOf(value) === Object.prototype ||
      Object.getPrototypeOf(value) === null,
  );
}
function keys(value: Record<string, unknown>, allowed: string[]) {
  requireValue(Object.keys(value).every((k) => allowed.includes(k)));
}
function id(value: unknown): asserts value is string {
  requireValue(
    typeof value === "string" && value.length > 0 && value.length <= 256,
  );
  requireValue(
    value !== "__proto__" && value !== "constructor" && value !== "prototype",
  );
}
function integer(value: unknown, min = 0): asserts value is number {
  requireValue(Number.isSafeInteger(value) && (value as number) >= min);
}
export function validateReference(
  value: unknown,
): asserts value is ReferenceBinding {
  record(value);
  keys(value, [
    "id",
    "source",
    "bindingGeneration",
    "mediaKind",
    "role",
    "order",
  ]);
  id(value.id);
  id(value.role);
  integer(value.bindingGeneration, 1);
  integer(value.order);
  requireValue(["image", "video", "audio"].includes(value.mediaKind as string));
  const s = value.source;
  record(s);
  if (s.kind === "pending_upload") {
    keys(s, ["kind", "clientUploadId", "uploadAttemptId", "uploadSessionId"]);
    id(s.clientUploadId);
    id(s.uploadAttemptId);
    if (s.uploadSessionId !== undefined) id(s.uploadSessionId);
  } else {
    requireValue(s.kind === "media" || s.kind === "subject");
    keys(
      s,
      s.kind === "media"
        ? ["kind", "sourceId", "sourceVersion"]
        : ["kind", "sourceId", "sourceVersion", "description", "media"],
    );
    id(s.sourceId);
    id(s.sourceVersion);
    if (s.kind === "subject") {
      requireValue(typeof s.description === "string" && Array.isArray(s.media));
      for (const item of s.media) {
        record(item);
        keys(item, ["sourceId", "sourceVersion", "mediaKind", "bindingId"]);
        id(item.sourceId);
        id(item.sourceVersion);
        if (item.bindingId !== undefined) id(item.bindingId);
        requireValue(
          ["image", "video", "audio"].includes(item.mediaKind as string),
        );
      }
    }
  }
}
export function validatePromptDraft(
  value: unknown,
): asserts value is PromptDraft {
  record(value);
  keys(value, [
    "schemaVersion",
    "document",
    "references",
    "modelVersion",
    "mode",
    "params",
    "labelState",
  ]);
  requireValue(value.schemaVersion === 1);
  if (value.mode !== null) id(value.mode);
  if (value.modelVersion !== null) id(value.modelVersion);
  record(value.params);
  for (const [key, param] of Object.entries(value.params)) {
    id(key);
    requireValue(
      param === null ||
        typeof param === "string" ||
        typeof param === "boolean" ||
        (typeof param === "number" && Number.isFinite(param)),
    );
  }
  requireValue(Array.isArray(value.references));
  const ids = new Set<string>();
  for (const [index, binding] of value.references.entries()) {
    validateReference(binding);
    requireValue(!ids.has(binding.id) && binding.order === index);
    ids.add(binding.id);
  }
  const labels = value.labelState;
  record(labels);
  keys(labels, ["highWater", "allocations"]);
  record(labels.highWater);
  keys(labels.highWater, labelKinds);
  for (const kind of labelKinds) integer(labels.highWater[kind]);
  record(labels.allocations);
  requireValue(Object.keys(labels.allocations).length <= MAX_LABEL_ALLOCATIONS);
  const used = new Set<string>();
  for (const [referenceId, allocation] of Object.entries(labels.allocations)) {
    id(referenceId);
    record(allocation);
    keys(allocation, ["labelKind", "ordinal"]);
    requireValue(labelKinds.includes(allocation.labelKind as string));
    integer(allocation.ordinal, 1);
    requireValue(
      allocation.ordinal <=
        (labels.highWater[allocation.labelKind as string] as number),
    );
    const key = `${allocation.labelKind}:${allocation.ordinal}`;
    requireValue(!used.has(key));
    used.add(key);
  }
  for (const binding of value.references as ReferenceBinding[]) {
    const allocation = labels.allocations[binding.id];
    record(allocation);
    requireValue(allocation.labelKind === labelKind(binding));
  }
  const doc = value.document;
  record(doc);
  keys(doc, ["type", "content"]);
  requireValue(
    doc.type === "doc" && Array.isArray(doc.content) && doc.content.length > 0,
  );
  for (const paragraph of doc.content) {
    record(paragraph);
    keys(paragraph, ["type", "content"]);
    requireValue(paragraph.type === "paragraph");
    if (paragraph.content === undefined) continue;
    requireValue(Array.isArray(paragraph.content));
    for (const inline of paragraph.content) {
      record(inline);
      if (inline.type === "text") {
        keys(inline, ["type", "text"]);
        requireValue(typeof inline.text === "string" && inline.text.length > 0);
      } else if (inline.type === "reference") {
        keys(inline, ["type", "attrs"]);
        record(inline.attrs);
        keys(inline.attrs, ["referenceId"]);
        id(inline.attrs.referenceId);
        requireValue(ids.has(inline.attrs.referenceId));
      } else {
        keys(inline, ["type"]);
        requireValue(inline.type === "hardBreak");
      }
    }
  }
}
export function labelKind(binding: ReferenceBinding): LabelKind {
  return binding.source.kind === "subject" ? "subject" : binding.mediaKind;
}
export function referenceLabel(labels: LabelState, referenceId: string) {
  const allocation = labels.allocations[referenceId];
  return allocation
    ? `${labelNames[allocation.labelKind]}${allocation.ordinal}`
    : "无效引用";
}
export function sameReference(a: ReferenceBinding, b: ReferenceBinding) {
  if (
    a.role !== b.role ||
    a.mediaKind !== b.mediaKind ||
    a.source.kind !== b.source.kind
  )
    return false;
  if (a.source.kind === "pending_upload" || b.source.kind === "pending_upload")
    return (
      a.id === b.id &&
      a.bindingGeneration === b.bindingGeneration &&
      JSON.stringify(a.source) === JSON.stringify(b.source)
    );
  return (
    a.source.sourceId === b.source.sourceId &&
    a.source.sourceVersion === b.source.sourceVersion &&
    (a.source.kind !== "subject" ||
      JSON.stringify(a.source) === JSON.stringify(b.source))
  );
}
