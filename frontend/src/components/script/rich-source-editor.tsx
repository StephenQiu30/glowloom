"use client";

import { useEffect, useId, useState } from "react";
import { EditorContent, useEditor, useEditorState } from "@tiptap/react";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { assertRichPaste } from "./editor-paste";
import { scriptEditorExtensions } from "./editor-extensions";
import {
  canonicalPlainText,
  normalizeScriptText,
  validRichColor,
  validRichLink,
  type RichDocument,
} from "./rich-document";
import { editorToRich, richToEditor } from "./tiptap-adapter";

export function RichSourceEditor({
  initialDocument,
  disabled,
  onChange,
  onInvalid,
}: {
  initialDocument: RichDocument;
  disabled: boolean;
  onChange: (document: RichDocument) => void;
  onInvalid: (message: string | null) => void;
}) {
  const [initialContent] = useState(() => richToEditor(initialDocument));
  const [count, setCount] = useState(
    () => Array.from(canonicalPlainText(initialDocument)).length,
  );
  const [linkOpen, setLinkOpen] = useState(false);
  const [link, setLink] = useState("");
  const [color, setColor] = useState("#111111");
  const [highlight, setHighlight] = useState("#fef08a");
  const [paste, setPaste] = useState<{ text: string; reason: string } | null>(
    null,
  );
  const id = useId();
  const editor = useEditor({
    extensions: scriptEditorExtensions(),
    content: initialContent,
    immediatelyRender: false,
    shouldRerenderOnTransaction: false,
    editable: !disabled,
    editorProps: {
      attributes: {
        role: "textbox",
        "aria-label": "来源正文",
        "aria-multiline": "true",
        class:
          "min-h-64 w-full rounded-lg border bg-background px-4 py-3 outline-none focus-visible:ring-2 focus-visible:ring-ring [&_p]:my-2 [&_h1]:text-2xl [&_h2]:text-xl [&_h3]:text-lg [&_blockquote]:border-l-2 [&_blockquote]:pl-4 [&_ul]:list-disc [&_ul]:pl-6 [&_ol]:list-decimal [&_ol]:pl-6 [&_pre]:overflow-auto [&_pre]:bg-muted [&_pre]:p-3 [&_a]:underline [&_hr]:my-4",
      },
      handlePaste: (_view, event) => {
        if (event.clipboardData?.files?.length) {
          event.preventDefault();
          onInvalid("正文不直接接收文件。请使用项目原文导入入口并确认版权。");
          return true;
        }
        const html = event.clipboardData?.getData("text/html");
        if (!html) return false;
        try {
          assertRichPaste(html);
          return false;
        } catch (error) {
          event.preventDefault();
          setPaste({
            text: event.clipboardData?.getData("text/plain") ?? "",
            reason:
              error instanceof Error
                ? error.message
                : "剪贴板格式无法完整保留。",
          });
          return true;
        }
      },
      handleDrop: (_view, event) => {
        if (event.dataTransfer?.files.length) {
          event.preventDefault();
          onInvalid("正文不直接接收文件。请使用项目原文导入入口并确认版权。");
          return true;
        }
        return false;
      },
    },
    onUpdate: ({ editor }) => {
      try {
        const document = editorToRich(editor.getJSON());
        setCount(Array.from(canonicalPlainText(document)).length);
        onInvalid(null);
        onChange(document);
      } catch (error) {
        onInvalid(
          error instanceof Error
            ? error.message
            : "正文格式无法完整保存，请保留原稿后检查。",
        );
      }
    },
  });
  const formats = useEditorState({
    editor,
    selector: ({ editor }) => ({
      bold: editor?.isActive("bold") ?? false,
      italic: editor?.isActive("italic") ?? false,
      underline: editor?.isActive("underline") ?? false,
      strike: editor?.isActive("strike") ?? false,
      code: editor?.isActive("code") ?? false,
      bulletList: editor?.isActive("bulletList") ?? false,
      orderedList: editor?.isActive("orderedList") ?? false,
      blockquote: editor?.isActive("blockquote") ?? false,
      codeBlock: editor?.isActive("codeBlock") ?? false,
      block: editor?.isActive("heading", { level: 1 })
        ? "1"
        : editor?.isActive("heading", { level: 2 })
          ? "2"
          : editor?.isActive("heading", { level: 3 })
            ? "3"
            : "paragraph",
      align:
        ["left", "center", "right", "justify"].find((value) =>
          editor?.isActive({ textAlign: value }),
        ) ?? "default",
      undo: editor?.can().undo() ?? false,
      redo: editor?.can().redo() ?? false,
    }),
  });
  useEffect(() => {
    editor?.setEditable(!disabled, false);
  }, [disabled, editor]);
  const unavailable = disabled || !editor;
  const marks = [
    ["粗体", "bold"],
    ["斜体", "italic"],
    ["下划线", "underline"],
    ["删除线", "strike"],
    ["行内代码", "code"],
  ] as const;
  return (
    <div className="min-w-0 space-y-3">
      <div
        role="group"
        aria-label="正文格式"
        className="flex flex-wrap items-center gap-2"
      >
        <Select
          value={formats?.block ?? "paragraph"}
          disabled={unavailable}
          onValueChange={(value) => {
            if (value === "paragraph")
              editor?.chain().focus().setParagraph().run();
            else
              editor
                ?.chain()
                .focus()
                .setHeading({ level: Number(value) as 1 | 2 | 3 })
                .run();
          }}
        >
          <SelectTrigger aria-label="段落或标题">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="paragraph">正文</SelectItem>
            {[1, 2, 3].map((level) => (
              <SelectItem key={level} value={String(level)}>
                标题 {level}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        {marks.map(([label, mark]) => (
          <Button
            key={mark}
            type="button"
            variant="outline"
            disabled={unavailable}
            aria-pressed={formats?.[mark] ?? false}
            onClick={() => editor?.chain().focus().toggleMark(mark).run()}
          >
            {label}
          </Button>
        ))}
        <Button
          type="button"
          variant="outline"
          disabled={unavailable}
          aria-pressed={formats?.bulletList ?? false}
          onClick={() => editor?.chain().focus().toggleBulletList().run()}
        >
          无序列表
        </Button>
        <Button
          type="button"
          variant="outline"
          disabled={unavailable}
          aria-pressed={formats?.orderedList ?? false}
          onClick={() => editor?.chain().focus().toggleOrderedList().run()}
        >
          有序列表
        </Button>
        <Button
          type="button"
          variant="outline"
          disabled={unavailable}
          aria-pressed={formats?.blockquote ?? false}
          onClick={() => editor?.chain().focus().toggleBlockquote().run()}
        >
          引用
        </Button>
        <Button
          type="button"
          variant="outline"
          disabled={unavailable}
          aria-pressed={formats?.codeBlock ?? false}
          onClick={() => editor?.chain().focus().toggleCodeBlock().run()}
        >
          代码块
        </Button>
        <Button
          type="button"
          variant="outline"
          disabled={unavailable}
          onClick={() => editor?.chain().focus().setHorizontalRule().run()}
        >
          分隔线
        </Button>
        <Select
          value={formats?.align ?? "default"}
          disabled={unavailable}
          onValueChange={(value) => {
            if (value === "default")
              editor?.chain().focus().unsetTextAlign().run();
            else editor?.chain().focus().setTextAlign(value).run();
          }}
        >
          <SelectTrigger aria-label="文本对齐">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="default">默认对齐</SelectItem>
            <SelectItem value="left">左对齐</SelectItem>
            <SelectItem value="center">居中</SelectItem>
            <SelectItem value="right">右对齐</SelectItem>
            <SelectItem value="justify">两端对齐</SelectItem>
          </SelectContent>
        </Select>
        <Button
          type="button"
          variant="outline"
          disabled={unavailable}
          onClick={() => {
            setLink(String(editor?.getAttributes("link").href ?? ""));
            setLinkOpen(true);
          }}
        >
          链接
        </Button>
        <Button
          type="button"
          variant="outline"
          disabled={unavailable}
          onClick={() =>
            editor?.chain().focus().unsetAllMarks().clearNodes().run()
          }
        >
          清除格式
        </Button>
        <Button
          type="button"
          variant="outline"
          disabled={unavailable || !formats?.undo}
          onClick={() => editor?.chain().focus().undo().run()}
        >
          撤销
        </Button>
        <Button
          type="button"
          variant="outline"
          disabled={unavailable || !formats?.redo}
          onClick={() => editor?.chain().focus().redo().run()}
        >
          重做
        </Button>
      </div>
      <div className="flex flex-wrap items-end gap-2">
        <div className="min-w-0">
          <Label htmlFor={`${id}-color`}>文字颜色</Label>
          <Input
            id={`${id}-color`}
            className="w-36"
            value={color}
            disabled={unavailable}
            onChange={(event) => setColor(event.target.value)}
          />
        </div>
        <Button
          type="button"
          variant="outline"
          disabled={unavailable || !validRichColor(color)}
          onClick={() => editor?.chain().focus().setColor(color).run()}
        >
          应用文字色
        </Button>
        <Button
          type="button"
          variant="outline"
          disabled={unavailable}
          onClick={() => editor?.chain().focus().unsetColor().run()}
        >
          移除文字色
        </Button>
        <div className="min-w-0">
          <Label htmlFor={`${id}-highlight`}>高亮颜色</Label>
          <Input
            id={`${id}-highlight`}
            className="w-36"
            value={highlight}
            disabled={unavailable}
            onChange={(event) => setHighlight(event.target.value)}
          />
        </div>
        <Button
          type="button"
          variant="outline"
          disabled={unavailable || !validRichColor(highlight)}
          onClick={() =>
            editor?.chain().focus().setHighlight({ color: highlight }).run()
          }
        >
          应用高亮
        </Button>
        <Button
          type="button"
          variant="outline"
          disabled={unavailable}
          onClick={() => editor?.chain().focus().unsetHighlight().run()}
        >
          移除高亮
        </Button>
      </div>
      {!editor ? (
        <p role="status">正在加载正文编辑器…</p>
      ) : (
        <EditorContent editor={editor} />
      )}
      <p className="text-sm text-muted-foreground" aria-live="polite">
        规范正文 {count.toLocaleString("zh-CN")} 个 Unicode
        字符；段落与换行计入正文。
      </p>
      <Dialog open={linkOpen} onOpenChange={setLinkOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>正文链接</DialogTitle>
            <DialogDescription>
              允许 HTTP、HTTPS、邮件和相对链接；原稿不会因非法地址被改写。
            </DialogDescription>
          </DialogHeader>
          <Label htmlFor={`${id}-link`}>链接地址</Label>
          <Input
            id={`${id}-link`}
            value={link}
            onChange={(event) => setLink(event.target.value)}
            disabled={disabled}
          />
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => setLinkOpen(false)}
            >
              取消
            </Button>
            <Button
              type="button"
              variant="outline"
              disabled={disabled}
              onClick={() => {
                editor
                  ?.chain()
                  .focus()
                  .extendMarkRange("link")
                  .unsetLink()
                  .run();
                setLinkOpen(false);
              }}
            >
              移除链接
            </Button>
            <Button
              type="button"
              disabled={disabled || !validRichLink(link)}
              onClick={() => {
                editor
                  ?.chain()
                  .focus()
                  .extendMarkRange("link")
                  .setLink({ href: link })
                  .run();
                setLinkOpen(false);
              }}
            >
              应用链接
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      <Dialog
        open={paste !== null}
        onOpenChange={(open) => {
          if (!open) setPaste(null);
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>确认剪贴板格式</DialogTitle>
            <DialogDescription>{paste?.reason}</DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => setPaste(null)}
            >
              取消粘贴
            </Button>
            <Button
              type="button"
              disabled={disabled || !paste?.text}
              onClick={() => {
                if (paste) {
                  const text = normalizeScriptText(paste.text);
                  const content = text
                    .split("\n")
                    .flatMap((line, index) => [
                      ...(index ? [{ type: "hardBreak" }] : []),
                      ...(line ? [{ type: "text", text: line }] : []),
                    ]);
                  editor?.chain().focus().insertContent(content).run();
                  setPaste(null);
                }
              }}
            >
              仅粘贴纯文本
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
