import { Hand, MousePointer2, Puzzle } from "lucide-react";
import {
  isValidElement,
  useCallback,
  useMemo,
  type Dispatch,
  type MutableRefObject,
  type SetStateAction,
} from "react";

import { canvasThemes } from "@/components/canvas/runtime/lib/theme";
import type { CanvasOperation } from "@/components/canvas/lib/canvas-operations";
import { buildGenerationConfig } from "@/components/canvas/lib/canvas-generation-helpers";
import { getNodeDefinition } from "@/components/canvas/lib/node-registry";
import { buildNodeContext } from "@/components/canvas/lib/plugin-node-context";
import {
  requestEdit,
  requestGeneration,
  requestImageQuestion,
  type AiTextMessage,
} from "@/components/canvas/runtime/lib/generation/image";
import {
  requestVideoGeneration,
  storeGeneratedVideo,
} from "@/components/canvas/runtime/lib/generation/video";
import { imageToDataUrl } from "@/components/canvas/runtime/lib/media/image-storage";
import {
  decodeChannelModel,
  selectableModelsByCapability,
  type AiConfig,
  type ModelCapability,
} from "@/components/canvas/runtime/store/use-config-store";
import type {
  CanvasConnection,
  CanvasNodeData,
  ViewportTransform,
} from "@/components/canvas/types/canvas";
import type {
  CanvasNodeToolbarItem,
  CanvasPluginAi,
  CanvasPluginHost,
} from "@/components/canvas/types/canvas-plugin";
import type { ReferenceImage } from "@/components/canvas/runtime/types/image";

type CanvasTheme = (typeof canvasThemes)[keyof typeof canvasThemes];

type PluginHostParams = {
  effectiveConfig: AiConfig;
  isAiConfigReady: (config: AiConfig, model: string) => boolean;
  theme: CanvasTheme;
  nodesRef: MutableRefObject<CanvasNodeData[]>;
  connectionsRef: MutableRefObject<CanvasConnection[]>;
  viewportRef: MutableRefObject<ViewportTransform>;
  setNodes: Dispatch<SetStateAction<CanvasNodeData[]>>;
  setDialogNodeId: Dispatch<SetStateAction<string | null>>;
  applyCanvasOps: (ops?: CanvasOperation[]) => unknown;
};

/**
 * Plugin node host capabilities: expose host-side AI generation, canvas access, and panel controls
 * through host/ai objects. Returns renderers for registered built-in node panels and toolbars.
 */
