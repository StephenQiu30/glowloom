"use client";

import { Node, Extension, type Editor } from "@tiptap/core";
import Mention, { type MentionOptions } from "@tiptap/extension-mention";
import Placeholder from "@tiptap/extension-placeholder";
import { Plugin, PluginKey } from "@tiptap/pm/state";
import { history } from "@tiptap/pm/history";
import { DOMParser as ProseMirrorDOMParser } from "@tiptap/pm/model";
import type { NodeTextBinding } from "@glowloom/canvas-collab/model";
import {
  EditorContent,
  NodeViewWrapper,
  ReactNodeViewRenderer,
  useEditor,
  useEditorState,
  type NodeViewProps,
} from "@tiptap/react";
import StarterKit from "@tiptap/starter-kit";
import TextAlign from "@tiptap/extension-text-align";
import Collaboration from "@tiptap/extension-collaboration";
import { ySyncPluginKey } from "@tiptap/y-tiptap";
import { persistentRichText } from "@glowloom/canvas-collab/rich-text";
import type { XmlFragment } from "yjs";
import { exitSuggestion, type SuggestionProps } from "@tiptap/suggestion";
import {
  AlignLeft,
  AlignCenter,
  AlignRight,
  Bold,
  Italic,
  Underline,
  Strikethrough,
  ChevronDown,
  Type,
  X,
  List,
  ListOrdered,
  Quote,
  Undo2,
  Redo2,
  Maximize2,
  Download,
  AtSign,
  FileText,
  ImageIcon,
  Video,
  Music2,
  Group,
} from "lucide-react";
import {
  Fragment,
  useCallback,
  createContext,
  useContext,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import { createPortal } from "react-dom";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Toggle } from "@/components/ui/toggle";
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuItem,
} from "@/components/ui/dropdown-menu";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Popover,
  PopoverAnchor,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { MediaPreview } from "@/components/canvas/runtime/components/generation/workspace/media-preview";
import { PreviewImage } from "@/components/canvas/runtime/components/media/preview-image";
import {
  isImeComposing,
  isPlainEnterKey,
} from "@/components/canvas/runtime/lib/keyboard-event";
import { cn } from "@/lib/utils";
import {
  documentForText,
  documentText,
  parseRichTextDocument,
  referenceToken,
  textToDocument,
} from "../lib/rich-text-document";
import type {
  RichTextDocument,
  RichTextInputProps,
  RichTextReference,
} from "../types/rich-text";

type MentionMenu = {
  items: RichTextReference[];
  rect: DOMRect | null;
  select: (reference: RichTextReference) => void;
  active: number;
};

const emptyReferences: RichTextReference[] = [];
const ReferenceContext = createContext<RichTextReference[]>(emptyReferences);

function ReferenceChip({ node }: NodeViewProps) {
  const references = useContext(ReferenceContext);
  const reference = references.find(
    (item) => item.id === String(node.attrs.id ?? node.attrs.referenceId),
  );
  const [preview, setPreview] = useState(false);
  const label = reference?.label ?? String(node.attrs.label ?? "素材引用");
  return (
    <NodeViewWrapper
      as="span"
      data-type="mention"
      data-id={node.attrs.id ?? undefined}
      data-label={node.attrs.label ?? undefined}
      {...{ token: node.attrs.token ?? undefined }}
      contentEditable={false}
      className="rich-text-reference"
    >
      <Button
        variant="secondary"
        size="sm"
        type="button"
        className="rich-text-reference-button"
        aria-label={
          reference?.kind === "image" && reference.previewUrl
            ? `预览引用：${reference.title}`
            : `引用：${reference?.title ?? label}`
        }
        onClick={() => {
          if (reference?.kind === "image" && reference.previewUrl)
            setPreview(true);
        }}
      >
        {reference?.kind === "image" && reference.previewUrl ? (
          <PreviewImage
            src={reference.previewUrl}
            alt=""
            className="size-6 rounded object-cover"
          />
        ) : (
          <ReferenceIcon kind={reference?.kind ?? "text"} />
        )}
        <span className="truncate">{label}</span>
      </Button>
      {preview && reference?.previewUrl ? (
        <MediaPreview
          src={reference.previewUrl}
          alt={reference.title}
          open
          onOpenChange={setPreview}
          style={{ display: "none" }}
        />
      ) : null}
    </NodeViewWrapper>
  );
}

function ReferenceIcon({ kind }: { kind: RichTextReference["kind"] }) {
  const Icon =
    kind === "image"
      ? ImageIcon
      : kind === "video"
        ? Video
        : kind === "audio"
          ? Music2
          : kind === "group"
            ? Group
            : FileText;
  return <Icon aria-hidden="true" className="size-4 shrink-0" />;
}

