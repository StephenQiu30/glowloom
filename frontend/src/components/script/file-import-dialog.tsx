"use client";
import { useId, useState } from "react";
import { useInfiniteQuery, useQuery } from "@tanstack/react-query";
import { Controller, useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Label } from "@/components/ui/label";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import type { ScriptScope } from "./source-intent";
import type { DocumentAsset } from "./document-media";
import {
  fileImportScopeKey,
  getFileImport,
  listFileImports,
} from "./file-import-queries";
import { fileImportActions, type FileImportJob } from "./file-import-model";
import type { useFileImport } from "./use-file-import";
import { FileImportConflict, FileImportRecovery } from "./file-import-recovery";
import { SourceExtractionWarnings } from "./source-extraction-warnings";
type Writer = ReturnType<typeof useFileImport>;
const statuses: Record<FileImportJob["status"], string> = {
  queued: "排队",
  running: "处理中",
  partial: "部分成功",
  failed: "失败",
  cancel_requested: "已请求取消",
  succeeded: "已完成",
  cancelled: "已取消",
};
const stages: Record<FileImportJob["stage"], string> = {
  queued: "排队",
  extracting: "提取正文",
  normalizing: "归一正文",
  storing: "保存正文对象",
  committing: "发布版本",
  completed: "本次尝试结束",
  failed: "本次尝试失败",
  cancelling: "取消处理中",
  awaiting_reconciliation: "等待显式核验",
};
export function FileImportDialog({
  scope,
  writer,
  onClose,
  onReadCurrent,
  initialJobId,
  admissionConfirmed,
}: {
  scope: ScriptScope;
  writer: Writer;
  onClose: () => void;
  onReadCurrent: () => void;
  initialJobId?: string;
  admissionConfirmed?: boolean;
}) {
  const [selected, setSelected] = useState<string | undefined>(initialJobId);
  const list = useInfiniteQuery({
    queryKey: [...fileImportScopeKey(scope), "list"],
    initialPageParam: 0,
    queryFn: ({ pageParam, signal }) =>
      listFileImports(scope, pageParam, signal),
    getNextPageParam: (page) => page.next_after,
    staleTime: 0,
  });
  const original = writer.intent ?? writer.rejected?.intent;
  const focused = original
    ? original.action === "create"
      ? undefined
      : original.jobId
    : selected;
  const detail = useQuery({
    queryKey: [...fileImportScopeKey(scope), "job", focused],
    queryFn: ({ signal }) => getFileImport(scope, focused!, signal),
    enabled: Boolean(focused),
    staleTime: 0,
    refetchInterval: (query) =>
      query.state.data &&
      ["queued", "running", "cancel_requested"].includes(
        query.state.data.status,
      )
        ? 3000
        : false,
  });
  const items = list.data?.pages.flatMap((page) => page.items) ?? [];
  const duplicated = new Set(items.map((job) => job.id)).size !== items.length;
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !writer.locked) onClose();
      }}
    >
      <DialogContent
        showCloseButton={!writer.locked}
        className="max-h-[calc(100dvh-2rem)] overflow-y-auto sm:max-w-5xl"
        onEscapeKeyDown={(event) => {
          if (writer.locked) event.preventDefault();
        }}
        onInteractOutside={(event) => {
          if (writer.locked) event.preventDefault();
        }}
      >
        <DialogHeader>
          <DialogTitle>文件导入任务</DialogTitle>
          <DialogDescription>
            成功原件按冻结顺序发布一版草稿。部分失败只重试失败原件，已发布来源继续保留；取消仅影响尚未发布工作。受理回执不表示已经停止或清理。
          </DialogDescription>
        </DialogHeader>
        {admissionConfirmed && (
          <p role="status">
            原请求已受理，请读取实际结果；永久回执不会覆盖当前任务事实。
          </p>
        )}
        <FileImportRecovery writer={writer} />
        <FileImportConflict
          key={writer.rejected?.intent.key}
          scope={scope}
          writer={writer}
        />
        <div className="grid min-w-0 gap-4 md:grid-cols-[280px_minmax(0,1fr)]">
          <section className="min-w-0 space-y-2" aria-label="导入任务列表">
            {list.isPending && <p role="status">正在读取导入任务…</p>}
            {(list.error || duplicated) && (
              <p role="alert">
                {duplicated
                  ? "任务分页出现重复身份，请重新读取。"
                  : list.error?.message}
              </p>
            )}
            {!list.isPending && !items.length && !list.isError && (
              <p>尚无文件导入任务。</p>
            )}
            <ul className="max-h-96 space-y-2 overflow-y-auto">
              {!duplicated &&
                items.map((job) => (
                  <li key={job.id}>
                    <Button
                      type="button"
                      variant={focused === job.id ? "secondary" : "outline"}
                      className="h-auto w-full justify-start text-left whitespace-normal"
                      aria-label={`选择导入任务 ${job.id}`}
                      aria-current={focused === job.id ? "true" : undefined}
                      disabled={writer.locked}
                      onClick={() => setSelected(job.id)}
                    >
                      <span className="min-w-0 space-y-1">
                        <span className="block break-all">{job.id}</span>
                        <span className="block">
                          {statuses[job.status]} · 尝试 {job.attempt} · 修订{" "}
                          {job.revision}
                        </span>
                        <span className="block">
                          原件 {job.files.length} 个
                        </span>
                      </span>
                    </Button>
                  </li>
                ))}
            </ul>
            <Button
              type="button"
              variant="outline"
              disabled={list.isFetching}
              onClick={() => void list.refetch()}
            >
              重新读取导入任务
            </Button>
            {list.hasNextPage && (
              <Button
                type="button"
                variant="outline"
                disabled={list.isFetchingNextPage}
                onClick={() => void list.fetchNextPage()}
              >
                读取导入任务下一页
              </Button>
            )}
          </section>
          <section className="min-w-0 space-y-3" aria-label="导入任务实际详情">
            {!focused && <p>选择任务后读取实际阶段与每份原件结果。</p>}
            {focused && detail.isPending && (
              <p role="status">正在读取选中导入任务…</p>
            )}
            {detail.error && <p role="alert">{detail.error.message}</p>}
            {focused && (
              <Button
                type="button"
                variant="outline"
                disabled={detail.isFetching}
                onClick={() => void detail.refetch()}
              >
                重新读取选中任务
              </Button>
            )}
            {detail.data && <ImportFacts job={detail.data} writer={writer} />}
            {detail.data?.files.some((file) => file.source_id) && (
              <Button
                type="button"
                variant="outline"
                disabled={writer.locked}
                onClick={onReadCurrent}
              >
                读取最新脚本事实
              </Button>
            )}
          </section>
        </div>
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            disabled={writer.locked}
            onClick={onClose}
          >
            关闭文件导入任务
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
function ImportFacts({ job, writer }: { job: FileImportJob; writer: Writer }) {
  const actions = fileImportActions(job);
  const published = job.files.filter((file) => file.source_id).length;
  return (
    <>
      <p>
        {statuses[job.status]} · {stages[job.stage]} · 尝试 {job.attempt} · 修订{" "}
        {job.revision}
      </p>
      <p>
        已发布 {published} / {job.files.length} 个原件
      </p>
      <p>
        已提取 {job.files.filter((file) => file.status === "succeeded").length}
        ；失败 {job.files.filter((file) => file.status === "failed").length}。
      </p>
      {job.latest_version_id && (
        <p className="text-xs break-all">
          最近脚本版本 {job.latest_version_id} · 脚本修订{" "}
          {job.latest_script_revision}
        </p>
      )}
      {job.active_io && (
        <p>当前有实际输入输出所有者；取消受理后仍需读取真实停止与清理结果。</p>
      )}
      {job.needs_reconciliation && (
        <p>执行或对象事实尚未核验，请保留原任务并明确请求核验。</p>
      )}
      {job.cancellation_requested && (
        <p>取消意图已记录；已发布的成功来源保持保留。</p>
      )}
      {job.reconciliation_requested && <p>核验意图已受理，等待当前事实。</p>}
      {job.failure_code && (
        <p role="alert" className="break-all">
          {job.failure_code}
        </p>
      )}
      <ol className="max-h-96 space-y-3 overflow-y-auto">
        {job.files.map((file) => (
          <li key={file.asset_id} className="space-y-1 rounded-lg border p-3">
            <p className="break-words">
              {file.position + 1}. {file.file_name} ·{" "}
              {file.status === "queued"
                ? "等待处理"
                : file.status === "failed"
                  ? "失败"
                  : file.source_id
                    ? "已发布"
                    : "已提取，尚未发布"}{" "}
              · 尝试 {file.attempt}
            </p>
            <p className="text-xs break-all">原件 {file.asset_id}</p>
            {file.source_id && (
              <p className="text-xs break-all">
                来源快照 {file.source_id}；稳定身份 {file.source_lineage_id}
              </p>
            )}
            {file.failure_code && (
              <p role="alert" className="break-all">
                {file.failure_code}
              </p>
            )}
            <SourceExtractionWarnings warnings={file.warnings} />
          </li>
        ))}
      </ol>
      <div className="flex flex-wrap gap-2">
        {actions.cancel && (
          <Button
            type="button"
            variant="outline"
            disabled={writer.locked}
            onClick={() =>
              void writer.submit({
                action: "cancel",
                jobId: job.id,
                body: { expected_revision: job.revision },
              })
            }
          >
            请求取消未发布工作
          </Button>
        )}
        {actions.retry && (
          <Button
            type="button"
            disabled={writer.locked}
            onClick={() =>
              void writer.submit({
                action: "retry",
                jobId: job.id,
                body: { expected_revision: job.revision },
              })
            }
          >
            仅重试失败原件
          </Button>
        )}
        {actions.reconcile && (
          <Button
            type="button"
            variant="outline"
            disabled={writer.locked}
            onClick={() =>
              void writer.submit({
                action: "reconcile",
                jobId: job.id,
                body: { expected_revision: job.revision },
              })
            }
          >
            明确核验原导入任务
          </Button>
        )}
      </div>
    </>
  );
}
const rightsSchema = z.object({
  rights: z.boolean().refine((value) => value, "请确认所有原件均有使用权限。"),
});
export function FileImportCreateDialog({
  scope,
  assets,
  base,
  writer,
  onClose,
  onRebase,
}: {
  scope: ScriptScope;
  assets: DocumentAsset[];
  base: { expected_revision: number; base_version_id?: string };
  writer: Writer;
  onClose: () => void;
  onRebase: (base: {
    expected_revision: number;
    base_version_id?: string;
  }) => void;
}) {
  const id = useId();
  const form = useForm({
    resolver: zodResolver(rightsSchema),
    defaultValues: { rights: false },
  });
  const unavailable = writer.locked || form.formState.isSubmitting;
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !writer.locked) onClose();
      }}
    >
      <DialogContent
        showCloseButton={!writer.locked}
        className="max-h-[calc(100dvh-2rem)] overflow-y-auto sm:max-w-2xl"
        onEscapeKeyDown={(event) => {
          if (writer.locked) event.preventDefault();
        }}
        onInteractOutside={(event) => {
          if (writer.locked) event.preventDefault();
        }}
      >
        <DialogHeader>
          <DialogTitle>创建文件导入任务</DialogTitle>
          <DialogDescription>
            所选原件顺序与脚本修订 {base.expected_revision}{" "}
            已冻结。原件继续保留；本次成功文件一次发布草稿，失败文件可另明确重试。
          </DialogDescription>
        </DialogHeader>
        <FileImportRecovery writer={writer} />
        <FileImportConflict
          key={writer.rejected?.intent.key}
          scope={scope}
          writer={writer}
          onRebase={onRebase}
          onDiscard={onClose}
        />
        <ol className="max-h-64 space-y-2 overflow-y-auto">
          {assets.map((asset, index) => (
            <li key={asset.id} className="rounded-lg border p-2 text-sm">
              <p className="break-words">
                {index + 1}. {asset.file_name} ·{" "}
                {asset.byte_size.toLocaleString("zh-CN")} 字节 · 修订{" "}
                {asset.revision}
              </p>
              <p className="text-xs break-all">{asset.id}</p>
            </li>
          ))}
        </ol>
        <form
          className="space-y-3"
          onSubmit={form.handleSubmit(async () => {
            if (!unavailable)
              await writer.submit({
                action: "create",
                body: {
                  ...base,
                  rights_confirmed: true,
                  asset_ids: assets.map((asset) => asset.id),
                },
              });
          })}
        >
          <fieldset disabled={unavailable} className="space-y-3">
            <div className="flex items-start gap-2">
              <Controller
                name="rights"
                control={form.control}
                render={({ field }) => (
                  <Checkbox
                    id={`${id}-rights`}
                    checked={field.value}
                    onCheckedChange={(value) => field.onChange(value === true)}
                  />
                )}
              />
              <Label htmlFor={`${id}-rights`}>已获授权使用所选全部原件</Label>
            </div>
            {form.formState.errors.rights && (
              <p role="alert">{form.formState.errors.rights.message}</p>
            )}
            <Button type="submit">确认创建导入任务</Button>
          </fieldset>
        </form>
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            disabled={writer.locked}
            onClick={onClose}
          >
            放弃本地选择并关闭
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
