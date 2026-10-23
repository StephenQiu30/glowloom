import { Editor } from "@tiptap/core";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  ComposerSession,
  composerEditorOptions,
} from "@/components/generation/composer-session";
import {
  emptyPromptDraft,
  type ReferenceBinding,
} from "@/components/generation/prompt-document";

const editors: Editor[] = [];
function create(draft = emptyPromptDraft(), replicaId = "tab-a") {
  const editor = new Editor(
    composerEditorOptions({ contextKey: "u/p/d", replicaId, draft }),
  );
  editors.push(editor);
  return new ComposerSession(editor);
}
function media(id: string, role = "reference_image"): ReferenceBinding {
  return {
    id,
    mediaKind: "image",
    role,
    order: 0,
    bindingGeneration: 1,
    source: { kind: "media", sourceId: "asset-1", sourceVersion: "v1" },
  };
}
afterEach(() => {
  editors.splice(0).forEach((e) => e.destroy());
  vi.useRealTimers();
});

describe("E0 完整意图与单一历史", () => {
  it("重复提及复用绑定，不同角色保留；整组移除可一次撤销", () => {
    const s = create();
    s.editor.commands.insertContent("让");
    s.insertReference(media("person"));
    s.insertReference(media("duplicate"));
    s.insertReference(media("last", "last_frame"));
    expect(s.snapshot().draft.references.map((r) => r.id)).toEqual([
      "person",
      "last",
    ]);
    expect(
      JSON.stringify(s.snapshot().draft.document).match(/person/g),
    ).toHaveLength(2);
    const before = s.snapshot().draft;
    s.removeReference("person");
    expect(JSON.stringify(s.snapshot().draft.document)).not.toContain("person");
    s.undo();
    expect(s.snapshot().draft).toEqual(before);
    s.redo();
    expect(s.snapshot().draft.references.map((r) => r.id)).toEqual(["last"]);
  });
  it("正文、引用、模式、参数共用历史；撤销到空后重做不丢编号", () => {
    const s = create();
    s.editor.commands.insertContent("场景");
    s.insertReference(media("one"));
    const request = s.beginModeSelection({
      modelVersion: "m-v1",
      mode: "omni",
    });
    s.resolveModeSelection(request);
    s.updateParams({ duration: 5 });
    const complete = s.snapshot().draft;
    for (let i = 0; i < 4; i++) s.undo();
    expect(s.editor.getText()).toBe("");
    expect(s.snapshot().draft.references).toEqual([]);
    expect(s.snapshot().draft.labelState.highWater.image).toBe(1);
    for (let i = 0; i < 4; i++) s.redo();
    expect(s.snapshot().draft).toEqual(complete);
  });
  it("删掉正文最后一处提及仍保留卡片", () => {
    const s = create();
    s.insertReference(media("one"));
    s.editor.commands.selectAll();
    s.editor.commands.deleteSelection();
    expect(s.snapshot().draft.references).toHaveLength(1);
    expect(s.editor.getText()).toBe("");
    s.undo();
    expect(JSON.stringify(s.snapshot().draft.document)).toContain("one");
  });
  it("拒绝悬空 reference 和未知 schema，不静默转空白", () => {
    const s = create();
    s.editor.commands.insertContent({
      type: "reference",
      attrs: { referenceId: "forged" },
    });
    expect(s.editor.getText()).toBe("");
    expect(() =>
      create({ ...emptyPromptDraft(), schemaVersion: 99 } as never),
    ).toThrow();
  });
});

