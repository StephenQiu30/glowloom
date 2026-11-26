"use client";
import { useId, useRef, useState } from "react";
import { useCopyText } from "@/components/canvas/runtime/hooks/use-copy-text";
import { ArrowUp, LoaderCircle, Maximize2, X } from "lucide-react";
import { notifyError } from "@/components/canvas/runtime/components/feedback/error-notification";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Field, FieldLabel } from "@/components/ui/field";
import { CanvasModelSelect } from "../parameters/canvas-model-select.tsx";
import { RichTextInput } from "../runtime/components/editor/rich-text/rich-text-input.tsx";
import { documentText } from "../runtime/components/editor/lib/rich-text-document.ts";
import type { CanvasNodeData } from "../types/canvas.ts";
import { toCanvasGraph } from "../lib/canvas-document-adapter.ts";
import { CanvasNodeReferenceBar } from "./canvas-node-reference-bar.tsx";
import type { CanvasResourceReference } from "../lib/canvas-resource-references.ts";
import { useCanvasReferencePreviews } from "../hooks/use-canvas-reference-previews.ts";
import type { CanvasDocumentIntegration } from "../types/canvas-document-integration.ts";
import { completedGenerationStates } from "../lib/canvas-generation-projection.ts";
import { CanvasGenerationParameters } from "../parameters/canvas-generation-parameters.tsx";
import { CanvasConfigParameterDialog } from "../parameters/canvas-config-parameter-dialog.tsx";
import type { CanvasPendingForms } from "../types/canvas-recovery.ts";

