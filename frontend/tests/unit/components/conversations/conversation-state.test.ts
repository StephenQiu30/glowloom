import { expect, it } from "vitest";
import { emptyPromptDraft } from "@/components/generation/prompt-document";
import {
  canSubmitConversation,
  mergeMessages,
  mergeRun,
  budgetMicros,
} from "@/components/conversations/conversation-state";
const model = {
  key: "planner",
  status: "active",
  provider_status: "active",
  credential_present: true,
  available: true,
  mode_availability: [{ mode: "text_to_text", available: true }],
  capability: "text.generate",
  input_roles: ["prompt", "reference_image"],
  current_version: {
    id: "version",
    limits: { roles: { reference_image: { types: ["image"], max_count: 2 } } },
  },
  current_price: { id: "price" },
} as API.ModelResponse;
function ready(draft = emptyPromptDraft()) {
  return {
    draft,
    planningModel: model,
    authorized: true,
    budget: "1",
    busy: false,
    active: false,
  };
}
it("以视频实际抽帧数量校验视觉模型，小说文档不伪装成图片", () => {
  const draft = emptyPromptDraft();
  draft.references = [
    {
      id: "video",
      bindingGeneration: 1,
      mediaKind: "video",
      role: "reference_video",
      order: 0,
      source: { kind: "media", sourceId: "asset", sourceVersion: "1" },
    },
  ];
  const enough = structuredClone(model);
  (
    enough.current_version!.limits!.roles as Record<
      string,
      { max_count: number }
    >
  ).reference_image!.max_count = 3;
  expect(
    canSubmitConversation({ ...ready(draft), planningModel: enough }).allowed,
  ).toBe(true);
  expect(canSubmitConversation(ready(draft)).allowed).toBe(false);
  draft.references[0]!.mediaKind = "document";
  draft.references[0]!.role = "reference_document";
  const textOnly = {
    ...model,
    input_roles: ["prompt"],
    current_version: { ...model.current_version, limits: {} },
  } as API.ModelResponse;
  expect(
    canSubmitConversation({ ...ready(draft), planningModel: textOnly }).allowed,
  ).toBe(true);
});
it("distinguishes empty, image-only and new text from quoted context", () => {
  const draft = emptyPromptDraft();
  expect(canSubmitConversation(ready(draft)).allowed).toBe(false);
  draft.references.push({
    id: "image",
    bindingGeneration: 1,
    mediaKind: "image",
    role: "reference_image",
    order: 0,
    source: { kind: "media", sourceId: "asset", sourceVersion: "2" },
  });
  expect(canSubmitConversation(ready(draft)).allowed).toBe(true);
  draft.references = [];
  draft.document.content[0]!.content = [{ type: "text", text: "新的要求" }];
  expect(canSubmitConversation(ready(draft)).allowed).toBe(true);
  expect(canSubmitConversation({ ...ready(draft), active: true }).allowed).toBe(
    false,
  );
});
it("does not silently omit pending or unsupported references", () => {
  const draft = emptyPromptDraft();
  draft.document.content[0]!.content = [{ type: "text", text: "创作" }];
  draft.references.push({
    id: "pending",
    bindingGeneration: 1,
    order: 0,
    mediaKind: "image",
    role: "reference_image",
    source: {
      kind: "pending_upload",
      clientUploadId: "file",
      uploadAttemptId: "attempt",
    },
  });
  expect(canSubmitConversation(ready(draft)).allowed).toBe(false);
  draft.references[0]!.source = {
    kind: "media",
    sourceId: "asset",
    sourceVersion: "2",
  };
  draft.references[0]!.mediaKind = "video";
  expect(canSubmitConversation(ready(draft)).allowed).toBe(false);
});
it("merges committed message parts by identity without collapsing repeated text", () => {
  const part = {
    run_id: "run",
    conversation_id: 1,
    sequence: "2:1",
    message_id: "one",
    part_index: 0,
    kind: "assistant",
    content: "相同文字",
  };
  expect(
    mergeMessages(
      [part],
      [part, { ...part, message_id: "two", sequence: "3:1" }],
    ),
  ).toHaveLength(2);
  expect(
    mergeMessages([{ ...part, part_index: 1, content: "后" }], [part]).map(
      (p) => p.part_index,
    ),
  ).toEqual([0, 1]);
});
it("parses a budget as exact micros and rejects invalid numbers", () => {
  expect(budgetMicros("0.12")).toBe(120000);
  expect(budgetMicros("1.000001")).toBe(1000001);
  for (const value of ["0", "NaN", "1e6", "-1", "0.0000001", "1000001"])
    expect(budgetMicros(value)).toBeUndefined();
});
it("exposes a recovery gap while retaining progress for overlapping snapshots", () => {
  const part = (n: number) => ({
    message_id: `m${n}`,
    run_id: "run",
    conversation_id: 1,
    sequence: `${n}:1`,
    part_index: 0,
    kind: "assistant",
    content: String(n),
  });
  const snapshot = (items: API.PublicMessage[], next_cursor?: string) =>
    ({
      root_run_id: "run",
      session_id: "sid",
      submission_id: "sub",
      goal: "",
      manifest: {},
      execution_available: true,
      events: [],
      snapshot_cursor: `${items.at(-1)?.sequence ?? "0:0"}`,
      messages_page: {
        items,
        through: items.at(-1)?.sequence ?? "0:0",
        ...(next_cursor ? { next_cursor } : {}),
      },
      state: {
        status: "running",
        execution_revision: 1,
        pending_proposal: null,
      },
    }) satisfies API.RunSnapshot;
  const first = mergeRun(undefined, snapshot([part(1)]));
  const recovered = mergeRun(
    first,
    snapshot(
      Array.from({ length: 50 }, (_, i) => part(i + 52)),
      "gap-cursor",
    ),
  );
  expect(recovered.older).toEqual({ before: "gap-cursor", through: "101:1" });
  const loaded = {
    ...recovered,
    older: { before: "progress-cursor", through: "101:1" },
    messages: [part(20), ...recovered.messages],
  };
  expect(
    mergeRun(
      loaded,
      snapshot(
        Array.from({ length: 50 }, (_, i) => part(i + 53)),
        "latest",
      ),
    ).older?.before,
  ).toBe("progress-cursor");
});
