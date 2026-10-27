"use client";
import { useId, useState } from "react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Field, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Spinner } from "@/components/ui/spinner";
import { cn } from "@/lib/utils";
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
import {
  AlertDialog,
  AlertDialogContent,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogCancel,
} from "@/components/ui/alert-dialog";
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
  batch: Omit<
    ReturnType<typeof useBatchOperation<T>>,
    "registerResultTrigger" | "registerFallbackTrigger"
  >;
}) {
  const id = useId();
  const [reason, setReason] = useState("");
  const [password, setPassword] = useState("");
  const ready = batch.entries.filter((e) => e.status === "ready").length;
  const failed = batch.entries.filter((e) => e.status === "failed").length;
  const compact = batch.action?.compact;
  const Root = compact ? AlertDialog : Dialog;
  const Content = compact ? AlertDialogContent : DialogContent;
  const Header = compact ? AlertDialogHeader : DialogHeader;
  const Title = compact ? AlertDialogTitle : DialogTitle;
  const Description = compact ? AlertDialogDescription : DialogDescription;
  const Footer = compact ? AlertDialogFooter : DialogFooter;
  const Cancel = compact ? AlertDialogCancel : Button;
  const success = batch.entries.filter((e) => e.status === "success").length;
  const skipped = batch.entries.filter((e) => e.status === "skipped").length;
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
    <Root
      open={batch.open}
      onOpenChange={(open) => {
        batch.setOpen(open);
        if (!open) setPassword("");
      }}
    >
      <Content
        className={cn(
          "flex max-h-[calc(100dvh-2rem)] flex-col overflow-hidden",
          compact ? "sm:max-w-md" : "sm:max-w-2xl",
        )}
        onCloseAutoFocus={(event) => {
          if (batch.restoreFocus()) event.preventDefault();
        }}
      >
        <Header>
          <Title>
            {compact
              ? batch.busy || batch.submitted
                ? batch.action?.title
                : ready
                  ? `${batch.action?.title.replace("账号", "")} ${ready} 个账号？`
                  : `没有可${batch.action?.title.replace("账号", "")}的账号`
              : batch.action?.title}
          </Title>
          <Description>{batch.action?.effect}</Description>
        </Header>
        <div className="flex min-h-0 flex-col gap-4 overflow-y-auto">
          {!compact || batch.busy || skipped > 0 ? (
            <p aria-live="polite" className="text-sm text-muted-foreground">
              {batch.busy ? <Spinner className="mr-2 inline" /> : null}
              {compact
                ? batch.busy
                  ? batch.submitted
                    ? "正在处理…"
                    : "正在检查…"
                  : `已跳过 ${skipped} 项`
                : batch.submitted
                  ? `成功 ${success} / 已提交 ${batch.entries.filter((e) => e.status === "submitted").length} / 失败 ${failed} / 结果未知 ${batch.entries.filter((e) => e.status === "unknown").length} / 未执行 ${skipped}`
                  : `已选 ${batch.entries.length} 项 · 可执行 ${ready} 项${skipped ? ` · 跳过 ${skipped} 项` : ""}${failed ? ` · 检查失败 ${failed} 项` : ""}`}
            </p>
          ) : null}
          {!compact ? (
            <DataTable
              label="批量操作逐项结果"
              columns={columns}
              data={batch.entries}
              loading={batch.busy && !batch.entries.length}
            />
          ) : null}
          {compact && (failed > 0 || batch.unresolved) ? (
            <Alert variant="destructive">
              <AlertDescription>
                {batch.unresolved
                  ? "部分结果待确认，请先查询原请求。"
                  : `${failed} 项未完成。${batch.entries.find((entry) => entry.status === "failed")?.message ?? "请重新读取后再试。"}`}
              </AlertDescription>
            </Alert>
          ) : null}
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
          {batch.submitted && !compact ? (
            <p className="text-xs text-muted-foreground">
              {batch.action?.recover
                ? "成功项已移出选择。结果未知时可查询回执；失败项重新读取后再确认。"
                : "成功项已移出选择。结果未知时请刷新页面核对状态，避免重复提交。"}
            </p>
          ) : null}
        </div>
        <Footer className="flex-wrap">
          <Cancel
            variant="outline"
            onClick={() => {
              batch.setOpen(false);
              setPassword("");
            }}
          >
            {batch.busy
              ? "关闭并稍后查看"
              : batch.unresolved || failed
                ? "关闭"
                : batch.submitted
                  ? "完成"
                  : "取消"}
          </Cancel>
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
              重试失败项（{failed}）
            </Button>
          ) : null}
          {!batch.submitted || (batch.unresolved && batch.action?.recover) ? (
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
                : compact
                  ? batch.action?.title.replace("账号", "")
                  : `确认${batch.action?.title ?? "操作"}（${ready}）`}
            </Button>
          ) : null}
        </Footer>
      </Content>
    </Root>
  );
}
