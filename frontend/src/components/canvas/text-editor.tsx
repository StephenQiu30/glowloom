"use client";

import {
  useRef,
  useState,
  useLayoutEffect,
  type RefObject,
  type CSSProperties,
} from "react";
import { Maximize2, Minimize2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import { PromptEditor } from "@/components/generation/prompt-editor";
import { ReferenceControls } from "@/components/generation/reference-controls";
import type { ComposerSession } from "@/components/generation/composer-session";
import type { CanvasReferenceSource } from "@/components/generation/reference-source";
import { textDraft, withTextDraft } from "./text-content";
import type { CanvasNodeData } from "./types";

export function CanvasTextEditor({
  node,
  anchorRef,
  projectID,
  sources,
  onSave,
  onClose,
}: {
  node: CanvasNodeData;
  anchorRef?: RefObject<HTMLDivElement | null>;
  projectID?: string;
  sources?: CanvasReferenceSource[];
  onSave: (node: CanvasNodeData) => void | boolean | Promise<boolean>;
  onClose: () => void;
}) {
  const [initial] = useState(() => textDraft(node));
  const [draft, setDraft] = useState(initial);
  const [session, setSession] = useState<ComposerSession | null>(null);
  const [expanded, setExpanded] = useState(false);
  const [saving, setSaving] = useState(false);
  const inFlight = useRef(false);
  const [error, setError] = useState("");
  const [anchorStyle, setAnchorStyle] = useState<CSSProperties>();
  useLayoutEffect(() => {
    const element = anchorRef?.current;
    if (!element) return;
    function place() {
      const rect = element!.getBoundingClientRect();
      const width = Math.min(Math.max(320, rect.width), window.innerWidth - 32);
      const height = Math.min(
        Math.max(320, rect.height),
        window.innerHeight - 32,
      );
      setAnchorStyle({
        left: Math.max(16, Math.min(rect.left, window.innerWidth - width - 16)),
        top: Math.max(16, Math.min(rect.top, window.innerHeight - height - 16)),
        width,
        height,
        maxWidth: "none",
        translate: "none",
      });
    }
    place();
    const observer = new ResizeObserver(place);
    observer.observe(element);
    window.addEventListener("resize", place);
    return () => {
      observer.disconnect();
      window.removeEventListener("resize", place);
    };
  }, [anchorRef]);
  const save = async () => {
    if (!session || session.composing || inFlight.current) return;
    inFlight.current = true;
    const value = session.snapshot().draft;
    setSaving(true);
    setError("");
    session.editor.setEditable(false);
    try {
      const confirmed = await onSave(withTextDraft(node, value));
      if (confirmed === false)
        throw new Error("保存未完成，正文与引用已保留。请确认连接状态后重试。");
      onClose();
    } catch (error) {
      setError(
        error instanceof Error ? error.message : "保存失败，正文与引用已保留",
      );
    } finally {
      inFlight.current = false;
      setSaving(false);
      if (!session.editor.isDestroyed) session.editor.setEditable(true);
    }
  };
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !saving && !session?.composing) onClose();
      }}
    >
      <DialogContent
        data-canvas-text-editing
        data-canvas-no-zoom
        showCloseButton={false}
        overlayClassName={anchorRef && !expanded ? "bg-transparent" : undefined}
        style={anchorRef && !expanded ? anchorStyle : undefined}
        className={`flex flex-col gap-3 p-4 ${expanded ? "h-[calc(100dvh-2rem)] sm:max-w-[1012px]" : anchorRef ? "border border-muted-foreground bg-card shadow-none" : "sm:max-w-[680px]"}`}
        onPointerDown={(e) => e.stopPropagation()}
        onDoubleClick={(e) => e.stopPropagation()}
        onKeyDown={(e) => e.stopPropagation()}
        onInteractOutside={(e) => e.preventDefault()}
        onEscapeKeyDown={(e) => e.preventDefault()}
        onOpenAutoFocus={(e) => e.preventDefault()}
      >
        <div className="flex shrink-0 items-center justify-end gap-2">
          <DialogTitle
            className={anchorRef && !expanded ? "sr-only" : "truncate"}
          >
            {node.title}
          </DialogTitle>
          <Button
            variant="ghost"
            size="icon-sm"
            aria-label={expanded ? "收起编辑器" : "展开编辑器"}
            disabled={saving || !!session?.composing}
            onMouseDown={(e) => e.preventDefault()}
            onClick={() => setExpanded(!expanded)}
          >
            {expanded ? <Minimize2 /> : <Maximize2 />}
          </Button>
        </div>
        <DialogDescription className={anchorRef ? "sr-only" : undefined}>
          输入 @ 引用图片、视频、音频或主体。同一参考可多次提及。
        </DialogDescription>
        {session && (projectID || draft.references.length > 0) && (
          <div className="max-h-24 shrink-0 overflow-auto">
            <ReferenceControls
              session={session}
              draft={draft}
              projectID={projectID ?? ""}
              authorized={!!projectID}
              disabled={saving || !projectID}
              compact
              canvasSources={sources}
            />
          </div>
        )}
        <div
          className={`min-w-0 overflow-y-auto ${anchorRef ? "min-h-0 flex-1" : expanded ? "min-h-48 flex-1 rounded-lg border bg-background" : "max-h-[45dvh] rounded-lg border bg-background"}`}
        >
          <PromptEditor
            contextKey={`canvas-text:${projectID ?? "local"}:${node.id}`}
            replicaId={node.id}
            draft={initial}
            editable={!saving}
            label="文本内容"
            placeholder="输入文本，使用 @ 引用参考内容"
            className={
              anchorRef
                ? "min-h-full p-2 text-sm leading-[22px] whitespace-pre-wrap outline-none [&_p]:min-h-6"
                : "min-h-40 p-4 text-base whitespace-pre-wrap outline-none sm:text-sm [&_p]:min-h-6"
            }
            onReady={(value) => {
              setSession(value);
              value.editor.commands.focus("end");
            }}
            onChange={(snapshot) => setDraft(snapshot.draft)}
            onRequestSave={() => void save()}
          />
        </div>
        {error && (
          <p role="alert" className="text-sm text-destructive">
            {error}
          </p>
        )}
        <div className="flex shrink-0 flex-wrap items-center justify-between gap-2">
          <span
            className={
              anchorRef && !expanded
                ? "sr-only"
                : "text-xs text-muted-foreground"
            }
          >
            {draft.references.length} 项参考 · ⌘ / Ctrl + S 保存
          </span>
          <div className="ml-auto flex gap-2">
            <Button variant="ghost" disabled={saving} onClick={onClose}>
              取消
            </Button>
            <Button disabled={!session || saving} onClick={() => void save()}>
              {saving ? "保存中…" : "保存文本"}
            </Button>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  );
}
