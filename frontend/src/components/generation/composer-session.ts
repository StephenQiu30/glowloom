import {
  Decoration,
  DecorationSet,
  type Decoration as PMDecoration,
} from "@tiptap/pm/view";
import type { TextInputEdit } from "./text-input-intent";
import Suggestion, { exitSuggestion } from "@tiptap/suggestion";
import { Editor, Extension, Node, type EditorOptions } from "@tiptap/core";
import Document from "@tiptap/extension-document";
import Paragraph from "@tiptap/extension-paragraph";
import Text from "@tiptap/extension-text";
import HardBreak from "@tiptap/extension-hard-break";
import { Node as PMNode, Slice, Fragment } from "@tiptap/pm/model";
import {
  Plugin,
  PluginKey,
  NodeSelection,
  Selection,
  type Transaction,
  type EditorState,
} from "@tiptap/pm/state";
import {
  emptyPromptDraft,
  labelKind,
  referenceLabel,
  sameReference,
  validatePromptDraft,
  validateReference,
  MAX_LABEL_ALLOCATIONS,
  type PromptDraft,
  type LabelState,
  type ReferenceBinding,
} from "./prompt-document";

type Intent = Omit<PromptDraft, "document" | "labelState">;
type Frame = {
  doc: PMNode;
  selection: ReturnType<Selection["toJSON"]>;
  intent: Intent;
};
export type ModeTarget = { modelVersion: string; mode: string };
export type ModeRequest = ModeTarget & {
  contextKey: string;
  replicaId: string;
  selectionEpoch: number;
};
export type ComposerSnapshot = {
  contextKey: string;
  replicaId: string;
  localSeq: number;
  draft: PromptDraft;
};
declare module "@tiptap/core" {
  interface EditorEvents {
    composerSaveRequested: ComposerSnapshot;
    composerMention: {
      query: string;
      range: { from: number; to: number };
      index: number;
    } | null;
    composerSubmitRequested: undefined;
    composerReferenceSelected: { referenceId: string };
    composerFiles: { files: File[] };
  }
}
type HistoryEntry = { before: Frame; after: Frame };
type SessionState = {
  contextKey: string;
  replicaId: string;
  frame: Frame;
  completedUploads: Record<string, ReferenceBinding["source"]>;
  labels: LabelState;
  past: HistoryEntry[];
  future: HistoryEntry[];
  localSeq: number;
  selectionEpoch: number;
  pendingTarget: ModeRequest | null;
  deferredMode: ModeRequest | null;
  deferredSave: boolean;
  composing: boolean;
  group: { kind: string; time: number } | null;
};
type Action = {
  authoritative?: boolean;
  settledUpload?: { key: string; source: ReferenceBinding["source"] };
  intent?: Intent;
  labels?: LabelState;
  isolated?: boolean;
  history?: "undo" | "redo";
  selection?: ModeTarget | "cancel";
  composing?: boolean;
  breakGroup?: boolean;
  deferMode?: ModeRequest;
  deferSave?: boolean;
};
const sessionKey = new PluginKey<SessionState>("composerSession");
const HISTORY_LIMIT = 100;
const mentionKey = new PluginKey("promptReferences");
function referenceSuggestions() {
  return Extension.create({
    name: "referenceSuggestions",
    addProseMirrorPlugins() {
      const editor = this.editor;
      let index = 0;
      let candidates: ReferenceBinding[] = [];
      let range = { from: 0, to: 0 };
      let query = "";
      const show = (props: {
        query: string;
        range: { from: number; to: number };
        items: ReferenceBinding[];
      }) => {
        query = props.query;
        range = props.range;
        candidates = props.items;
        index = Math.min(index, Math.max(0, candidates.length - 1));
        editor.emit("composerMention", { query, range, index });
      };
      return [
        Suggestion<ReferenceBinding>({
          editor,
          pluginKey: mentionKey,
          char: "@",
          allowedPrefixes: null,
          allow: () => editor.isEditable && !inComposition(editor),
          items: ({ query }) => {
            const state = stateOf(editor.state);
            return state.frame.intent.references.filter((r) =>
              referenceLabel(state.labels, r.id).includes(query),
            );
          },
          render: () => ({
            onStart: (props) => {
              index = 0;
              show(props);
            },
            onUpdate: show,
            onExit: () => editor.emit("composerMention", null),
            onKeyDown: ({ event }) => {
              if (event.key === "Escape") {
                exitSuggestion(editor.view, mentionKey);
                return true;
              }
              if (event.key === "ArrowDown" || event.key === "ArrowUp") {
                index =
                  (index +
                    (event.key === "ArrowDown" ? 1 : -1) +
                    Math.max(1, candidates.length)) %
                  Math.max(1, candidates.length);
                editor.emit("composerMention", { query, range, index });
                return true;
              }
              if (event.key === "Enter" && candidates[index]) {
                new ComposerSession(editor).insertReference(
                  candidates[index],
                  true,
                  range,
                );
                return true;
              }
              return false;
            },
          }),
        }),
      ];
    },
  });
}

