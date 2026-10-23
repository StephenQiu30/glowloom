"use client";
import { Button } from "@/components/ui/button";
import { RichTextInput } from "@/components/canvas/runtime/components/editor/rich-text/rich-text-input";
import type { RichTextReference } from "@/components/canvas/runtime/components/editor/types/rich-text";
import { X } from "lucide-react";
import type { CanvasNodeData } from "@/components/canvas/types/canvas";
import type { NodeGenerationInput } from "../lib/canvas-node-generation";
import { CanvasNodeReferenceBar } from "../nodes/canvas-node-reference-bar";
type CanvasConfigComposerProps = {
  nodeId: string;
  nodes: CanvasNodeData[];
  value: string;
  inputs: NodeGenerationInput[];
  connectedNodes?: CanvasNodeData[];
  onChange: (value: string) => void;
  onClose: () => void;
  onDisconnectReference?: (fromNodeId: string, toNodeId: string) => void;
  onStartReferenceSelection?: (nodeId: string) => void;
};

export function CanvasConfigComposer({
  nodeId,
  nodes,
  value,
  inputs,
  connectedNodes = [],
  onChange,
  onClose,
  onDisconnectReference,
  onStartReferenceSelection,
}: CanvasConfigComposerProps) {
  const references: RichTextReference[] = inputs.map((input) => ({
    id: input.nodeId,
    label: input.title,
    title: input.title,
    kind: input.type,
    token: "@[node:" + input.nodeId + "]",
    text: input.type === "text" ? input.text : undefined,
    previewUrl:
      input.type === "image"
        ? input.image?.url || input.image?.dataUrl
        : undefined,
  }));
  return (
    <div
      data-canvas-no-zoom
      className="canvas-prompt-panel"
      onPointerDown={(event) => event.stopPropagation()}
      onWheel={(event) => event.stopPropagation()}
    >
      <div className="canvas-prompt-header">
        <p>组装提示词</p>
        <Button
          type="button"
          variant="ghost"
          size="icon-sm"
          aria-label="关闭组装提示词"
          onClick={onClose}
        >
          <X />
        </Button>
      </div>
      <CanvasNodeReferenceBar
        nodeId={nodeId}
        nodes={nodes}
        connectedNodes={connectedNodes}
        onDisconnect={onDisconnectReference}
        onStartSelection={onStartReferenceSelection}
      />
      <RichTextInput
        value={value}
        references={references}
        onChange={onChange}
        aria-label="生成配置提示词"
        className="canvas-prompt-input"
        placeholder="输入提示词，按 @ 引用连接的图片、文本或组"
      />
    </div>
  );
}
