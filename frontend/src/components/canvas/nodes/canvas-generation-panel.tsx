"use client";

import { useMemo, useRef, useState, useSyncExternalStore } from "react";
import dynamic from "next/dynamic";
import { Button } from "@/components/ui/button";
import { getMediaPreview } from "@/api/media";
import { getOperation } from "@/api/operations";
import {
  emptyPromptDraft,
  type PromptDraft,
} from "@/components/generation/prompt-document";
import { requireTask } from "@/components/generation/contracts";
import type { CanvasComposerOwner } from "@/components/generation/rich-generation";
import type { ComposerSession } from "@/components/generation/composer-session";
import type { CanvasYjsLiveSession } from "../yjs-live-session";
import { replaceCanvasNodeAsset } from "../lib/canvas-asset-replacement";
import { textToDocument } from "../runtime/components/editor/lib/rich-text-document";
import { canvasSubjectSources } from "../lib/canvas-subject-source";
import { CanvasSubjectConnectionBar } from "../subject/canvas-subject-connection-bar";
import { canonical } from "@/components/generation/draft-save";

const CanvasCinemaKitBar = dynamic(
  () =>
    import("../cinema-kit/canvas-cinema-kit").then((m) => m.CanvasCinemaKitBar),
  { loading: () => <p role="status">正在读取工具箱设置…</p> },
);
const RichGeneration = dynamic(
  () =>
    import("@/components/generation/rich-generation").then(
      (module) => module.RichGeneration,
    ),
  {
    loading: () => (
      <p role="status" className="p-4 text-sm text-muted-foreground">
        正在打开生成器…
      </p>
    ),
  },
);