describe("E0 模式代次与完整保存快照", () => {
  it("B 已生效、C 待返回时 undo 回 A；迟到 C 不能覆盖，redo 仅回 B", () => {
    const draft = { ...emptyPromptDraft(), mode: "A", modelVersion: "m-v1" };
    const s = create(draft);
    s.resolveModeSelection(
      s.beginModeSelection({ modelVersion: "m-v1", mode: "B" }),
    );
    const c = s.beginModeSelection({ modelVersion: "m-v1", mode: "C" });
    expect(s.snapshot().draft.mode).toBe("B");
    s.undo();
    expect(s.resolveModeSelection(c)).toBe(false);
    expect(s.snapshot().draft.mode).toBe("A");
    s.redo();
    expect(s.snapshot().draft.mode).toBe("B");
  });
  it("普通输入不取消待选；返回按最新文档应用模式且保留所有参考", () => {
    const s = create();
    const c = s.beginModeSelection({ modelVersion: "m2", mode: "text" });
    s.insertReference(media("one"));
    s.editor.commands.insertContent("仍然保留");
    expect(s.resolveModeSelection(c)).toBe(true);
    expect(s.snapshot().draft.references).toHaveLength(1);
    expect(s.editor.getText()).toContain("仍然保留");
    s.undo();
    expect(s.snapshot().draft.mode).toBe(null);
    expect(s.snapshot().draft.references).toHaveLength(1);
  });
  it("无历史 undo 也取消查询；取消、跨实例和销毁后旧回执不能生效", () => {
    const s = create();
    const c = s.beginModeSelection({ modelVersion: "m1", mode: "C" });
    s.undo();
    expect(s.resolveModeSelection(c)).toBe(false);
    const d = s.beginModeSelection({ modelVersion: "m1", mode: "D" });
    const other = create(emptyPromptDraft(), "tab-b");
    expect(other.resolveModeSelection(d)).toBe(false);
    s.cancelModeSelection();
    expect(s.resolveModeSelection(d)).toBe(false);
    const e = s.beginModeSelection({ modelVersion: "m1", mode: "E" });
    s.editor.destroy();
    expect(s.resolveModeSelection(e)).toBe(false);
  });
  it("保存快照不含待选与历史；冻结后继续编辑不会改写旧请求内容", () => {
    const s = create();
    s.insertReference(media("one"));
    const frozen = s.snapshot();
    s.beginModeSelection({ modelVersion: "m", mode: "text" });
    s.editor.commands.insertContent("新内容");
    expect(JSON.stringify(frozen)).not.toContain("新内容");
    expect(JSON.stringify(s.snapshot())).not.toMatch(
      /pendingTarget|selectionEpoch|past|future/,
    );
    expect(s.snapshot().localSeq).toBeGreaterThan(frozen.localSeq);
    frozen.draft.references.length = 0;
    expect(s.snapshot().draft.references).toHaveLength(1);
  });
  it("分配后撤销的编号元数据仍需保存，往返后不复用已删除号", () => {
    const s = create();
    const initial = s.snapshot();
    s.insertReference(media("one"));
    s.undo();
    const after = s.snapshot();
    expect(after.draft.document).toEqual(initial.draft.document);
    expect(after.draft.labelState).not.toEqual(initial.draft.labelState);
    expect(after.localSeq).toBeGreaterThan(initial.localSeq);
    const restored = create(JSON.parse(JSON.stringify(after.draft)), "tab-new");
    restored.insertReference(media("two"));
    expect(restored.snapshot().draft.labelState.allocations.two.ordinal).toBe(
      2,
    );
  });
  it("无素材 ID 的 pending 占位可完整往返，但拒绝伪造 ready 或 URL", () => {
    const s = create();
    const pending: ReferenceBinding = {
      ...media("upload"),
      source: {
        kind: "pending_upload",
        clientUploadId: "file",
        uploadAttemptId: "attempt",
      },
    };
    s.insertReference(pending);
    expect(create(s.snapshot().draft).snapshot().draft).toEqual(
      s.snapshot().draft,
    );
    expect(() =>
      s.insertReference({
        ...pending,
        id: "bad",
        source: {
          ...pending.source,
          ready: true,
          url: "https://private.example",
        },
      } as never),
    ).toThrow();
  });
});

