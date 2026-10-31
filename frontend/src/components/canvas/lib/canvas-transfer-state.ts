import {
  getScopeGeneration,
  getSessionGeneration,
  subscribeContextChange,
} from "@/lib/request";
import {
  instantiateCanvasClipboard,
  type CanvasClipboard,
} from "./canvas-clipboard";
import { canvasGraphFingerprint } from "./canvas-json";

export type CanvasTransferOwner = {
  actorID: string;
  sessionID: string;
  scopeID: string;
  projectID: string;
};
export type CanvasTransfer = {
  id: string;
  owner: string;
  sourceID: string;
  targetID: string;
  sourceNodeIDs: string[];
  copy: CanvasClipboard;
  applied: boolean;
};
let pending: CanvasTransfer | null = null;
const listeners = new Set<() => void>();
const emit = () => listeners.forEach((listener) => listener());
subscribeContextChange(() => {
  pending = null;
  emit();
});
export const subscribeCanvasTransfer = (listener: () => void) => {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
};
export const canvasTransferOwnerKey = (owner: CanvasTransferOwner) =>
  JSON.stringify([
    owner.sessionID,
    owner.actorID,
    owner.scopeID,
    owner.projectID,
    getSessionGeneration(),
    getScopeGeneration(),
  ]);
export function pendingCanvasTransfer(
  owner: CanvasTransferOwner,
  targetID?: string,
) {
  return pending?.owner === canvasTransferOwnerKey(owner) &&
    (!targetID || pending.targetID === targetID)
    ? pending
    : null;
}
export function clearCanvasTransfer(intent: CanvasTransfer) {
  if (pending?.id !== intent.id || pending.owner !== intent.owner) return;
  pending = null;
  emit();
}
export function markCanvasTransferApplied(intent: CanvasTransfer) {
  if (pending?.id !== intent.id || pending.owner !== intent.owner)
    throw new Error("保存意图已变化");
  if (pending.applied) return false;
  pending = { ...pending, applied: true };
  emit();
  return true;
}
export function beginCanvasTransfer(
  owner: CanvasTransferOwner,
  sourceID: string,
  target: API.DocumentDTO,
  selection: CanvasClipboard,
) {
  if (
    !owner.sessionID ||
    !owner.actorID ||
    !owner.scopeID ||
    !sourceID ||
    !target.id ||
    sourceID === target.id ||
    target.project_id !== owner.projectID ||
    !target.permissions?.write ||
    !selection.nodes.length
  )
    throw new Error("请选择当前项目内可编辑的其他画布");
  if (pending?.owner === canvasTransferOwnerKey(owner))
    throw new Error("请先处理尚未确认的跨画布保存");
  const ids = new Set(selection.nodes.map((node) => node.id));
  for (const node of selection.nodes) {
    const external = [
      ...(node.serviceData?.config?.timeline?.clips ?? []).map(
        (clip) => clip.node_id,
      ),
      ...(node.serviceData?.config?.director?.objects ?? []).map(
        (object) => object.source_node_id,
      ),
    ].filter((id) => id && !ids.has(id));
    if (external.length)
      throw new Error("请同时选择时间线或导演台引用的来源节点");
  }
  const copy = instantiateCanvasClipboard(selection, { x: 36, y: 36 });
  pending = {
    id: crypto.randomUUID(),
    owner: canvasTransferOwnerKey(owner),
    sourceID,
    targetID: target.id,
    sourceNodeIDs: [...ids],
    copy,
    applied: false,
  };
  emit();
  return pending;
}

export function requireTransferSource(
  intent: CanvasTransfer,
  owner: CanvasTransferOwner,
  source: API.DocumentDTO,
) {
  if (
    pendingCanvasTransfer(owner)?.id !== intent.id ||
    source.id !== intent.sourceID ||
    source.project_id !== owner.projectID ||
    !source.permissions?.read ||
    intent.sourceNodeIDs.some(
      (id) => !source.nodes?.some((node) => node.id === id),
    )
  )
    throw new Error("来源画布、选区或当前身份已变化，请重新选择");
}

export function requireTransferReceipt(
  intent: CanvasTransfer,
  owner: CanvasTransferOwner,
  target: API.DocumentDTO,
) {
  if (
    pendingCanvasTransfer(owner)?.id !== intent.id ||
    target.id !== intent.targetID ||
    target.project_id !== owner.projectID ||
    !target.permissions?.read ||
    intent.copy.nodes.some(
      (node) => !target.nodes?.some((saved) => saved.id === node.id),
    ) ||
    intent.copy.connections.some(
      (edge) => !target.edges?.some((saved) => saved.id === edge.id),
    )
  )
    throw new Error("目标尚未完整读回，保留原保存操作继续核验");
  for (const node of intent.copy.nodes) {
    const saved = target.nodes!.find((item) => item.id === node.id)!;
    if (saved.node_type !== node.type || saved.title !== node.title)
      throw new Error("目标内容已变化，请核对当前副本");
    for (const key of ["subject", "draft"] as const) {
      const expected = node.serviceData?.config?.[key];
      if (
        expected &&
        (canvasGraphFingerprint(saved.config?.[key]) !==
          canvasGraphFingerprint(expected) ||
          !saved.input_revision ||
          saved.input_revision < 1)
      )
        throw new Error("目标输入内容或固定版本尚未完整读回");
    }
  }
}
