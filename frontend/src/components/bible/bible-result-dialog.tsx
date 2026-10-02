"use client";
import { useId, useState } from "react";
import { useInfiniteQuery, useQuery } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Label } from "@/components/ui/label";
import { BibleWriteDialog } from "./bible-entry-dialog";
import {
  bibleScopeKey,
  getBibleResult,
  getBibleResults,
} from "./bible-queries";
import { kindLabels, type BibleIdentity, type BibleKind } from "./bible-model";
import type { BibleWriter } from "./use-bible-writer";
export function BibleResultDialog({
  identity,
  kind,
  id: entryId,
  revision: initialRevision,
  writer,
  onClose,
  onCloseAutoFocus,
}: {
  identity: BibleIdentity;
  kind: BibleKind;
  id?: string;
  revision: number;
  writer: BibleWriter;
  onClose: () => void;
  onCloseAutoFocus?: (event: Event) => void;
}) {
  const id = useId(),
    [revision, setRevision] = useState(initialRevision),
    [operation, setOperation] = useState(""),
    [output, setOutput] = useState(""),
    [reviewed, setReviewed] = useState(false);
  const tasks = useInfiniteQuery({
    queryKey: [...bibleScopeKey(identity), "results"],
    queryFn: ({ signal, pageParam }) =>
      getBibleResults(identity, pageParam, signal),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (page) => page.next_cursor ?? undefined,
    staleTime: 0,
    gcTime: 0,
    retry: false,
  });
  const detail = useQuery({
    queryKey: [...bibleScopeKey(identity), "result", operation],
    queryFn: ({ signal }) => getBibleResult(identity, operation, signal),
    enabled: Boolean(operation),
    staleTime: 0,
    gcTime: 0,
    retry: false,
    refetchOnWindowFocus: false,
  });
  const outputs =
    detail.isSuccess && detail.isFetchedAfterMount
      ? detail.data.outputs.filter(
          (item) => item.kind === "text" && item.moderation_status === "passed",
        )
      : [];
  async function prepare() {
    const latest = writer.latest;
    if (!latest || (entryId && latest.detail?.head.id !== entryId)) return;
    if (await writer.discardRejected()) {
      setRevision(latest.detail?.head.revision ?? 0);
      setReviewed(false);
    }
  }
  return (
    <BibleWriteDialog
      title={`${entryId ? "采纳到当前" : "从正式结果新建"}${kindLabels[kind]}`}
      writer={writer}
      dirty={Boolean(operation || output)}
      onClose={onClose}
      onCloseAutoFocus={onCloseAutoFocus}
      onPrepareLatest={prepare}
    >
      <p>
        当前列表是待后端核验的已完成文本结果候选。通过审核文本不等于设定集抽取；服务端仍须核验实际任务用途、冻结来源、结构化内容与输入/输出SHA，核验依赖未接入时无法采纳。
      </p>
      <p>
        原设定身份 {entryId ?? "尚未创建"} · 冻结版本 {revision}
      </p>
      <Label htmlFor={`${id}-task`}>待核验的已完成任务</Label>
      <select
        id={`${id}-task`}
        className="h-10 w-full rounded-md bg-muted px-2"
        disabled={writer.locked}
        value={operation}
        onChange={(event) => {
          setOperation(event.target.value);
          setOutput("");
          setReviewed(false);
        }}
      >
        <option value="">选择实际任务候选</option>
        {tasks.data?.pages
          .flatMap((page) => page.items)
          .map((task) => (
            <option key={task.id} value={task.id}>
              {task.model_name} · {task.id} · {task.finished_at}
            </option>
          ))}
      </select>
      {tasks.hasNextPage && (
        <Button
          variant="outline"
          disabled={writer.locked || tasks.isFetchingNextPage}
          onClick={() => void tasks.fetchNextPage()}
        >
          读取更多正式结果任务
        </Button>
      )}
      {tasks.isError && (
        <div>
          <p role="alert">正式任务无法读取。</p>
          <Button variant="outline" onClick={() => void tasks.refetch()}>
            重试结果任务列表
          </Button>
        </div>
      )}
      {tasks.isSuccess &&
        !tasks.data.pages.some((page) => page.items.length) && (
          <p>当前项目尚无已完成的正式文本结果。</p>
        )}
      <Label htmlFor={`${id}-output`}>通过审核的文本候选</Label>
      <select
        id={`${id}-output`}
        className="h-10 w-full rounded-md bg-muted px-2"
        disabled={writer.locked || !outputs.length}
        value={output}
        onChange={(event) => {
          setOutput(event.target.value);
          setReviewed(false);
        }}
      >
        <option value="">选择已完成输出</option>
        {outputs.map((item) => (
          <option key={item.id} value={item.id}>
            输出{item.sequence} · {item.id}
          </option>
        ))}
      </select>
      {detail.isError && <p role="alert">选中任务当前已完成事实无法读取。</p>}
      <div className="flex items-start gap-2">
        <Checkbox
          id={`${id}-review`}
          checked={reviewed}
          disabled={
            writer.locked || !outputs.some((item) => item.id === output)
          }
          onCheckedChange={(value) => setReviewed(value === true)}
        />
        <Label htmlFor={`${id}-review`}>
          我已确认选中正式结果用于当前设定；采用后产生独立不可变版本。
        </Label>
      </div>
      <Button
        disabled={
          writer.locked ||
          !reviewed ||
          !outputs.some((item) => item.id === output)
        }
        onClick={() =>
          void writer.submit({
            action: entryId ? "adopt_result" : "create_result",
            kind,
            ...(entryId ? { id: entryId } : {}),
            body: {
              expected_revision: revision,
              operation_id: operation,
              output_id: output,
            },
          })
        }
      >
        明确采纳选中正式结果
      </Button>
    </BibleWriteDialog>
  );
}
