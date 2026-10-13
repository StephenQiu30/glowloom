"use client";
// BeefTV a2196d3: canvas-node-prompt-panel.tsx compact composer (MIT).
import { ImageIcon, VideoIcon, ChevronDown, ArrowUp, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { isDraft, type Draft, type Node } from "./editor-state";

export function CanvasComposer({
  node,
  nodes,
  details,
  onDetails,
  onSelect,
  onUpdate,
  onDraft,
  onGenerate,
  generating = false,
}: {
  node: Node;
  nodes: Node[];
  details: boolean;
  onDetails: (open: boolean) => void;
  onSelect: (id?: string) => void;
  onUpdate: (patch: Partial<Node>) => void;
  onDraft: (patch: Partial<Draft>) => void;
  onGenerate?: () => void;
  generating?: boolean;
}) {
  const Icon = node.node_type === "video_generation" ? VideoIcon : ImageIcon;
  return (
    <aside
      aria-label="节点编辑"
      className="canvas-node-composer"
      data-canvas-no-zoom
      data-canvas-wheel-scroll
    >
      <div className="flex items-center gap-2 text-xs">
        <Icon className="size-3.5" />
        <label htmlFor="node-picker" className="sr-only">
          选择节点
        </label>
        <select
          id="node-picker"
          className="min-w-0 flex-1 bg-transparent outline-offset-2"
          value={node.id}
          onChange={(event) => onSelect(event.target.value || undefined)}
        >
          {nodes.map((n) => (
            <option key={n.id} value={n.id}>
              {n.title}
            </option>
          ))}
        </select>
        <Button
          variant="ghost"
          size="icon-sm"
          aria-label="关闭节点编辑"
          onClick={() => onSelect(undefined)}
        >
          <X />
        </Button>
      </div>
      {isDraft(node) ? (
        <>
          <label htmlFor="node-prompt" className="sr-only">
            提示词
          </label>
          <Textarea
            id="node-prompt"
            placeholder="描述你想创作的画面…"
            className="canvas-prompt-input"
            value={node.config.draft.prompt}
            maxLength={10000}
            onChange={(e) => onDraft({ prompt: e.target.value })}
          />
          <div className="canvas-composer-footer">
            <span className="text-xs text-muted-foreground">选择模型生成</span>
            <div className="ml-auto flex items-center gap-2">
              <label htmlFor="draft-ratio" className="sr-only">
                草稿画幅
              </label>
              <select
                id="draft-ratio"
                className="h-7 rounded bg-transparent text-xs"
                value={node.config.draft.aspect_ratio}
                onChange={(e) => onDraft({ aspect_ratio: e.target.value })}
              >
                {["9:16", "16:9", "1:1"].map((r) => (
                  <option key={r}>{r}</option>
                ))}
              </select>
              <label htmlFor="draft-count" className="sr-only">
                候选数量
              </label>
              <Input
                id="draft-count"
                className="h-7 w-12 text-xs"
                type="number"
                min={1}
                max={8}
                value={node.config.draft.candidate_count}
                onChange={(e) =>
                  onDraft({ candidate_count: Number(e.target.value) })
                }
              />
              <Button
                size="icon-sm"
                disabled={generating || !node.config.draft.prompt.trim()}
                onClick={onGenerate}
                aria-label="生成当前节点"
                title="选择模型生成"
              >
                <ArrowUp />
              </Button>
            </div>
          </div>
          <details
            open={details}
            onToggle={(e) => onDetails(e.currentTarget.open)}
          >
            <summary className="flex cursor-pointer list-none items-center gap-2 py-2 text-xs text-muted-foreground">
              节点参数
              <ChevronDown className="ml-auto size-3" />
            </summary>
            <div className="space-y-3 pb-2">
              <div>
                <label htmlFor="node-title" className="mb-1 block text-xs">
                  节点名称
                </label>
                <Input
                  id="node-title"
                  value={node.title}
                  maxLength={128}
                  onChange={(e) => onUpdate({ title: e.target.value })}
                />
              </div>
              <div>
                <label htmlFor="negative-prompt" className="mb-1 block text-xs">
                  负面提示词
                </label>
                <Textarea
                  id="negative-prompt"
                  value={node.config.draft.negative_prompt}
                  maxLength={10000}
                  onChange={(e) => onDraft({ negative_prompt: e.target.value })}
                />
              </div>
              <div className="grid grid-cols-2 gap-3">
                {(["x", "y"] as const).map((axis) => (
                  <div key={axis}>
                    <label
                      htmlFor={`node-${axis}`}
                      className="mb-1 block text-xs"
                    >
                      {axis.toUpperCase()} 坐标
                    </label>
                    <Input
                      id={`node-${axis}`}
                      type="number"
                      min={-1e6}
                      max={1e6}
                      value={node[axis]}
                      onChange={(e) =>
                        onUpdate({
                          [axis]: Math.max(
                            -1e6,
                            Math.min(1e6, Number(e.target.value)),
                          ),
                        })
                      }
                    />
                  </div>
                ))}
              </div>
              <div className="grid grid-cols-2 gap-3">
                {(["width", "height"] as const).map((axis) => (
                  <div key={axis}>
                    <label
                      htmlFor={`node-${axis}`}
                      className="mb-1 block text-xs"
                    >
                      {axis === "width" ? "节点宽度" : "节点高度"}
                    </label>
                    <Input
                      id={`node-${axis}`}
                      type="number"
                      min={40}
                      max={2000}
                      value={node[axis] ?? (axis === "width" ? 288 : 200)}
                      onChange={(e) =>
                        onUpdate({
                          width: node.width ?? 288,
                          height: node.height ?? 200,
                          [axis]: Math.max(
                            40,
                            Math.min(2000, Number(e.target.value)),
                          ),
                        })
                      }
                    />
                  </div>
                ))}
              </div>
            </div>
          </details>
          <p className="text-[11px] leading-4 text-muted-foreground">
            生成前先保存画布，再选择模型并确认报价。
          </p>
        </>
      ) : (
        <p className="py-4 text-sm text-muted-foreground">
          此节点暂时仅支持查看。
        </p>
      )}
    </aside>
  );
}
