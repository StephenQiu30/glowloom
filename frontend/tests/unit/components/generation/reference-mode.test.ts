import { Editor } from "@tiptap/core";
import { expect, it } from "vitest";
import {
  referenceMode,
  referenceGeneration,
} from "@/components/generation/reference-mode";
import {
  emptyPromptDraft,
  type ReferenceBinding,
} from "@/components/generation/prompt-document";
import {
  ComposerSession,
  composerEditorOptions,
} from "@/components/generation/composer-session";

const model: API.ModelResponse = {
  current_version: {
    id: "v1",
    modes: ["words", "reference"],
    limits: {
      roles: {
        reference_image: {
          types: ["image"],
          min_count: 1,
          for_modes: ["reference"],
        },
      },
    },
  },
  input_roles: ["reference_image"],
};
const binding: ReferenceBinding = {
  id: "image",
  role: "reference_image",
  mediaKind: "image",
  order: 0,
  bindingGeneration: 1,
  source: { kind: "media", sourceId: "asset", sourceVersion: "1" },
};

it("全能参考优先使用目录声明的统一模式，不推断模式别名", () => {
  const draft = { ...emptyPromptDraft(), modelVersion: "v1" };
  expect(referenceMode(draft, model)).toBe("words");
  expect(referenceMode({ ...draft, references: [binding] }, model)).toBe(
    "reference",
  );
  expect(
    referenceMode(draft, {
      ...model,
      current_version: {
        ...model.current_version,
        modes: ["words", "omni_reference"],
      },
    }),
  ).toBe("omni_reference");
  expect(
    referenceMode(
      { ...draft, references: [{ ...binding, mediaKind: "video" }] },
      model,
    ),
  ).toBeNull();
});

it("添加参考与内部模式变化是同一次编辑，撤销重做不会留下错误模式", () => {
  const editor = new Editor(
    composerEditorOptions({
      contextKey: "user/project/draft",
      replicaId: "one",
      draft: { ...emptyPromptDraft(), modelVersion: "v1" },
      resolveGeneration: (draft, previous) =>
        referenceGeneration(draft, model, previous),
    }),
  );
  try {
    const session = new ComposerSession(editor);
    expect(session.snapshot().draft.mode).toBe("words");
    session.insertReference(binding);
    expect(session.snapshot().draft.mode).toBe("reference");
    session.undo();
    expect(session.snapshot().draft.references).toHaveLength(0);
    expect(session.snapshot().draft.mode).toBe("words");
    expect(session.undo()).toBe(false);
    session.redo();
    expect(session.snapshot().draft.mode).toBe("reference");
    expect(session.snapshot().draft.references).toHaveLength(1);
  } finally {
    editor.destroy();
  }
});

it("模式所需参考数量按实际素材去重，同一素材重复引用不会满足双参考合同", () => {
  const pair = {
    ...model,
    current_version: {
      ...model.current_version,
      modes: ["reference"],
      limits: {
        roles: {
          reference_image: { types: ["image"], min_count: 2 },
        },
      },
    },
  };
  const draft = {
    ...emptyPromptDraft(),
    modelVersion: "v1",
    references: [binding, { ...binding, id: "duplicate" }],
  };
  expect(referenceMode(draft, pair)).toBeNull();
  draft.references[1] = {
    ...binding,
    id: "second",
    source: { kind: "media", sourceId: "second", sourceVersion: "1" },
  };
  expect(referenceMode(draft, pair)).toBe("reference");
});

it("模型参数随合同替换，移除不可编辑的旧字段，并与模型一起撤销重做", () => {
  const next: API.ModelResponse = {
    current_version: {
      id: "v2",
      modes: ["omni_reference"],
      limits: { max_outputs: 2 },
      param_schema: [
        {
          field: "aspect_ratio",
          label: "比例",
          type: "string",
          component: "segmented",
          enum: ["1:1", "16:9"],
          default: "1:1",
        },
        {
          field: "resolution",
          label: "分辨率",
          type: "string",
          component: "select",
          enum: ["720P"],
          default: "720P",
        },
      ],
    },
  };
  const before = {
    ...emptyPromptDraft(),
    modelVersion: "v1",
    mode: "words",
    params: {
      aspect_ratio: "16:9",
      resolution: "4K",
      old_parameter: true,
      output_count: 4,
    },
  };
  const editor = new Editor(
    composerEditorOptions({
      contextKey: "user/project/schema",
      replicaId: "one",
      draft: before,
      resolveGeneration: (draft, previous) =>
        referenceGeneration(
          draft,
          draft.modelVersion === "v2" ? next : model,
          previous,
        ),
    }),
  );
  try {
    const session = new ComposerSession(editor);
    session.resolveModeSelection(
      session.beginModeSelection({
        modelVersion: "v2",
        mode: "omni_reference",
      }),
    );
    expect(session.snapshot().draft.params).toEqual({
      aspect_ratio: "16:9",
      resolution: "720P",
      output_count: 2,
    });
    session.undo();
    expect(session.snapshot().draft.params).toEqual(before.params);
    expect(session.snapshot().draft.modelVersion).toBe("v1");
    session.redo();
    expect(session.snapshot().draft.params).not.toHaveProperty("old_parameter");
    session.updateParams({ aspect_ratio: "9:16", output_count: 1 });
    expect(session.snapshot().draft.params.aspect_ratio).toBe("9:16"); // Editing does not silently rewrite input.
  } finally {
    editor.destroy();
  }
});
