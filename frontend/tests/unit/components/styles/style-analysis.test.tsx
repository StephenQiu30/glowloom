import { useState } from "react";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import {
  createStyleAnalysisDraft,
  StyleAnalysisDialog,
} from "@/components/styles/style-analysis-dialog";
import { validatePromptDraft } from "@/components/generation/prompt-document";
import { emptyPromptDraft } from "@/components/generation/prompt-document";
import {
  ConversationProvider,
  useConversationContext,
} from "@/components/conversations/conversation-context";
const data = vi.hoisted(() => ({
  ensure: vi.fn(),
  createCopy: vi.fn(),
  execute: vi.fn(),
  writable: true,
  reply: "风格名称：电影感\n\n风格描述：低饱和冷色、柔和侧光。",
}));
vi.mock("@/api/projects", () => ({ ensureIndependentCreation: data.ensure }));
vi.mock("@/components/media/copy-personal-references", () => ({
  createPersonalReferenceCopy: data.createCopy,
  executePersonalReferenceCopy: data.execute,
}));
const actorId = "11111111-1111-4111-8111-111111111111";
const scopeId = "22222222-2222-4222-8222-222222222222";
const projectId = "33333333-3333-4333-8333-333333333333";
const sourceId = "44444444-4444-4444-8444-444444444444";
const targetId = "55555555-5555-4555-8555-555555555555";
vi.mock("@/components/auth/session", () => ({
  useOptionalSession: () => ({
    actor: { id: "11111111-1111-4111-8111-111111111111" },
  }),
}));
vi.mock("@/components/spaces/space-context", () => ({
  useSpace: () => ({
    selected: {
      id: "22222222-2222-4222-8222-222222222222",
      writable: data.writable,
    },
    pending: false,
  }),
}));
vi.mock("@/components/conversations/conversation-workspace", () => ({
  ConversationWorkspace: function FakeWorkspace({
    initialDraft,
    onUseReply,
  }: {
    initialDraft: { references: unknown };
    onUseReply: (text: string) => void;
  }) {
    const memory = useConversationContext();
    return (
      <section aria-label="正常 Agent 分析">
        <output>{JSON.stringify(initialDraft.references)}</output>
        <button onClick={() => onUseReply(data.reply)}>采用回复</button>
        <button
          onClick={() => {
            const draft = emptyPromptDraft();
            draft.mode = "analysis";
            memory?.saveDraft(
              "33333333-3333-4333-8333-333333333333/new",
              draft,
            );
          }}
        >
          编辑分析输入
        </button>
      </section>
    );
  },
}));
const references = [
  {
    id: sourceId,
    revision: 2,
    asset_id: sourceId,
    asset_revision: 1,
    title: "参考图",
  },
];
const asset = {
  id: targetId,
  project_id: projectId,
  kind: "image",
  revision: 1,
};
const command = {
  key: "same-copy-key",
  input: { items: [{ id: sourceId, revision: 2 }] },
};
beforeEach(() => {
  vi.clearAllMocks();
  data.writable = true;
  data.reply = "风格名称：电影感\n\n风格描述：低饱和冷色、柔和侧光。";
  data.ensure.mockResolvedValue({
    id: projectId,
    canvas_id: null,
    name: "独立创作",
    aspect_ratio: "16:9",
    style_key: null,
    revision: 1,
    update_time: "2026-11-07T00:00:00Z",
  });
  data.createCopy.mockResolvedValue(command);
  data.execute.mockResolvedValue([asset]);
});
afterEach(cleanup);
it("prepares only copied project images and waits for manual Agent reply adoption", async () => {
  const use = vi.fn();
  render(
    <StyleAnalysisDialog
      owner={{ actorId, scopeId }}
      references={references}
      onUse={use}
      onClose={() => {}}
    />,
  );
  const workspace = await screen.findByRole("region", {
    name: "正常 Agent 分析",
  });
  expect(workspace.textContent).toContain(targetId);
  expect(workspace.textContent).not.toContain(sourceId);
  expect(use).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "采用回复" }));
  expect(use).toHaveBeenCalledWith(data.reply);
});
it("does not prepare or run an analysis without a writable matching space", async () => {
  data.writable = false;
  render(
    <StyleAnalysisDialog
      owner={{ actorId, scopeId }}
      references={references}
      onUse={() => {}}
      onClose={() => {}}
    />,
  );
  await screen.findByText("请先准备创作空间");
  expect(data.ensure).not.toHaveBeenCalled();
  expect(data.execute).not.toHaveBeenCalled();
});
it("recovers an uncertain copy by reusing the frozen command", async () => {
  data.execute
    .mockRejectedValueOnce(new Error("copy unknown"))
    .mockResolvedValueOnce([asset]);
  render(
    <StyleAnalysisDialog
      owner={{ actorId, scopeId }}
      references={references}
      onUse={() => {}}
      onClose={() => {}}
    />,
  );
  fireEvent.click(await screen.findByRole("button", { name: "重试原复制" }));
  await screen.findByRole("region", { name: "正常 Agent 分析" });
  expect(data.createCopy).toHaveBeenCalledTimes(1);
  expect(data.execute.mock.calls[0][0]).toBe(data.execute.mock.calls[1][0]);
});
it("constructs a valid labeled image prompt and rejects foreign project assets", () => {
  const draft = createStyleAnalysisDraft(projectId, [asset]);
  expect(() => validatePromptDraft(draft)).not.toThrow();
  expect(draft.modelVersion).toBeNull();
  expect(draft.document.content[0]?.content?.[0]).toMatchObject({
    text: expect.stringContaining("不要生成图片"),
  });
  expect(() =>
    createStyleAnalysisDraft(projectId, [{ ...asset, project_id: sourceId }]),
  ).toThrow();
});
function OuterDraft() {
  const memory = useConversationContext();
  const [, setTick] = useState(0);
  return (
    <>
      <output aria-label="原页面草稿">
        {memory?.readDraft(`${projectId}/new`)?.mode}
      </output>
      <button
        onClick={() => {
          const draft = emptyPromptDraft();
          draft.mode = "outer";
          memory?.saveDraft(`${projectId}/new`, draft);
          setTick((n) => n + 1);
        }}
      >
        保留原页面草稿
      </button>
      <button onClick={() => setTick((n) => n + 1)}>读取原页面草稿</button>
    </>
  );
}
it("isolates analysis edits from the page conversation draft", async () => {
  render(
    <ConversationProvider>
      <OuterDraft />
      <StyleAnalysisDialog
        owner={{ actorId, scopeId }}
        references={references}
        onUse={() => {}}
        onClose={() => {}}
      />
    </ConversationProvider>,
  );
  fireEvent.click(
    screen.getByRole("button", { name: "保留原页面草稿", hidden: true }),
  );
  fireEvent.click(await screen.findByRole("button", { name: "编辑分析输入" }));
  fireEvent.click(
    screen.getByRole("button", { name: "读取原页面草稿", hidden: true }),
  );
  expect(screen.getByLabelText("原页面草稿").textContent).toBe("outer");
});
it("refuses an overlong analysis reply without adopting or truncating it", async () => {
  data.reply = `风格名称：电影感\n\n风格描述：${"光".repeat(2001)}`;
  const adopted = vi.fn();
  render(
    <StyleAnalysisDialog
      owner={{ actorId, scopeId }}
      references={references}
      onUse={adopted}
      onClose={() => {}}
    />,
  );
  fireEvent.click(await screen.findByRole("button", { name: "采用回复" }));
  await screen.findByText("分析描述必须为 1 至 2000 字，请修改后再采用。");
  expect(adopted).not.toHaveBeenCalled();
  expect(screen.getByRole("region", { name: "正常 Agent 分析" })).toBeTruthy();
});