describe("E0 输入事务边界", () => {
  it("相邻文字 500ms 内合并，移动选区、超时和参数操作断开分组", () => {
    const s = create();
    s.editor.view.dispatch(s.editor.state.tr.insertText("a").setTime(1000));
    s.editor.view.dispatch(s.editor.state.tr.insertText("b").setTime(1400));
    s.undo();
    expect(s.editor.getText()).toBe("");
    s.redo();
    s.editor.view.dispatch(s.editor.state.tr.insertText("c").setTime(2000));
    s.undo();
    expect(s.editor.getText()).toBe("ab");
    s.editor.commands.setTextSelection(1);
    s.editor.view.dispatch(s.editor.state.tr.insertText("x").setTime(2010));
    s.undo();
    expect(s.editor.getText()).toBe("ab");
  });
  it("一次合成跨越 500ms 仍是一组，合成期间不执行业务命令", () => {
    const s = create();
    s.editor.view.dom.dispatchEvent(
      new CompositionEvent("compositionstart", { bubbles: true }),
    );
    s.editor.view.dispatch(
      s.editor.state.tr
        .insertText("中")
        .setMeta("composition", 1)
        .setTime(1000),
    );
    s.editor.view.dispatch(
      s.editor.state.tr
        .insertText("文")
        .setMeta("composition", 1)
        .setTime(2500),
    );
    expect(s.insertReference(media("ignored"))).toBe(false);
    expect(s.undo()).toBe(false);
    s.editor.view.dom.dispatchEvent(
      new CompositionEvent("compositionend", { bubbles: true }),
    );
    // PM clears view.composing on its own compositionend listener.
    s.undo();
    expect(s.editor.getText()).toBe("");
    s.redo();
    expect(s.editor.getText()).toBe("中文");
  });
  it("原生 historyUndo/historyRedo 与应用命令共用历史，阻止浏览器私自回滚", () => {
    const s = create();
    s.insertReference(media("one"));
    const undo = new InputEvent("beforeinput", {
      inputType: "historyUndo",
      bubbles: true,
      cancelable: true,
    });
    s.editor.view.dom.dispatchEvent(undo);
    expect(undo.defaultPrevented).toBe(true);
    expect(s.snapshot().draft.references).toHaveLength(0);
    s.editor.view.dom.dispatchEvent(
      new InputEvent("beforeinput", {
        inputType: "historyRedo",
        bubbles: true,
        cancelable: true,
      }),
    );
    expect(s.snapshot().draft.references).toHaveLength(1);
  });
  it("Backspace 第一次选中原子标签，第二次只删提及", () => {
    const s = create();
    s.insertReference(media("one"));
    const key = () =>
      s.editor.view.dom.dispatchEvent(
        new KeyboardEvent("keydown", {
          key: "Backspace",
          bubbles: true,
          cancelable: true,
        }),
      );
    key();
    expect(s.editor.state.selection.toJSON().type).toBe("node");
    expect(s.snapshot().draft.references).toHaveLength(1);
    key();
    expect(s.editor.getText()).toBe("");
    expect(s.snapshot().draft.references).toHaveLength(1);
    s.undo();
    expect(s.editor.getText()).toBe("图片1");
  });
  it("空历史原生 undo 仍取消待选，不产生额外本地保存序号", () => {
    const s = create();
    const request = s.beginModeSelection({ mode: "omni", modelVersion: "m1" });
    s.editor.view.dom.dispatchEvent(
      new InputEvent("beforeinput", {
        inputType: "historyUndo",
        bubbles: true,
        cancelable: true,
      }),
    );
    expect(s.resolveModeSelection(request)).toBe(false);
    expect(s.snapshot().localSeq).toBe(0);
  });
  it("两个相同目标选择仍按代次判断，旧回执不能抢先生效", () => {
    const s = create();
    const a = s.beginModeSelection({ mode: "omni", modelVersion: "m1" });
    const b = s.beginModeSelection({ mode: "omni", modelVersion: "m1" });
    expect(s.resolveModeSelection(a)).toBe(false);
    expect(s.resolveModeSelection(b)).toBe(true);
  });
  it("只读时不能通过原生 undo 或业务命令绕过", () => {
    const s = create();
    s.insertReference(media("one"));
    s.editor.setEditable(false);
    expect(s.undo()).toBe(false);
    expect(s.removeReference("one")).toBe(false);
    expect(s.snapshot().draft.references).toHaveLength(1);
  });
  it("初始未知节点、额外属性及重复编号被拒绝", () => {
    const s = create();
    s.insertReference(media("one"));
    const draft = s.snapshot().draft;
    expect(() =>
      create({
        ...draft,
        document: { type: "doc", content: [{ type: "script" }] },
      } as never),
    ).toThrow();
    draft.labelState.allocations.other = { labelKind: "image", ordinal: 1 };
    expect(() => create(draft)).toThrow();
  });
});

it("模式能力在合成期间返回时延后应用，完成文字后只产生一个模式历史项", async () => {
  const s = create();
  const request = s.beginModeSelection({ modelVersion: "m", mode: "omni" });
  s.editor.view.dom.dispatchEvent(
    new CompositionEvent("compositionstart", { bubbles: true }),
  );
  s.editor.view.dispatch(
    s.editor.state.tr.insertText("中文").setMeta("composition", 1),
  );
  expect(s.resolveModeSelection(request)).toBe(true);
  expect(s.snapshot().draft.mode).toBe(null);
  s.editor.view.dom.dispatchEvent(
    new CompositionEvent("compositionend", { bubbles: true }),
  );
  await vi.waitFor(() => expect(s.snapshot().draft.mode).toBe("omni"));
  s.undo();
  expect(s.snapshot().draft.mode).toBe(null);
  expect(s.editor.getText()).toBe("中文");
  s.undo();
  expect(s.editor.getText()).toBe("");
});

