import { useEffect, type ReactNode } from "react";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { emptyPromptDraft } from "@/components/generation/prompt-document";
const api = vi.hoisted(() => ({
  list: vi.fn(),
  create: vi.fn(),
  read: vi.fn(),
  append: vi.fn(),
  draft: vi.fn(),
  submission: vi.fn(),
  insert: vi.fn(),
}));
vi.mock("@/api/canvasAssistant", () => ({
  listCanvasAssistantSessions: api.list,
  createCanvasAssistantSession: api.create,
  readCanvasAssistantSession: api.read,
  appendCanvasAssistantTurn: api.append,
}));
vi.mock("@/api/promptEditor", () => ({
  createPromptDraft: api.draft,
  currentPromptSubmission: api.submission,
}));
vi.mock("next/dynamic", () => ({
  default: () =>
    function Generator(props: {
      presentation?: string;
      headerActions?: ReactNode;
      onComposerSession?: (value: unknown) => void;
    }) {
      const { onComposerSession } = props;
      useEffect(() => {
        const chain = {
          focus: () => chain,
          insertContent: (value: unknown) => {
            api.insert(value);
            return chain;
          },
          run: () => true,
        };
        onComposerSession?.({
          editor: { isEditable: true, chain: () => chain },
        });
        return () => onComposerSession?.(null);
      }, [onComposerSession]);
      return (
        <div
          data-testid="formal-generator"
          data-presentation={props.presentation}
        >
          {props.headerActions}
        </div>
      );
    },
}));
import { CanvasAssistantPanel } from "@/components/canvas/assistant/canvas-assistant-panel";
const canvas = "e4647966-7348-4a34-a677-bcb46cc23402",
  project = "e4647966-7348-4a34-a677-bcb46cc23403",
  draft = "e4647966-7348-4a34-a677-bcb46cc23404";
beforeEach(() => {
  vi.clearAllMocks();
  api.list.mockResolvedValue({ items: [] });
  api.create.mockImplementation((_p, body) =>
    Promise.resolve({
      id: body.id,
      canvas_id: canvas,
      project_id: project,
      title: body.title,
      revision: 1,
      turns: [],
    }),
  );
  api.draft.mockResolvedValue({
    draft_id: draft,
    revision: 1,
    draft_hash: "a".repeat(64),
    input_hash: "b".repeat(64),
    draft: emptyPromptDraft(),
  });
  api.append.mockImplementation((p) =>
    Promise.resolve({
      id: p.sid,
      canvas_id: canvas,
      project_id: project,
      title: "新对话",
      revision: 2,
      turns: [{ draft_id: draft }],
    }),
  );
});
afterEach(async () => {
  cleanup();
  // Radix restores focus in a deferred task; let it finish in this JSDOM realm.
  await new Promise((resolve) => setTimeout(resolve, 5));
});
describe("assistant durable input binding", () => {
  it("mounts the generation editor only after its draft is durably bound", async () => {
    let resolve: (value: unknown) => void = () => {};
    api.append.mockReturnValue(
      new Promise((r) => {
        resolve = r;
      }),
    );
    render(
      <CanvasAssistantPanel
        canvasID={canvas}
        projectID={project}
        onClose={() => {}}
      />,
    );
    await waitFor(() => expect(api.append).toHaveBeenCalledOnce());
    expect(screen.queryByTestId("formal-generator")).toBeNull();
    const sid = api.append.mock.calls[0]![0].sid;
    resolve({
      id: sid,
      canvas_id: canvas,
      project_id: project,
      title: "新对话",
      revision: 2,
      turns: [{ draft_id: draft }],
    });
    await screen.findByTestId("formal-generator");
  });
  it("retries an unknown bind using the same session and draft identity", async () => {
    api.append.mockRejectedValueOnce(new Error("network lost"));
    render(
      <CanvasAssistantPanel
        canvasID={canvas}
        projectID={project}
        onClose={() => {}}
      />,
    );
    fireEvent.click(await screen.findByRole("button", { name: "重试" }));
    await screen.findByTestId("formal-generator");
    expect(api.draft).toHaveBeenCalledOnce();
    expect(api.append.mock.calls[1]![0]).toEqual(api.append.mock.calls[0]![0]);
    expect(api.append.mock.calls[1]![1]).toEqual(api.append.mock.calls[0]![1]);
  });
  it("does not mount a foreign canvas response", async () => {
    api.create.mockResolvedValue({
      id: draft,
      canvas_id: draft,
      project_id: project,
      title: "Foreign",
      revision: 1,
      turns: [],
    });
    render(
      <CanvasAssistantPanel
        canvasID={canvas}
        projectID={project}
        onClose={() => {}}
      />,
    );
    await screen.findByRole("button", { name: "重试" });
    expect(api.draft).not.toHaveBeenCalled();
    expect(screen.queryByTestId("formal-generator")).toBeNull();
  });
  it("restores an existing empty draft and blocks another empty session", async () => {
    api.list.mockResolvedValue({
      items: [
        {
          id: draft,
          canvas_id: canvas,
          project_id: project,
          title: "新对话",
          revision: 2,
          turns: [{ draft_id: draft }],
        },
      ],
    });
    render(
      <CanvasAssistantPanel
        canvasID={canvas}
        projectID={project}
        onClose={() => {}}
      />,
    );
    const editor = await screen.findByTestId("formal-generator");
    expect(editor.getAttribute("data-presentation")).toBe("assistant");
    expect(api.create).not.toHaveBeenCalled();
    expect(api.append).not.toHaveBeenCalled();
    expect(
      screen.getByRole("button", { name: "新建会话" }).hasAttribute("disabled"),
    ).toBe(true);
    expect(screen.getByRole("button", { name: "新建会话" }).title).toBe(
      "仅支持新建一个空会话",
    );
  });
  it("source shortcuts insert a request through the bound composer without creating a task", async () => {
    render(
      <CanvasAssistantPanel
        canvasID={canvas}
        projectID={project}
        onClose={() => {}}
      />,
    );
    await screen.findByTestId("formal-generator");
    fireEvent.click(screen.getByRole("button", { name: "／ 剧本开发" }));
    expect(api.insert).toHaveBeenCalledWith([
      {
        type: "paragraph",
        content: [
          {
            type: "text",
            text: "/ 剧本开发：根据我的创意完善故事、人物和剧本",
          },
        ],
      },
    ]);
    fireEvent.click(screen.getByRole("button", { name: "使用技能" }));
    fireEvent.click(await screen.findByRole("button", { name: /海报设计/ }));
    expect(api.insert).toHaveBeenCalledTimes(2);
    expect(api.create).toHaveBeenCalledOnce();
    expect(api.append).toHaveBeenCalledOnce();
  });
});
