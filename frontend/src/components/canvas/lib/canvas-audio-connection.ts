import { getSubjectVersion } from "@/api/subjects";
import { getSessionGeneration, getScopeGeneration } from "@/lib/request";
import type { PromptDraft } from "@/components/generation/prompt-document";
import type { UnifiedSubject } from "@/components/subjects/unified-contracts";
import type { CanvasYjsLiveSession } from "../yjs-live-session";
import type { CanvasNodeData } from "../types/canvas";
import { subjectConfig } from "../subject/subject-node-state";
import { canvasGraphFingerprint } from "./canvas-json";
import { requireCanvasSubjectVersion } from "./canvas-subject-source";
import { connectCanvasDocumentSources } from "./canvas-document-connections";
import { readCanvasAudioModelCatalog } from "./canvas-audio-model-catalog";
import { canvasAudioSourceInputs } from "./canvas-audio-connection-preview";
import {
  assertCanvasAudioConnectionCapability,
  createCanvasAudioConnectionDraft,
  canvasAudioComposerForPersistence,
  type CanvasAudioCatalogIdentity,
  type CanvasAudioModelCatalog,
} from "./canvas-audio-connection-capability";

export type CanvasAudioConnectionRequest = {
  sourceNodeIds: string[];
  targetNodeId?: string;
  newTarget?: CanvasNodeData;
  newSources?: CanvasNodeData[];
};
type CatalogReader = (
  identity: CanvasAudioCatalogIdentity,
  signal?: AbortSignal,
) => Promise<CanvasAudioModelCatalog>;
const identityOf = (node: CanvasNodeData) =>
  canvasGraphFingerprint({
    type: node.type,
    metadata: node.metadata,
    parameters: node.documentData?.parameters,
    files: node.documentData?.fileReferences,
    config: node.serviceData?.config,
    ref: node.serviceData?.ref_id,
    media: node.serviceData?.media_ref,
  });

