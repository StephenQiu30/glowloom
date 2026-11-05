"use client";
import { useId, useState } from "react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Field, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { FeedbackToast } from "@/components/feedback/feedback-toast";
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
  const unknown = batch.entries.filter((e) => e.status === "unknown").length;
  const submitted = batch.entries.filter(
    (e) => e.status === "submitted",
  ).length;
  const blocked =
    !batch.busy &&
    !batch.submitted &&
    skipped > 0 &&
    skipped === batch.entries.length;
  const hasDescription =
    !blocked && !batch.submitted && Boolean(batch.action?.effect);
  const hasReason = !blocked && !batch.submitted && batch.action?.reason;
  const hasPassword =
    !blocked &&
    batch.action?.password &&
    (!batch.submitted || batch.unresolved);
  const canSubmit =
    (!batch.submitted && ready > 0) ||
    (batch.unresolved && Boolean(batch.action?.recover));
  const accountVerb = batch.action?.title.replace("账号", "");
  const resultSummary = [
    success ? `成功 ${success}` : "",
    submitted ? `已提交 ${submitted}` : "",
    failed ? `失败 ${failed}` : "",
    unknown ? `结果未知 ${unknown}` : "",
    skipped ? `未执行 ${skipped}` : "",
  ]
    .filter(Boolean)
    .join(" · ");
  const skipReasons = new Map<string, number>();
  if (blocked && compact) {
    for (const entry of batch.entries) {
      if (entry.status === "skipped")
        skipReasons.set(
          entry.message,
          (skipReasons.get(entry.message) ?? 0) + 1,
        );
    }
  }
  const columns: DataTableColumn<BatchEntry<T>>[] = [
    {
      id: "target",
      header: "目标",
      cell: ({ row }) => (
        <div className="flex min-w-0 flex-col gap-1">
          <span className="break-all whitespace-normal">
            {row.original.label}
          </span>
          {row.original.status !== "ready" &&
          row.original.status !== "running" &&
          row.original.message ? (
            <span className="text-xs break-words whitespace-normal text-muted-foreground">
              {row.original.message}
            </span>
          ) : null}
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
          compact || blocked ? "sm:max-w-md" : "sm:max-w-2xl",
        )}
        {...(blocked
          ? { "aria-describedby": `${id}-skipped` }
          : !hasDescription
            ? { "aria-describedby": undefined }
            : {})}
        {...(!compact && blocked ? { showCloseButton: false } : {})}
        onCloseAutoFocus={(event) => {
          if (batch.restoreFocus()) event.preventDefault();
        }}
      >
        <Header>
          <Title>
            {blocked
              ? compact
                ? `没有可${accountVerb}的账号`
                : `${batch.action?.title}：无可执行项`
              : compact && !batch.busy && !batch.submitted && ready
                ? `${accountVerb} ${ready} 个账号？`
                : batch.action?.title}
          </Title>
          {hasDescription ? (
            <Description>{batch.action?.effect}</Description>
          ) : null}
        </Header>
        <div className="flex min-h-0 flex-col gap-4 overflow-y-auto empty:hidden has-[>form:only-child:empty]:hidden">
          {batch.busy ? (
            <p
              role="status"
              className="flex items-center gap-2 text-sm text-muted-foreground"
            >
              <Spinner />
              {batch.submitted ? "正在处理…" : "正在检查…"}
            </p>
          ) : batch.submitted && !compact && resultSummary ? (
            <p role="status" className="text-sm text-muted-foreground">
              {resultSummary}
            </p>
          ) : compact && skipped > 0 && !blocked ? (
            <p className="text-sm text-muted-foreground">已跳过 {skipped} 项</p>
          ) : null}
          {blocked ? (
            <ul
              id={`${id}-skipped`}
              aria-label="跳过原因"
              className="grid gap-1 text-sm break-words text-muted-foreground"
            >
              {compact
                ? Array.from(skipReasons, ([message, count]) => (
                    <li key={message}>
                      {count} 项 · {message}
                    </li>
                  ))
                : batch.entries.map((entry) => (
                    <li key={entry.id}>
                      {entry.label} · {entry.message}
                    </li>
                  ))}
            </ul>
          ) : !compact && batch.entries.length ? (
            <DataTable
              label={batch.submitted ? "批量操作逐项结果" : "批量操作目标"}
              columns={columns}
              data={batch.entries}
            />
          ) : null}
          {compact && (failed > 0 || batch.unresolved) ? (
            <FeedbackToast
              message={
                batch.unresolved
                  ? "部分结果待确认，请先查询原请求。"
                  : `${failed} 项未完成。${batch.entries.find((entry) => entry.status === "failed")?.message ?? "请重新读取后再试。"}`
              }
            />
          ) : null}
          {batch.error && compact ? (
            <FeedbackToast message={batch.error} />
          ) : batch.error ? (
            <Alert variant="destructive">
              <AlertDescription>{batch.error}</AlertDescription>
            </Alert>
          ) : null}
          {canSubmit ? (
            <form
              id={id}
              className="empty:hidden"
              onSubmit={(event) => {
                event.preventDefault();
                void batch.run(reason, password, batch.unresolved);
                setPassword("");
              }}
            >
              {hasReason || hasPassword ? (
                <FieldGroup>
                  {hasReason ? (
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
                  {hasPassword ? (
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
              ) : null}
            </form>
          ) : null}
          {batch.submitted && unknown > 0 && !batch.action?.recover ? (
            <p className="text-sm text-muted-foreground">
              请刷新页面核对结果，再决定是否重试。
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
              : batch.unresolved || failed || blocked
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
          {canSubmit ? (
            <Button
              form={id}
              type="submit"
              variant={batch.action?.destructive ? "destructive" : "default"}
              disabled={
                batch.busy ||
                (batch.action?.password && !password) ||
                (!batch.submitted &&
                  batch.action?.reason &&
                  reason.trim().length < 5)
              }
            >
              {batch.unresolved
                ? "使用原请求恢复"
                : compact
                  ? accountVerb
                  : `确认${batch.action?.title ?? "操作"}（${ready}）`}
            </Button>
          ) : null}
        </Footer>
      </Content>
    </Root>
  );
}