const authoritativeDocument = new PluginKey("glowloom-authoritative-document");

function createEditorExtensions({
  documentMode,
  collaborationFragment,
  mentionKey,
  readProps,
  readMenu,
  updateMenu,
}: {
  documentMode?: boolean;
  collaborationFragment?: XmlFragment;
  mentionKey: PluginKey;
  readProps: () => RichTextInputProps;
  readMenu: () => MentionMenu | null;
  updateMenu: (menu: MentionMenu | null) => void;
}) {
  let blockedLength = false;
  return [
    Node.create({
      name: "reference",
      group: "inline",
      inline: true,
      atom: true,
      addAttributes() {
        return { referenceId: { default: null } };
      },
      parseHTML() {
        return [
          {
            tag: "span[data-reference-id]",
            getAttrs: (element) => ({
              referenceId: element.getAttribute("data-reference-id"),
            }),
          },
        ];
      },
      renderHTML({ node }) {
        return [
          "span",
          { "data-reference-id": node.attrs.referenceId },
          "[引用]",
        ];
      },
      renderText() {
        return "[引用]";
      },
      addNodeView() {
        return ReactNodeViewRenderer(ReferenceChip);
      },
    }),
    StarterKit.configure({
      undoRedo: collaborationFragment ? false : {},
      heading: documentMode ? { levels: [1, 2, 3] } : false,
      bold: documentMode ? {} : false,
      italic: documentMode ? {} : false,
      underline: documentMode ? {} : false,
      strike: documentMode ? {} : false,
      bulletList: documentMode ? {} : false,
      orderedList: documentMode ? {} : false,
      listItem: documentMode ? {} : false,
      listKeymap: documentMode ? {} : false,
      blockquote: documentMode ? {} : false,
      code: documentMode ? {} : false,
      codeBlock: documentMode ? {} : false,
      horizontalRule: documentMode ? {} : false,
      link: documentMode
        ? { openOnClick: false, protocols: ["https", "http"] }
        : false,
    }),
    ...(documentMode
      ? [
          TextAlign.configure({
            types: ["paragraph", "heading"],
            alignments: ["left", "center", "right"],
          }),
        ]
      : []),
    ...(collaborationFragment
      ? [Collaboration.configure({ fragment: collaborationFragment })]
      : []),
    Placeholder.configure({
      placeholder: () => readProps().placeholder ?? "请输入内容…",
    }),
    Mention.extend<MentionOptions<RichTextReference, RichTextReference>>({
      addAttributes() {
        return { ...this.parent?.(), token: { default: null } };
      },
      addNodeView() {
        return ReactNodeViewRenderer(ReferenceChip);
      },
    }).configure({
      HTMLAttributes: { class: "rich-text-mention" },
      deleteTriggerWithBackspace: true,
      renderText: ({ node }) =>
        String(node.attrs.token ?? node.attrs.label ?? ""),
      suggestion: {
        pluginKey: mentionKey,
        items: ({ query }: { query: string }) =>
          (readProps().references ?? emptyReferences)
            .filter((r) =>
              `${r.label} ${r.title} ${r.text ?? ""}`
                .toLowerCase()
                .includes(query.toLowerCase()),
            )
            .slice(0, 20),
        command: ({
          editor: activeEditor,
          range,
          props: item,
        }: {
          editor: Editor;
          range: { from: number; to: number };
          props: RichTextReference;
        }) => {
          activeEditor
            .chain()
            .focus()
            .insertContentAt(range, [
              {
                type: "mention",
                attrs: {
                  id: item.id,
                  label: item.label,
                  token: referenceToken(item),
                },
              },
              { type: "text", text: " " },
            ])
            .run();
        },
        render: () => {
          const update = (
            state: SuggestionProps<RichTextReference, RichTextReference>,
          ) =>
            updateMenu(
              state.items.length
                ? {
                    items: state.items,
                    rect: state.clientRect?.() ?? null,
                    select: state.command,
                    active: 0,
                  }
                : null,
            );
          return {
            onStart: update,
            onUpdate: update,
            onExit: () => updateMenu(null),
            onKeyDown: ({ event, view }) => {
              if (isImeComposing(event)) return false;
              const current = readMenu();
              if (!current) return false;
              if (event.key === "ArrowDown" || event.key === "ArrowUp") {
                updateMenu({
                  ...current,
                  active:
                    (current.active +
                      (event.key === "ArrowDown" ? 1 : -1) +
                      current.items.length) %
                    current.items.length,
                });
                return true;
              }
              if (event.key === "Enter" || event.key === "Tab") {
                current.select(current.items[current.active]);
                return true;
              }
              if (event.key === "Escape") {
                exitSuggestion(view, mentionKey);
                return true;
              }
              return false;
            },
          };
        },
      },
    }),
    Extension.create({
      name: "glowloom-content-limit",
      addProseMirrorPlugins() {
        return [
          new Plugin({
            filterTransaction(transaction, state) {
              if (transaction.getMeta(authoritativeDocument)) return true;
              if (
                readProps().collaboration &&
                transaction.getMeta(ySyncPluginKey)?.isChangeOrigin
              )
                return true;
              if (
                transaction.docChanged &&
                readProps().acceptDocument?.({
                  schemaVersion: 1,
                  content: persistentRichText(transaction.doc.toJSON()),
                }) === false
              )
                return false;
              const max = readProps().maxLength;
              const length = documentText(transaction.doc.toJSON()).length;
              if (
                transaction.docChanged &&
                max &&
                length > max &&
                length > documentText(state.doc.toJSON()).length
              ) {
                if (!blockedLength) {
                  blockedLength = true;
                  queueMicrotask(() => toast.error(`内容最多 ${max} 个字符`));
                }
                return false;
              }
              blockedLength = false;
              return true;
            },
          }),
        ];
      },
    }),
  ];
}