it("合成中的保存快捷键延后冻结完整文字，待选模式暂停提交快捷键", async () => {
  const save = vi.fn(),
    submit = vi.fn();
  const editor = new Editor(
    composerEditorOptions({
      contextKey: "u/p/d",
      replicaId: "keys",
      draft: emptyPromptDraft(),
    }),
  );
  editors.push(editor);
  const s = new ComposerSession(editor);
  editor.on("composerSaveRequested", save);
  editor.on("composerSubmitRequested", submit);
  editor.view.dom.dispatchEvent(
    new CompositionEvent("compositionstart", { bubbles: true }),
  );
  const event = new KeyboardEvent("keydown", {
    key: "s",
    ctrlKey: true,
    cancelable: true,
    bubbles: true,
    isComposing: true,
  });
  editor.view.dom.dispatchEvent(event);
  expect(event.defaultPrevented).toBe(true);
  expect(save).not.toHaveBeenCalled();
  editor.view.dispatch(
    editor.state.tr.insertText("完整中文").setMeta("composition", 1),
  );
  editor.view.dom.dispatchEvent(
    new CompositionEvent("compositionend", { bubbles: true }),
  );
  await vi.waitFor(() => expect(save).toHaveBeenCalledTimes(1));
  expect(JSON.stringify(save.mock.calls[0][0])).toContain("完整中文");
  s.beginModeSelection({ mode: "omni", modelVersion: "m1" });
  editor.view.dom.dispatchEvent(
    new KeyboardEvent("keydown", {
      key: "Enter",
      ctrlKey: true,
      bubbles: true,
      cancelable: true,
    }),
  );
  expect(submit).not.toHaveBeenCalled();
});

describe("E2 参考与异步身份", () => {
  it("替换、排序和粘贴各作为完整操作撤销", () => {
    const s = create();
    s.insertReference(media("one"));
    s.insertReference(media("two", "last_frame"));
    const before = s.snapshot().draft;
    s.replaceReference("one", {
      ...media("ignored"),
      source: { kind: "media", sourceId: "asset-2", sourceVersion: "2" },
    });
    expect(s.snapshot().draft.references[0].bindingGeneration).toBe(2);
    expect(s.snapshot().draft.labelState.allocations.one).toEqual(
      before.labelState.allocations.one,
    );
    s.undo();
    expect(s.snapshot().draft).toEqual(before);
    s.reorderReference("two", -1);
    expect(s.snapshot().draft.references[0].id).toBe("two");
    s.undo();
    expect(s.snapshot().draft.references[0].id).toBe("one");
    const target = create();
    expect(target.importFragment(before, 0)).toBe(true);
    expect(target.snapshot().draft.references).toHaveLength(2);
    expect(target.snapshot().draft.references[0].id).not.toBe("one");
    target.undo();
    expect(target.snapshot().draft.references).toEqual([]);
    expect(target.snapshot().draft.labelState.highWater.image).toBe(2);
  });
  it("上传结果不能跨实例、删除后复活或覆盖新尝试", () => {
    const s = create(),
      ref = {
        ...media("upload"),
        source: {
          kind: "pending_upload" as const,
          clientUploadId: "file",
          uploadAttemptId: "attempt",
        },
      };
    s.insertReference(ref);
    const ticket = {
      contextKey: "u/p/d",
      replicaId: "tab-a",
      referenceId: "upload",
      generation: 1,
      attempt: "attempt",
    };
    const source = {
      kind: "media" as const,
      sourceId: "asset",
      sourceVersion: "1",
    };
    expect(s.resolveUpload({ ...ticket, replicaId: "tab-b" }, source)).toBe(
      false,
    );
    s.removeReference("upload");
    expect(s.resolveUpload(ticket, source)).toBe(false);
    s.undo();
    s.replaceReference("upload", {
      ...ref,
      source: { ...ref.source, uploadAttemptId: "new" },
    });
    expect(s.resolveUpload(ticket, source)).toBe(false);
    expect(
      s.resolveUpload({ ...ticket, generation: 2, attempt: "new" }, source),
    ).toBe(true);
  });
  it("迟到转换和粘贴不覆盖后续输入", () => {
    const s = create();
    const seq = s.snapshot().localSeq;
    s.editor.commands.insertContent("new");
    expect(s.applyConversion(seq, {}, [])).toBe(false);
    expect(s.importFragment(emptyPromptDraft(), seq)).toBe(false);
    expect(s.editor.getText()).toBe("new");
  });
});
it("删除期间完成的上传不会复活绑定，撤销删除才复用同代结果", () => {
  const s = create();
  s.insertReference({
    ...media("upload"),
    source: {
      kind: "pending_upload",
      clientUploadId: "file",
      uploadAttemptId: "attempt",
    },
  });
  s.removeReference("upload");
  expect(
    s.resolveUpload(
      {
        contextKey: "u/p/d",
        replicaId: "tab-a",
        referenceId: "upload",
        generation: 1,
        attempt: "attempt",
      },
      { kind: "media", sourceId: "ready", sourceVersion: "1" },
    ),
  ).toBe(false);
  expect(s.snapshot().draft.references).toEqual([]);
  s.undo();
  expect(s.snapshot().draft.references[0].source).toEqual({
    kind: "media",
    sourceId: "ready",
    sourceVersion: "1",
  });
  s.redo();
  expect(s.snapshot().draft.references).toEqual([]);
});

