"use client";
import { useState, useSyncExternalStore } from "react";
import dynamic from "next/dynamic";
import { Camera, Palette, Clapperboard, MoveRight, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { canonical } from "@/components/generation/draft-save";
import type { CanvasYjsLiveSession } from "../yjs-live-session";
import {
  cinemaKinds,
  cinemaLabels,
  cinemaSummary,
  cinemaSupports,
  kitKind,
  readCinemaKit,
  type CinemaKind,
  type CinemaKit,
} from "./cinema-kit-model";
import { effectiveCinema, updateCinemaSetting } from "./cinema-kit-graph";
import { CinemaPreview } from "./cinema-preview";
import "./cinema-kit.css";
export const CinemaKitPicker = dynamic(
  () => import("./cinema-kit-picker").then((m) => m.CinemaKitPicker),
  { loading: () => <p role="status">正在打开导演工具箱…</p> },
);
const icons = {
  camera: Camera,
  palette: Palette,
  style: Clapperboard,
  motion: MoveRight,
};
type Editing = {
  kind: CinemaKind;
  initial?: CinemaKit;
  baseline: string;
  epoch: ReturnType<CanvasYjsLiveSession["getDocumentEpoch"]>;
};
function identity(client: CanvasYjsLiveSession, nodeID: string) {
  const graph = client.getSnapshot().document,
    node = graph?.nodes.find((n) => n.id === nodeID);
  if (!graph || !node) throw new Error("设置节点已不存在");
  return canonical({
    config: node.serviceData?.config?.cinema_kit,
    edges: graph.connections.filter((e) => e.toNodeId === nodeID),
    sources: graph.nodes
      .filter(
        (n) =>
          graph.connections.some(
            (e) => e.toNodeId === nodeID && e.fromNodeId === n.id,
          ) && n.type === "cinema_studio",
      )
      .map((n) => ({ id: n.id, config: n.serviceData?.config?.cinema_kit })),
  });
}
async function save(
  client: CanvasYjsLiveSession,
  nodeID: string,
  editing: Editing,
  value?: CinemaKit,
) {
  const graph = client.getSnapshot().document;
  if (
    !graph ||
    client.getDocumentEpoch() !== editing.epoch ||
    identity(client, nodeID) !== editing.baseline
  )
    throw new Error("设置已变化，请重新打开后编辑");
  const node = graph.nodes.find((n) => n.id === nodeID);
  const next =
    node?.type === "cinema_studio"
      ? {
          ...graph,
          nodes: graph.nodes.map((n) =>
            n.id === nodeID
              ? {
                  ...n,
                  serviceData: {
                    ...n.serviceData,
                    id: n.id,
                    config: {
                      ...n.serviceData?.config,
                      cinema_kit: readCinemaKit(value),
                    },
                  },
                }
              : n,
          ),
        }
      : updateCinemaSetting(graph, nodeID, editing.kind, value);
  if (!client.canAccept(next)) throw new Error("画布当前不可编辑");
  client.change(next, true);
  const expected = identity(client, nodeID);
  await client.waitForSaved(() => {
    if (
      client.getDocumentEpoch() !== editing.epoch ||
      identity(client, nodeID) !== expected
    )
      throw new Error("设置再次变化，请核对保存结果");
  });
}
export function CanvasCinemaKitNode({
  client,
  nodeID,
}: {
  client: CanvasYjsLiveSession;
  nodeID: string;
}) {
  const view = useSyncExternalStore(
      client.subscribe,
      client.getSnapshot,
      client.getSnapshot,
    ),
    [editing, setEditing] = useState<Editing | null>(null);
  const node = view.document?.nodes.find((n) => n.id === nodeID);
  if (!node) return null;
  let kit: CinemaKit;
  try {
    kit = readCinemaKit(node.serviceData?.config?.cinema_kit);
  } catch {
    return <p role="alert">导演工具箱设置无法读取</p>;
  }
  const kind = kitKind(kit),
    Icon = icons[kind];
  return (
    <div
      className="cinema-kit-node"
      data-canvas-no-zoom
      data-canvas-shortcuts-ignore
      onPointerDown={(e) => e.stopPropagation()}
      onMouseDown={(e) => e.stopPropagation()}
    >
      <Icon className="size-8 self-center" />
      {kind === "camera" ? (
        <dl>
          {[
            ["相机", kit.camera!.body],
            ["镜头", kit.camera!.lens],
            ["焦距", kit.camera!.focal_length],
            ["光圈", kit.camera!.aperture],
          ].map(([label, value]) => (
            <div className="contents" key={label}>
              <dt>{label}</dt>
              <dd>{value}</dd>
            </div>
          ))}
        </dl>
      ) : kind === "palette" ? (
        <>
          <span className="cinema-swatches">
            {kit.palette!.colors.map((c, i) => (
              <i key={i} style={{ backgroundColor: c }} />
            ))}
          </span>
          <p>{kit.palette!.name}</p>
        </>
      ) : (
        <>
          <CinemaPreview />
          <p>{cinemaSummary(kit, kind)}</p>
        </>
      )}
      <Button
        variant="secondary"
        disabled={!view.writable}
        onClick={() =>
          setEditing({
            kind,
            initial: kit,
            baseline: identity(client, nodeID),
            epoch: client.getDocumentEpoch(),
          })
        }
      >
        编辑{cinemaLabels[kind]}设置
      </Button>
      {editing ? (
        <CinemaKitPicker
          kind={editing.kind}
          initial={editing.initial}
          onConfirm={(value) => save(client, nodeID, editing, value)}
          onClose={() => setEditing(null)}
        />
      ) : null}
    </div>
  );
}
export function CanvasCinemaKitBar({
  client,
  nodeID,
}: {
  client: CanvasYjsLiveSession;
  nodeID: string;
}) {
  const view = useSyncExternalStore(
      client.subscribe,
      client.getSnapshot,
      client.getSnapshot,
    ),
    [editing, setEditing] = useState<Editing | null>(null),
    [error, setError] = useState("");
  const node = view.document?.nodes.find((n) => n.id === nodeID);
  if (!node || !["image", "video"].includes(node.type) || !view.document)
    return null;
  let kit: CinemaKit | undefined;
  try {
    kit = effectiveCinema(view.document, nodeID);
  } catch {
    return <p role="alert">工具箱引用存在冲突，请移除重复连接</p>;
  }
  const edit = (kind: CinemaKind): Editing => ({
    kind,
    initial:
      kit?.[kind] !== undefined ? { version: 1, [kind]: kit[kind] } : undefined,
    baseline: identity(client, nodeID),
    epoch: client.getDocumentEpoch(),
  });
  return (
    <div
      className="cinema-kit-bar"
      data-canvas-no-zoom
      data-canvas-shortcuts-ignore
      onPointerDown={(e) => e.stopPropagation()}
      onMouseDown={(e) => e.stopPropagation()}
    >
      {cinemaKinds
        .filter((kind) => cinemaSupports(kind, node.type))
        .map((kind) => {
          const Icon = icons[kind],
            active = kit?.[kind] !== undefined;
          return (
            <span className={active ? "cinema-kit-chip" : ""} key={kind}>
              <Button
                variant="ghost"
                size="sm"
                disabled={!view.writable}
                onClick={() => setEditing(edit(kind))}
              >
                <Icon />
                {active ? cinemaSummary(kit!, kind) : cinemaLabels[kind]}
              </Button>
              {active ? (
                <Button
                  variant="ghost"
                  size="icon-sm"
                  disabled={!view.writable}
                  aria-label={`清除${cinemaLabels[kind]}`}
                  onClick={async () => {
                    try {
                      await save(client, nodeID, edit(kind));
                      setError("");
                    } catch (e) {
                      setError((e as Error).message);
                    }
                  }}
                >
                  <X />
                </Button>
              ) : null}
            </span>
          );
        })}
      {error ? <p role="alert">{error}</p> : null}
      {editing ? (
        <CinemaKitPicker
          kind={editing.kind}
          initial={editing.initial}
          onConfirm={(value) => save(client, nodeID, editing, value)}
          onClose={() => setEditing(null)}
        />
      ) : null}
    </div>
  );
}