function stateOf(state: EditorState): SessionState {
  const session = sessionKey.getState(state);
  if (!session) throw new Error("composer_session_missing");
  return session;
}
function frameDraft(frame: Frame, labels: LabelState): PromptDraft {
  return { ...frame.intent, document: frame.doc.toJSON(), labelState: labels };
}
function uploadKey(reference: ReferenceBinding) {
  return reference.source.kind === "pending_upload"
    ? JSON.stringify([
        reference.id,
        reference.bindingGeneration,
        reference.source.uploadAttemptId,
      ])
    : "";
}
function completedIntent(
  intent: Intent,
  completed: SessionState["completedUploads"],
): Intent {
  return {
    ...intent,
    references: intent.references.map((ref) => {
      const source = completed[uploadKey(ref)];
      return source ? { ...ref, source: structuredClone(source) } : ref;
    }),
  };
}
function candidate(
  previous: SessionState,
  tr: Transaction,
  resolveGeneration?: ComposerOptions["resolveGeneration"],
): Frame {
  const action: Action = tr.getMeta(sessionKey) ?? {};
  const frame = {
    doc: tr.doc,
    selection: tr.selection.toJSON(),
    intent:
      action.authoritative && action.intent
        ? action.intent
        : action.intent || action.settledUpload
          ? completedIntent(
              action.intent ?? previous.frame.intent,
              action.settledUpload
                ? {
                    ...previous.completedUploads,
                    [action.settledUpload.key]: action.settledUpload.source,
                  }
                : previous.completedUploads,
            )
          : previous.frame.intent,
  };
  if (resolveGeneration && !action.authoritative) {
    const selection = resolveGeneration(
      frameDraft(frame, action.labels ?? previous.labels),
      previous.frame.intent,
    );
    if (
      selection.mode !== frame.intent.mode ||
      selection.params !== frame.intent.params
    )
      frame.intent = { ...frame.intent, ...selection };
  }
  return frame;
}
function apply(
  tr: Transaction,
  previous: SessionState,
  collaborative = false,
  resolveGeneration?: ComposerOptions["resolveGeneration"],
): SessionState {
  const action: Action = tr.getMeta(sessionKey) ?? {};
  let next = {
    ...previous,
    frame: candidate(previous, tr, resolveGeneration),
    labels: action.labels ?? previous.labels,
  };
  if (action.settledUpload) {
    next.completedUploads = {
      ...previous.completedUploads,
      [action.settledUpload.key]: structuredClone(action.settledUpload.source),
    };
    return { ...next, localSeq: previous.localSeq + 1, group: null };
  }
  if (action.deferMode) next.deferredMode = action.deferMode;
  if (action.deferSave !== undefined) next.deferredSave = action.deferSave;
  if (action.selection || action.history) {
    next.deferredMode = null;
    next.selectionEpoch++;
    next.pendingTarget =
      action.selection && action.selection !== "cancel"
        ? {
            ...action.selection,
            contextKey: next.contextKey,
            replicaId: next.replicaId,
            selectionEpoch: next.selectionEpoch,
          }
        : null;
  }
  if (action.composing !== undefined) next.composing = action.composing;
  if (action.history) {
    const entries = action.history === "undo" ? previous.past : previous.future;
    const entry = entries.at(-1);
    if (!entry) return { ...next, group: null };
    next =
      action.history === "undo"
        ? {
            ...next,
            past: previous.past.slice(0, -1),
            future: [...previous.future, entry],
          }
        : {
            ...next,
            past: [...previous.past, entry],
            future: previous.future.slice(0, -1),
          };
    return { ...next, localSeq: previous.localSeq + 1, group: null };
  }
  const changed =
    !tr.doc.eq(previous.frame.doc) ||
    next.frame.intent !== previous.frame.intent;
  if (!changed) {
    if (
      action.breakGroup ||
      action.composing === true ||
      (tr.selectionSet && !tr.docChanged)
    )
      next.group = null;
    return next;
  }
  if (action.authoritative || collaborative)
    return {
      ...next,
      completedUploads: action.authoritative ? {} : next.completedUploads,
      past: [],
      future: [],
      localSeq: previous.localSeq + 1,
      group: null,
    };
  const composition = tr.getMeta("composition");
  const kind =
    composition !== undefined
      ? `composition:${composition}`
      : next.composing
        ? "composition"
        : "typing";
  const atomic = !!action.intent || action.isolated;
  const merge =
    !atomic &&
    previous.group?.kind === kind &&
    previous.past.length > 0 &&
    (kind.startsWith("composition") || tr.time - previous.group.time <= 500);
  const entry: HistoryEntry = { before: previous.frame, after: next.frame };
  const past = merge
    ? [
        ...previous.past.slice(0, -1),
        {
          before: previous.past.at(-1)!.before,
          after: next.frame,
        },
      ]
    : [...previous.past, entry].slice(-HISTORY_LIMIT);
  return {
    ...next,
    past,
    future: [],
    localSeq: previous.localSeq + 1,
    group: atomic ? null : { kind, time: tr.time },
  };
}
function history(editor: Editor, direction: "undo" | "redo") {
  if (editor.isDestroyed || !editor.isEditable) return false;
  const owner = collaborationOptions(editor)?.history;
  if (owner) return owner[direction]();
  const current = stateOf(editor.state);
  if (current.composing || editor.view.composing) return false;
  const entry = (direction === "undo" ? current.past : current.future).at(-1);
  const frame = entry && (direction === "undo" ? entry.before : entry.after);
  let tr = editor.state.tr;
  if (frame) {
    // Only history restores the document; ordinary input keeps PM selection mappings intact.
    tr = tr.replaceWith(0, tr.doc.content.size, frame.doc.content);
    tr.setSelection(Selection.fromJSON(tr.doc, frame.selection));
  }
  tr.setMeta(sessionKey, {
    history: direction,
    intent: frame?.intent,
  } satisfies Action);
  editor.view.dispatch(tr);
  return !!entry;
}
function inComposition(editor: Editor, event?: KeyboardEvent | InputEvent) {
  return (
    stateOf(editor.state).composing ||
    editor.view.composing ||
    event?.isComposing ||
    (event instanceof KeyboardEvent && event.keyCode === 229)
  );
}

