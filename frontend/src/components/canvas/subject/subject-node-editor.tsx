"use client";
import { useContext, useEffect, useRef, useState } from "react";
import { CanvasMediaSelectionContext } from "../lib/canvas-media-selection";
import dynamic from "next/dynamic";
import { getMediaPreview } from "@/api/media";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Label } from "@/components/ui/label";
import {
  Dialog,
  DialogContent,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "@/components/ui/dialog";
import { canonical } from "@/components/generation/draft-save";
import { getScopeGeneration, getSessionGeneration } from "@/lib/request";
import type { UnifiedSubject } from "@/components/subjects/unified-contracts";
import type { CanvasYjsLiveSession } from "../yjs-live-session";
import type { CanvasNodeData } from "../types/canvas";
import {
  localSubjectConfig,
  materializeSubject,
  projectSubjectMedia,
  requireSubjectLocal,
  setSubjectConfig,
  subjectConfig,
  type CanvasSubjectLocal,
} from "./subject-node-state";
import { reconcileSubjectSourceRelations } from "../lib/canvas-subject-connections";
import { SubjectNodeVoice } from "./subject-node-voice";
import { SubjectNodeMedia } from "./subject-node-media";
const ProjectAssetDialog = dynamic(
  () =>
    import("@/components/media/project-asset-dialog").then(
      (module) => module.ProjectAssetDialog,
    ),
  { loading: () => <p role="status">正在读取资产…</p> },
);
export type SubjectMediaTarget = "primary" | "reference" | "voice";
type Selection = {
  target: SubjectMediaTarget;
  upload?: boolean;
  canvas?: boolean;
};
export function SubjectNodeEditor({
  client,
  node,
  subject,
  projectID,
  actorID,
  scopeID,
  initialSelection,
  onClose,
}: {
  client: CanvasYjsLiveSession;
  node: CanvasNodeData;
  subject?: UnifiedSubject;
  projectID: string;
  actorID: string;
  scopeID: string;
  initialSelection?: Selection;
  onClose: () => void;
}) {
  const selectCanvasMedia = useContext(CanvasMediaSelectionContext);
  const canvasFlight = useRef<AbortController | null>(null);
  useEffect(() => () => canvasFlight.current?.abort(), []);
  const [baseline] = useState(() => ({
    config: subjectConfig(node),
    title: node.title,
    epoch: client.getDocumentEpoch(),
    session: getSessionGeneration(),
    scope: getScopeGeneration(),
  }));
  const [local, setLocal] = useState(() =>
    materializeSubject(baseline.config, subject),
  );
  const [title, setTitle] = useState(node.title);
  const [selection, setSelection] = useState<Selection | null>(
    initialSelection ?? null,
  );
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [applied, setApplied] = useState(false);
  const [cleared, setCleared] = useState(false);
  const lock = useRef(false);
  const currentConfig = { ...baseline.config, local };
  function active() {
    if (
      client.getDocumentEpoch() !== baseline.epoch ||
      getSessionGeneration() !== baseline.session ||
      getScopeGeneration() !== baseline.scope
    )
      throw new Error("画布、账号或空间已变化，请重新打开主体编辑");
  }
  function change(next: CanvasSubjectLocal) {
    requireSubjectLocal(next, !!baseline.config.subject_id);
    setLocal(next);
    setCleared(false);
    setError("");
  }
  function add(assets: API.AssetSummary[], chosen = selection) {
    active();
    if (!chosen) throw new Error("素材选择已取消");
    const media = assets.map((asset) => projectSubjectMedia(asset, projectID));
    if (!media.length) throw new Error("请选择素材");
    let next: CanvasSubjectLocal;
    if (chosen.target === "primary") next = { ...local, primary: media[0] };
    else if (chosen.target === "reference")
      next = { ...local, references: [...local.references, ...media] };
    else
      next = {
        ...local,
        voice: { name: assets[0].file_name || "音色", media: media[0] },
      };
    change(next);
    setSelection(null);
  }
  async function save() {
    if (lock.current) return;
    lock.current = true;
    setBusy(true);
    setError("");
    try {
      active();
      if (!applied) {
        if (!title.trim() || Array.from(title.trim()).length > 128)
          throw new Error("主体名称应为1至128个字符");
        const graph = client.getSnapshot().document;
        const target = graph?.nodes.find((item) => item.id === node.id);
        if (
          !graph ||
          !target ||
          canonical(subjectConfig(target)) !== canonical(baseline.config) ||
          target.title !== baseline.title
        )
          throw new Error("主体已被其他编辑更新，请保留当前内容并重新打开");
        const config = cleared
          ? { version: 1 as const }
          : localSubjectConfig(baseline.config, local);
        const next = reconcileSubjectSourceRelations(graph, {
          ...graph,
          nodes: graph.nodes.map((item) =>
            item.id === node.id
              ? setSubjectConfig({ ...item, title: title.trim() }, config)
              : item,
          ),
        });
        if (!client.canAccept(next))
          throw new Error("画布当前不可编辑，修改已保留");
        client.change(next, true);
        setApplied(true);
      }
      await client.waitForSaved();
      active();
      onClose();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "保存尚未确认");
    } finally {
      lock.current = false;
      setBusy(false);
    }
  }
  const kinds =
    selection?.target === "voice"
      ? ["audio"]
      : selection?.target === "reference"
        ? ["image"]
        : ["image", "video"];
  async function choose(
    target: SubjectMediaTarget,
    upload = false,
    canvas = false,
  ) {
    const choice = { target, upload, canvas };
    if (!canvas) {
      setSelection(choice);
      return;
    }
    if (!selectCanvasMedia) {
      setError("画布选择暂不可用");
      return;
    }
    canvasFlight.current?.abort();
    const controller = new AbortController();
    canvasFlight.current = controller;
    setSelection(choice);
    setError("");
    try {
      active();
      const allowed =
        target === "voice"
          ? ["audio"]
          : target === "reference"
            ? ["image"]
            : ["image", "video"];
      const selected = await selectCanvasMedia(
        node.id,
        allowed,
        controller.signal,
      );
      if (!selected || controller.signal.aborted) return;
      active();
      const assetID = selected.serviceData?.media_ref?.asset_id;
      if (!assetID) throw new Error("所选节点没有已保存素材");
      const preview = await getMediaPreview(
        { pid: projectID, asset_id: assetID },
        { signal: controller.signal },
      );
      if (controller.signal.aborted) return;
      active();
      const current = client
        .getSnapshot()
        .document?.nodes.find((item) => item.id === selected.id);
      if (
        !current ||
        current.serviceData?.media_ref?.asset_id !== assetID ||
        !preview.asset ||
        preview.asset.kind !== selected.type
      )
        throw new Error("所选节点素材已变化，请重新选择");
      add([preview.asset], choice);
    } catch (cause) {
      if (!controller.signal.aborted)
        setError(cause instanceof Error ? cause.message : "素材无法读取");
    } finally {
      if (canvasFlight.current === controller) {
        canvasFlight.current = null;
        setSelection(null);
      }
    }
  }
  const liveEpoch = client.getDocumentEpoch();
  useEffect(() => {
    if (liveEpoch !== baseline.epoch) canvasFlight.current?.abort();
  }, [liveEpoch, baseline.epoch]);
  return (
    <div
      onMouseDown={(event) => event.stopPropagation()}
      onPointerDown={(event) => event.stopPropagation()}
    >
      <Dialog
        open={!selection?.canvas}
        onOpenChange={(open) => {
          if (!open && !busy) onClose();
        }}
      >
        <DialogContent
          className="canvas-subject-editor sm:max-w-2xl"
          onMouseDown={(event) => event.stopPropagation()}
          onPointerDown={(event) => event.stopPropagation()}
          onEscapeKeyDown={(event) => {
            if (busy || selection) event.preventDefault();
          }}
        >
          <DialogTitle>编辑画布主体</DialogTitle>
          <DialogDescription>
            修改只保存在此画布，不会更改主体库。
          </DialogDescription>
          <div className="grid gap-2">
            <Label htmlFor={`${node.id}-name`}>主体名称</Label>
            <Input
              id={`${node.id}-name`}
              value={title}
              onChange={(event) => setTitle(event.target.value)}
              disabled={busy || applied}
              maxLength={128}
            />
          </div>
          <div className="grid gap-2">
            <Label htmlFor={`${node.id}-description`}>主体说明</Label>
            <Textarea
              id={`${node.id}-description`}
              value={local.description}
              onChange={(event) =>
                setLocal({ ...local, description: event.target.value })
              }
              disabled={busy || applied}
              maxLength={8192}
            />
          </div>
          <section aria-label="主媒体" className="grid gap-2">
            <h3>主媒体</h3>
            {local.primary && (
              <div className="h-44 overflow-hidden rounded-md bg-muted">
                <SubjectNodeMedia
                  media={local.primary}
                  config={currentConfig}
                  projectID={projectID}
                  label={title}
                  interactive
                />
              </div>
            )}
            <div className="flex flex-wrap gap-2">
              <Button
                variant="outline"
                disabled={busy || applied}
                onClick={() => choose("primary", true)}
              >
                本地添加
              </Button>
              <Button
                variant="outline"
                disabled={busy || applied}
                onClick={() => choose("primary")}
              >
                从资产库选择
              </Button>
              <Button
                variant="outline"
                disabled={busy || applied}
                onClick={() => choose("primary", false, true)}
              >
                从画布添加
              </Button>
              {local.primary && (
                <Button
                  variant="ghost"
                  disabled={busy || applied}
                  onClick={() => {
                    setLocal({ description: "", references: [] });
                    setCleared(true);
                  }}
                >
                  移除主媒体并清除主体
                </Button>
              )}
            </div>
          </section>
          <section aria-label="辅助媒体" className="grid gap-2">
            <h3>辅助媒体 · {local.references.length}</h3>
            <div className="flex flex-wrap gap-2">
              {local.references.map((media) => (
                <div className="w-24" key={media.asset_id}>
                  <div className="h-20">
                    <SubjectNodeMedia
                      media={media}
                      config={currentConfig}
                      projectID={projectID}
                      label="辅助媒体"
                    />
                  </div>
                  <Button
                    variant="ghost"
                    size="sm"
                    aria-label={`移除辅助媒体 ${media.asset_id}`}
                    disabled={busy || applied}
                    onClick={() =>
                      change({
                        ...local,
                        references: local.references.filter(
                          (item) => item.asset_id !== media.asset_id,
                        ),
                      })
                    }
                  >
                    移除
                  </Button>
                </div>
              ))}
            </div>
            <div className="flex flex-wrap gap-2">
              <Button
                variant="outline"
                disabled={
                  !local.primary ||
                  local.references.length >= 120 ||
                  busy ||
                  applied
                }
                onClick={() => choose("reference", true)}
              >
                本地添加辅助媒体
              </Button>
              <Button
                variant="outline"
                disabled={
                  !local.primary ||
                  local.references.length >= 120 ||
                  busy ||
                  applied
                }
                onClick={() => choose("reference")}
              >
                资产库辅助媒体
              </Button>
              <Button
                variant="outline"
                disabled={
                  !local.primary ||
                  local.references.length >= 120 ||
                  busy ||
                  applied
                }
                onClick={() => choose("reference", false, true)}
              >
                画布辅助媒体
              </Button>
            </div>
          </section>
          <section aria-label="音色" className="grid gap-2">
            <h3>音色</h3>
            <SubjectNodeVoice
              actorID={actorID}
              scopeID={scopeID}
              projectID={projectID}
              disabled={
                !local.primary ||
                busy ||
                applied ||
                liveEpoch !== baseline.epoch ||
                !client.getSnapshot().writable
              }
              onReady={(name, media) => {
                active();
                change({ ...local, voice: { name, media } });
              }}
            />
            {local.voice && (
              <>
                <Label htmlFor={`${node.id}-voice`}>音色名称</Label>
                <Input
                  id={`${node.id}-voice`}
                  value={local.voice.name}
                  maxLength={512}
                  disabled={busy || applied}
                  onChange={(event) =>
                    setLocal({
                      ...local,
                      voice: { ...local.voice!, name: event.target.value },
                    })
                  }
                />
                <SubjectNodeMedia
                  media={local.voice.media}
                  config={currentConfig}
                  projectID={projectID}
                  label={local.voice.name}
                  interactive
                />
              </>
            )}
            <div className="flex flex-wrap gap-2">
              <Button
                variant="outline"
                disabled={!local.primary || busy || applied}
                onClick={() => choose("voice", true)}
              >
                {local.voice ? "本地替换音色" : "本地添加音色"}
              </Button>
              <Button
                variant="outline"
                disabled={!local.primary || busy || applied}
                onClick={() => choose("voice")}
              >
                资产库音色
              </Button>
              <Button
                variant="outline"
                disabled={!local.primary || busy || applied}
                onClick={() => choose("voice", false, true)}
              >
                画布音色
              </Button>
              {local.voice && (
                <Button
                  variant="ghost"
                  disabled={busy || applied}
                  onClick={() => {
                    const next = { ...local };
                    delete next.voice;
                    change(next);
                  }}
                >
                  删除音色
                </Button>
              )}
            </div>
          </section>
          {error && <p role="alert">{error}</p>}
          <DialogFooter>
            <Button variant="outline" disabled={busy} onClick={onClose}>
              取消
            </Button>
            <Button disabled={busy} onClick={() => void save()}>
              {busy ? "正在保存…" : applied ? "检查保存状态" : "保存到画布"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      {selection && !selection.canvas && (
        <ProjectAssetDialog
          projectID={projectID}
          initialKind={kinds[0]}
          allowedKinds={kinds}
          selectionLimit={
            selection.target === "reference"
              ? Math.max(1, 120 - local.references.length)
              : 1
          }
          initialUpload={selection.upload}
          canvasPresentation
          confirmLabel="添加到主体"
          onClose={() => setSelection(null)}
          onAdd={add}
        />
      )}
    </div>
  );
}