function readEditorDraft(
  editor: Editor | null,
  binding: NodeTextBinding | undefined,
  force = false,
) {
  if (!editor || editor.isDestroyed || !binding?.protectDraft) return null;
  const contentDOM = editor.view.dom.cloneNode(true) as HTMLElement;
  // ProseMirror adds this caret padding after empty blocks and real breaks.
  // It belongs to the view; parsing it as content creates a permanent draft.
  contentDOM
    .querySelectorAll("br.ProseMirror-trailingBreak")
    .forEach((node) => node.remove());
  const content = persistentRichText(
    ProseMirrorDOMParser.fromSchema(editor.schema)
      .parse(contentDOM, { preserveWhitespace: "full" })
      .toJSON(),
  );
  if (
    force ||
    editor.view.composing ||
    binding.isAttached?.() === false ||
    JSON.stringify(content) !==
      JSON.stringify(persistentRichText(editor.getJSON()))
  )
    return parseRichTextDocument({ schemaVersion: 1, content });
  return null;
}

export function RichTextEditor({ rootRef, ...props }: RichTextInputProps) {
  const editorInstance = useRef<Editor | null>(null);
  const latest = useRef({ ...props });
  const lastEmitted = useRef(props.value);
  const lastDocument = useRef<RichTextDocument | undefined>(props.document);
  const [localExpanded, setLocalExpanded] = useState(false);
  const expanded = props.expandedView?.open ?? localExpanded;
  const setExpanded = (open: boolean) => {
    if (props.expandedView) props.expandedView.onOpenChange(open);
    else setLocalExpanded(open);
  };
  const expandedRef = useRef(false);
  const [menu, setMenu] = useState<MentionMenu | null>(null);
  const menuRef = useRef<MentionMenu | null>(null);
  const readProps = useCallback(() => latest.current, []);
  const readMenu = useCallback(() => menuRef.current, []);
  const mentionKey = useMemo(() => new PluginKey("glowloom-reference"), []);
  useLayoutEffect(() => {
    latest.current = { ...props };
    expandedRef.current = expanded;
  }, [props, expanded]);

  const updateMenu = useCallback((next: MentionMenu | null) => {
    menuRef.current = next;
    setMenu(next);
  }, []);

  const extensions = useMemo(
    // Tiptap invokes the supplied readers from plugin events, after render.
    // eslint-disable-next-line react-hooks/refs
    () =>
      createEditorExtensions({
        documentMode: props.collaboration?.fragment ? true : props.documentMode,
        collaborationFragment: props.collaboration?.fragment,
        mentionKey,
        readProps,
        readMenu,
        updateMenu,
      }),
    [
      props.documentMode,
      props.collaboration?.fragment,
      mentionKey,
      updateMenu,
      readProps,
      readMenu,
    ],
  );

  const editor = useEditor(
    {
      extensions,
      content: props.collaboration?.fragment
        ? undefined
        : documentForText(props.value, props.document, props.references),
      immediatelyRender: false,
      shouldRerenderOnTransaction: false,
      editable: !props.disabled,
      autofocus: props.autoFocus ? "end" : false,
      editorProps: {
        attributes: {
          id: props.id ?? "",
          class: "rich-text-surface thin-scrollbar",
          role: "textbox",
          "aria-multiline": "true",
          "aria-label": props["aria-label"] ?? props.placeholder ?? "文字内容",
          "aria-invalid": String(Boolean(props["aria-invalid"])),
          "aria-required": String(Boolean(props.required)),
          "aria-disabled": String(Boolean(props.disabled)),
        },
        handleKeyDown: (view, event) => {
          if (view.composing || isImeComposing(event) || menuRef.current)
            return false;
          if (event.key === "Escape") {
            if (expandedRef.current) {
              if (latest.current.expandedView)
                latest.current.expandedView.onOpenChange(false);
              else setLocalExpanded(false);
              return true;
            }
            if (latest.current.onEscape) {
              latest.current.onEscape();
              return true;
            }
          }
          const submitKey =
            latest.current.submitShortcut === "mod-enter"
              ? event.key === "Enter" &&
                (event.ctrlKey || event.metaKey) &&
                !event.shiftKey &&
                !event.altKey
              : isPlainEnterKey(event);
          if (latest.current.onSubmit && submitKey) {
            if (!event.repeat) latest.current.onSubmit();
            return true;
          }
          return false;
        },
        handlePaste: (_view, event) => {
          if (latest.current.documentMode) return false;
          const plain = event.clipboardData?.getData("text/plain");
          if (plain === undefined) return false;
          editorInstance.current?.commands.insertContent(
            textToDocument(plain, latest.current.references).content ?? [],
          );
          return true;
        },
      },
      onUpdate: ({ editor: activeEditor, transaction }) => {
        const binding = latest.current.collaboration;
        if (binding?.isAttached?.() === false) {
          if (!transaction.getMeta(ySyncPluginKey)?.isChangeOrigin)
            binding.protectDraft?.(
              readEditorDraft(activeEditor, binding, true),
            );
          return;
        }
        if (
          latest.current.collaboration &&
          !transaction.getMeta(ySyncPluginKey)?.isChangeOrigin
        ) {
          binding?.protectDraft?.(readEditorDraft(activeEditor, binding));
          if (binding?.markChanged() === false) return;
        }
        const document = parseRichTextDocument({
          schemaVersion: 1,
          content: persistentRichText(activeEditor.getJSON()),
        });
        const value = documentText(document.content);
        lastEmitted.current = value;
        lastDocument.current = document;
        latest.current.onChange(value, document);
      },
    },
    [props.collaboration?.fragment],
  );

  useLayoutEffect(() => {
    editorInstance.current = editor;
    return () => {
      editorInstance.current = null;
    };
  }, [editor]);

  useLayoutEffect(() => {
    const binding = props.collaboration;
    if (!editor || !binding?.protectDraft) return;
    const read = () => readEditorDraft(editor, binding);
    const unregister = binding.registerDraftReader?.(read);
    return () => {
      // Remote deletion already captures composition before detaching. Reading
      // the detached DOM here would manufacture a draft from accepted content.
      if (binding.isAttached?.() !== false) binding.protectDraft?.(read());
      unregister?.();
    };
  }, [editor, props.collaboration]);

  useEffect(() => {
    if (!editor) return;
    editor.setEditable(!props.disabled, false);
  }, [editor, props.disabled]);

  useLayoutEffect(() => {
    if (!editor || editor.view.composing || props.collaboration?.fragment)
      return;
    const content = documentForText(
      props.value,
      props.document,
      props.references,
    );
    if (
      props.value === lastEmitted.current &&
      (!props.document ||
        props.document === lastDocument.current ||
        editor.schema.nodeFromJSON(content).eq(editor.state.doc))
    )
      return;
    const selection = editor.state.selection;
    editor
      .chain()
      .setMeta(authoritativeDocument, true)
      .setContent(content, { emitUpdate: false })
      .run();
    // Whole-field remote updates cannot share the prior local text undo stack.
    editor.unregisterPlugin("history");
    editor.registerPlugin(history());
    if (editor.isFocused)
      editor.commands.setTextSelection(
        Math.min(selection.from, editor.state.doc.content.size),
      );
    lastEmitted.current = props.value;
    lastDocument.current = props.document;
  }, [
    editor,
    props.value,
    props.document,
    props.references,
    props.collaboration?.fragment,
  ]);

  const content = editor ? (
    <ReferenceContext value={props.references ?? emptyReferences}>
      <EditorContent editor={editor} className="rich-text-content" />
    </ReferenceContext>
  ) : null;
  const toolbar =
    editor && props.documentMode ? (
      <EditorToolbar
        editor={editor}
        disabled={props.disabled}
        references={props.references ?? emptyReferences}
        onExpand={expanded ? undefined : () => setExpanded(true)}
      />
    ) : null;
  return (
    <Popover open={Boolean(props.floatingToolbar && toolbar && !expanded)}>
      <PopoverAnchor asChild>
        <div
          ref={rootRef}
          data-rich-text-scope
          data-editor-engine="tiptap"
          data-canvas-no-zoom
          data-canvas-shortcuts-ignore
          data-document-mode={Boolean(props.documentMode)}
          onInputCapture={() =>
            props.collaboration?.protectDraft?.(
              readEditorDraft(editor, props.collaboration),
            )
          }
          onCompositionStartCapture={() =>
            props.collaboration?.protectDraft?.(
              readEditorDraft(editor, props.collaboration, true),
            )
          }
          onCompositionEndCapture={() => {
            const binding = props.collaboration;
            binding?.protectDraft?.(readEditorDraft(editor, binding, true));
            queueMicrotask(() => {
              if (editor && !editor.isDestroyed)
                binding?.protectDraft?.(readEditorDraft(editor, binding));
            });
          }}
          className={cn("rich-text-editor", props.className)}
          style={props.style}
          onPointerDown={(event) => event.stopPropagation()}
          onMouseDown={(event) => event.stopPropagation()}
          onWheel={(event) => event.stopPropagation()}
          onKeyDown={(event) => {
            if (
              event.key === "Escape" &&
              props.documentMode &&
              !expanded &&
              !editor?.view.composing &&
              !isImeComposing(event.nativeEvent) &&
              !event.defaultPrevented
            ) {
              event.preventDefault();
              props.onEscape?.();
            }
            if (event.key !== "Escape" || props.documentMode)
              event.stopPropagation();
          }}
        >
          {!expanded ? (
            <>
              {props.floatingToolbar || props.compactToolbar ? null : toolbar}
              {content}
            </>
          ) : (
            <p className="p-3 text-sm text-muted-foreground">
              正在展开编辑文档
            </p>
          )}
          <Dialog open={expanded} onOpenChange={setExpanded}>
            <DialogContent
              data-rich-text-floating
              data-document-export={props.documentExport}
              className={
                props.documentMode
                  ? "canvas-document-fullscreen top-0 left-0 translate-x-0 translate-y-0"
                  : "canvas-composer-dialog canvas-expanded-prompt"
              }
              showCloseButton={false}
              data-canvas-no-zoom
              data-canvas-shortcuts-ignore
              onEscapeKeyDown={(event) => {
                if (editor?.view.composing || isImeComposing(event))
                  event.preventDefault();
                event.stopPropagation();
              }}
              onPointerDown={(event) => event.stopPropagation()}
              onMouseDown={(event) => event.stopPropagation()}
              onClick={(event) => event.stopPropagation()}
              onWheel={(event) => event.stopPropagation()}
              onKeyDown={(event) => {
                event.stopPropagation();
                if (
                  event.key === "Escape" &&
                  !event.defaultPrevented &&
                  !editor?.view.composing &&
                  !isImeComposing(event.nativeEvent)
                ) {
                  event.preventDefault();
                  setExpanded(false);
                }
              }}
              onOpenAutoFocus={(event) => {
                event.preventDefault();
                requestAnimationFrame(() => {
                  if (editor && !editor.isDestroyed) editor.commands.focus();
                });
              }}
              onCloseAutoFocus={(event) => {
                event.preventDefault();
                requestAnimationFrame(() => {
                  if (props.expandedView?.onCloseAutoFocus)
                    props.expandedView.onCloseAutoFocus();
                  else if (editor && !editor.isDestroyed)
                    editor.commands.focus();
                });
              }}
            >
              <DialogHeader className="sr-only">
                <DialogTitle>
                  {props.documentMode ? "编辑文档" : "编辑提示词"}
                </DialogTitle>
                <DialogDescription>
                  编辑当前节点内容及引用，关闭后保留修改。
                </DialogDescription>
              </DialogHeader>
              {props.documentMode ? (
                <div className="canvas-document-header">
                  <span className="truncate">
                    {props.documentTitle || "文本"}.
                    {props.documentExport === "markdown" ? "md" : "json"}
                  </span>
                  <div className="ml-auto flex items-center gap-1">
                    <Button
                      variant="ghost"
                      size="icon-lg"
                      aria-label="下载文档"
                      onClick={() =>
                        editor &&
                        void (
                          props.documentExport === "markdown"
                            ? import("../lib/rich-text-download").then(
                                ({ downloadRichTextMarkdown }) =>
                                  downloadRichTextMarkdown(
                                    {
                                      schemaVersion: 1,
                                      content: persistentRichText(
                                        editor.getJSON(),
                                      ),
                                    },
                                    props.documentTitle || "文本",
                                    props.references,
                                  ),
                              )
                            : downloadDocument(editor, props.documentTitle)
                        ).catch(() => toast.error("文档下载失败，请重试"))
                      }
                    >
                      <Download />
                    </Button>
                    <Button
                      variant="ghost"
                      size="icon-lg"
                      aria-label="关闭文档编辑"
                      onClick={() => setExpanded(false)}
                    >
                      <X />
                    </Button>
                  </div>
                </div>
              ) : (
                <div className="canvas-prompt-header">
                  {props.expandedView?.header}
                  <Button
                    variant="ghost"
                    size="icon-lg"
                    aria-label="收起编辑"
                    onClick={() => setExpanded(false)}
                  >
                    <Maximize2 />
                  </Button>
                </div>
              )}
              {expanded ? (
                <>
                  {toolbar}
                  <div className="canvas-expanded-body">{content}</div>
                  {props.expandedView?.footer}
                </>
              ) : null}
            </DialogContent>
          </Dialog>
          {menu && editor
            ? createPortal(
                <Popover
                  open
                  onOpenChange={(open) => {
                    if (!open) {
                      exitSuggestion(editor.view, mentionKey);
                      updateMenu(null);
                    }
                  }}
                >
                  <PopoverAnchor asChild>
                    <span
                      className="fixed size-px"
                      style={{
                        left: menu.rect?.left ?? 0,
                        top: menu.rect?.bottom ?? 0,
                      }}
                    />
                  </PopoverAnchor>
                  <PopoverContent
                    data-rich-text-floating
                    aria-label="选择素材引用"
                    className="rich-text-suggestions"
                    onEscapeKeyDown={(event) => {
                      if (editor.view.composing || isImeComposing(event))
                        event.preventDefault();
                      event.stopPropagation();
                    }}
                    sideOffset={4}
                    align="start"
                    onOpenAutoFocus={(event) => event.preventDefault()}
                    onCloseAutoFocus={(event) => event.preventDefault()}
                  >
                    {menu.items.map((reference, index) => (
                      <Button
                        key={reference.id}
                        type="button"
                        variant={index === menu.active ? "secondary" : "ghost"}
                        className="h-auto w-full justify-start gap-2 p-2 text-left"
                        aria-current={
                          index === menu.active ? "true" : undefined
                        }
                        onMouseDown={(event) => event.preventDefault()}
                        onClick={() => menu.select(reference)}
                      >
                        <ReferenceIcon kind={reference.kind} />
                        <span className="min-w-0">
                          <span className="block truncate">
                            {reference.label}
                          </span>
                          <span className="block truncate text-xs text-muted-foreground">
                            {reference.title}
                          </span>
                        </span>
                      </Button>
                    ))}
                  </PopoverContent>
                </Popover>,
                document.body,
              )
            : null}
        </div>
      </PopoverAnchor>
      {props.floatingToolbar && !expanded ? (
        <PopoverContent
          side="top"
          align="center"
          sideOffset={52}
          collisionPadding={16}
          sticky="always"
          hideWhenDetached
          className="canvas-document-toolbar w-max max-w-[calc(100dvw-2rem)] p-1"
          data-rich-text-floating
          data-canvas-no-zoom
          data-canvas-shortcuts-ignore
          onOpenAutoFocus={(event) => event.preventDefault()}
          onCloseAutoFocus={(event) => event.preventDefault()}
          onInteractOutside={(event) => event.preventDefault()}
          onEscapeKeyDown={(event) => {
            event.preventDefault();
            event.stopPropagation();
            if (!editor?.view.composing && !isImeComposing(event))
              props.onEscape?.();
          }}
          onPointerDown={(event) => event.stopPropagation()}
          onMouseDown={(event) => event.stopPropagation()}
          onClick={(event) => event.stopPropagation()}
          onKeyDown={(event) => event.stopPropagation()}
        >
          {toolbar}
        </PopoverContent>
      ) : null}
    </Popover>
  );
}