/** Only text, paragraphs, hard breaks and opaque reference IDs. Never parse reference IDs from HTML. */
function referenceToken(
  initialLabels: LabelState,
  pending?: (id: string) => boolean,
) {
  const display = (id: string, labels: LabelState) =>
    pending?.(id) ? "待确认引用" : referenceLabel(labels, id);
  return Node.create({
    name: "reference",
    group: "inline",
    inline: true,
    atom: true,
    selectable: true,
    addProseMirrorPlugins() {
      return [
        new Plugin({
          props: {
            decorations(state) {
              const labels =
                  sessionKey.getState(state)?.labels ?? initialLabels,
                decorations: PMDecoration[] = [];
              state.doc.descendants((node, pos) => {
                if (node.type.name === "reference") {
                  const label = display(node.attrs.referenceId, labels);
                  decorations.push(
                    Decoration.node(
                      pos,
                      pos + node.nodeSize,
                      { "data-reference-label": label },
                      { referenceLabel: label },
                    ),
                  );
                }
              });
              return DecorationSet.create(state.doc, decorations);
            },
          },
        }),
      ];
    },
    addNodeView() {
      return ({ node, decorations }) => {
        const dom = document.createElement("span");
        dom.setAttribute("data-prompt-reference", "");
        dom.contentEditable = "false";
        dom.className = "rounded bg-muted px-1 text-foreground";
        const update = (
          current: PMNode,
          currentDecorations: readonly PMDecoration[],
        ) => {
          if (current.type.name !== "reference") return false;
          const label =
            currentDecorations.find(
              (d) => typeof d.spec.referenceLabel === "string",
            )?.spec.referenceLabel ??
            display(
              current.attrs.referenceId,
              sessionKey.getState(this.editor.state)?.labels ?? initialLabels,
            );
          dom.textContent = String(label);
          dom.setAttribute("aria-label", String(label));
          return true;
        };
        update(node, decorations);
        return { dom, update };
      };
    },
    addAttributes: () => ({ referenceId: { default: null, rendered: false } }),
    renderHTML({ node }) {
      return [
        "span",
        {
          "data-prompt-reference": "",
          contenteditable: "false",
          class: "rounded bg-muted px-1 text-foreground",
        },
        display(
          node.attrs.referenceId,
          (this.editor && sessionKey.getState(this.editor.state)?.labels) ??
            initialLabels,
        ),
      ];
    },
    renderText({ node }) {
      return display(
        node.attrs.referenceId,
        (this.editor && sessionKey.getState(this.editor.state)?.labels) ??
          initialLabels,
      );
    },
  });
}
export type ComposerCollaboration = {
  history: {
    undo: () => boolean;
    redo: () => boolean;
    canUndo?: () => boolean;
    canRedo?: () => boolean;
  };
  pendingReference: (id: string) => boolean;
  composition: (active: boolean, source?: string) => void;
};
function collaborationOptions(
  editor: Editor,
): ComposerCollaboration | undefined {
  return editor.extensionManager.extensions.find(
    (extension) => extension.name === "composerSession",
  )?.options.collaboration as ComposerCollaboration | undefined;
}
export type ComposerOptions = {
  collaboration?: ComposerCollaboration;
  contextKey: string;
  replicaId: string;
  draft: PromptDraft;
  resolveGeneration?: (
    draft: PromptDraft,
    previous?: Pick<PromptDraft, "modelVersion" | "mode">,
  ) => Pick<PromptDraft, "mode" | "params">;
};
export function composerEditorOptions(
  options: ComposerOptions,
): Partial<EditorOptions> {
  validatePromptDraft(options.draft);
  if (!options.contextKey || !options.replicaId)
    throw new Error("composer_context_required");
  const draft = structuredClone(options.draft);
  if (options.resolveGeneration && !options.collaboration)
    Object.assign(draft, options.resolveGeneration(draft));
  const { document: initialDocument, labelState, ...intent } = draft;
  const sessionExtension = Extension.create({
    name: "composerSession",
    addOptions() {
      return { collaboration: options.collaboration };
    },
    priority: 1000,
    addProseMirrorPlugins() {
      const editor = this.editor;
      let settleTimer: ReturnType<typeof setTimeout> | undefined;
      return [
        new Plugin<SessionState>({
          key: sessionKey,
          state: {
            init: (_, state) => ({
              contextKey: options.contextKey,
              replicaId: options.replicaId,
              frame: {
                doc: state.doc,
                selection: state.selection.toJSON(),
                intent,
              },
              labels: labelState,
              completedUploads: {},
              past: [],
              future: [],
              localSeq: 0,
              selectionEpoch: 0,
              pendingTarget: null,
              deferredMode: null,
              deferredSave: false,
              composing: false,
              group: null,
            }),
            apply: (tr, previous) =>
              apply(
                tr,
                previous,
                !!options.collaboration,
                options.resolveGeneration,
              ),
          },
          filterTransaction(tr, state) {
            if (!tr.docChanged && !tr.getMeta(sessionKey)?.intent) return true;
            const previous = stateOf(state);
            try {
              validatePromptDraft(
                frameDraft(
                  candidate(previous, tr, options.resolveGeneration),
                  tr.getMeta(sessionKey)?.labels ?? previous.labels,
                ),
              );
              return true;
            } catch {
              return false;
            }
          },
          view: () => ({
            destroy: () => {
              clearTimeout(settleTimer);
              options.collaboration?.composition(false, options.contextKey);
            },
          }),
          props: {
            handleClickOn: (_view, _pos, node, _nodePos, _event, direct) => {
              if (direct && node.type.name === "reference")
                editor.emit("composerReferenceSelected", {
                  referenceId: node.attrs.referenceId,
                });
              return false;
            },
            handleDOMEvents: {
              keydown: (_, event) => {
                event.stopPropagation();
                if (
                  (event.metaKey || event.ctrlKey) &&
                  event.key.toLowerCase() === "s" &&
                  inComposition(editor, event)
                ) {
                  event.preventDefault();
                  editor.view.dispatch(
                    editor.state.tr.setMeta(sessionKey, {
                      deferSave: true,
                    } satisfies Action),
                  );
                  return true;
                }
                return false;
              },
              beforeinput: (_, event) => {
                const input = event as InputEvent;
                if (
                  input.inputType !== "historyUndo" &&
                  input.inputType !== "historyRedo"
                )
                  return false;
                input.preventDefault();
                input.stopPropagation();
                if (!inComposition(editor, input))
                  history(
                    editor,
                    input.inputType === "historyUndo" ? "undo" : "redo",
                  );
                return true;
              },
              compositionstart: (view) => {
                clearTimeout(settleTimer);
                options.collaboration?.composition(true, options.contextKey);
                view.dispatch(
                  view.state.tr.setMeta(sessionKey, {
                    composing: true,
                  } satisfies Action),
                );
                return false;
              },
              compositionend: (view) => {
                view.dispatch(
                  view.state.tr.setMeta(sessionKey, {
                    composing: false,
                  } satisfies Action),
                );
                // Wait until PM has flushed the final composed DOM text before applying a mode.
                clearTimeout(settleTimer);
                settleTimer = setTimeout(() => {
                  if (editor.isDestroyed) return;
                  if (!inComposition(editor))
                    options.collaboration?.composition(
                      false,
                      options.contextKey,
                    );
                  const request = stateOf(editor.state).deferredMode;
                  if (request)
                    new ComposerSession(editor).resolveModeSelection(request);
                  if (
                    stateOf(editor.state).deferredSave &&
                    !inComposition(editor)
                  ) {
                    editor.view.dispatch(
                      editor.state.tr.setMeta(sessionKey, {
                        deferSave: false,
                      } satisfies Action),
                    );
                    editor.emit(
                      "composerSaveRequested",
                      new ComposerSession(editor).snapshot(),
                    );
                  }
                }, 0);
                return false;
              },
              blur: (view) => {
                view.dispatch(
                  view.state.tr.setMeta(sessionKey, {
                    breakGroup: true,
                  } satisfies Action),
                );
                return false;
              },
            },
            handleKeyDown(view, event) {
              if (inComposition(editor, event)) return false;
              const mod = event.metaKey || event.ctrlKey;
              const key = event.key.toLowerCase();
              if (mod && (key === "z" || key === "y")) {
                history(
                  editor,
                  key === "y" || event.shiftKey ? "redo" : "undo",
                );
                return true;
              }
              if (mod && key === "s") {
                editor.emit(
                  "composerSaveRequested",
                  new ComposerSession(editor).snapshot(),
                );
                return true;
              }
              if (mod && key === "enter") {
                if (!stateOf(editor.state).pendingTarget)
                  editor.emit("composerSubmitRequested", undefined);
                return true;
              }
              if (
                key === "enter" &&
                view.state.selection instanceof NodeSelection &&
                view.state.selection.node.type.name === "reference"
              ) {
                event.preventDefault();
                editor.emit("composerReferenceSelected", {
                  referenceId: view.state.selection.node.attrs.referenceId,
                });
                return true;
              }
              if (key !== "backspace" && key !== "delete") return false;
              const selection = view.state.selection;
              if (
                selection instanceof NodeSelection &&
                selection.node.type.name === "reference"
              ) {
                view.dispatch(
                  view.state.tr
                    .deleteSelection()
                    .setMeta(sessionKey, { isolated: true } satisfies Action),
                );
                return true;
              }
              if (!selection.empty) return false;
              const node =
                key === "backspace"
                  ? selection.$from.nodeBefore
                  : selection.$from.nodeAfter;
              if (node?.type.name !== "reference") return false;
              const pos =
                key === "backspace"
                  ? selection.from - node.nodeSize
                  : selection.from;
              view.dispatch(
                view.state.tr.setSelection(
                  NodeSelection.create(view.state.doc, pos),
                ),
              );
              return true;
            },
            handlePaste(view, event) {
              if (event.clipboardData?.files.length) {
                editor.emit("composerFiles", {
                  files: Array.from(event.clipboardData.files),
                });
                return true;
              }
              const text = event.clipboardData?.getData("text/plain");
              if (text === undefined || text === "") return false;
              const nodes = text
                .split(/\r\n?|\n/)
                .map((line) =>
                  view.state.schema.nodes.paragraph.create(
                    null,
                    line ? view.state.schema.text(line) : undefined,
                  ),
                );
              view.dispatch(
                view.state.tr
                  .replaceSelection(new Slice(Fragment.from(nodes), 1, 1))
                  .setMeta(sessionKey, { isolated: true } satisfies Action),
              );
              return true;
            },
            handleDrop: (_, event) => {
              if (event.dataTransfer?.files.length)
                editor.emit("composerFiles", {
                  files: Array.from(event.dataTransfer.files),
                });
              return true;
            },
          },
        }),
      ];
    },
  });
  return {
    extensions: [
      Document,
      Paragraph,
      Text,
      HardBreak,
      referenceToken(labelState, options.collaboration?.pendingReference),
      sessionExtension,
      referenceSuggestions(),
    ],
    content: initialDocument,
    enableContentCheck: true,
    editorProps: {
      attributes: {
        role: "textbox",
        "aria-multiline": "true",
        "aria-label": "创作提示词",
        class:
          "min-h-32 max-h-80 overflow-y-auto rounded-md border p-3 outline-none focus-visible:ring-2 focus-visible:ring-ring",
      },
    },
  };
}

