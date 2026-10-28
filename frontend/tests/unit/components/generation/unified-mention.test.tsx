import { Editor } from "@tiptap/core";
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import {
  ComposerSession,
  composerEditorOptions,
} from "@/components/generation/composer-session";
import { emptyPromptDraft } from "@/components/generation/prompt-document";
import { ReferenceControls } from "@/components/generation/reference-controls";

const fixture = vi.hoisted(() => ({
  actor: "11111111-1111-4111-8111-111111111111",
  scope: "22222222-2222-4222-8222-222222222222",
  list: vi.fn(),
}));
vi.mock("@/components/auth/session", () => ({
  useOptionalSession: () => ({ actor: { id: fixture.actor } }),
}));
vi.mock("@/components/spaces/space-context", () => ({
  useSpace: () => ({ selected: { id: fixture.scope } }),
}));
vi.mock("@/api/subjects", () => ({ listSubjects: fixture.list }));
vi.mock("@/components/subjects/unified-media", () => ({
  UnifiedSubjectMedia: () => <span>缩略图</span>,
}));
// The settings/library children have their own request and form tests. Here we
// exercise the parent dialog-return transaction with the real composer editor.
vi.mock("next/dynamic", () => ({
  default: () =>
    function SubjectDialogFixture(props: {
      onSaved?: (value: ReturnType<typeof createdSubject>) => void;
      onSelect?: (value: ReturnType<typeof createdSubject>) => void;
      onClose?: () => void;
    }) {
      return (
        <>
          <button
            onClick={() =>
              (props.onSaved ?? props.onSelect)?.(createdSubject())
            }
          >
            使用返回的主体
          </button>
          {props.onClose ? (
            <button onClick={props.onClose}>取消设置</button>
          ) : null}
        </>
      );
    },
}));
function createdSubject() {
  return {
    id: "33333333-3333-4333-8333-333333333333",
    owner_scope_id: fixture.scope,
    version_id: "44444444-4444-4444-8444-444444444444",
    revision: 1,
    name: "新主体",
    description: "",
    primary: {
      binding_id: "55555555-5555-4555-8555-555555555555",
      asset_id: "66666666-6666-4666-8666-666666666666",
      asset_revision: 1,
      kind: "image" as const,
      mime_type: "image/png",
      file_name: "new.png",
    },
    references: [],
    favorite: false,
    created_at: "2026-10-28T00:00:00Z",
    updated_at: "2026-10-28T00:00:00Z",
  };
}
const editors: Editor[] = [];
afterEach(() => {
  cleanup();
  editors.splice(0).forEach((editor) => {
    const dom = editor.view.dom;
    editor.destroy();
    dom.remove();
  });
  vi.clearAllMocks();
});

it("@直接读取当前空间主体，选择后替换触发文本并冻结跨项目绑定", async () => {
  const subject = {
    id: "33333333-3333-4333-8333-333333333333",
    owner_scope_id: fixture.scope,
    version_id: "44444444-4444-4444-8444-444444444444",
    revision: 1,
    name: "蓝色几何体",
    description: "稳定的几何主体",
    primary: {
      binding_id: "55555555-5555-4555-8555-555555555555",
      asset_id: "66666666-6666-4666-8666-666666666666",
      asset_revision: 2,
      kind: "image",
      mime_type: "image/png",
      file_name: "geometry.png",
    },
    references: [],
    favorite: false,
    created_at: "2026-10-28T00:00:00Z",
    updated_at: "2026-10-28T00:00:00Z",
  };
  fixture.list.mockResolvedValue({
    current_actor_id: fixture.actor,
    current_scope_id: fixture.scope,
    items: [subject],
    page: 1,
    page_size: 24,
    total: 1,
    capabilities: { intelligent_voice: { available: false } },
  });
  const editor = new Editor(
    composerEditorOptions({
      contextKey: "unified-mention",
      replicaId: "test",
      draft: emptyPromptDraft(),
    }),
  );
  editors.push(editor);
  const session = new ComposerSession(editor);
  render(
    <ReferenceControls
      session={session}
      draft={session.snapshot().draft}
      projectID="project-consuming-subject"
      authorized
      compact
    />,
  );
  act(() => {
    editor.commands.insertContent("保留正文 @");
  });
  expect(await screen.findByRole("option", { name: "创建主体" })).toBeTruthy();
  fireEvent.click(await screen.findByRole("option", { name: /蓝色几何体/ }));
  await waitFor(() => expect(editor.getText()).toBe("保留正文 主体1"));
  const draft = session.snapshot().draft;
  expect(draft.references).toHaveLength(1);
  expect(draft.references[0].source).toMatchObject({
    kind: "subject",
    sourceId: subject.id,
    sourceVersion: subject.version_id,
    media: [
      {
        sourceId: subject.primary.asset_id,
        sourceVersion: "2",
        bindingId: subject.primary.binding_id,
      },
    ],
  });
  expect(fixture.list.mock.calls[0][0]).not.toHaveProperty("project_id");
  expect(screen.queryByRole("dialog", { name: "引用候选" })).toBeNull();
});

