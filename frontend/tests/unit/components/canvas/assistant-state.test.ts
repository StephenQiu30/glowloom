import { describe, expect, it } from "vitest";
import {
  assistantContinuation,
  assistantInitialDraft,
  requireAssistantSession,
} from "@/components/canvas/assistant/assistant-state";
import { emptyPromptDraft } from "@/components/generation/prompt-document";
import { draftRequest } from "@/components/generation/draft-save";
const id = "e4647966-7348-4a34-a677-bcb46cc23401",
  canvas = "e4647966-7348-4a34-a677-bcb46cc23402",
  project = "e4647966-7348-4a34-a677-bcb46cc23403",
  draftID = "e4647966-7348-4a34-a677-bcb46cc23404";
function task(): API.TaskDetail {
  const draft = emptyPromptDraft();
  draft.modelVersion = id;
  draft.mode = "text.from_text";
  draft.document.content = [
    {
      type: "paragraph",
      content: [
        { type: "text", text: "Previous context and current question" },
      ],
    },
  ];
  return {
    id,
    project_id: project,
    status: "completed",
    capability: "text.generate",
    editor_draft: draftRequest(draft),
    outputs: [
      {
        id: draftID,
        kind: "json",
        moderation_status: "passed",
        text: "Reviewed answer",
      },
    ],
  };
}
describe("canvas assistant authoritative conversation", () => {
  it("rejects foreign canvas/project or malformed duplicated identities", () => {
    const session = {
      id,
      canvas_id: canvas,
      project_id: project,
      title: "Chat",
      revision: 2,
      turns: [{ draft_id: draftID }],
    };
    expect(
      requireAssistantSession(session, canvas, project).turns,
    ).toHaveLength(1);
    expect(() => requireAssistantSession(session, id, project)).toThrow(
      "invalid_receipt",
    );
    expect(() =>
      requireAssistantSession({ ...session, project_id: id }, canvas, project),
    ).toThrow();
    expect(() =>
      requireAssistantSession(
        { ...session, turns: [session.turns[0]!, session.turns[0]!] },
        canvas,
        project,
      ),
    ).toThrow();
    expect(() =>
      requireAssistantSession(
        { ...session, turns: [{ draft_id: draftID, skill: "invented" }] },
        canvas,
        project,
      ),
    ).toThrow();
  });
  it("uses frozen reviewed output once and preserves input identities", () => {
    const value = task();
    const before = structuredClone(value);
    const next = assistantContinuation(value, project, id);
    expect(next.document.content).toHaveLength(4);
    expect(JSON.stringify(next)).toContain("Reviewed answer");
    expect(JSON.stringify(next).split("Previous context")).toHaveLength(2);
    expect(value).toEqual(before);
  });
  it("never turns an unreviewed or wrong-owner reply into context", () => {
    expect(() => assistantContinuation(task(), id, id)).toThrow();
    expect(() => assistantContinuation(task(), project, draftID)).toThrow();
    expect(() =>
      assistantContinuation({ ...task(), status: "unknown" }, project, id),
    ).toThrow();
    expect(() =>
      assistantContinuation(
        {
          ...task(),
          outputs: [
            {
              id: draftID,
              kind: "json",
              moderation_status: "rejected",
              text: "blocked",
            },
          ],
        },
        project,
        id,
      ),
    ).toThrow();
    expect(() =>
      assistantContinuation({ ...task(), outputs: [] }, project, id),
    ).toThrow("assistant_reply_unavailable");
  });
  it("does not invent an assistant answer for a terminal failed task", () => {
    const value = task();
    value.status = "failed";
    value.outputs = [];
    const draft = assistantContinuation(value, project, id);
    expect(JSON.stringify(draft)).not.toContain("Reviewed answer");
    expect(draft.document.content).toHaveLength(2);
  });
  it("copies initial references without mutating the supplied selection", () => {
    const initial = emptyPromptDraft();
    const next = assistantInitialDraft(initial);
    next.document.content.push({ type: "paragraph" });
    expect(initial.document.content).toHaveLength(1);
  });
  it("continues from actual reviewed image outputs using reauthorized fixed references", () => {
    const value = task();
    value.capability = "image.generate";
    value.outputs = [
      {
        id: draftID,
        kind: "image",
        media_asset_id: canvas,
        moderation_status: "passed",
      },
    ];
    expect(() => assistantContinuation(value, project, id)).toThrow(
      "assistant_reply_unavailable",
    );
    const next = assistantContinuation(value, project, id, [
      { id: canvas, project_id: project, kind: "image", revision: 5 },
    ]);
    expect(next.references[0]?.source).toEqual({
      kind: "media",
      sourceId: canvas,
      sourceVersion: "5",
    });
    expect(next.labelState.highWater.image).toBe(1);
    expect(next.modelVersion).toBeNull();
    expect(next.mode).toBeNull();
    expect(() =>
      assistantContinuation(value, project, id, [
        { id: canvas, project_id: id, kind: "image", revision: 5 },
      ]),
    ).toThrow();
  });
  it("separates discussion prompts from explicit image execution", () => {
    const discussion = assistantInitialDraft(undefined, "poster");
    const image = assistantInitialDraft(undefined, "poster", "image");
    expect(JSON.stringify(discussion)).toContain("讨论创作方案");
    expect(JSON.stringify(image)).toContain("生成一张完整海报");
    expect(() =>
      requireAssistantSession(
        {
          id,
          canvas_id: canvas,
          project_id: project,
          title: "test",
          revision: 2,
          turns: [
            { draft_id: draftID, skill: "storyboard", execution: "image" },
          ],
        },
        canvas,
        project,
      ),
    ).toThrow();
  });
});