export function usePluginHost(params: PluginHostParams) {
  const {
    effectiveConfig,
    isAiConfigReady,
    theme,
    nodesRef,
    connectionsRef,
    viewportRef,
    setNodes,
    setDialogNodeId,
    applyCanvasOps,
  } = params;

  // Host capabilities available to plugin nodes; methods receive nodeId and are not bound to a specific node.
  const pluginAi = useMemo<CanvasPluginAi>(() => {
    // Convert plugin reference images (data URLs or URLs) into the ReferenceImage[] expected by the host generation CanvasContract.
    const toReferences = (refs?: string[]): ReferenceImage[] =>
      (refs || []).filter(Boolean).map((src, index) => ({
        id: `plugin-ref-${index}`,
        name: `ref-${index}.png`,
        type: "image/png",
        dataUrl: src,
      }));
    // Open the configuration dialog and throw when AI is not configured, allowing the plugin to handle the error.
    const ensureReady = (config: AiConfig) => {
      if (!isAiConfigReady(config, config.model)) {
        throw new Error("生成服务尚未接入");
      }
    };
    return {
      generateImage: async (prompt, options) => {
        const config = {
          ...buildGenerationConfig(effectiveConfig, undefined, "image"),
          count: String(options?.count || 1),
          ...(options?.model ? { model: options.model } : {}),
          ...(options?.size ? { size: options.size } : {}),
        };
        ensureReady(config);
        const references = toReferences(options?.references);
        const items = references.length
          ? await requestEdit(config, prompt, references, {
              signal: options?.signal,
            })
          : await requestGeneration(config, prompt, {
              signal: options?.signal,
            });
        const images = await Promise.all(
          items.map(async (item) => {
            try {
              return await imageToDataUrl(
                { dataUrl: item.dataUrl },
                { signal: options?.signal },
              );
            } catch (error) {
              if (options?.signal?.aborted) throw error;
              return item.dataUrl;
            }
          }),
        );
        return { images };
      },
      generateVideo: async (prompt, options) => {
        const config = {
          ...buildGenerationConfig(effectiveConfig, undefined, "video"),
          ...(options?.model ? { model: options.model } : {}),
          ...(options?.size ? { size: options.size } : {}),
          ...(options?.seconds ? { videoSeconds: options.seconds } : {}),
        };
        ensureReady(config);
        const file = await storeGeneratedVideo(
          await requestVideoGeneration(
            config,
            prompt,
            toReferences(options?.references),
            { signal: options?.signal },
          ),
        );
        return {
          url: file.url,
          mimeType: file.mimeType,
          width: file.width,
          height: file.height,
          durationMs: file.durationMs,
        };
      },
      generateText: async (prompt, options) => {
        const config = {
          ...buildGenerationConfig(effectiveConfig, undefined, "text"),
          ...(options?.model ? { model: options.model } : {}),
        };
        ensureReady(config);
        const messages: AiTextMessage[] = [
          ...(options?.system
            ? [{ role: "system" as const, content: options.system }]
            : []),
          { role: "user" as const, content: prompt },
        ];
        const text = await requestImageQuestion(
          config,
          messages,
          (delta) => options?.onDelta?.(delta),
          { signal: options?.signal },
        );
        return { text };
      },
      // List configured models for a capability; labels use the model name without the channel prefix.
      listModels: (capability) =>
        selectableModelsByCapability(
          effectiveConfig,
          capability as ModelCapability | undefined,
        ).map((value) => ({
          value,
          label: decodeChannelModel(value)?.model || value,
        })),
      defaultModel: (capability) =>
        buildGenerationConfig(effectiveConfig, undefined, capability).model,
    };
  }, [effectiveConfig, isAiConfigReady]);

  const pluginHost = useMemo<CanvasPluginHost>(
    () => ({
      getNode: (id) => nodesRef.current.find((node) => node.id === id) || null,
      getNodes: () => nodesRef.current,
      getConnections: () => connectionsRef.current,
      getUpstream: (nodeId) =>
        connectionsRef.current
          .filter((conn) => conn.toNodeId === nodeId)
          .map((conn) =>
            nodesRef.current.find((node) => node.id === conn.fromNodeId),
          )
          .filter((node): node is CanvasNodeData => Boolean(node)),
      getDownstream: (nodeId) =>
        connectionsRef.current
          .filter((conn) => conn.fromNodeId === nodeId)
          .map((conn) =>
            nodesRef.current.find((node) => node.id === conn.toNodeId),
          )
          .filter((node): node is CanvasNodeData => Boolean(node)),
      updateNode: (nodeId, patch) =>
        setNodes((prev) =>
          prev.map((node) =>
            node.id === nodeId ? { ...node, ...patch } : node,
          ),
        ),
      updateMetadata: (nodeId, patch) =>
        setNodes((prev) =>
          prev.map((node) =>
            node.id === nodeId
              ? { ...node, metadata: { ...node.metadata, ...patch } }
              : node,
          ),
        ),
      applyOps: (ops) => applyCanvasOps(ops),
      ai: pluginAi,
      openPanel: (nodeId) => setDialogNodeId(nodeId),
      closePanel: () => setDialogNodeId(null),
    }),
    [
      applyCanvasOps,
      pluginAi,
      connectionsRef,
      nodesRef,
      setDialogNodeId,
      setNodes,
    ],
  );

  const renderPluginPanel = useCallback(
    (panelNode: CanvasNodeData) => {
      const Panel = getNodeDefinition(panelNode.type)?.Panel;
      if (!Panel) return null;
      const ctx = buildNodeContext(
        pluginHost,
        panelNode,
        theme,
        viewportRef.current.k,
      );
      return <Panel ctx={ctx} onClose={() => setDialogNodeId(null)} />;
    },
    [pluginHost, theme, viewportRef, setDialogNodeId],
  );

  // Build the node toolbar from plugin items and a host-provided interaction/move toggle when enabled.
  const buildNodeToolbarItems = useCallback(
    (node: CanvasNodeData): CanvasNodeToolbarItem[] => {
      const definition = getNodeDefinition(node.type);
      const ctx = buildNodeContext(
        pluginHost,
        node,
        theme,
        viewportRef.current.k,
      );
      const custom = (definition?.toolbar?.(ctx) || []).map((item) => ({
        ...item,
        icon: isValidElement(item.icon) ? (
          item.icon
        ) : (
          <Puzzle className="size-4" aria-hidden />
        ),
      }));
      // Show the interaction/move toggle only for nodes with content that are not forced into an interactive state.
      if (
        !definition?.interactionToggle ||
        !node.metadata?.content ||
        definition.forceInteractive?.(node)
      )
        return custom;
      const interactive = Boolean(node.metadata?.interactive);
      const toggle: CanvasNodeToolbarItem = {
        id: "node-interaction-toggle",
        title: interactive
          ? "当前：交互中。点击切回「移动」——拖动可移动节点"
          : "当前：可移动。点击切到「交互」——可操作节点内容（如转动全景）",
        label: interactive ? "移动" : "交互",
        icon: interactive ? (
          <Hand className="size-4" aria-hidden />
        ) : (
          <MousePointer2 className="size-4" aria-hidden />
        ),
        active: interactive,
        onClick: () =>
          pluginHost.updateMetadata(node.id, { interactive: !interactive }),
      };
      return [toggle, ...custom];
    },
    [pluginHost, theme, viewportRef],
  );

  return { pluginHost, renderPluginPanel, buildNodeToolbarItems };
}
