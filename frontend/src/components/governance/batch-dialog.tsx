"use client";
import { useId, useState } from "react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Field, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Spinner } from "@/components/ui/spinner";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "@/components/ui/dialog";
import {
  DataTable,
  type DataTableColumn,
} from "@/components/canvas/runtime/components/account/list/data-table";
import type { BatchEntry, BatchStatus } from "./batch-execution";
import type { useBatchOperation } from "./use-batch-operation";
const labels: Record<BatchStatus, string> = {
  ready: "将执行",
  running: "处理中",
  success: "成功",
  submitted: "已提交",
  failed: "失败",
  unknown: "结果未知",
  skipped: "不执行",
};
export function BatchDialog<T extends { id?: string }>({
  batch,
}: {
  batch: Omit<ReturnType<typeof useBatchOperation<T>>, "registerResultTrigger">;
}) {
  const id = useId();
  const [reason, setReason] = useState("");
  const [password, setPassword] = useState("");
  const ready = batch.entries.filter((e) => e.status === "ready").length;
  const failed = batch.entries.filter((e) => e.status === "failed").length;
  const columns: DataTableColumn<BatchEntry<T>>[] = [
    {
      id: "target",
      header: "目标",
      cell: ({ row }) => (
        <div className="flex min-w-0 flex-col gap-1">
          <span className="break-all whitespace-normal">
            {row.original.label}
          </span>
          <span className="text-xs break-words whitespace-normal text-muted-foreground">
            {row.original.message}
          </span>
        </div>
      ),
    },
    {
      id: "status",
      header: "结果",
      cell: ({ row }) => (
        <Badge variant="secondary">{labels[row.original.status]}</Badge>
      ),
    },
  ];
  return (
    <Dialog
      open={batch.open}
      onOpenChange={(open) => {
        batch.setOpen(open);
        if (!open) setPassword("");
      }}
    >
      <DialogContent
        className="flex max-h-[calc(100dvh-2rem)] flex-col overflow-hidden sm:max-w-2xl"
        onCloseAutoFocus={(event) => {
          if (batch.restoreFocus()) event.preventDefault();
        }}
      >
        <DialogHeader>
          <DialogTitle>{batch.action?.title}</DialogTitle>
          <DialogDescription>{batch.action?.effect}</DialogDescription>
        </DialogHeader>
        <div className="flex min-h-0 flex-col gap-4 overflow-y-auto">
          <p aria-live="polite" className="text-sm text-muted-foreground">
            {batch.busy ? <Spinner className="mr-2 inline" /> : null}已选{" "}
            {batch.entries.length} 项 ·{" "}
            {batch.submitted
              ? `成功 ${batch.entries.filter((e) => e.status === "success").length} / 已提交 ${batch.entries.filter((e) => e.status === "submitted").length} / 失败 ${failed} / 结果未知 ${batch.entries.filter((e) => e.status === "unknown").length} / 未执行 ${batch.entries.filter((e) => e.status === "skipped").length}`
              : `可执行 ${ready} 项`}
          </p>
          <DataTable
            label="批量操作逐项结果"
            columns={columns}
            data={batch.entries}
            loading={batch.busy && !batch.entries.length}
          />
          {batch.error ? (
            <Alert variant="destructive">
              <AlertDescription>{batch.error}</AlertDescription>
            </Alert>
          ) : null}
          <form
            id={id}
            onSubmit={(event) => {
              event.preventDefault();
              void batch.run(reason, password, batch.unresolved);
              setPassword("");
            }}
          >
            <FieldGroup>
              {!batch.submitted && batch.action?.reason ? (
                <Field>
                  <FieldLabel htmlFor={`${id}-reason`}>操作原因</FieldLabel>
                  <Textarea
                    id={`${id}-reason`}
                    value={reason}
                    onChange={(e) => setReason(e.target.value)}
                    required
                    minLength={5}
                    maxLength={500}
                    disabled={batch.busy}
                  />
                </Field>
              ) : null}
              {batch.action?.password &&
              (!batch.submitted || batch.unresolved) ? (
                <Field>
                  <FieldLabel htmlFor={`${id}-password`}>
                    当前管理员密码
                  </FieldLabel>
                  <Input
                    id={`${id}-password`}
                    type="password"
                    autoComplete="current-password"
                    value={password}
                    onChange={(e) => setPassword(e.target.value)}
                    required
                    disabled={batch.busy}
                  />
                </Field>
              ) : null}
            </FieldGroup>
          </form>
          {batch.submitted ? (
            <p className="text-xs text-muted-foreground">
              成功项已移出选择。结果未知时先查回执，恢复使用原请求标识；失败项重新读取并确认后再执行。
            </p>
          ) : null}
        </div>
        <DialogFooter className="flex-wrap">
          <Button
            variant="outline"
            onClick={() => {
              batch.setOpen(false);
              setPassword("");
            }}
          >
            {batch.busy ? "关闭并稍后查看" : "关闭"}
          </Button>
          {batch.unresolved && batch.action?.recover ? (
            <Button
              variant="outline"
              disabled={batch.busy}
              onClick={() => void batch.recover()}
            >
              查询原请求
            </Button>
          ) : null}
          {failed && !batch.unresolved ? (
            <Button
              variant="outline"
              disabled={batch.busy}
              onClick={() => {
                setReason("");
                void batch.retry();
              }}
            >
              重新读取失败项（{failed}）
            </Button>
          ) : null}
          {!batch.submitted || batch.unresolved ? (
            <Button
              form={id}
              type="submit"
              variant={batch.action?.destructive ? "destructive" : "default"}
              disabled={
                batch.busy ||
                (!ready && !batch.unresolved) ||
                (batch.action?.password && !password) ||
                (!batch.submitted &&
                  batch.action?.reason &&
                  reason.trim().length < 5)
              }
            >
              {batch.unresolved
                ? "使用原请求恢复"
                : `确认${batch.action?.title ?? "操作"}（${ready}）`}
            </Button>
          ) : null}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