function EditorToolbar({
  editor,
  disabled,
  references,
  onExpand,
}: {
  editor: Editor;
  disabled?: boolean;
  references: RichTextReference[];
  onExpand?: () => void;
}) {
  const importInput = useRef<HTMLInputElement>(null);
  const importDocument = async (file?: File) => {
    if (!file) return;
    try {
      if (file.size > 1024 * 1024) throw new Error("文档不能超过 1 MiB");
      const document = parseRichTextDocument(JSON.parse(await file.text()));
      editor.commands.setContent(document.content, {
        errorOnInvalidContent: true,
      });
      editor.commands.focus("end");
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "无法读取文档");
    } finally {
      if (importInput.current) importInput.current.value = "";
    }
  };
  const state = useEditorState({
    editor,
    selector: ({ editor: current }) => ({
      bold: current.isActive("bold"),
      italic: current.isActive("italic"),
      underline: current.isActive("underline"),
      strike: current.isActive("strike"),
      heading: current.isActive("heading")
        ? String(current.getAttributes("heading").level)
        : "paragraph",
      bullet: current.isActive("bulletList"),
      ordered: current.isActive("orderedList"),
      quote: current.isActive("blockquote"),
      alignment:
        (current.isActive("heading")
          ? current.getAttributes("heading")
          : current.getAttributes("paragraph")
        ).textAlign ?? "left",
      undo: current.can().undo(),
      redo: current.can().redo(),
    }),
  });
  const AlignmentIcon =
    state.alignment === "center"
      ? AlignCenter
      : state.alignment === "right"
        ? AlignRight
        : AlignLeft;
  const tools = [
    {
      label: "加粗",
      Icon: Bold,
      active: state.bold,
      run: () => editor.chain().focus().toggleBold().run(),
    },
    {
      label: "斜体",
      Icon: Italic,
      active: state.italic,
      run: () => editor.chain().focus().toggleItalic().run(),
    },
    {
      label: "下划线",
      Icon: Underline,
      active: state.underline,
      run: () => editor.chain().focus().toggleUnderline().run(),
    },
    {
      label: "删除线",
      Icon: Strikethrough,
      active: state.strike,
      run: () => editor.chain().focus().toggleStrike().run(),
    },
    {
      label: "无序列表",
      Icon: List,
      active: state.bullet,
      run: () => editor.chain().focus().toggleBulletList().run(),
    },
    {
      label: "有序列表",
      Icon: ListOrdered,
      active: state.ordered,
      run: () => editor.chain().focus().toggleOrderedList().run(),
    },
    {
      label: "引用",
      Icon: Quote,
      active: state.quote,
      run: () => editor.chain().focus().toggleBlockquote().run(),
    },
  ];
  const orderedTools = [
    "无序列表",
    "有序列表",
    "引用",
    "加粗",
    "删除线",
    "斜体",
    "下划线",
  ];
  return (
    <div
      className="rich-text-toolbar"
      role="toolbar"
      aria-label="文档格式"
      onMouseDown={(event) => event.preventDefault()}
    >
      <DropdownMenu modal={false}>
        <DropdownMenuTrigger asChild>
          <Button
            variant="ghost"
            className="rich-text-style-trigger"
            disabled={disabled}
            aria-label="文本样式"
            title="文本样式"
          >
            <Type aria-hidden="true" />
            <span className="sr-only">
              {state.heading === "paragraph" ? "正文" : `标题 ${state.heading}`}
            </span>
            <ChevronDown />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent
          className="canvas-text-style-menu"
          align="start"
          data-rich-text-floating
          data-canvas-shortcuts-ignore
          onCloseAutoFocus={(event) => {
            event.preventDefault();
            editor.commands.focus();
          }}
          onEscapeKeyDown={(event) => event.stopPropagation()}
          onPointerDown={(event) => event.stopPropagation()}
          onMouseDown={(event) => event.stopPropagation()}
          onClick={(event) => event.stopPropagation()}
        >
          <DropdownMenuRadioGroup
            value={state.heading}
            onValueChange={(value) => {
              if (value === "paragraph")
                editor.chain().focus().setParagraph().run();
              else
                editor
                  .chain()
                  .focus()
                  .setHeading({ level: Number(value) as 1 | 2 | 3 })
                  .run();
            }}
          >
            <DropdownMenuRadioItem value="paragraph">
              正文
            </DropdownMenuRadioItem>
            {([1, 2, 3] as const).map((level) => (
              <DropdownMenuRadioItem
                key={level}
                value={String(level)}
                data-heading={level}
              >
                标题 {level}
              </DropdownMenuRadioItem>
            ))}
          </DropdownMenuRadioGroup>
          <DropdownMenuItem
            disabled={disabled}
            onSelect={() => importInput.current?.click()}
          >
            导入结构化文档
          </DropdownMenuItem>
          <DropdownMenuItem onSelect={() => void downloadDocument(editor)}>
            导出结构化文档
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
      {tools
        .sort(
          (a, b) =>
            orderedTools.indexOf(a.label) - orderedTools.indexOf(b.label),
        )
        .map(({ label, Icon, active, run }) => (
          <Fragment key={label}>
            <Toggle
              type="button"
              size="sm"
              pressed={active}
              disabled={disabled}
              aria-label={label}
              title={label}
              onPressedChange={run}
            >
              <Icon aria-hidden="true" />
            </Toggle>
            {label === "引用" ? (
              <DropdownMenu modal={false}>
                <DropdownMenuTrigger asChild>
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon-sm"
                    disabled={disabled}
                    className="canvas-text-alignment-trigger"
                    aria-label="段落对齐"
                    title="段落对齐"
                  >
                    <AlignmentIcon aria-hidden="true" />
                    <ChevronDown aria-hidden="true" />
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent
                  align="end"
                  className="canvas-text-alignment-menu"
                  data-rich-text-floating
                  data-canvas-shortcuts-ignore
                  onCloseAutoFocus={(event) => {
                    event.preventDefault();
                    editor.commands.focus();
                  }}
                  onEscapeKeyDown={(event) => event.stopPropagation()}
                  onPointerDown={(event) => event.stopPropagation()}
                  onMouseDown={(event) => event.stopPropagation()}
                  onClick={(event) => event.stopPropagation()}
                >
                  <DropdownMenuRadioGroup
                    value={state.alignment}
                    onValueChange={(value) => {
                      if (
                        !disabled &&
                        editor.isEditable &&
                        ["left", "center", "right"].includes(value)
                      )
                        editor.chain().focus().setTextAlign(value).run();
                    }}
                  >
                    <DropdownMenuRadioItem value="left">
                      <AlignLeft aria-hidden="true" />
                      左对齐
                    </DropdownMenuRadioItem>
                    <DropdownMenuRadioItem value="center">
                      <AlignCenter aria-hidden="true" />
                      居中对齐
                    </DropdownMenuRadioItem>
                    <DropdownMenuRadioItem value="right">
                      <AlignRight aria-hidden="true" />
                      右对齐
                    </DropdownMenuRadioItem>
                  </DropdownMenuRadioGroup>
                </DropdownMenuContent>
              </DropdownMenu>
            ) : null}
          </Fragment>
        ))}
      <Button
        type="button"
        variant="ghost"
        size="icon-sm"
        aria-label="撤销文字编辑"
        title="撤销文字编辑"
        disabled={disabled || !state.undo}
        onClick={() => editor.chain().focus().undo().run()}
      >
        <Undo2 />
      </Button>
      <Button
        type="button"
        variant="ghost"
        size="icon-sm"
        aria-label="重做文字编辑"
        title="重做文字编辑"
        disabled={disabled || !state.redo}
        onClick={() => editor.chain().focus().redo().run()}
      >
        <Redo2 />
      </Button>
      {references.length ? (
        <Popover>
          <PopoverTrigger asChild>
            <Button
              type="button"
              variant="ghost"
              size="icon-sm"
              disabled={disabled}
              aria-label="插入素材引用"
              title="插入素材引用"
            >
              <AtSign />
            </Button>
          </PopoverTrigger>
          <PopoverContent
            data-rich-text-floating
            className="rich-text-suggestions"
            aria-label="文档素材引用"
          >
            {references.map((r) => (
              <Button
                key={r.id}
                variant="ghost"
                className="w-full justify-start"
                onClick={() =>
                  editor
                    .chain()
                    .focus()
                    .insertContent({
                      type: "mention",
                      attrs: {
                        id: r.id,
                        label: r.label,
                        token: referenceToken(r),
                      },
                    })
                    .run()
                }
              >
                {r.label}
              </Button>
            ))}
          </PopoverContent>
        </Popover>
      ) : null}
      <Input
        ref={importInput}
        type="file"
        accept=".json,application/json"
        className="hidden"
        aria-label="选择结构化文档"
        disabled={disabled}
        onChange={(event) => void importDocument(event.target.files?.[0])}
      />

      {onExpand ? (
        <Button
          type="button"
          variant="ghost"
          size="icon-sm"
          aria-label="展开文档编辑"
          title="展开文档编辑"
          onClick={onExpand}
        >
          <Maximize2 />
        </Button>
      ) : null}
    </div>
  );
}

async function downloadDocument(editor: Editor, title = "文本") {
  const document = parseRichTextDocument({
    schemaVersion: 1,
    content: persistentRichText(editor.getJSON()),
  });
  const { saveAs } = await import("file-saver");
  saveAs(
    new Blob([JSON.stringify(document, null, 2)], { type: "application/json" }),
    `${title.replace(/[\\/:*?"<>|]/g, "_")}.json`,
  );
}