export function CanvasDocumentNodePanel({
  node,
  onChange,
  onClose,
  nodes = [],
  connectedNodes = [],
  mentionReferences = [],
  onDisconnectReference,
  onStartReferenceSelection,
  generation,
  onGenerate,
  pendingForm,
  onPendingForm,
  collaboration,
}: {
  node: CanvasNodeData;
  projectId?: string;
  onChange: (node: CanvasNodeData) => void;
  onClose: () => void;
  nodes?: CanvasNodeData[];
  connectedNodes?: CanvasNodeData[];
  mentionReferences?: CanvasResourceReference[];
  onDisconnectReference?: (fromNodeId: string, toNodeId: string) => void;
  onStartReferenceSelection?: (nodeId: string) => void;
  generation?: CanvasDocumentIntegration["generation"];
  onGenerate?: () => void;
  pendingForm?: CanvasPendingForms[string];
  onPendingForm?: (form: CanvasPendingForms[string] | undefined) => void;
  collaboration?: import("@glowloom/canvas/model").NodeTextBinding;
}) {
  const previews = useCanvasReferencePreviews(mentionReferences);
  const copyText = useCopyText();
  const [expanded, setExpanded] = useState(false);
  const availabilityId = useId();
  const expandButtonRef = useRef<HTMLButtonElement>(null);
  const source =
    node.documentData ??
    toCanvasGraph({
      nodes: [node],
      connections: [],
      backgroundMode: "dots",
      showImageInfo: false,
      viewport: { x: 0, y: 0, k: 1 },
    }).nodes[0];
  const parameters = source.parameters;
  const supported = ["text", "image", "video"].includes(node.type);
  const selected = parameters?.modelSelection;
  const processing =
    parameters?.mode === "PROCESS" || parameters?.mode === "AGENT";
  const run = generation?.runs.find((value) => value.nodeId === node.id);
  const submitting = generation?.submitting.includes(node.id);
  const pending =
    !!run?.pending && !completedGenerationStates.has(run.state ?? "");
  const canGenerate =
    supported &&
    !!onGenerate &&
    !processing &&
    generation?.ready === true &&
    !!selected &&
    !!(
      node.metadata?.richText
        ? documentText(node.metadata.richText.content)
        : node.type === "text"
          ? node.metadata?.content
          : node.metadata?.prompt
    )?.trim() &&
    !pending &&
    !submitting;
  const generate = () => {
    if (canGenerate) onGenerate?.();
  };
  const patch = (change: Partial<CanvasContract.CanvasDocumentNodeDTO>) =>
    onChange({ ...node, documentData: { ...source, ...change } });
  const referenceBar = (inDialog = false) => (
    <CanvasNodeReferenceBar
      nodeId={node.id}
      nodes={nodes}
      connectedNodes={connectedNodes}
      onDisconnect={onDisconnectReference}
      onStartSelection={
        onStartReferenceSelection
          ? (id) => {
              if (inDialog) setExpanded(false);
              onStartReferenceSelection(id);
            }
          : undefined
      }
    />
  );
  const generationControls = (
    <div className="canvas-prompt-controls">
      <div className="canvas-prompt-parameters">
        {supported && !processing && (
          <CanvasModelSelect
            key={node.id}
            type={node.type}
            selected={selected}
            onSelect={(modelSelection) =>
              patch({
                parameters: {
                  ...parameters,
                  mode: "GENERATE",
                  modelSelection,
                },
              })
            }
          />
        )}
        {["image", "video"].includes(node.type) && selected && (
          <CanvasGenerationParameters
            type={node.type}
            parameters={parameters}
            hasReferences={
              connectedNodes.length > 0 ||
              !!source.fileReferences?.some((reference) =>
                reference.purpose?.startsWith("REFERENCE_"),
              )
            }
            onChange={(parameters) => patch({ parameters })}
          />
        )}
      </div>
      {supported && !processing && (
        <Button
          size="icon-lg"
          className="canvas-prompt-submit"
          aria-label={pending || submitting ? "正在生成" : "生成"}
          disabled={!canGenerate}
          aria-describedby={!generation ? availabilityId : undefined}
          title={
            !generation
              ? "当前画布的生成服务尚未接入"
              : !selected
                ? "请先选择模型"
                : pending
                  ? "生成中"
                  : undefined
          }
          onClick={generate}
        >
          {pending || submitting ? (
            <LoaderCircle className="animate-spin motion-reduce:animate-none" />
          ) : (
            <ArrowUp />
          )}
        </Button>
      )}
    </div>
  );
  const promptEditor = (
    <RichTextInput
      key={node.id}
      collaboration={collaboration}
      documentMode={node.type === "text"}
      compactToolbar
      documentTitle={node.title}
      aria-label={node.type === "text" ? "文字内容" : "画面描述"}
      className="canvas-prompt-input"
      expandedView={{
        open: expanded,
        onOpenChange: setExpanded,
        header: referenceBar(true),
        footer: generationControls,
        onCloseAutoFocus: () =>
          expandButtonRef.current?.focus({ preventScroll: true }),
      }}
      placeholder={
        node.type === "text"
          ? "输入文字，或 @ 引用已连接的参考素材"
          : node.type === "video"
            ? "上传参考图、输入文字或 @ 引用素材，描述你想生成的视频"
            : "描述你的想法，或添加参考素材"
      }
      references={previews.references.filter((reference) => reference.active)}
      value={
        node.type === "text"
          ? (node.metadata?.content ?? "")
          : (node.metadata?.prompt ?? "")
      }
      document={node.metadata?.richText}
      submitShortcut="mod-enter"
      onSubmit={generate}
      onChange={(_text, richText) =>
        onChange({
          ...node,
          documentData: { ...source, textDocument: richText },
          metadata: {
            ...node.metadata,
            richText,
            ...(node.type === "text"
              ? { content: documentText(richText.content) }
              : { prompt: documentText(richText.content) }),
          },
        })
      }
    />
  );
  return (
    <div
      className="canvas-prompt-panel"
      data-canvas-no-zoom
      onPointerDown={(event) => event.stopPropagation()}
      onWheel={(event) => event.stopPropagation()}
    >
      <div className="canvas-prompt-header">
        {node.type === "config" ? <span>共享参数</span> : referenceBar()}
        {node.type !== "config" && (
          <Button
            variant="ghost"
            size="icon-sm"
            aria-label="放大编辑"
            ref={expandButtonRef}
            onClick={() => setExpanded(true)}
          >
            <Maximize2 />
          </Button>
        )}
        <Button
          variant="ghost"
          size="icon-sm"
          className="canvas-composer-close"
          aria-label="关闭节点参数"
          onClick={onClose}
        >
          <X />
        </Button>
      </div>
      {node.type === "config" ? (
        <div className="grid gap-3 p-3">
          {Object.entries(parameters?.values ?? {}).map(([key, value]) => (
            <Field key={key}>
              <FieldLabel htmlFor={node.id + key}>{key}</FieldLabel>
              <Input
                id={node.id + key}
                value={
                  pendingForm?.values[key] ??
                  (typeof value === "object"
                    ? JSON.stringify(value)
                    : String(value ?? ""))
                }
                readOnly={
                  typeof value !== "string" && typeof value !== "number"
                }
                onChange={(event) =>
                  onPendingForm
                    ? onPendingForm({
                        values: {
                          ...pendingForm?.values,
                          [key]: event.target.value,
                        },
                      })
                    : patch({
                        parameters: {
                          values: {
                            ...parameters?.values,
                            [key]:
                              typeof value === "number"
                                ? Number(event.target.value)
                                : event.target.value,
                          },
                        },
                      })
                }
              />
              <CanvasConfigParameterDialog
                values={parameters?.values ?? {}}
                parameterKey={key}
                disabled={!!pendingForm}
                onChange={(values) =>
                  patch({ parameters: { ...parameters, values } })
                }
              />
            </Field>
          ))}
          {pendingForm && (
            <div className="flex gap-2">
              <Button
                onClick={() => {
                  const values = { ...parameters?.values };
                  for (const [key, raw] of Object.entries(pendingForm.values)) {
                    if (typeof values[key] === "number") {
                      if (!raw.trim() || !Number.isFinite(Number(raw))) {
                        notifyError(
                          new Error("请输入有效数字"),
                          "参数尚未应用",
                        );
                        return;
                      }
                      values[key] = Number(raw);
                    } else values[key] = raw;
                  }
                  onPendingForm?.(undefined);
                  patch({ parameters: { ...parameters, values } });
                }}
              >
                应用参数
              </Button>
              <Button
                variant="outline"
                onClick={() => onPendingForm?.(undefined)}
              >
                放弃未应用参数
              </Button>
            </div>
          )}
          <CanvasConfigParameterDialog
            values={parameters?.values ?? {}}
            disabled={!!pendingForm}
            onChange={(values) =>
              patch({ parameters: { ...parameters, values } })
            }
          />
          {!Object.keys(parameters?.values ?? {}).length && (
            <p className="text-sm text-muted-foreground">尚未设置共享参数。</p>
          )}
        </div>
      ) : (
        promptEditor
      )}
      {!expanded && generationControls}
      {supported && !processing && !generation && (
        <p
          id={availabilityId}
          role="status"
          className="px-3 pb-3 text-xs text-muted-foreground"
        >
          当前画布的生成服务尚未接入，提示词会随画布保存。
        </p>
      )}
      {node.type === "text" &&
        run &&
        ["UNCONFIRMED", "SUBMISSION_UNKNOWN", "ARCHIVE_FAILED"].includes(
          run.state ?? "",
        ) && (
          <div className="flex items-center gap-2 px-3 pb-3 text-sm">
            <span role="status">状态待确认</span>
            <Button
              variant="ghost"
              size="sm"
              disabled={submitting}
              onClick={() =>
                void generation?.refresh(
                  run,
                  run.state === "UNCONFIRMED" && !!run.pending,
                )
              }
            >
              确认状态
            </Button>
          </div>
        )}
      {run?.state === "SUCCEEDED" && run.textOutput && (
        <section className="grid gap-2 px-3 pb-3" aria-label="生成文字结果">
          <div className="flex items-center justify-between">
            <span className="text-sm font-medium">生成结果</span>
            <Button
              variant="ghost"
              size="sm"
              onClick={() => copyText(run.textOutput!, "已复制生成结果")}
            >
              复制结果
            </Button>
          </div>
          <p className="max-h-60 overflow-y-auto text-sm whitespace-pre-wrap">
            {run.textOutput}
          </p>
        </section>
      )}
    </div>
  );
}