it.each(["创建主体", "全部主体"])(
  "@进入%s再返回时保留触发区间，正文不残留@或重复标签",
  async (entry) => {
    fixture.list.mockResolvedValue({
      current_actor_id: fixture.actor,
      current_scope_id: fixture.scope,
      items: [],
      page: 1,
      page_size: 24,
      total: 0,
      capabilities: { intelligent_voice: { available: false } },
    });
    const editor = new Editor(
      composerEditorOptions({
        contextKey: `subject-dialog-${entry}`,
        replicaId: "test",
        draft: emptyPromptDraft(),
      }),
    );
    editors.push(editor);
    const session = new ComposerSession(editor);
    document.body.append(editor.view.dom);
    editor.view.dom.focus();
    render(
      <ReferenceControls
        session={session}
        draft={session.snapshot().draft}
        projectID="project"
        authorized
        compact
      />,
    );
    act(() => {
      editor.commands.insertContent("正文不丢 @");
    });
    fireEvent.click(await screen.findByRole("option", { name: entry }));
    fireEvent.click(
      await screen.findByRole("button", { name: "使用返回的主体" }),
    );
    await waitFor(() => expect(editor.getText()).toBe("正文不丢 主体1"));
    expect(session.snapshot().draft.references).toHaveLength(1);
  },
);

it.each(["创建主体", "全部主体"])(
  "@进入%s后取消保留正文、原光标和编辑器焦点",
  async (entry) => {
    fixture.list.mockResolvedValue({
      current_actor_id: fixture.actor,
      current_scope_id: fixture.scope,
      items: [],
      page: 1,
      page_size: 24,
      total: 0,
      capabilities: { intelligent_voice: { available: false } },
    });
    const editor = new Editor(
      composerEditorOptions({
        contextKey: `cancel-${entry}`,
        replicaId: "test",
        draft: emptyPromptDraft(),
      }),
    );
    editors.push(editor);
    const session = new ComposerSession(editor);
    document.body.append(editor.view.dom);
    editor.view.dom.focus();
    render(
      <ReferenceControls
        session={session}
        draft={session.snapshot().draft}
        projectID="project"
        authorized
        compact
      />,
    );
    act(() => {
      editor.commands.insertContent("保留正文 @");
    });
    const position = editor.state.selection.from;
    fireEvent.click(await screen.findByRole("option", { name: entry }));
    fireEvent.click(
      await screen.findByRole("button", {
        name: entry === "创建主体" ? "取消设置" : "关闭对话框",
      }),
    );
    await waitFor(() => expect(document.activeElement).toBe(editor.view.dom));
    expect(editor.getText()).toBe("保留正文 @");
    expect(editor.state.selection.from).toBe(position);
    expect(session.snapshot().draft.references).toHaveLength(0);
  },
);