export function CanvasGenerationPanel({
  client,
  nodeID,
  canvasID,
  projectID,
}: {
  client: CanvasYjsLiveSession;
  nodeID: string;
  canvasID: string;
  projectID: string;
}) {
  const view = useSyncExternalStore(
    client.subscribe,
    client.getSnapshot,
    client.getSnapshot,
  );
  const empty = useMemo(() => emptyPromptDraft(), []);
  const [recoveryError, setRecoveryError] = useState("");
  const retained = view.inputDrafts[nodeID];
  const editor = useRef<ComposerSession | null>(null);
  const node = view.document?.nodes.find((item) => item.id === nodeID);
  if (!node || !["text", "image", "video", "audio"].includes(node.type))
    return null;
  const capability = node.type as "text" | "image" | "video" | "audio";
  const input =
    retained?.draft ??
    (node.serviceData?.config?.draft?.composer as PromptDraft | undefined) ??
    empty;
  const references = (view.document?.nodes ?? []).flatMap((item) => {
    const assetID = item.documentData?.fileReferences?.find(
      (ref) => ref.purpose === "CONTENT",
    )?.fileId;
    return item.id !== nodeID &&
      ["image", "video", "audio"].includes(item.type) &&
      assetID
      ? [
          {
            nodeID: item.id,
            title: item.title,
            kind: item.type as "image" | "video" | "audio",
            assetID,
          },
        ]
      : [];
  });
  const owner: CanvasComposerOwner = {
    contextKey: `${canvasID}/${nodeID}`,
    readOnly: !view.writable || view.migration,
    draft: input,
    onChange: (draft) => client.changeInput(nodeID, draft),
    onSession: (session) => {
      editor.current = session;
      // Binding also guards IME and keeps the authoritative draft in this editor.
      const dispose = client.bindInputComposer(nodeID, session);
      return () => {
        dispose();
        if (editor.current === session) editor.current = null;
      };
    },
    waitForNode: async () => {
      await client.waitForSaved(() => {
        if (
          !client
            .getSnapshot()
            .document?.nodes.some((item) => item.id === nodeID)
        )
          throw new Error("节点已不存在");
      });
    },
    flush: async () => {
      if (editor.current?.composing) throw new Error("请先完成当前输入法组合");
      if (
        !client.getSnapshot().document?.nodes.find((item) => item.id === nodeID)
          ?.serviceData?.config?.draft?.composer
      ) {
        await client.waitForSaved();
        const current = client
          .getSnapshot()
          .document?.nodes.find((item) => item.id === nodeID);
        if (!current) throw new Error("节点已不存在");
        if (
          !current.serviceData?.config?.draft?.composer &&
          !client.changeInput(nodeID, emptyPromptDraft(), true)
        )
          throw new Error("生成输入尚未保存");
      }
      return client.prepareInput(nodeID);
    },
    referenceSources: references,
    subjectSources: canvasSubjectSources(view.document?.nodes ?? [], canvasID),
    incomingSources: (view.document?.connections ?? []).flatMap((edge) => {
      const source = references.find((item) => item.nodeID === edge.fromNodeId);
      return edge.toNodeId === nodeID && source
        ? [{ ...source, edgeID: edge.id }]
        : [];
    }),
  };
  const resultIdentity = () => {
    const current = client
      .getSnapshot()
      .document?.nodes.find((item) => item.id === nodeID);
    const epoch = client.getDocumentEpoch();
    if (!current || !client.getSnapshot().writable || !epoch)
      throw new Error("结果目标已删除或不可编辑");
    return canonical({
      epoch,
      text: current.metadata?.richText ?? current.metadata?.content ?? "",
      files: current.documentData?.fileReferences ?? [],
    });
  };
  const apply = async (
    before: string,
    update: (current: NonNullable<typeof node>) => NonNullable<typeof node>,
  ) => {
    const graph = client.getSnapshot().document;
    if (!graph || resultIdentity() !== before)
      throw new Error("节点结果已变化，请重新确认采用");
    const next = {
      ...graph,
      nodes: graph.nodes.map((item) =>
        item.id === nodeID ? update(item) : item,
      ),
    };
    if (!client.canAccept(next))
      throw new Error("画布当前不可编辑，结果仍保留在任务中");
    client.change(next, true);
    const expected = resultIdentity();
    await client.waitForSaved(() => {
      if (resultIdentity() !== expected)
        throw new Error("节点结果再次变化，请核对保存结果");
    });
  };
  return (
    <div
      className="canvas-formal-generator"
      data-canvas-no-zoom
      data-canvas-shortcuts-ignore
    >
      {retained && (
        <div role="alert" className="space-y-2 rounded-xl border p-3 text-sm">
          <p>{retained.reason}</p>
          <div className="flex gap-2">
            {(["server", "local"] as const).map((choice) => (
              <Button
                key={choice}
                variant="outline"
                size="sm"
                disabled={!view.writable}
                onClick={() => {
                  try {
                    client.resolveInputDraft(nodeID, choice);
                    setRecoveryError("");
                  } catch (cause) {
                    setRecoveryError(
                      cause instanceof Error ? cause.message : "草稿尚未恢复",
                    );
                  }
                }}
              >
                {choice === "server" ? "使用已保存版本" : "重新提交本页草稿"}
              </Button>
            ))}
          </div>
          {recoveryError && <p>{recoveryError}</p>}
        </div>
      )}
      <CanvasSubjectConnectionBar graph={view.document} nodeID={nodeID} />
      <CanvasCinemaKitBar client={client} nodeID={nodeID} />
      <RichGeneration
        key={`${canvasID}/${nodeID}`}
        projectID={projectID}
        seed={{
          prompt: "",
          capability,
          source: { canvas_id: canvasID, node_id: nodeID },
        }}
        presentation="canvas"
        canvas={owner}
        onAdopt={async (assetID, kind) => {
          if (kind !== capability || kind === "text")
            throw new Error("结果类型与节点不一致");
          const before = resultIdentity();
          const preview = await getMediaPreview({
            pid: projectID,
            asset_id: assetID,
          });
          const asset = preview.asset;
          if (
            asset?.id !== assetID ||
            asset.project_id !== projectID ||
            asset.kind !== kind ||
            !asset.revision ||
            !preview.url
          )
            throw new Error("结果素材不可用");
          await apply(before, (current) =>
            replaceCanvasNodeAsset(
              current,
              kind === "image"
                ? {
                    kind,
                    title: current.title,
                    fileId: assetID,
                    fileVersion: String(asset.revision),
                    dataUrl: preview.url!,
                    width: asset.width,
                    height: asset.height,
                  }
                : {
                    kind: kind as "video" | "audio",
                    title: current.title,
                    fileId: assetID,
                    fileVersion: String(asset.revision),
                    url: preview.url!,
                  },
            ),
          );
        }}
        onAdoptText={async (operationID, outputID) => {
          if (capability !== "text") throw new Error("此节点不能采用文字结果");
          const before = resultIdentity();
          const task = requireTask(
            await getOperation({ id: operationID, project_id: projectID }),
            projectID,
          );
          const output = task.outputs?.find((item) => item.id === outputID);
          if (
            task.status !== "completed" ||
            output?.kind !== "json" ||
            output.moderation_status !== "passed" ||
            !output.text
          )
            throw new Error("文字结果尚未就绪");
          await apply(before, (current) => ({
            ...current,
            metadata: {
              ...current.metadata,
              content: output.text!,
              richText: {
                schemaVersion: 1,
                content: textToDocument(output.text!),
              },
            },
          }));
        }}
      />
    </div>
  );
}
