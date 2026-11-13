"use client";
import { useEffect, useRef, useState, useSyncExternalStore } from "react";
import { toast } from "@/components/feedback/notification";
import { FeedbackToast } from "@/components/feedback/feedback-toast";
import {
  AtSign,
  Contact,
  FolderOpen,
  Heart,
  LoaderCircle,
  MousePointer2,
  Pencil,
  Settings2,
  Upload,
  X,
} from "lucide-react";
import { getSubjectVersion, listSubjects } from "@/api/subjects";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import {
  SubjectNodeEditor,
  type SubjectMediaTarget,
} from "./subject-node-editor";
import { SubjectNodeMedia } from "./subject-node-media";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import {
  Empty,
  EmptyContent,
  EmptyDescription,
  EmptyHeader,
  EmptyTitle,
} from "@/components/ui/empty";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from "@/components/ui/dialog";
import { UnifiedSubjectMedia } from "@/components/subjects/unified-media";
import {
  requireUnifiedSubjectPage,
  type UnifiedSubject,
  type UnifiedSubjectPage,
  type SubjectMedia,
} from "@/components/subjects/unified-contracts";
import { getScopeGeneration, getSessionGeneration } from "@/lib/request";
import { canonical } from "@/components/generation/draft-save";
import { requireCanvasSubjectVersion } from "../lib/canvas-subject-source";
import { reconcileSubjectSourceRelations } from "../lib/canvas-subject-connections";
import type { CanvasYjsLiveSession } from "../yjs-live-session";
import {
  materializeSubject,
  localSubjectConfig,
  type CanvasSubjectConfig,
  importCanvasSubjects,
  setSubjectConfig,
  subjectConfig,
} from "./subject-node-state";
import "./subject-node.css";
function boundMedia(subject: UnifiedSubject, media: SubjectMedia) {
  return {
    ...media,
    bound: {
      subjectID: subject.id,
      versionID: subject.version_id,
      bindingID: media.binding_id,
    },
  };
}
export function CanvasSubjectNode({
  client,
  nodeID,
  actorID,
  scopeID,
  projectID,
}: {
  client: CanvasYjsLiveSession;
  nodeID: string;
  actorID: string;
  scopeID: string;
  projectID: string;
}) {
  const view = useSyncExternalStore(
    client.subscribe,
    client.getSnapshot,
    client.getSnapshot,
  );
  const node = view.document?.nodes.find((item) => item.id === nodeID);
  const [open, setOpen] = useState(false);
  const [editing, setEditing] = useState<{
    initialSelection?: {
      target: SubjectMediaTarget;
      upload?: boolean;
      canvas?: boolean;
    };
  } | null>(null);
  const canvasButton = useRef<HTMLButtonElement>(null);
  const nameButton = useRef<HTMLButtonElement>(null);
  const descriptionButton = useRef<HTMLButtonElement>(null);
  const inlineLock = useRef(false);
  const inlineCancelled = useRef(false);
  const [inline, setInline] = useState<{
    field: "title" | "description";
    value: string;
    title: string;
    config: CanvasSubjectConfig;
    epoch: string | null;
    session: number;
    scope: number;
  } | null>(null);
  const [inlineBusy, setInlineBusy] = useState(false);
  const [retry, setRetry] = useState(0);
  const [result, setResult] = useState<{
    key: string;
    subject?: UnifiedSubject;
    error?: string;
  } | null>(null);
  const [error, setError] = useState("");
  const config = node ? subjectConfig(node) : null;
  const subjectID = config?.subject_id,
    versionID = config?.version_id;
  const identity = `${scopeID}/${subjectID ?? ""}/${versionID ?? ""}/${retry}`;
  useEffect(() => {
    if (!subjectID || !versionID) return;
    const controller = new AbortController();
    getSubjectVersion(
      { id: subjectID, version: versionID },
      { signal: controller.signal },
    )
      .then((value) =>
        requireCanvasSubjectVersion(value, scopeID, {
          subject_id: subjectID,
          version_id: versionID,
        }),
      )
      .then((subject) => {
        if (!controller.signal.aborted) setResult({ key: identity, subject });
      })
      .catch(() => {
        if (!controller.signal.aborted)
          setResult({ key: identity, error: "主体已删除或当前空间无权读取" });
      });
    return () => controller.abort();
  }, [subjectID, versionID, scopeID, identity]);
  if (!node || !config) return null;
  const current = result?.key === identity ? result : null;
  const subject = current?.subject;
  const local = config.local;
  const canEdit =
    view.writable && !view.migration && (!subjectID || !!subject || !!local);
  const description = local?.description ?? subject?.description ?? "";
  function beginInline(field: "title" | "description") {
    if (!canEdit || inlineLock.current) return;
    inlineCancelled.current = false;
    setInline({
      field,
      value: field === "title" ? node!.title : description,
      title: node!.title,
      config: structuredClone(config!),
      epoch: client.getDocumentEpoch(),
      session: getSessionGeneration(),
      scope: getScopeGeneration(),
    });
  }
  function restoreInlineFocus(field: "title" | "description") {
    queueMicrotask(() =>
      (field === "title" ? nameButton : descriptionButton).current?.focus(),
    );
  }
  async function commitInline(restoreFocus = false) {
    if (!inline || inlineLock.current || inlineCancelled.current) return;
    const value = inline.field === "title" ? inline.value.trim() : inline.value;
    if (value === (inline.field === "title" ? inline.title : description)) {
      setInline(null);
      return;
    }
    inlineLock.current = true;
    setInlineBusy(true);
    setError("");
    try {
      if (
        !inline.epoch ||
        client.getDocumentEpoch() !== inline.epoch ||
        getSessionGeneration() !== inline.session ||
        getScopeGeneration() !== inline.scope
      )
        throw new Error("画布、账号或空间已变化，请重新编辑");
      const graph = client.getSnapshot().document;
      const target = graph?.nodes.find((item) => item.id === nodeID);
      if (
        !graph ||
        !target ||
        target.title !== inline.title ||
        canonical(subjectConfig(target)) !== canonical(inline.config)
      )
        throw new Error("主体已被其他编辑更新，当前输入已保留");
      if (
        inline.field === "title" &&
        (!value || Array.from(value).length > 128 || value.includes("\0"))
      )
        throw new Error("主体名称应为1至128个字符");
      const nextConfig =
        inline.field === "description"
          ? localSubjectConfig(inline.config, {
              ...materializeSubject(inline.config, subject),
              description: value,
            })
          : inline.config;
      const next = reconcileSubjectSourceRelations(graph, {
        ...graph,
        nodes: graph.nodes.map((item) =>
          item.id === nodeID
            ? setSubjectConfig(
                {
                  ...item,
                  title: inline.field === "title" ? value : item.title,
                },
                nextConfig,
              )
            : item,
        ),
      });
      if (!client.canAccept(next))
        throw new Error("画布当前不可编辑，当前输入已保留");
      client.change(next, true);
      await client.waitForSaved();
      setInline(null);
      if (restoreFocus) restoreInlineFocus(inline.field);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "保存尚未确认");
    } finally {
      inlineLock.current = false;
      setInlineBusy(false);
    }
  }
  function inlineKey(event: React.KeyboardEvent) {
    if (event.nativeEvent.isComposing || event.keyCode === 229) return;
    if (event.key === "Escape") {
      event.preventDefault();
      event.stopPropagation();
      inlineCancelled.current = true;
      if (inline) restoreInlineFocus(inline.field);
      setInline(null);
    } else if (
      event.key === "Enter" &&
      (inline?.field === "title" || event.metaKey || event.ctrlKey)
    ) {
      event.preventDefault();
      event.stopPropagation();
      void commitInline(true);
    }
  }
  const clear = async () => {
    try {
      const graph = client.getSnapshot().document;
      if (!graph || !client.canAccept(graph))
        throw new Error("画布当前不可编辑");
      const next = reconcileSubjectSourceRelations(graph, {
        ...graph,
        nodes: graph.nodes.map((item) =>
          item.id === nodeID ? setSubjectConfig(item, { version: 1 }) : item,
        ),
      });
      client.change(next, true);
      await client.waitForSaved();
      setError("");
      toast.success("主体引用已移除");
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "移除尚未确认");
    }
  };
  return (
    <div
      className="canvas-subject-node"
      data-canvas-no-zoom
      data-canvas-shortcuts-ignore
      onMouseDown={(event) => {
        if (
          (event.target as Element).closest(
            "button,input,textarea,select,a,audio,video",
          )
        )
          event.stopPropagation();
      }}
      onPointerDown={(event) => {
        if (
          (event.target as Element).closest(
            "button,input,textarea,select,a,audio,video",
          )
        )
          event.stopPropagation();
      }}
    >
      <div className="canvas-subject-heading">
        {inline?.field === "title" ? (
          <Input
            aria-label="主体名称"
            autoFocus
            value={inline.value}
            disabled={inlineBusy}
            maxLength={128}
            onChange={(event) =>
              setInline({ ...inline, value: event.target.value })
            }
            onKeyDown={inlineKey}
            onBlur={() => void commitInline()}
          />
        ) : (
          <Button
            ref={nameButton}
            variant="ghost"
            size="sm"
            className="min-w-0 flex-1 justify-start"
            aria-label="编辑主体名称"
            disabled={!canEdit}
            onClick={() => beginInline("title")}
          >
            <AtSign data-icon="inline-start" />
            <span className="truncate">{node.title}</span>
            <Pencil data-icon="inline-end" />
          </Button>
        )}
        <Button
          variant="ghost"
          size="icon-sm"
          aria-label="编辑主体"
          disabled={!canEdit}
          onClick={() => setEditing({})}
        >
          <Settings2 data-icon="inline-start" />
        </Button>
      </div>
      {inline?.field === "description" ? (
        <Textarea
          aria-label="主体描述"
          placeholder="添加描述…"
          autoFocus
          value={inline.value}
          disabled={inlineBusy}
          maxLength={8192}
          className="min-h-16 resize-none"
          onChange={(event) =>
            setInline({ ...inline, value: event.target.value })
          }
          onKeyDown={inlineKey}
          onBlur={() => void commitInline()}
        />
      ) : (
        <Button
          ref={descriptionButton}
          variant="ghost"
          size="sm"
          className="w-full justify-start"
          aria-label="编辑主体描述"
          disabled={!canEdit}
          onClick={() => beginInline("description")}
        >
          <span className="truncate">{description || "添加描述…"}</span>
        </Button>
      )}
      <div className="canvas-subject-main">
        {local?.primary ? (
          <SubjectNodeMedia
            media={local.primary}
            config={config}
            projectID={projectID}
            label={node.title}
            interactive
          />
        ) : subject && !local ? (
          <UnifiedSubjectMedia
            media={boundMedia(subject, subject.primary)}
            label={subject.name}
            interactive
          />
        ) : (
          <Empty className="canvas-subject-empty">
            <EmptyHeader className="sr-only">
              <EmptyTitle>添加主体主媒体</EmptyTitle>
              <EmptyDescription>
                {subjectID && !local
                  ? (current?.error ?? "正在读取主体…")
                  : "导入主体或添加图片、视频"}
              </EmptyDescription>
            </EmptyHeader>
            {!subjectID || local ? (
              <EmptyContent className="w-full max-w-none gap-1">
                <Button
                  variant="ghost"
                  size="sm"
                  disabled={!canEdit}
                  onClick={() => setOpen(true)}
                >
                  <Contact data-icon="inline-start" />
                  导入主体
                </Button>
                <Button
                  ref={canvasButton}
                  variant="ghost"
                  size="sm"
                  disabled={!canEdit}
                  onClick={() =>
                    setEditing({
                      initialSelection: { target: "primary", canvas: true },
                    })
                  }
                >
                  <MousePointer2 data-icon="inline-start" />
                  从画布选择
                </Button>
                <Button
                  variant="ghost"
                  size="sm"
                  disabled={!canEdit}
                  onClick={() =>
                    setEditing({ initialSelection: { target: "primary" } })
                  }
                >
                  <FolderOpen data-icon="inline-start" />
                  从资产库选择
                </Button>
                <Button
                  variant="ghost"
                  size="sm"
                  disabled={!canEdit}
                  onClick={() =>
                    setEditing({
                      initialSelection: { target: "primary", upload: true },
                    })
                  }
                >
                  <Upload data-icon="inline-start" />
                  本地添加
                </Button>
              </EmptyContent>
            ) : (
              <p role={current?.error ? "alert" : "status"}>
                {current?.error ?? "正在读取主体…"}
              </p>
            )}
          </Empty>
        )}
        {(subjectID || local?.primary) && view.writable && (
          <Button
            variant="ghost"
            size="icon-sm"
            className="canvas-subject-clear"
            aria-label="移除主媒体并清除主体"
            onClick={() => void clear()}
          >
            <X />
          </Button>
        )}
      </div>
      {local && (
        <>
          <div className="canvas-subject-references" aria-label="辅助媒体">
            {local.references.map((media) => (
              <div key={media.asset_id}>
                <SubjectNodeMedia
                  media={media}
                  config={config}
                  projectID={projectID}
                  label="辅助媒体"
                />
              </div>
            ))}
          </div>
          {local.voice && (
            <SubjectNodeMedia
              media={local.voice.media}
              config={config}
              projectID={projectID}
              label={local.voice.name}
              interactive
            />
          )}
        </>
      )}
      {subject && !local && (
        <>
          <div className="canvas-subject-references" aria-label="辅助媒体">
            {subject.references.map((media) => (
              <div key={media.binding_id}>
                <UnifiedSubjectMedia
                  media={boundMedia(subject, media)}
                  label={media.file_name}
                />
              </div>
            ))}
          </div>
          {subject.voice && (
            <UnifiedSubjectMedia
              media={boundMedia(subject, subject.voice.media)}
              label={subject.voice.name}
              interactive
            />
          )}
        </>
      )}
      {current?.error && (
        <Button
          variant="outline"
          size="sm"
          onClick={() => setRetry((value) => value + 1)}
        >
          重试读取
        </Button>
      )}
      {error && <FeedbackToast message={error} />}
      {editing && (
        <SubjectNodeEditor
          actorID={actorID}
          scopeID={scopeID}
          client={client}
          node={node}
          subject={subject}
          projectID={projectID}
          initialSelection={editing.initialSelection}
          onClose={() => {
            setEditing(null);
            if (editing.initialSelection?.canvas)
              queueMicrotask(() => canvasButton.current?.focus());
          }}
        />
      )}
      {open && (
        <SubjectImport
          client={client}
          nodeID={nodeID}
          actorID={actorID}
          scopeID={scopeID}
          onClose={() => setOpen(false)}
        />
      )}
    </div>
  );
}
function SubjectImport({
  client,
  nodeID,
  actorID,
  scopeID,
  onClose,
}: {
  client: CanvasYjsLiveSession;
  nodeID: string;
  actorID: string;
  scopeID: string;
  onClose: () => void;
}) {
  const [query, setQuery] = useState("");
  const [favorite, setFavorite] = useState(false);
  const [page, setPage] = useState(1);
  const [selected, setSelected] = useState<UnifiedSubject[]>([]);
  const [listing, setListing] = useState<{
    key: string;
    page?: UnifiedSubjectPage;
    error?: string;
  } | null>(null);
  const [busy, setBusy] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const flight = useRef<AbortController | null>(null);
  const [applied, setApplied] = useState(false);
  const key = `${actorID}/${scopeID}/${query}/${favorite}/${page}`;
  useEffect(() => {
    const controller = new AbortController();
    const timer = setTimeout(() => {
      listSubjects(
        {
          q: query.trim() || undefined,
          favorite: favorite || undefined,
          page,
          page_size: 24,
          order: "updated_desc",
        },
        { signal: controller.signal },
      )
        .then((value) => requireUnifiedSubjectPage(value, actorID, scopeID))
        .then((value) => {
          if (!controller.signal.aborted) setListing({ key, page: value });
        })
        .catch(() => {
          if (!controller.signal.aborted)
            setListing({ key, error: "暂时无法读取主体库" });
        });
    }, 200);
    return () => {
      clearTimeout(timer);
      controller.abort();
    };
  }, [actorID, scopeID, query, favorite, page, key]);
  useEffect(() => () => flight.current?.abort(), []);
  const current = listing?.key === key ? listing : null;
  const close = () => {
    if (saving) return;
    flight.current?.abort();
    onClose();
  };
  async function confirm() {
    if (busy || !selected.length) return;
    if (applied) {
      setBusy(true);
      setSaving(true);
      setError("");
      try {
        await client.waitForSaved();
        onClose();
      } catch (cause) {
        setError(cause instanceof Error ? cause.message : "保存仍未确认");
      } finally {
        setBusy(false);
        setSaving(false);
      }
      return;
    }
    const controller = new AbortController();
    flight.current = controller;
    setBusy(true);
    setError("");
    const epoch = client.getDocumentEpoch();
    const session = getSessionGeneration(),
      scope = getScopeGeneration();
    const initial = client
      .getSnapshot()
      .document?.nodes.find((node) => node.id === nodeID);
    const baseline = initial ? canonical(subjectConfig(initial)) : null;
    try {
      const verified = await Promise.all(
        selected.map(async (subject) =>
          requireCanvasSubjectVersion(
            await getSubjectVersion(
              { id: subject.id, version: subject.version_id },
              { signal: controller.signal },
            ),
            scopeID,
            { subject_id: subject.id, version_id: subject.version_id },
          ),
        ),
      );
      if (controller.signal.aborted) return;
      if (!epoch || client.getDocumentEpoch() !== epoch)
        throw new Error("画布历史版本已变化，请重新确认导入");
      if (session !== getSessionGeneration() || scope !== getScopeGeneration())
        throw new Error("账号或空间已变化，请重新打开主体库");
      const graph = client.getSnapshot().document;
      const target = graph?.nodes.find((node) => node.id === nodeID);
      if (!graph || !target || canonical(subjectConfig(target)) !== baseline)
        throw new Error("主体节点已变化，请重新确认导入");
      const next = importCanvasSubjects(graph, nodeID, verified);
      if (!client.canAccept(next))
        throw new Error("画布当前不可编辑，选择已保留");
      setSaving(true);
      client.change(next, true);
      setApplied(true);
      await client.waitForSaved();
      if (!controller.signal.aborted) onClose();
    } catch (cause) {
      if (!controller.signal.aborted)
        setError(cause instanceof Error ? cause.message : "导入尚未确认");
    } finally {
      if (!controller.signal.aborted) {
        setBusy(false);
        setSaving(false);
      }
    }
  }
  return (
    <Dialog
      open
      onOpenChange={(value) => {
        if (!value) close();
      }}
    >
      <DialogContent
        aria-describedby={undefined}
        className="canvas-subject-import"
        onMouseDown={(event) => event.stopPropagation()}
        showCloseButton={!busy}
        onPointerDown={(event) => event.stopPropagation()}
        onEscapeKeyDown={(event) => {
          if (saving) event.preventDefault();
        }}
      >
        <DialogHeader>
          <DialogTitle>主体导入</DialogTitle>
        </DialogHeader>
        <div className="canvas-subject-import-tools">
          <Input
            aria-label="搜索主体"
            placeholder="搜索主体"
            value={query}
            onChange={(event) => {
              setQuery(event.target.value);
              setPage(1);
            }}
          />
          <Button
            variant="ghost"
            size="icon"
            aria-label="只看收藏主体"
            aria-pressed={favorite}
            onClick={() => {
              setFavorite((value) => !value);
              setPage(1);
            }}
          >
            <Heart fill={favorite ? "currentColor" : "none"} />
          </Button>
        </div>
        <div className="canvas-subject-import-grid">
          {current?.page?.items.map((subject) => (
            <Label key={subject.id} className="canvas-subject-import-card">
              <div>
                <UnifiedSubjectMedia
                  media={boundMedia(subject, subject.primary)}
                  label={subject.name}
                />
              </div>
              <span>{subject.name}</span>
              <Checkbox
                aria-label={`选择主体 ${subject.name}`}
                disabled={
                  busy ||
                  (!selected.some((item) => item.id === subject.id) &&
                    selected.length >= 32)
                }
                checked={selected.some((item) => item.id === subject.id)}
                onCheckedChange={(value) =>
                  setSelected((items) =>
                    value
                      ? [...items, subject]
                      : items.filter((item) => item.id !== subject.id),
                  )
                }
              />
            </Label>
          ))}
        </div>
        {!current && <p role="status">正在读取主体…</p>}
        {current?.error && <FeedbackToast message={current.error} />}
        {current?.page?.total === 0 && (
          <Empty>
            <EmptyHeader>
              <EmptyTitle>暂无主体</EmptyTitle>
              <EmptyDescription>
                可调整搜索条件，或先在主体库添加主体。
              </EmptyDescription>
            </EmptyHeader>
          </Empty>
        )}
        {error && <FeedbackToast message={error} />}
        <DialogFooter>
          <span className="mr-auto text-sm text-muted-foreground">
            已选择 {selected.length} 个主体
          </span>
          <Button
            variant="ghost"
            disabled={busy || page <= 1}
            onClick={() => setPage((value) => value - 1)}
          >
            上一页
          </Button>
          <Button
            variant="ghost"
            disabled={busy || !current?.page || page * 24 >= current.page.total}
            onClick={() => setPage((value) => value + 1)}
          >
            下一页
          </Button>
          <Button variant="outline" disabled={saving} onClick={close}>
            取消
          </Button>
          <Button
            disabled={busy || !selected.length}
            onClick={() => void confirm()}
          >
            {busy ? (
              <>
                <LoaderCircle className="animate-spin" />
                {saving ? "正在保存…" : "正在导入…"}
              </>
            ) : applied ? (
              "检查保存状态"
            ) : (
              "导入"
            )}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