/** An editor and its application history form one session. No network or independent media state here. */
export class ComposerSession {
  constructor(readonly editor: Editor) {
    stateOf(editor.state);
  }
  snapshot(): ComposerSnapshot {
    const s = stateOf(this.editor.state);
    return {
      contextKey: s.contextKey,
      replicaId: s.replicaId,
      localSeq: s.localSeq,
      draft: structuredClone(frameDraft(s.frame, s.labels)),
    };
  }
  get pendingTarget(): ModeRequest | null {
    return structuredClone(stateOf(this.editor.state).pendingTarget);
  }
  get composing() {
    return inComposition(this.editor);
  }
  referencePending(id: string) {
    return collaborationOptions(this.editor)?.pendingReference(id) ?? false;
  }
  applyAuthoritative(draft: PromptDraft, tr: Transaction) {
    if (this.editor.isDestroyed || inComposition(this.editor)) return false;
    validatePromptDraft(draft);
    if (
      !tr.before.eq(this.editor.state.doc) ||
      !tr.doc.eq(this.editor.schema.nodeFromJSON(draft.document))
    )
      throw new Error("unmapped_authoritative_document");
    const {
      document: _document,
      labelState: labels,
      ...intent
    } = structuredClone(draft);
    void _document;
    tr.setMeta(sessionKey, {
      intent,
      labels,
      authoritative: true,
    } satisfies Action)
      .setMeta("composer-authoritative", true)
      .setMeta("addToHistory", false);
    this.editor.view.dispatch(tr);
    return true;
  }
  updateTextParam(field: string, value: string, edit: TextInputEdit) {
    if (!this.editable() || field !== "negative_prompt") return false;
    const state = stateOf(this.editor.state),
      old = String(state.frame.intent.params[field] ?? "");
    if (
      edit.from < 0 ||
      edit.to < edit.from ||
      edit.to > old.length ||
      old.slice(0, edit.from) + edit.text + old.slice(edit.to) !== value
    )
      return false;
    const params = { ...state.frame.intent.params };
    if (value) params[field] = value;
    else delete params[field];
    const intent = { ...state.frame.intent, params };
    this.editor.view.dispatch(
      this.editor.state.tr
        .setMeta(sessionKey, { intent, isolated: true } satisfies Action)
        .setMeta("composer-text-parameter", { field, ...edit }),
    );
    return true;
  }
  undo() {
    return history(this.editor, "undo");
  }
  redo() {
    return history(this.editor, "redo");
  }
  private editable() {
    return (
      !this.editor.isDestroyed &&
      !inComposition(this.editor) &&
      this.editor.isEditable
    );
  }
  insertReference(
    input: ReferenceBinding,
    insertOccurrence = true,
    range?: { from: number; to: number },
  ) {
    if (!this.editable()) return false;
    validateReference(input);
    const s = stateOf(this.editor.state);
    const existing = s.frame.intent.references.find((r) =>
      sameReference(r, input),
    );
    const binding =
      existing ??
      structuredClone({ ...input, order: s.frame.intent.references.length });
    let labels = s.labels;
    if (!existing) {
      // Reusing a historical ID for a different binding would make its tombstone ambiguous.
      if (s.labels.allocations[binding.id])
        throw new Error("reference_id_already_allocated");
      const kind = labelKind(binding);
      const ordinal = s.labels.highWater[kind] + 1;
      if (
        !Number.isSafeInteger(ordinal) ||
        Object.keys(s.labels.allocations).length >= MAX_LABEL_ALLOCATIONS
      )
        throw new Error("label_allocation_limit");
      labels = {
        highWater: { ...s.labels.highWater, [kind]: ordinal },
        allocations: {
          ...s.labels.allocations,
          [binding.id]: { labelKind: kind, ordinal },
        },
      };
    }
    if (existing && !insertOccurrence) return true;
    const intent = existing
      ? s.frame.intent
      : {
          ...s.frame.intent,
          references: [...s.frame.intent.references, binding],
        };
    let tr = this.editor.state.tr;
    if (insertOccurrence) {
      const node = this.editor.schema.nodes.reference.create({
        referenceId: binding.id,
      });
      if (
        range &&
        range.to <= tr.doc.content.size &&
        tr.doc.textBetween(range.from, range.to).startsWith("@")
      ) {
        tr = tr.replaceWith(range.from, range.to, node);
        tr.setSelection(Selection.near(tr.doc.resolve(range.from + 1)));
      } else tr = tr.replaceSelectionWith(node);
    }
    this.editor.view.dispatch(
      tr.setMeta(sessionKey, {
        intent,
        labels,
        isolated: true,
      } satisfies Action),
    );
    return true;
  }
  removeReference(id: string) {
    if (!this.editable()) return false;
    const s = stateOf(this.editor.state);
    if (!s.frame.intent.references.some((r) => r.id === id)) return false;
    const positions: number[] = [];
    this.editor.state.doc.descendants((node, pos) => {
      if (node.type.name === "reference" && node.attrs.referenceId === id)
        positions.push(pos);
    });
    const tr = this.editor.state.tr;
    for (const pos of positions.reverse()) tr.delete(pos, pos + 1);
    const references = s.frame.intent.references
      .filter((r) => r.id !== id)
      .map((r, order) => ({ ...r, order }));
    this.editor.view.dispatch(
      tr.setMeta(sessionKey, {
        intent: { ...s.frame.intent, references },
      } satisfies Action),
    );
    return true;
  }
  /** Replaces a source or role in place; tokens and display number keep their identity. */
  replaceReference(
    id: string,
    replacement: Omit<ReferenceBinding, "id" | "order" | "bindingGeneration">,
  ) {
    if (!this.editable()) return false;
    const state = stateOf(this.editor.state);
    const previous = state.frame.intent.references.find((r) => r.id === id);
    if (
      !previous ||
      labelKind(previous) !== labelKind({ ...previous, ...replacement })
    )
      return false;
    const references = state.frame.intent.references.map((r) =>
      r.id === id
        ? {
            ...r,
            source: structuredClone(replacement.source),
            role: replacement.role,
            mediaKind: replacement.mediaKind,
            bindingGeneration: r.bindingGeneration + 1,
          }
        : r,
    );
    return this.replaceIntent({ ...state.frame.intent, references });
  }
  reorderReference(id: string, offset: number) {
    if (!this.editable()) return false;
    const state = stateOf(this.editor.state),
      references = [...state.frame.intent.references];
    const from = references.findIndex((r) => r.id === id),
      to = from + offset;
    if (from < 0 || to < 0 || to >= references.length) return false;
    const [item] = references.splice(from, 1);
    references.splice(to, 0, item);
    return this.replaceIntent({
      ...state.frame.intent,
      references: references.map((r, order) => ({ ...r, order })),
    });
  }
  private replaceIntent(intent: Intent) {
    const state = stateOf(this.editor.state);
    validatePromptDraft(frameDraft({ ...state.frame, intent }, state.labels));
    this.editor.view.dispatch(
      this.editor.state.tr.setMeta(sessionKey, {
        intent,
        isolated: true,
      } satisfies Action),
    );
    return true;
  }
  /** An upload can settle only its original owner, binding generation and attempt. */
  resolveUpload(
    ticket: {
      contextKey: string;
      replicaId: string;
      referenceId: string;
      generation: number;
      attempt: string;
    },
    source: ReferenceBinding["source"],
  ) {
    if (!this.editable()) return false;
    const state = stateOf(this.editor.state);
    if (
      ticket.contextKey !== state.contextKey ||
      ticket.replicaId !== state.replicaId ||
      source.kind !== "media"
    )
      return false;
    const key = JSON.stringify([
      ticket.referenceId,
      ticket.generation,
      ticket.attempt,
    ]);
    const ref = state.frame.intent.references.find((r) => uploadKey(r) === key);
    const known =
      ref ||
      [...state.past, ...state.future].some((entry) =>
        [
          ...entry.before.intent.references,
          ...entry.after.intent.references,
        ].some((r) => uploadKey(r) === key),
      );
    if (!known) return false;
    this.editor.view.dispatch(
      this.editor.state.tr.setMeta(sessionKey, {
        settledUpload: { key, source },
      } satisfies Action),
    );
    return !!ref;
  }
  applyConversion(
    expectedSeq: number,
    params: PromptDraft["params"],
    references: ReferenceBinding[],
  ) {
    if (!this.editable()) return false;
    const state = stateOf(this.editor.state);
    if (state.localSeq !== expectedSeq) return false;
    // Conversion never silently discards bindings. Removal is its own reviewed command.
    if (
      references.length !== state.frame.intent.references.length ||
      references.some(
        (r) => !state.frame.intent.references.some((old) => old.id === r.id),
      )
    )
      return false;
    return this.replaceIntent({
      ...state.frame.intent,
      params: structuredClone(params),
      references: structuredClone(references),
    });
  }
  adoptDraft(draft: PromptDraft) {
    if (!this.editable()) return false;
    validatePromptDraft(draft);
    const state = stateOf(this.editor.state),
      labels = structuredClone(draft.labelState);
    for (const [id, allocation] of Object.entries(state.labels.allocations)) {
      if (
        labels.allocations[id] &&
        JSON.stringify(labels.allocations[id]) !== JSON.stringify(allocation)
      )
        throw new Error("label_allocation_conflict");
      if (
        !labels.allocations[id] &&
        Object.values(labels.allocations).some(
          (a) =>
            a.labelKind === allocation.labelKind &&
            a.ordinal === allocation.ordinal,
        )
      )
        throw new Error("label_allocation_conflict");
      labels.allocations[id] = allocation;
    }
    for (const kind of Object.keys(
      labels.highWater,
    ) as (keyof LabelState["highWater"])[])
      labels.highWater[kind] = Math.max(
        labels.highWater[kind],
        state.labels.highWater[kind],
      );
    const { document, labelState: _labels, ...intent } = structuredClone(draft);
    void _labels;
    this.editor.view.dispatch(
      this.editor.state.tr
        .replaceWith(
          0,
          this.editor.state.doc.content.size,
          this.editor.schema.nodeFromJSON(document).content,
        )
        .setMeta(sessionKey, {
          intent,
          labels,
          selection: "cancel",
          isolated: true,
        } satisfies Action),
    );
    return true;
  }
  insertReferences(
    inputs: ReferenceBinding[],
    range?: { from: number; to: number },
  ) {
    const fragment = emptyPromptDraft();
    fragment.references = inputs.map((input, order) => ({ ...input, order }));
    for (const ref of fragment.references) {
      const kind = labelKind(ref);
      fragment.labelState.allocations[ref.id] = {
        labelKind: kind,
        ordinal: ++fragment.labelState.highWater[kind],
      };
    }
    fragment.document.content[0].content = inputs.map((input) => ({
      type: "reference",
      attrs: { referenceId: input.id },
    }));
    return this.importFragment(fragment, this.snapshot().localSeq, range);
  }
  importFragment(
    fragment: PromptDraft,
    expectedSeq: number,
    range?: { from: number; to: number },
  ) {
    if (!this.editable()) return false;
    validatePromptDraft(fragment);
    const state = stateOf(this.editor.state);
    if (state.localSeq !== expectedSeq) return false;
    const labels = structuredClone(state.labels),
      references = structuredClone(state.frame.intent.references),
      mapping = new Map<string, string>();
    for (const old of fragment.references) {
      const known = references.find((r) => sameReference(r, old));
      if (known) {
        mapping.set(old.id, known.id);
        continue;
      }
      const ref = structuredClone(old);
      ref.id = crypto.randomUUID();
      ref.order = references.length;
      if (ref.source.kind === "pending_upload") {
        ref.bindingGeneration++;
        ref.source = {
          kind: "pending_upload",
          clientUploadId: crypto.randomUUID(),
          uploadAttemptId: crypto.randomUUID(),
        };
      }
      const kind = labelKind(ref);
      labels.allocations[ref.id] = {
        labelKind: kind,
        ordinal: ++labels.highWater[kind],
      };
      references.push(ref);
      mapping.set(old.id, ref.id);
    }
    const document = structuredClone(fragment.document);
    for (const p of document.content)
      for (const n of p.content ?? [])
        if (n.type === "reference")
          n.attrs.referenceId = mapping.get(n.attrs.referenceId)!;
    const slice = new Slice(
      this.editor.schema.nodeFromJSON(document).content,
      1,
      1,
    );
    const tr = this.editor.state.tr;
    if (
      range &&
      range.to <= tr.doc.content.size &&
      tr.doc.textBetween(range.from, range.to).startsWith("@")
    )
      tr.replaceRange(range.from, range.to, slice);
    else tr.replaceSelection(slice);
    const intent = { ...state.frame.intent, references };
    validatePromptDraft({
      ...intent,
      document: tr.doc.toJSON(),
      labelState: labels,
    });
    this.editor.view.dispatch(
      tr.setMeta(sessionKey, {
        intent,
        labels,
        isolated: true,
      } satisfies Action),
    );
    return true;
  }
  updateParams(params: PromptDraft["params"]) {
    if (!this.editable()) return false;
    const s = stateOf(this.editor.state);
    const intent = { ...s.frame.intent, params: structuredClone(params) };
    validatePromptDraft(frameDraft({ ...s.frame, intent }, s.labels));
    if (JSON.stringify(intent.params) === JSON.stringify(s.frame.intent.params))
      return true;
    this.editor.view.dispatch(
      this.editor.state.tr.setMeta(sessionKey, { intent } satisfies Action),
    );
    return true;
  }
  beginModeSelection(target: ModeTarget): ModeRequest {
    if (!this.editable()) throw new Error("composer_not_editable");
    const s = stateOf(this.editor.state);
    validatePromptDraft(
      frameDraft(
        { ...s.frame, intent: { ...s.frame.intent, ...target } },
        s.labels,
      ),
    );
    this.editor.view.dispatch(
      this.editor.state.tr.setMeta(sessionKey, {
        selection: { ...target },
      } satisfies Action),
    );
    return this.pendingTarget!;
  }
  cancelModeSelection() {
    if (this.editor.isDestroyed) return;
    this.editor.view.dispatch(
      this.editor.state.tr.setMeta(sessionKey, {
        selection: "cancel",
      } satisfies Action),
    );
  }
  resolveModeSelection(request: ModeRequest) {
    if (this.editor.isDestroyed || !this.editor.isEditable) return false;
    const s = stateOf(this.editor.state),
      pending = s.pendingTarget;
    if (
      !pending ||
      request.contextKey !== s.contextKey ||
      request.replicaId !== s.replicaId ||
      request.selectionEpoch !== s.selectionEpoch ||
      request.modelVersion !== pending.modelVersion ||
      request.mode !== pending.mode
    )
      return false;
    if (inComposition(this.editor)) {
      this.editor.view.dispatch(
        this.editor.state.tr.setMeta(sessionKey, {
          deferMode: { ...request },
        } satisfies Action),
      );
      return true;
    }
    const unchanged =
      s.frame.intent.mode === request.mode &&
      s.frame.intent.modelVersion === request.modelVersion;
    this.editor.view.dispatch(
      this.editor.state.tr.setMeta(sessionKey, {
        selection: "cancel",
        intent: unchanged
          ? undefined
          : {
              ...s.frame.intent,
              mode: request.mode,
              modelVersion: request.modelVersion,
            },
      } satisfies Action),
    );
    return true;
  }
}
