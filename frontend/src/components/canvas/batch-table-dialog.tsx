"use client";

import { useInfiniteQuery } from "@tanstack/react-query";
import { BatchRowTask } from "./batch-row-task";
import type { MediaAsset } from "./queries";
import {
  OPERATIONS_KEY,
  queryTasks,
  type Task,
} from "@/components/operation/queries";
import { useRef, useState } from "react";
import { useVirtualizer } from "@tanstack/react-virtual";
import { ArrowLeft, ArrowRight, Copy, Plus, Trash2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Field, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Textarea } from "@/components/ui/textarea";
import {
  batchTableSchema,
  createBatchRow,
  reorderBatchColumns,
  swapBatchReferences,
  synchronizeBatchRows,
  type BatchTableConfig,
} from "./batch-table";
import { CanvasNodeType, type CanvasNodeData } from "./model";
import type { GenerationSource } from "@/components/operation/queries";
import { BatchGenerationPanel } from "./batch-generation-panel";

type Props = {
  node: CanvasNodeData;
  projectId: string;
  canvasId: string;
  onResults: (assets: MediaAsset[]) => Promise<boolean>;
  source: () => GenerationSource;
  nodes: CanvasNodeData[];
  readOnly: boolean;
  onClose: () => void;
  onSave: (value: BatchTableConfig) => Promise<boolean>;
};
export function BatchTableDialog({
  node,
  nodes,
  readOnly,
  onClose,
  onSave,
  projectId,
  canvasId,
  onResults,
  source,
}: Props) {
  "use no memo"; // TanStack Virtual owns mutable measurements; do not memoize its instance.
  const [draft, setDraft] = useState(() =>
    batchTableSchema.parse(node.batchTable),
  );
  const [error, setError] = useState("");
  const [saving, setSaving] = useState(false);
  const [retryRowId, setRetryRowId] = useState<string>();
  const [generationBusy, setGenerationBusy] = useState(false);
  const scroll = useRef<HTMLDivElement>(null);
  const draggedReference = useRef<{ rowId: string; column: number } | null>(
    null,
  );
  const draggedColumn = useRef<string | null>(null);
  const disabled = readOnly || saving || generationBusy;
  const images = nodes.filter(
    (item) => item.type === CanvasNodeType.Image && item.assetId,
  );
  const nodeTitles = new Map(images.map((item) => [item.id, item.title]));
  // eslint-disable-next-line react-hooks/incompatible-library -- The component opts out of compiler memoization; measurements stay owned by TanStack Virtual.
  const virtual = useVirtualizer({
    count: draft.rows.length,
    getScrollElement: () => scroll.current,
    estimateSize: () => 116,
    overscan: 4,
  });
  function rowUpdate(
    id: string,
    update: Partial<BatchTableConfig["rows"][number]>,
  ) {
    setDraft((value) => ({
      ...value,
      rows: value.rows.map((row) =>
        row.id === id ? { ...row, ...update } : row,
      ),
    }));
  }
  function save() {
    if (disabled) return;
    const parsed = batchTableSchema.safeParse(draft);
    if (!parsed.success) {
      setError(parsed.error.issues[0].message);
      return;
    }
    setSaving(true);
    setError("");
    void onSave(parsed.data)
      .then((saved) => {
        if (saved) onClose();
        else setError("表格未确认保存，请修复画布错误后重试。");
      })
      .catch(() => setError("表格保存失败。"))
      .finally(() => setSaving(false));
  }
  const tasks = useInfiniteQuery({
    queryKey: [...OPERATIONS_KEY, projectId, "canvas-batch", canvasId, node.id],
    initialPageParam: undefined as string | undefined,
    queryFn: ({ pageParam, signal }) =>
      queryTasks(
        projectId,
        { canvas_id: canvasId, node_id: node.id, cursor: pageParam },
        signal,
      ),
    getNextPageParam: (data) => data.next_cursor ?? undefined,
    retry: false,
    refetchInterval: (query) =>
      query.state.data?.pages.some((page) =>
        page.items.some((task) =>
          [
            "confirmed",
            "submitting",
            "submitted",
            "unknown",
            "reconciling",
            "ingesting",
            "succeeded",
            "cancelling",
          ].includes(task.status),
        ),
      )
        ? 5000
        : false,
  });
  const byRow = new Map<string, Task>();
  for (const task of tasks.data?.pages.flatMap((page) => page.items) ?? []) {
    if (
      task.source?.canvas_id === canvasId &&
      task.source.node_id === node.id &&
      task.source.row_id &&
      !byRow.has(task.source.row_id)
    )
      byRow.set(task.source.row_id, task);
  }
  const virtualRows = virtual.getVirtualItems();
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !saving) onClose();
      }}
    >
      <DialogContent
        className="flex max-h-[92dvh] w-[96vw] flex-col sm:max-w-[1400px]"
        onEscapeKeyDown={(event) => {
          if (saving) event.preventDefault();
        }}
      >
        <DialogHeader>
          <DialogTitle>{node.title} · 批量创作</DialogTitle>
          <DialogDescription>
            每行独立提示词；参考列按顺序表达人物、服装或其他素材关系。保存后可继续生成。
          </DialogDescription>
        </DialogHeader>
        <div className="grid gap-4 sm:grid-cols-[160px_130px_1fr]">
          <Field>
            <FieldLabel>创作方式</FieldLabel>
            <Select
              value={draft.operation}
              disabled={disabled}
              onValueChange={(operation: BatchTableConfig["operation"]) =>
                setDraft((value) => ({ ...value, operation }))
              }
            >
              <SelectTrigger aria-label="批量创作方式">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="creative">创意图片</SelectItem>
                <SelectItem value="try_on">批量换装</SelectItem>
              </SelectContent>
            </Select>
          </Field>
          <Field>
            <FieldLabel htmlFor="batch-concurrency">并行任务</FieldLabel>
            <Input
              id="batch-concurrency"
              type="number"
              min={1}
              max={32}
              value={draft.concurrency}
              disabled={disabled}
              onChange={(event) =>
                setDraft((value) => ({
                  ...value,
                  concurrency: Number(event.target.value),
                }))
              }
            />
          </Field>
          <Field>
            <FieldLabel htmlFor="batch-global-prompt">
              统一提示词（留空时使用每行内容）
            </FieldLabel>
            <Textarea
              id="batch-global-prompt"
              maxLength={10000}
              rows={2}
              value={draft.globalPrompt}
              disabled={disabled}
              onChange={(event) =>
                setDraft((value) => ({
                  ...value,
                  globalPrompt: event.target.value,
                }))
              }
            />
          </Field>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <Button
            variant="outline"
            disabled={disabled || draft.rows.length >= 500}
            onClick={() =>
              setDraft((value) => ({
                ...value,
                rows: [...value.rows, createBatchRow(value)],
              }))
            }
          >
            <Plus />
            添加一行
          </Button>
          <Button
            variant="outline"
            disabled={disabled || !images.length}
            onClick={() => {
              try {
                setDraft((value) =>
                  synchronizeBatchRows(
                    value,
                    value.referenceColumns.map((_, index) =>
                      index === 0 ? images.map((image) => image.id) : [],
                    ),
                  ),
                );
              } catch {
                setError("素材超过 500 行，请分批加入。");
              }
            }}
          >
            画布图片加入首列
          </Button>
          <Button
            variant="outline"
            disabled={disabled || draft.referenceColumns.length >= 6}
            onClick={() =>
              setDraft((value) => ({
                ...value,
                referenceColumns: [
                  ...value.referenceColumns,
                  {
                    id: crypto.randomUUID(),
                    label: `参考图 ${value.referenceColumns.length + 1}`,
                  },
                ],
                rows: value.rows.map((row) => ({
                  ...row,
                  inputNodeIds: [...row.inputNodeIds, null],
                })),
              }))
            }
          >
            增加参考列
          </Button>
          <Button
            variant="ghost"
            disabled={disabled || draft.referenceColumns.length <= 1}
            onClick={() =>
              setDraft((value) => ({
                ...value,
                referenceColumns: value.referenceColumns.slice(0, -1),
                rows: value.rows.map((row) => ({
                  ...row,
                  inputNodeIds: row.inputNodeIds.slice(0, -1),
                })),
              }))
            }
          >
            移除末列
          </Button>
          <span className="ml-auto text-xs text-muted-foreground">
            {draft.rows.filter((row) => row.enabled).length} /{" "}
            {draft.rows.length} 行启用
          </span>
        </div>
        <div
          ref={scroll}
          className="min-h-40 flex-1 overflow-auto rounded-md border"
        >
          <Table className="min-w-[950px] table-fixed">
            <TableHeader className="sticky top-0 z-10 bg-background">
              <TableRow>
                <TableHead className="w-16">
                  <Checkbox
                    checked={
                      draft.rows.length > 0 &&
                      draft.rows.every((row) => row.enabled)
                    }
                    disabled={disabled || !draft.rows.length}
                    aria-label="启用所有行"
                    onCheckedChange={(enabled) =>
                      setDraft((value) => ({
                        ...value,
                        rows: value.rows.map((row) => ({
                          ...row,
                          enabled: enabled === true,
                        })),
                      }))
                    }
                  />
                </TableHead>
                {draft.referenceColumns.map((column, index) => (
                  <TableHead
                    key={column.id}
                    className="w-44"
                    draggable={!disabled}
                    onDragStart={() => {
                      draggedColumn.current = column.id;
                    }}
                    onDragEnd={() => {
                      draggedColumn.current = null;
                    }}
                    onDragOver={(event) => {
                      if (draggedColumn.current) event.preventDefault();
                    }}
                    onDrop={(event) => {
                      event.preventDefault();
                      const from = draggedColumn.current;
                      if (from && !disabled)
                        setDraft((value) =>
                          reorderBatchColumns(value, from, column.id),
                        );
                      draggedColumn.current = null;
                    }}
                  >
                    <div className="flex items-center gap-1">
                      <span>{column.label}</span>
                      <Button
                        size="icon"
                        variant="ghost"
                        aria-label={`左移${column.label}`}
                        disabled={disabled || index === 0}
                        onClick={() =>
                          setDraft((value) =>
                            reorderBatchColumns(
                              value,
                              column.id,
                              value.referenceColumns[index - 1].id,
                            ),
                          )
                        }
                      >
                        <ArrowLeft />
                      </Button>
                      <Button
                        size="icon"
                        variant="ghost"
                        aria-label={`右移${column.label}`}
                        disabled={
                          disabled ||
                          index === draft.referenceColumns.length - 1
                        }
                        onClick={() =>
                          setDraft((value) =>
                            reorderBatchColumns(
                              value,
                              column.id,
                              value.referenceColumns[index + 1].id,
                            ),
                          )
                        }
                      >
                        <ArrowRight />
                      </Button>
                    </div>
                  </TableHead>
                ))}
                <TableHead className="min-w-64">行提示词</TableHead>
                <TableHead className="min-w-40">运行结果</TableHead>
                <TableHead className="w-24">操作</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {virtualRows[0]?.start ? (
                <TableRow aria-hidden>
                  <TableCell
                    colSpan={draft.referenceColumns.length + 4}
                    style={{ height: virtualRows[0].start, padding: 0 }}
                  />
                </TableRow>
              ) : null}
              {virtualRows.map((item) => {
                const row = draft.rows[item.index];
                return (
                  <TableRow
                    key={row.id}
                    data-index={item.index}
                    ref={virtual.measureElement}
                  >
                    <TableCell>
                      <Checkbox
                        aria-label={`启用第 ${item.index + 1} 行`}
                        disabled={disabled}
                        checked={row.enabled}
                        onCheckedChange={(enabled) =>
                          rowUpdate(row.id, { enabled: enabled === true })
                        }
                      />
                      <span className="mt-2 block text-xs text-muted-foreground">
                        {item.index + 1}
                      </span>
                    </TableCell>
                    {draft.referenceColumns.map((column, index) => (
                      <TableCell
                        key={column.id}
                        draggable={
                          !disabled && Boolean(row.inputNodeIds[index])
                        }
                        onDragStart={() => {
                          draggedReference.current = {
                            rowId: row.id,
                            column: index,
                          };
                        }}
                        onDragEnd={() => {
                          draggedReference.current = null;
                        }}
                        onDragOver={(event) => {
                          if (draggedReference.current) event.preventDefault();
                        }}
                        onDrop={(event) => {
                          event.preventDefault();
                          const source = draggedReference.current;
                          if (source && !disabled)
                            setDraft((value) =>
                              swapBatchReferences(
                                value,
                                source.rowId,
                                source.column,
                                row.id,
                                index,
                              ),
                            );
                          draggedReference.current = null;
                        }}
                      >
                        <Select
                          disabled={disabled}
                          value={row.inputNodeIds[index] ?? "empty"}
                          onValueChange={(reference) =>
                            rowUpdate(row.id, {
                              inputNodeIds: row.inputNodeIds.map(
                                (current, slot) =>
                                  slot === index
                                    ? reference === "empty"
                                      ? null
                                      : reference
                                    : current,
                              ),
                            })
                          }
                        >
                          <SelectTrigger
                            aria-label={`第 ${item.index + 1} 行${column.label}`}
                          >
                            <SelectValue placeholder="选择图片" />
                          </SelectTrigger>
                          <SelectContent>
                            <SelectItem value="empty">未选择</SelectItem>
                            {row.inputNodeIds[index] &&
                            !nodeTitles.has(row.inputNodeIds[index]!) ? (
                              <SelectItem value={row.inputNodeIds[index]!}>
                                素材已移除
                              </SelectItem>
                            ) : null}
                            {images.map((image) => (
                              <SelectItem key={image.id} value={image.id}>
                                {image.title}
                              </SelectItem>
                            ))}
                          </SelectContent>
                        </Select>
                      </TableCell>
                    ))}
                    <TableCell>
                      <Textarea
                        rows={3}
                        aria-label={`第 ${item.index + 1} 行提示词`}
                        maxLength={10000}
                        disabled={disabled}
                        value={row.prompt}
                        onChange={(event) =>
                          rowUpdate(row.id, { prompt: event.target.value })
                        }
                      />
                    </TableCell>
                    <TableCell>
                      <BatchRowTask
                        projectId={projectId}
                        task={byRow.get(row.id)}
                        disabled={disabled}
                        enabled={row.enabled}
                        onRetry={() => setRetryRowId(row.id)}
                        onResults={onResults}
                      />
                    </TableCell>
                    <TableCell>
                      <div className="flex">
                        <Button
                          variant="ghost"
                          size="icon"
                          aria-label={`复制第 ${item.index + 1} 行`}
                          disabled={disabled || draft.rows.length >= 500}
                          onClick={() =>
                            setDraft((value) => ({
                              ...value,
                              rows: [
                                ...value.rows.slice(0, item.index + 1),
                                {
                                  ...row,
                                  id: crypto.randomUUID(),
                                  inputNodeIds: [...row.inputNodeIds],
                                },
                                ...value.rows.slice(item.index + 1),
                              ],
                            }))
                          }
                        >
                          <Copy />
                        </Button>
                        <Button
                          variant="ghost"
                          size="icon"
                          aria-label={`删除第 ${item.index + 1} 行`}
                          disabled={disabled}
                          onClick={() =>
                            setDraft((value) => ({
                              ...value,
                              rows: value.rows.filter(
                                (current) => current.id !== row.id,
                              ),
                            }))
                          }
                        >
                          <Trash2 />
                        </Button>
                      </div>
                    </TableCell>
                  </TableRow>
                );
              })}
              {virtualRows.length ? (
                <TableRow aria-hidden>
                  <TableCell
                    colSpan={draft.referenceColumns.length + 4}
                    style={{
                      height: virtual.getTotalSize() - virtualRows.at(-1)!.end,
                      padding: 0,
                    }}
                  />
                </TableRow>
              ) : (
                <TableRow>
                  <TableCell
                    colSpan={draft.referenceColumns.length + 4}
                    className="h-32 text-center text-muted-foreground"
                  >
                    添加行或从画布加入图片。
                  </TableCell>
                </TableRow>
              )}
            </TableBody>
          </Table>
        </div>
        {error ? (
          <p role="alert" className="text-sm text-destructive">
            {error}
          </p>
        ) : null}
        <div className="flex shrink-0 flex-wrap gap-2 text-xs text-muted-foreground">
          {tasks.isError ? <p role="alert">行任务读取失败。</p> : null}
          <Button
            size="sm"
            variant="ghost"
            disabled={tasks.isFetching}
            onClick={() => void tasks.refetch()}
          >
            刷新行任务
          </Button>
          {tasks.hasNextPage ? (
            <Button
              size="sm"
              variant="ghost"
              disabled={tasks.isFetchingNextPage}
              onClick={() => void tasks.fetchNextPage()}
            >
              加载更早任务
            </Button>
          ) : null}
        </div>
        <BatchGenerationPanel
          retryRowId={retryRowId}
          onRetryCleared={() => setRetryRowId(undefined)}
          source={source}
          projectId={projectId}
          config={draft}
          nodes={nodes}
          disabled={readOnly || saving}
          onChange={setDraft}
          onPersist={onSave}
          onBusy={setGenerationBusy}
        />
        <DialogFooter>
          <Button variant="outline" disabled={saving} onClick={onClose}>
            关闭
          </Button>
          <Button disabled={disabled} onClick={save}>
            {saving ? "正在保存…" : "保存表格"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