/** Model release and source pins are resolved before one complete, undoable graph edit. */
export function createCanvasAudioConnector(
  client: CanvasYjsLiveSession,
  projectID: string,
  scopeID: string,
  readCatalog: CatalogReader = readCanvasAudioModelCatalog,
) {
  const identity = { projectID, scopeID };
  let disposed = false;
  let busy = false;
  let confirmationUnknown = false;
  const requests = new Set<AbortController>();
  return {
    start() {
      disposed = false;
    },
    dispose() {
      disposed = true;
      for (const request of requests) request.abort();
      requests.clear();
    },
    async connect(input: CanvasAudioConnectionRequest): Promise<string> {
      const request = structuredClone(input);
      if (confirmationUnknown)
        throw new Error("上一次连接保存结果尚未确认，请检查画布后重新打开");
      if (busy || disposed) throw new Error("正在处理连接，请稍后重试");
      const sessionGeneration = getSessionGeneration();
      const scopeGeneration = getScopeGeneration();
      const contextCurrent = () =>
        getSessionGeneration() === sessionGeneration &&
        getScopeGeneration() === scopeGeneration;
      const view = client.getSnapshot();
      const before = view.document;
      const epoch = client.getDocumentEpoch();
      if (
        !before ||
        !epoch ||
        !view.writable ||
        !client.canAcceptConnectionGraph(before)
      )
        throw new Error("画布暂时无法接收修改，请稍后重试");
      if (
        !!request.newTarget === !!request.targetNodeId ||
        (request.newTarget && request.newSources?.length)
      )
        throw new Error("连接目标无效");
      if (
        !request.sourceNodeIds.length ||
        new Set(request.sourceNodeIds).size !== request.sourceNodeIds.length
      )
        throw new Error("请选择不同的来源节点");
      const newSources = request.newSources ?? [];
      const freshIDs = new Set(newSources.map((node) => node.id));
      if (
        freshIDs.size !== newSources.length ||
        newSources.some(
          (node) =>
            before.nodes.some((item) => item.id === node.id) ||
            !request.sourceNodeIds.includes(node.id),
        )
      )
        throw new Error("新来源节点身份冲突");
      const target =
        request.newTarget ??
        before.nodes.find((node) => node.id === request.targetNodeId);
      if (
        !target ||
        target.type !== "audio" ||
        (request.newTarget &&
          before.nodes.some((node) => node.id === target.id))
      )
        throw new Error("音频目标已变化");
      if (
        view.inputDrafts?.[target.id] ||
        (!request.newTarget && !client.canAcceptAudioConnectionInput(target.id))
      )
        throw new Error("请先保存或恢复音频节点的生成草稿");
      const candidates = [...before.nodes, ...newSources];
      const sources = request.sourceNodeIds.map((id) => {
        const node = candidates.find((item) => item.id === id);
        if (!node) throw new Error("来源节点已不存在");
        return node;
      });
      const sourceIdentities = new Map(
        sources.map((node) => [node.id, identityOf(node)]),
      );
      const targetIdentity = identityOf(target);
      let published = false;
      const abort = new AbortController();
      requests.add(abort);
      busy = true;
      try {
        const [catalog, resolved] = await Promise.all([
          readCatalog(identity, abort.signal),
          Promise.all(
            sources
              .filter((source) => source.type === "subject")
              .map(async (source) => {
                const config = subjectConfig(source);
                if (config.local) return [source.id, undefined] as const;
                if (!config.subject_id || !config.version_id)
                  throw new Error("主体素材尚未就绪");
                const value = await getSubjectVersion(
                  { id: config.subject_id, version: config.version_id },
                  { signal: abort.signal },
                );
                return [
                  source.id,
                  requireCanvasSubjectVersion(value, scopeID, {
                    subject_id: config.subject_id,
                    version_id: config.version_id,
                  }),
                ] as const;
              }),
          ),
        ]);
        abort.signal.throwIfAborted();
        const pinned = new Map<string, UnifiedSubject>(
          resolved.flatMap(([id, value]) => (value ? [[id, value]] : [])),
        );
        const currentView = client.getSnapshot();
        const current = currentView.document;
        if (
          disposed ||
          !contextCurrent() ||
          !current ||
          !currentView.writable ||
          client.getDocumentEpoch() !== epoch ||
          currentView.inputDrafts?.[target.id] ||
          (!request.newTarget &&
            !client.canAcceptAudioConnectionInput(target.id))
        )
          throw new Error("画布权限、版本或生成输入已变化，请重新连接");
        for (const source of sources) {
          const node = current.nodes.find((item) => item.id === source.id);
          if (
            freshIDs.has(source.id)
              ? !!node
              : !node || identityOf(node) !== sourceIdentities.get(source.id)
          )
            throw new Error("来源素材已变化，请重新连接");
        }
        const currentTarget = current.nodes.find(
          (node) => node.id === target.id,
        );
        if (
          request.newTarget
            ? !!currentTarget
            : !currentTarget || identityOf(currentTarget) !== targetIdentity
        )
          throw new Error("连接目标已变化，请重新连接");
        const stored = target.serviceData?.config?.draft?.composer as
          PromptDraft | undefined;
        // Existing targets never inherit a different default. Preselected new drafts keep their exact release too.
        const draft =
          stored ??
          (request.newTarget
            ? createCanvasAudioConnectionDraft(catalog, identity).draft
            : undefined);
        assertCanvasAudioConnectionCapability(
          catalog,
          identity,
          draft,
          canvasAudioSourceInputs(sources, pinned),
        );
        const nextTarget: CanvasNodeData = request.newTarget
          ? {
              ...target,
              serviceData: {
                ...target.serviceData,
                id: target.id,
                node_type: "audio",
                node_action: "generate",
                config: {
                  ...target.serviceData?.config,
                  draft: {
                    ...target.serviceData?.config?.draft,
                    composer: canvasAudioComposerForPersistence(draft!),
                  },
                },
              },
            }
          : currentTarget!;
        const graph = {
          ...current,
          nodes: [
            ...current.nodes,
            ...newSources,
            ...(request.newTarget ? [nextTarget] : []),
          ],
        };
        const candidate = connectCanvasDocumentSources(
          graph,
          request.sourceNodeIds,
          target.id,
        );
        if (
          !contextCurrent() ||
          client.getSnapshot().document !== current ||
          client.getDocumentEpoch() !== epoch ||
          !client.canAcceptConnectionGraph(candidate) ||
          (!request.newTarget &&
            !client.canAcceptAudioConnectionInput(target.id))
        )
          throw new Error("画布已变化或素材尚未就绪，请重新连接");
        client.change(candidate, true);
        published = true;
        const saved = await client.waitForSaved(() => {
          if (
            disposed ||
            !contextCurrent() ||
            client.getDocumentEpoch() !== epoch
          )
            throw new Error("画布版本已变化");
        });
        const previousEdges = new Set(
          current.connections.map((edge) => edge.id),
        );
        const savedEdges = new Set(saved.edges?.map((edge) => edge.id));
        const savedTarget = saved.nodes?.find((node) => node.id === target.id);
        if (
          !savedTarget ||
          canvasGraphFingerprint(savedTarget.config?.draft?.composer) !==
            canvasGraphFingerprint(draft) ||
          newSources.some(
            (node) => !saved.nodes?.some((item) => item.id === node.id),
          ) ||
          candidate.connections.some(
            (edge) => !previousEdges.has(edge.id) && !savedEdges.has(edge.id),
          )
        )
          throw new Error("连接保存结果尚未确认，请检查画布后重试");
        return target.id;
      } catch (error) {
        if (published) confirmationUnknown = true;
        throw error;
      } finally {
        abort.abort();
        requests.delete(abort);
        busy = false;
      }
    },
  };
}
