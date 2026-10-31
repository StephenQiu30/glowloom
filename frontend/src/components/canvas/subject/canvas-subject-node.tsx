"use client";
import { useEffect, useRef, useState, useSyncExternalStore } from "react";
import { Contact, Heart, LoaderCircle, X } from "lucide-react";
import { getSubjectVersion, listSubjects } from "@/api/subjects";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import {
  SubjectNodeEditor,
  type SubjectMediaTarget,
} from "./subject-node-editor";
import { SubjectNodeMedia } from "./subject-node-media";
import { Input } from "@/components/ui/input";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
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
    initialSelection?: { target: SubjectMediaTarget; upload?: boolean };
  } | null>(null);
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
  const canEdit = view.writable && (!subjectID || !!subject || !!local);
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
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "移除尚未确认");
    }
  };
  return (
    <div
      className="canvas-subject-node"
      data-canvas-no-zoom
      data-canvas-shortcuts-ignore
      onMouseDown={(event) => event.stopPropagation()}
      onPointerDown={(event) => event.stopPropagation()}
    >
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
          <div className="canvas-subject-empty">
            <Contact />
            <p>
              {subjectID && !local
                ? (current?.error ?? "正在读取主体…")
                : "导入主体，保持角色与声音一致"}
            </p>
            {view.writable && (!subjectID || local) && (
              <div className="flex flex-wrap justify-center gap-2">
                <Button
                  size="sm"
                  onClick={() =>
                    setEditing({
                      initialSelection: { target: "primary", upload: true },
                    })
                  }
                >
                  本地添加
                </Button>
                <Button
                  size="sm"
                  variant="outline"
                  onClick={() =>
                    setEditing({ initialSelection: { target: "primary" } })
                  }
                >
                  从资产库选择
                </Button>
              </div>
            )}
          </div>
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
          <p className="canvas-subject-description">
            {local.description || "暂无描述"}
          </p>
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
          <p className="canvas-subject-description">
            {subject.description || "暂无描述"}
          </p>
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
      <Button
        variant="outline"
        size="sm"
        disabled={!canEdit}
        onClick={() => setEditing({})}
      >
        编辑主体
      </Button>
      <Button
        variant="secondary"
        size="sm"
        disabled={!view.writable}
        onClick={() => setOpen(true)}
      >
        主体导入
      </Button>
      {error && <p role="alert">{error}</p>}
      {editing && (
        <SubjectNodeEditor
          actorID={actorID}
          scopeID={scopeID}
          client={client}
          node={node}
          subject={subject}
          projectID={projectID}
          initialSelection={editing.initialSelection}
          onClose={() => setEditing(null)}
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
          <DialogDescription>选择要加入画布的主体</DialogDescription>
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
        {current?.error && <p role="alert">{current.error}</p>}
        {current?.page?.total === 0 && <p>暂无主体</p>}
        {error && <p role="alert">{error}</p>}
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
