"use client";
// BeefTV a2196d3: canvas-node-prompt-panel.tsx compact composer (MIT).
import type { ReactNode } from "react";
import {
  ArrowUp,
  AudioLines,
  Box,
  ImageIcon,
  SlidersHorizontal,
  VideoIcon,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Field, FieldLabel, FieldGroup } from "@/components/ui/field";
import {
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectGroup,
  SelectItem,
} from "@/components/ui/select";
import {
  Popover,
  PopoverContent,
  PopoverHeader,
  PopoverTitle,
  PopoverDescription,
  PopoverTrigger,
} from "@/components/ui/popover";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import {
  isDraft,
  isEditableNode,
  isMediaNode,
  type Draft,
  type Node,
} from "./editor-state";

export function CanvasComposer({
  node,
  details,
  onDetails,
  onUpdate,
  onDraft,
  richComposer,
  onPreview,
  onContinue,
  generating = false,
  blocked = false,
}: {
  node: Node;
  details: boolean;
  onDetails: (open: boolean) => void;
  onUpdate: (patch: Partial<Node>) => void;
  onDraft: (patch: Partial<Draft>) => void;
  richComposer?: (actions: ReactNode) => ReactNode;
  onPreview?: () => void;
  onContinue?: () => void;
  generating?: boolean;
  blocked?: boolean;
}) {
  const Icon = node.node_type.includes("video")
    ? VideoIcon
    : node.node_type === "audio"
      ? AudioLines
      : node.node_type === "model"
        ? Box
        : ImageIcon;
  const editable = isEditableNode(node);
  const generationBlocked = generating || blocked;
  const parameters = editable ? (
    <Popover open={details} onOpenChange={onDetails}>
      <PopoverTrigger asChild>
        <Button variant="ghost" size="icon-sm" aria-label="节点参数">
          <SlidersHorizontal data-icon="inline-start" />
        </Button>
      </PopoverTrigger>
      <PopoverContent
        className="canvas-parameters"
        aria-label="节点参数"
        align="end"
        side="top"
        collisionPadding={16}
      >
        <PopoverHeader>
          <PopoverTitle>节点参数</PopoverTitle>
          <PopoverDescription>
            {isDraft(node)
              ? "设置草稿与节点布局，生成前确认模型和报价。"
              : "调整当前素材的名称、位置与尺寸。"}
          </PopoverDescription>
        </PopoverHeader>
        <FieldGroup className="gap-4">
          <Field data-invalid={!node.title.trim()}>
            <FieldLabel htmlFor="node-title">节点名称</FieldLabel>
            <Input
              id="node-title"
              value={node.title}
              maxLength={128}
              aria-invalid={!node.title.trim()}
              onChange={(e) => onUpdate({ title: e.target.value })}
            />
          </Field>
          {isDraft(node) && !node.config.draft.composer && (
            <>
              <Field>
                <FieldLabel>草稿画幅</FieldLabel>
                <ToggleGroup
                  type="single"
                  variant="outline"
                  value={node.config.draft.aspect_ratio}
                  onValueChange={(v) => {
                    if (v) onDraft({ aspect_ratio: v });
                  }}
                  aria-label="草稿画幅"
                >
                  {["1:1", "9:16", "16:9"].map((v) => (
                    <ToggleGroupItem key={v} value={v}>
                      {v}
                    </ToggleGroupItem>
                  ))}
                </ToggleGroup>
              </Field>
              <Field>
                <FieldLabel htmlFor="draft-count">候选数量</FieldLabel>
                <Select
                  value={String(node.config.draft.candidate_count)}
                  onValueChange={(v) => onDraft({ candidate_count: Number(v) })}
                >
                  <SelectTrigger id="draft-count" aria-label="候选数量">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectGroup>
                      {Array.from({ length: 8 }, (_, i) => (
                        <SelectItem key={i + 1} value={String(i + 1)}>
                          {i + 1} 张
                        </SelectItem>
                      ))}
                    </SelectGroup>
                  </SelectContent>
                </Select>
              </Field>
              <Field>
                <FieldLabel htmlFor="negative-prompt">负面提示词</FieldLabel>
                <Textarea
                  id="negative-prompt"
                  value={node.config.draft.negative_prompt}
                  maxLength={10000}
                  placeholder="描述需要避免出现的内容"
                  onChange={(e) => onDraft({ negative_prompt: e.target.value })}
                />
              </Field>
            </>
          )}
          <FieldGroup className="grid grid-cols-2 gap-3">
            {(["x", "y"] as const).map((axis) => (
              <Field key={axis}>
                <FieldLabel htmlFor={`node-${axis}`}>
                  {axis.toUpperCase()} 坐标
                </FieldLabel>
                <Input
                  id={`node-${axis}`}
                  type="number"
                  min={-1e6}
                  max={1e6}
                  value={node[axis]}
                  onChange={(e) => {
                    if (e.target.value !== "")
                      onUpdate({
                        [axis]: Math.max(
                          -1e6,
                          Math.min(1e6, Number(e.target.value)),
                        ),
                      });
                  }}
                />
              </Field>
            ))}
          </FieldGroup>
          <FieldGroup className="grid grid-cols-2 gap-3">
            {(["width", "height"] as const).map((axis) => (
              <Field key={axis}>
                <FieldLabel htmlFor={`node-${axis}`}>
                  {axis === "width" ? "节点宽度" : "节点高度"}
                </FieldLabel>
                <Input
                  id={`node-${axis}`}
                  type="number"
                  min={40}
                  max={2000}
                  value={node[axis] ?? (axis === "width" ? 288 : 200)}
                  onChange={(e) => {
                    if (e.target.value !== "")
                      onUpdate({
                        width: node.width ?? 288,
                        height: node.height ?? 200,
                        [axis]: Math.max(
                          40,
                          Math.min(2000, Number(e.target.value)),
                        ),
                      });
                  }}
                />
              </Field>
            ))}
          </FieldGroup>
        </FieldGroup>
      </PopoverContent>
    </Popover>
  ) : null;
  return (
    <aside
      aria-label="节点编辑"
      className="canvas-node-composer"
      data-generation={isDraft(node)}
      data-canvas-no-zoom
      data-canvas-wheel-scroll
    >
      {!isDraft(node) ? (
        <div className="canvas-composer-parameters">{parameters}</div>
      ) : null}
      {isDraft(node) ? (
        richComposer?.(parameters)
      ) : isMediaNode(node) ? (
        <div className="canvas-resource-actions">
          <p className="text-sm text-muted-foreground">
            素材已在画布中，可预览或作为参考继续创作。
          </p>
          <div className="flex flex-wrap gap-2">
            {node.node_type !== "model" && (
              <Button variant="outline" size="sm" onClick={onPreview}>
                <Icon data-icon="inline-start" />
                预览素材
              </Button>
            )}
            {["image", "video", "audio"].includes(node.node_type) && (
              <Button
                size="sm"
                onClick={onContinue}
                disabled={generationBlocked}
              >
                <ArrowUp data-icon="inline-start" />
                以此素材继续创作
              </Button>
            )}
          </div>
        </div>
      ) : (
        <p className="py-4 text-sm text-muted-foreground">
          此业务节点由服务端管理，当前支持查看。
        </p>
      )}
    </aside>
  );
}
