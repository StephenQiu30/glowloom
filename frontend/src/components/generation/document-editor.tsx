"use client";

import type { Transaction } from "@tiptap/pm/state";
import type { ComposerCollaboration } from "./composer-session";

import { useEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { EditorContent, useEditorState } from "@tiptap/react";
import { Extension, generateHTML, type Editor } from "@tiptap/core";
import { Plugin } from "@tiptap/pm/state";
import StarterKit from "@tiptap/starter-kit";
import TextAlign from "@tiptap/extension-text-align";
import { Markdown } from "@tiptap/markdown";
import {
  Bold,
  Italic,
  Underline,
  Strikethrough,
  List,
  ListOrdered,
  Quote,
  AlignLeft,
  AlignCenter,
  AlignRight,
  Undo2,
  Redo2,
  Maximize2,
  X,
  ChevronDown,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { useEditorSurface } from "./editor-surface";
import "./document-editor.css";

const starterOptions = {
  heading: { levels: [1, 2, 3] as (1 | 2 | 3)[] },
  link: false as const,
  trailingNode: false as const,
};
const textAlign = TextAlign.configure({ types: ["heading", "paragraph"] });
const htmlExtensions = [StarterKit.configure(starterOptions), textAlign];
// Markdown carries normal text formatting; aligned blocks use schema-filtered HTML
// inside Markdown so alignment survives the existing string storage contract.
const documentKit = StarterKit.extend({
  addExtensions() {
    return (this.parent?.() ?? []).map((extension) =>
      ["paragraph", "heading"].includes(extension.name)
        ? extension.extend({
            renderMarkdown(node, helpers) {
              if (node.attrs?.textAlign && node.attrs.textAlign !== "left")
                return generateHTML(
                  { type: "doc", content: [node] },
                  htmlExtensions,
                );
              const content = helpers.renderChildren(node.content ?? []);
              return node.type === "heading"
                ? `${"#".repeat(node.attrs?.level ?? 1)} ${content}`
                : content || "&nbsp;";
            },
          })
        : extension,
    );
  },
}).configure(starterOptions);

export function DocumentEditor({
  id,
  text,
  editable,
  onChange,
  onReady,
  onFinish,
  onDispose,
  collaboration,
}: {
  id: string;
  text: string;
  editable: boolean;
  onChange: (text: string, transaction?: Transaction, editor?: Editor) => void;
  collaboration?: Pick<ComposerCollaboration, "history" | "composition">;
  onDispose?: () => void;
  onReady?: (editor: Editor) => void;
  onFinish?: () => void;
}) {
  const [expanded, setExpanded] = useState(false);
  const [error, setError] = useState("");
  const [compositionActive, setCompositionActive] = useState(false);
  const latest = useRef({ text, onChange, onReady, onDispose, collaboration });
  useEffect(() => {
    latest.current = { text, onChange, onReady, onDispose, collaboration };
  }, [text, onChange, onReady, onDispose, collaboration]);
  const last = useRef(text);
  const compositionTimer = useRef<ReturnType<typeof setTimeout> | undefined>(
      undefined,
    ),
    composing = useRef(false);
  const surface = useRef<HTMLDivElement>(null);
  const editor = useEditorSurface(
    {
      extensions: [
        documentKit.configure({ undoRedo: collaboration ? false : undefined }),
        textAlign,
        Markdown,
        Extension.create({
          name: "documentLength",
          priority: 1000,
          addProseMirrorPlugins() {
            return [
              new Plugin({
                props: {
                  handleKeyDown: (_view, event) => {
                    if (!collaboration || event.isComposing) return false;
                    const key = event.key.toLowerCase();
                    if (
                      (event.metaKey || event.ctrlKey) &&
                      ["z", "y"].includes(key)
                    ) {
                      collaboration.history[
                        key === "y" || event.shiftKey ? "redo" : "undo"
                      ]();
                      event.preventDefault();
                      return true;
                    }
                    return false;
                  },
                  handleDOMEvents: {
                    beforeinput: (_view, event) => {
                      const input = event as InputEvent;
                      if (
                        collaboration &&
                        ["historyUndo", "historyRedo"].includes(input.inputType)
                      ) {
                        event.preventDefault();
                        collaboration.history[
                          input.inputType === "historyUndo" ? "undo" : "redo"
                        ]();
                        return true;
                      }
                      return false;
                    },
                  },
                },
                filterTransaction: (transaction) => {
                  if (!transaction.docChanged) return true;
                  const next =
                    this.editor.markdown?.serialize(transaction.doc.toJSON()) ??
                    "";
                  if (Array.from(next).length <= 10000) return true;
                  setError("文本最多保存 10000 字，请缩短内容。");
                  return false;
                },
              }),
            ];
          },
        }),
      ],
      content: text,
      contentType: "markdown",
      onCreate: ({ editor }) => latest.current.onReady?.(editor),
      onDestroy: () => {
        clearTimeout(compositionTimer.current);
        if (composing.current) collaboration?.composition(false);
        latest.current.onDispose?.();
      },
      onUpdate: ({ editor, transaction }) => {
        const next = editor.getMarkdown();
        if (Array.from(next).length > 10000) {
          setError("文本最多保存 10000 字，请缩短内容。");
          return;
        }
        setError("");
        last.current = next;
        if (collaboration) latest.current.onChange(next, transaction, editor);
        else latest.current.onChange(next);
      },
      editorProps: {
        handleDOMEvents: {
          drop: (_, event) => {
            event.preventDefault();
            return true;
          },
        },
      },
    },
    [id],
    {
      role: editable ? "textbox" : "document",
      "aria-label": "文本内容",
      "aria-multiline": "true",
      "data-placeholder": "双击编辑文本",
      class: "rich-document-body",
    },
    editable || compositionActive,
  );
  useEffect(() => {
    if (!editor) return;
    const start = () => {
      clearTimeout(compositionTimer.current);
      composing.current = true;
      setCompositionActive(true);
      latest.current.collaboration?.composition(true);
    };
    const end = () => {
      clearTimeout(compositionTimer.current);
      compositionTimer.current = setTimeout(() => {
        composing.current = false;
        setCompositionActive(false);
        latest.current.collaboration?.composition(false);
      }, 0);
    };
    const dom = editor.view.dom;
    dom.addEventListener("compositionstart", start);
    dom.addEventListener("compositionend", end);
    return () => {
      dom.removeEventListener("compositionstart", start);
      dom.removeEventListener("compositionend", end);
      clearTimeout(compositionTimer.current);
      if (composing.current) latest.current.collaboration?.composition(false);
      composing.current = false;
    };
  }, [editor]);
  useEffect(() => {
    if (!editor || text === last.current) return;
    last.current = text;
    if (editor.getMarkdown() === text) return;
    editor.commands.setContent(text, {
      contentType: "markdown",
      emitUpdate: false,
    });
  }, [editor, text]);
  useEffect(() => {
    if (!editor || !editable) return;
    const frame = requestAnimationFrame(() => editor.commands.focus());
    return () => cancelAnimationFrame(frame);
  }, [editor, editable, expanded]);
  const content = (
    <div
      ref={surface}
      className={expanded ? "rich-document-fullscreen" : "rich-document"}
      role={expanded ? "dialog" : undefined}
      aria-modal={expanded || undefined}
      aria-label={expanded ? "文本编辑器" : undefined}
      data-canvas-no-zoom={editable || undefined}
      data-canvas-wheel-scroll={editable || undefined}
      onPointerDown={(event) => {
        if (editable) event.stopPropagation();
      }}
      onClick={(event) => {
        if (editable) event.stopPropagation();
      }}
      onKeyDown={(event) => {
        if (!editable) return;
        event.stopPropagation();
        if (event.key === "Escape") {
          event.preventDefault();
          if (expanded) {
            setExpanded(false);
            editor?.commands.focus();
          } else onFinish?.();
        }
        if (expanded && event.key === "Tab") {
          const controls = Array.from(
            surface.current?.querySelectorAll<HTMLElement>(
              'button:not(:disabled),[contenteditable="true"]',
            ) ?? [],
          );
          const first = controls[0],
            last = controls.at(-1);
          if (event.shiftKey && document.activeElement === first) {
            event.preventDefault();
            last?.focus();
          } else if (!event.shiftKey && document.activeElement === last) {
            event.preventDefault();
            first?.focus();
          }
        }
      }}
    >
      {editable && editor && (
        <DocumentToolbar
          collaboration={collaboration}
          editor={editor}
          expanded={expanded}
          onExpand={() => setExpanded(!expanded)}
        />
      )}
      {expanded && (
        <Button
          className="absolute top-4 right-4"
          variant="ghost"
          size="icon"
          aria-label="关闭全屏编辑"
          onClick={() => {
            setExpanded(false);
            editor?.commands.focus();
          }}
        >
          <X />
        </Button>
      )}
      <EditorContent editor={editor} className="rich-document-content" />
      {error && (
        <p role="alert" className="text-sm text-destructive">
          {error}
        </p>
      )}
    </div>
  );
  return expanded && editable ? createPortal(content, document.body) : content;
}

function DocumentToolbar({
  editor,
  expanded,
  onExpand,
  collaboration,
}: {
  collaboration?: Pick<ComposerCollaboration, "history" | "composition">;
  editor: Editor;
  expanded: boolean;
  onExpand: () => void;
}) {
  const state = useEditorState({
    editor,
    selector: ({ editor }) => ({
      heading:
        [1, 2, 3].find((level) => editor.isActive("heading", { level })) ?? 0,
      bold: editor.isActive("bold"),
      italic: editor.isActive("italic"),
      underline: editor.isActive("underline"),
      strike: editor.isActive("strike"),
      bulletList: editor.isActive("bulletList"),
      orderedList: editor.isActive("orderedList"),
      blockquote: editor.isActive("blockquote"),
      alignment: editor.isActive({ textAlign: "center" })
        ? "center"
        : editor.isActive({ textAlign: "right" })
          ? "right"
          : "left",
      undo: collaboration ? false : editor.can().undo(),
      redo: collaboration ? false : editor.can().redo(),
    }),
  });
  const actions = [
    {
      label: "无序列表",
      icon: List,
      active: state.bulletList,
      run: () => editor.chain().focus().toggleBulletList().run(),
    },
    {
      label: "有序列表",
      icon: ListOrdered,
      active: state.orderedList,
      run: () => editor.chain().focus().toggleOrderedList().run(),
    },
    {
      label: "引用",
      icon: Quote,
      active: state.blockquote,
      run: () => editor.chain().focus().toggleBlockquote().run(),
    },
    {
      label: "加粗",
      icon: Bold,
      active: state.bold,
      run: () => editor.chain().focus().toggleBold().run(),
    },
    {
      label: "删除线",
      icon: Strikethrough,
      active: state.strike,
      run: () => editor.chain().focus().toggleStrike().run(),
    },
    {
      label: "斜体",
      icon: Italic,
      active: state.italic,
      run: () => editor.chain().focus().toggleItalic().run(),
    },
    {
      label: "下划线",
      icon: Underline,
      active: state.underline,
      run: () => editor.chain().focus().toggleUnderline().run(),
    },
  ];
  return (
    <div
      role="toolbar"
      aria-label="文本格式"
      className="rich-document-toolbar"
      onPointerDown={(event) => event.stopPropagation()}
      onMouseDown={(event) => event.preventDefault()}
    >
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button variant="ghost" size="sm" aria-label="文本样式">
            {state.heading ? `标题 ${state.heading}` : "正文"}
            <ChevronDown />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent sideOffset={8} align="start">
          <DropdownMenuGroup>
            <DropdownMenuItem
              onSelect={() => editor.chain().focus().setParagraph().run()}
            >
              正文
            </DropdownMenuItem>
            {([1, 2, 3] as const).map((level) => (
              <DropdownMenuItem
                key={level}
                onSelect={() =>
                  editor.chain().focus().setHeading({ level }).run()
                }
              >
                标题 {level}
              </DropdownMenuItem>
            ))}
          </DropdownMenuGroup>
        </DropdownMenuContent>
      </DropdownMenu>
      {actions.slice(0, 3).map(({ label, icon: Icon, active, run }) => (
        <Button
          key={label}
          variant="ghost"
          size="icon-sm"
          aria-label={label}
          aria-pressed={active}
          onClick={run}
        >
          <Icon />
        </Button>
      ))}
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button variant="ghost" size="icon-sm" aria-label="文本对齐">
            {state.alignment === "center" ? (
              <AlignCenter />
            ) : state.alignment === "right" ? (
              <AlignRight />
            ) : (
              <AlignLeft />
            )}
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent sideOffset={8}>
          <DropdownMenuGroup>
            {[
              { value: "left", label: "左对齐", icon: AlignLeft },
              { value: "center", label: "居中对齐", icon: AlignCenter },
              { value: "right", label: "右对齐", icon: AlignRight },
            ].map(({ value, label, icon: Icon }) => (
              <DropdownMenuItem
                key={value}
                onSelect={() =>
                  editor.chain().focus().setTextAlign(value).run()
                }
              >
                <Icon />
                {label}
              </DropdownMenuItem>
            ))}
          </DropdownMenuGroup>
        </DropdownMenuContent>
      </DropdownMenu>
      {actions.slice(3).map(({ label, icon: Icon, active, run }) => (
        <Button
          key={label}
          variant="ghost"
          size="icon-sm"
          aria-label={label}
          aria-pressed={active}
          onClick={run}
        >
          <Icon />
        </Button>
      ))}
      <Button
        variant="ghost"
        size="icon-sm"
        aria-label="撤销文本编辑"
        disabled={
          collaboration
            ? !(collaboration.history.canUndo?.() ?? false)
            : !state.undo
        }
        onClick={() =>
          collaboration
            ? collaboration.history.undo()
            : editor.chain().focus().undo().run()
        }
      >
        <Undo2 />
      </Button>
      <Button
        variant="ghost"
        size="icon-sm"
        aria-label="重做文本编辑"
        disabled={
          collaboration
            ? !(collaboration.history.canRedo?.() ?? false)
            : !state.redo
        }
        onClick={() =>
          collaboration
            ? collaboration.history.redo()
            : editor.chain().focus().redo().run()
        }
      >
        <Redo2 />
      </Button>
      {!expanded && (
        <Button
          variant="ghost"
          size="icon-sm"
          aria-label="全屏编辑文本"
          onClick={onExpand}
        >
          <Maximize2 />
        </Button>
      )}
    </div>
  );
}
