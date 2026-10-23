"use client";
import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Field, FieldLabel } from "@/components/ui/field";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import {
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectGroup,
  SelectItem,
} from "@/components/ui/select";
import type { CanvasNodeData, CanvasConnection } from "../types/canvas";
import {
  canvasInputPort,
  canvasPortsFor,
} from "../constants/canvas-input-ports";

export function CanvasConnectionDialog({
  edge,
  agentProfiles,
  source,
  target,
  connections,
  onConnect,
  onClose,
}: {
  edge: CanvasContract.CanvasDocumentConnectionDTO;
  agentProfiles: CanvasContract.WorkflowAgentProfileVO[];
  source: CanvasNodeData;
  target: CanvasNodeData;
  connections: CanvasConnection[];
  onConnect: (edge: CanvasContract.CanvasDocumentConnectionDTO) => void;
  onClose: () => void;
}) {
  const sourcePort = edge.fromPortId;
  const sourceType = sourcePort === "last_frame" ? "image" : source.type;
  const validOptions = canvasPortsFor(sourceType, target.type).map(
    ({ id, label, purpose }) => [id, label, purpose],
  );
  const [port, setPort] = useState(edge.toPortId);
  const refs =
    source.documentData?.fileReferences?.filter(
      (ref) =>
        ref.purpose ===
        (sourcePort === "last_frame" ? "LAST_FRAME" : "CONTENT"),
    ) ?? [];
  const manualText =
    source.type === "config" ||
    (source.type === "text" &&
      source.documentData?.parameters?.mode === "MANUAL");
  const [binding, setBinding] = useState(
    edge.binding.kind === "NODE_VALUE"
      ? "text"
      : edge.binding.kind === "RUN_OUTPUT"
        ? "run"
        : (edge.binding.referenceKey ?? ""),
  );
  const [index, setIndex] = useState(
    String(edge.binding.selection?.index ?? 1),
  );
  const [selection, setSelection] = useState(
    edge.binding.selection?.mode ?? "ITEM",
  );
  const multiple = canvasInputPort(port)?.allOutputs ?? false;
  const runOutput = ["GENERATE", "PROCESS", "AGENT"].includes(
    source.documentData?.parameters?.mode ?? "",
  );
  const fields = ["PROCESS", "AGENT"].includes(
    target.documentData?.parameters?.mode ?? "",
  )
    ? (agentProfiles.find(
        (profile) =>
          profile.id === source.documentData?.parameters?.agent?.profile,
      )?.fields ?? [])
    : [];
  const [field, setField] = useState(
    edge.binding.selection?.field ?? "summary",
  );
  const selected = validOptions.find((item) => item[0] === port);
  const bound =
    binding === "text"
      ? manualText
      : binding === "run"
        ? runOutput &&
          (sourceType === "text" ||
            (multiple && selection === "ALL") ||
            /^[1-9]\d*$/.test(index))
        : refs.some((ref) => ref.referenceKey === binding);
  const allowed =
    !!selected &&
    bound &&
    (binding !== "run" ||
      field === "summary" ||
      fields.some((item) => item.key === field));
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle>
            调整 {source.title} 到 {target.title} 的引用
          </DialogTitle>
          <DialogDescription>
            引用已自动建立，可在这里调整用途和内容。
          </DialogDescription>
        </DialogHeader>
        {validOptions.length ? (
          <>
            <Field>
              <FieldLabel>输入用途</FieldLabel>
              <Select value={port} onValueChange={setPort}>
                <SelectTrigger aria-label="输入用途">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectGroup>
                    {validOptions.map(([value, label]) => (
                      <SelectItem key={value} value={value}>
                        {label}
                      </SelectItem>
                    ))}
                  </SelectGroup>
                </SelectContent>
              </Select>
            </Field>
            <Field>
              <FieldLabel>使用内容</FieldLabel>
              <Select value={binding} onValueChange={setBinding}>
                <SelectTrigger aria-label="使用内容">
                  <SelectValue placeholder="选择内容" />
                </SelectTrigger>
                <SelectContent>
                  <SelectGroup>
                    {manualText && (
                      <SelectItem value="text">当前节点内容</SelectItem>
                    )}
                    {refs.map((ref, position) => (
                      <SelectItem
                        key={ref.referenceKey}
                        value={ref.referenceKey}
                      >
                        已选素材 {position + 1}
                      </SelectItem>
                    ))}
                    {runOutput && (
                      <SelectItem value="run">本次运行的结果</SelectItem>
                    )}
                    {!manualText && !refs.length && !runOutput && (
                      <SelectItem disabled value="unavailable">
                        请先选择素材或生成模型
                      </SelectItem>
                    )}
                  </SelectGroup>
                </SelectContent>
              </Select>
            </Field>
            {binding === "run" &&
              (fields.length > 0 || field !== "summary") && (
                <Field>
                  <FieldLabel htmlFor="canvas-output-field">
                    输出字段
                  </FieldLabel>
                  <Select value={field} onValueChange={setField}>
                    <SelectTrigger id="canvas-output-field">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectGroup>
                        <SelectItem value="summary">摘要文本</SelectItem>
                        {fields.map((item) => (
                          <SelectItem key={item.key} value={item.key!}>
                            {item.label}
                          </SelectItem>
                        ))}
                      </SelectGroup>
                    </SelectContent>
                  </Select>
                </Field>
              )}
            {binding === "run" && multiple && (
              <Field>
                <FieldLabel>结果范围</FieldLabel>
                <Select value={selection} onValueChange={setSelection}>
                  <SelectTrigger aria-label="结果范围">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectGroup>
                      <SelectItem value="ITEM">指定结果</SelectItem>
                      <SelectItem value="ALL">全部结果</SelectItem>
                    </SelectGroup>
                  </SelectContent>
                </Select>
              </Field>
            )}
            {binding === "run" &&
              sourceType !== "text" &&
              (!multiple || selection === "ITEM") && (
                <Field>
                  <FieldLabel htmlFor="canvas-output-index">
                    结果序号
                  </FieldLabel>
                  <Input
                    id="canvas-output-index"
                    type="number"
                    min={1}
                    value={index}
                    onChange={(event) => setIndex(event.target.value)}
                  />
                </Field>
              )}
            <Button
              disabled={!allowed}
              onClick={() => {
                if (!selected) return;
                onConnect({
                  id: edge.id,
                  fromNodeId: source.id,
                  toNodeId: target.id,
                  fromPortId: sourcePort,
                  toPortId: port,
                  purpose: selected[2],
                  order:
                    edge.toPortId === port
                      ? edge.order
                      : Math.max(
                          -1,
                          ...connections
                            .filter(
                              (edge) =>
                                edge.toNodeId === target.id &&
                                edge.documentData?.toPortId === port,
                            )
                            .map((edge) => edge.documentData?.order ?? 0),
                        ) + 1,
                  binding:
                    binding === "text"
                      ? { kind: "NODE_VALUE" }
                      : binding === "run"
                        ? {
                            kind: "RUN_OUTPUT",
                            selection:
                              multiple && selection === "ALL"
                                ? { mode: "ALL" }
                                : {
                                    mode: "ITEM",
                                    ...(field !== "summary" ? { field } : {}),
                                    index:
                                      sourceType === "text" ? 1 : Number(index),
                                  },
                          }
                        : { kind: "FILE_REFERENCE", referenceKey: binding },
                });
              }}
            >
              保存调整
            </Button>
          </>
        ) : (
          <p>这两种节点之间没有可用的输入用途。</p>
        )}
      </DialogContent>
    </Dialog>
  );
}