it("批量候选按选择顺序替换 @ 查询，一次撤销恢复文字且编号不回退", () => {
  const session = create();
  session.editor.commands.insertContent("@图片");
  const first = media("one"),
    second = media("two", "last_frame");
  session.insertReferences([first, second], { from: 1, to: 4 });
  expect(session.snapshot().draft.references.map((ref) => ref.role)).toEqual([
    "reference_image",
    "last_frame",
  ]);
  expect(session.editor.getText()).not.toContain("@图片");
  expect(session.snapshot().draft.labelState.highWater.image).toBe(2);
  session.undo();
  expect(session.snapshot().draft.references).toHaveLength(0);
  expect(session.editor.getText()).toBe("@图片");
  expect(session.snapshot().draft.labelState.highWater.image).toBe(2);
});

it("routes collaborative history and accepts authoritative label corrections without echo history", () => {
  const undo = vi.fn(() => true),
    redo = vi.fn(() => true);
  let pending = true;
  const editor = new Editor(
    composerEditorOptions({
      contextKey: "ot",
      replicaId: "replica",
      draft: emptyPromptDraft(),
      collaboration: {
        history: { undo, redo },
        pendingReference: () => pending,
        composition: () => {},
      },
    }),
  );
  editors.push(editor);
  const session = new ComposerSession(editor);
  session.insertReference(media("r"));
  expect(
    editor.view.dom.querySelector("[data-prompt-reference]")?.textContent,
  ).toBe("待确认引用");
  const canonical = session.snapshot().draft;
  canonical.labelState.highWater.image = 2;
  canonical.labelState.allocations.r.ordinal = 2;
  canonical.references[0].bindingGeneration = 2;
  pending = false;
  session.applyAuthoritative(canonical, editor.state.tr);
  expect(session.snapshot().draft.labelState.allocations.r.ordinal).toBe(2);
  expect(
    editor.view.dom.querySelector("[data-prompt-reference]")?.textContent,
  ).toBe("图片2");
  expect(session.undo()).toBe(true);
  expect(undo).toHaveBeenCalledOnce();
  expect(session.snapshot().draft.references).toHaveLength(1);
  session.redo();
  expect(redo).toHaveBeenCalledOnce();
});

it("records a negative-prompt edit with the native input range", () => {
  const session = create();
  session.updateParams({ negative_prompt: "aaa" });
  let received: unknown;
  session.editor.on("transaction", ({ transaction }) => {
    received = transaction.getMeta("composer-text-parameter");
  });
  expect(
    session.updateTextParam("negative_prompt", "aa", {
      from: 1,
      to: 2,
      text: "",
    }),
  ).toBe(true);
  expect(received).toEqual({
    field: "negative_prompt",
    from: 1,
    to: 2,
    text: "",
  });
  expect(session.snapshot().draft.params.negative_prompt).toBe("aa");
});

it("unmount before composition settlement releases global composition", () => {
  vi.useFakeTimers();
  const events: boolean[] = [];
  const editor = new Editor(
    composerEditorOptions({
      contextKey: "review",
      replicaId: "review",
      draft: emptyPromptDraft(),
      collaboration: {
        history: { undo: () => false, redo: () => false },
        pendingReference: () => false,
        composition: (active) => events.push(active),
      },
    }),
  );
  editors.push(editor);
  // Invoke actual ProseMirror DOM handlers in browser event order; destruction
  // before the 0ms settlement is the ordinary React unmount race.
  for (const plugin of editor.state.plugins)
    plugin.props.handleDOMEvents?.compositionstart?.call(
      plugin,
      editor.view,
      new CompositionEvent("compositionstart"),
    );
  for (const plugin of editor.state.plugins)
    plugin.props.handleDOMEvents?.compositionend?.call(
      plugin,
      editor.view,
      new CompositionEvent("compositionend"),
    );
  editor.destroy();
  vi.runAllTimers();
  expect(events.at(-1)).toBe(false);
});
