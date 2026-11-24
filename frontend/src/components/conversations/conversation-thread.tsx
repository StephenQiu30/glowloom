"use client";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Fragment, useState } from "react";
import { Copy, Quote, RefreshCw } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Message,
  MessageContent,
  MessageFooter,
} from "@/components/ui/message";
import { Bubble, BubbleContent } from "@/components/ui/bubble";
import { Attachment, AttachmentMedia } from "@/components/ui/attachment";
import {
  Trace,
  TraceLine,
  TraceCollapsible,
  TraceCollapsibleContent,
  TraceCollapsibleTrigger,
} from "@/components/ui/trace";
import {
  MessageScroller,
  MessageScrollerButton,
  MessageScrollerContent,
  MessageScrollerItem,
  MessageScrollerProvider,
  MessageScrollerViewport,
} from "@/components/ui/message-scroller";
import { MarkdownContent } from "@glowloom/canvas/editor/runtime/components/editor/markdown-content.tsx";
import { ProjectMediaPreview } from "@/components/media/project-media";
import { money } from "@/components/generation/contracts";
import type { ConversationController } from "./use-conversation";
import {
  terminalStatuses,
  readShotDeliveryProposal,
  shotDeliveryCandidateKind,
} from "./conversation-state";
import { ConversationProduction } from "./conversation-production";
import { ConversationResults } from "./conversation-result";
const labels: Record<string, string> = {
  accepted: "已受理",
  running: "正在创作",
  waiting_approval: "等待确认方案",
  waiting_delivery: "等待制作产物",
  paused: "已暂停",
  failed: "本轮未完成",
  stopped: "已停止",
  completed: "已完成",
  needs_reconciliation: "原任务结果待核对",
  rejected: "方案已驳回",
};
export function ConversationThread({
  conversation: c,
  projectID,
  writable = false,
  onUseReply,
  replyAction,
  displayInput,
  onDraftProjectMemory,
  useReplyLabel = "作为剧本草稿",
}: {
  conversation: ConversationController;
  projectID: string;
  writable?: boolean;
  onUseReply?: (text: string) => void;
  replyAction?: (text: string, input?: API.AuthorizedInput) => React.ReactNode;
  displayInput?: (text: string) => string;
  onDraftProjectMemory?: (text: string) => void;
  useReplyLabel?: string;
}) {
  const [notice, setNotice] = useState("");
  return (
    <MessageScrollerProvider autoScroll defaultScrollPosition="end">
      <MessageScroller className="flex-1">
        <MessageScrollerViewport aria-label="对话消息" preserveScrollOnPrepend>
          <MessageScrollerContent className="mx-auto w-full max-w-[1014px] gap-6 pt-4 pb-8">
            {c.hasOlderRuns ? (
              <MessageScrollerItem messageId="load-older">
                <Button variant="ghost" onClick={() => void c.loadOlderRuns()}>
                  加载更早对话
                </Button>
              </MessageScrollerItem>
            ) : null}
            {c.summaries.map((summary) => {
              const run = c.runMap[summary.root_run_id];
              if (!run)
                return (
                  <MessageScrollerItem
                    messageId={summary.root_run_id}
                    key={summary.root_run_id}
                    className="rounded-xl border p-4"
                  >
                    <p className="mb-2 truncate text-sm">
                      {summary.input_excerpt || "参考创作"}
                    </p>
                    <Button
                      variant="outline"
                      onClick={() => void c.loadRun(summary.root_run_id)}
                    >
                      查看此轮对话
                    </Button>
                  </MessageScrollerItem>
                );
              const s = run.snapshot,
                state = s.state,
                latest = c.activeRunID === s.root_run_id;
              const delivery = readShotDeliveryProposal(
                state.proposal?.candidate,
              );
              const unreadableDelivery =
                state.proposal?.candidate.kind === shotDeliveryCandidateKind &&
                !delivery;
              const status =
                latest && c.stopping
                  ? "正在停止，等待执行回执"
                  : (labels[state.status] ?? "正在同步状态");
              const messages = [
                ...new Map(
                  run.messages
                    .filter((p) => p.kind === "assistant")
                    .map((p) => [p.message_id, p.message_id]),
                ).keys(),
              ].map((id) => ({
                id,
                text: run.messages
                  .filter((p) => p.message_id === id)
                  .sort((a, b) => a.part_index - b.part_index)
                  .map((p) => p.content)
                  .join(""),
              }));
              const retryAction =
                terminalStatuses.has(state.status) &&
                s.input &&
                state.failure?.code !== "project_memory_changed" &&
                state.failure?.code !== "project_evidence_changed" ? (
                  <Button
                    variant="secondary"
                    size="sm"
                    className="pointer-coarse:min-h-11"
                    disabled={
                      !writable ||
                      c.active ||
                      c.unresolved.length > 0 ||
                      !s.execution_available ||
                      c.phase !== "idle"
                    }
                    onClick={() => void c.retry(s.root_run_id)}
                  >
                    <RefreshCw data-icon="inline-start" />
                    重新生成
                  </Button>
                ) : null;
              return (
                <Fragment key={s.root_run_id}>
                  <MessageScrollerItem messageId={s.root_run_id} scrollAnchor>
                    <Message align="end" aria-label="用户消息">
                      <MessageContent>
                        <Bubble align="end" variant="muted">
                          <BubbleContent className="px-4 py-3">
                            {s.retry_of_run_id ? (
                              <p className="mb-2 text-xs text-muted-foreground">
                                重新生成 · 保留原尝试
                              </p>
                            ) : null}
                            {(s.input?.media_pins.length ?? 0) > 0 ? (
                              <div className="mb-2 flex flex-wrap gap-2">
                                {s.input!.media_pins.map((pin, index) => (
                                  <Attachment
                                    orientation="vertical"
                                    key={`${pin.bindingId ?? pin.sourceId}/${index}`}
                                    className="size-16 shrink-0 overflow-hidden rounded-md"
                                  >
                                    <AttachmentMedia variant="image">
                                      <ProjectMediaPreview
                                        projectID={projectID}
                                        id={pin.sourceId}
                                        kind={pin.mediaKind}
                                      />
                                    </AttachmentMedia>
                                  </Attachment>
                                ))}
                              </div>
                            ) : null}
                            <p className="break-words whitespace-pre-wrap">
                              {s.input
                                ? (displayInput?.(s.input.plain_text) ??
                                    s.input.plain_text) ||
                                  "参考创作"
                                : s.goal}
                            </p>
                            {s.input?.references_unavailable ? (
                              <p className="mt-2 text-xs text-muted-foreground">
                                部分参考当前不可访问，原输入仍保留。
                              </p>
                            ) : null}
                            {s.input?.quote ? (
                              <blockquote className="mt-2 line-clamp-3 border-l-2 pl-2 text-xs text-muted-foreground">
                                {s.input.quote.text}
                              </blockquote>
                            ) : null}
                          </BubbleContent>
                        </Bubble>
                      </MessageContent>
                    </Message>
                  </MessageScrollerItem>
                  <MessageScrollerItem
                    messageId={`${s.root_run_id}/reply`}
                    aria-label={`第${summary.accepted_revision}轮回复`}
                  >
                    <Message>
                      <MessageContent className="gap-4">
                        {run.older ? (
                          <Button
                            variant="ghost"
                            onClick={() =>
                              void c.loadOlderMessages(s.root_run_id)
                            }
                          >
                            加载本轮较早消息
                          </Button>
                        ) : null}
                        {state.plan?.length || state.delegations?.length ? (
                          <Trace>
                            <TraceCollapsible>
                              <TraceCollapsibleTrigger className="min-h-8 pointer-coarse:min-h-11">
                                <TraceLine role={latest ? "status" : undefined}>
                                  {status}
                                </TraceLine>
                              </TraceCollapsibleTrigger>
                              <TraceCollapsibleContent>
                                <ol className="space-y-2">
                                  {state.plan?.map((item) => (
                                    <li key={item.id}>
                                      {item.title}
                                      <span className="ml-2 text-xs text-muted-foreground">
                                        {labels[item.status] ?? item.status}
                                      </span>
                                    </li>
                                  ))}
                                </ol>
                                {state.delegations?.map((item) => (
                                  <p
                                    key={item.task_id}
                                    className="mt-2 text-muted-foreground"
                                  >
                                    {(
                                      {
                                        script: "编剧",
                                        art: "美术",
                                        storyboard: "分镜",
                                        shot: "镜头",
                                        voice: "声音",
                                        edit: "剪辑",
                                        qa: "质检",
                                      } as Record<string, string>
                                    )[item.role] ?? "专业任务"}{" "}
                                    · {labels[item.status] ?? item.status}
                                  </p>
                                ))}
                              </TraceCollapsibleContent>
                            </TraceCollapsible>
                          </Trace>
                        ) : (
                          <p
                            role={latest ? "status" : undefined}
                            className="text-sm text-muted-foreground"
                          >
                            {status}
                          </p>
                        )}
                        {messages.map((message, index) => (
                          <article key={message.id} className="group min-w-0">
                            <MarkdownContent
                              value={message.text}
                              className="break-words"
                            />
                            {terminalStatuses.has(state.status) ? (
                              <p className="mt-3 text-xs text-muted-foreground">
                                以上内容由 AI 生成
                              </p>
                            ) : null}
                            <MessageFooter className="mt-2 flex-wrap gap-1 px-0">
                              {index === messages.length - 1 && retryAction}
                              {state.status === "completed" &&
                                replyAction?.(message.text, s.input)}
                              {onDraftProjectMemory &&
                                state.status === "completed" &&
                                message.text.trim() && (
                                  <Button
                                    variant="ghost"
                                    size="sm"
                                    onClick={() =>
                                      onDraftProjectMemory(message.text)
                                    }
                                  >
                                    草拟项目约定
                                  </Button>
                                )}
                              {onUseReply &&
                                state.status === "completed" &&
                                message.text.trim() && (
                                  <Button
                                    variant="outline"
                                    size="sm"
                                    onClick={() => onUseReply(message.text)}
                                  >
                                    {useReplyLabel}
                                  </Button>
                                )}
                              <Button
                                variant="ghost"
                                size="sm"
                                className="pointer-coarse:min-h-11"
                                aria-label="复制回复"
                                onClick={() => {
                                  void navigator.clipboard
                                    .writeText(message.text)
                                    .then(
                                      () => setNotice("已复制回复"),
                                      () =>
                                        setNotice("复制未完成，请选择文字复制"),
                                    );
                                }}
                              >
                                <Copy className="size-3.5" />
                                复制
                              </Button>
                              <Button
                                variant="ghost"
                                size="sm"
                                onClick={() =>
                                  void c.quoteMessage(s.root_run_id, message.id)
                                }
                              >
                                <Quote data-icon="inline-start" />
                                引用
                              </Button>
                            </MessageFooter>
                          </article>
                        ))}
                        <ConversationResults
                          projectID={projectID}
                          messages={run.messages}
                          onReuse={(draft) => c.changeDraft(draft)}
                        />
                        <ConversationProduction
                          facts={state.production}
                          writable={writable}
                          projectID={projectID}
                          rootRunID={s.root_run_id}
                          onRefresh={c.refresh}
                        />
                        {s.recovery?.some((item) => item.parked_at) && (
                          <p
                            role="status"
                            className="text-sm text-muted-foreground"
                          >
                            执行恢复已暂停，请核对原任务记录后再继续。
                          </p>
                        )}
                        {state.status === "waiting_approval" &&
                        state.proposal ? (
                          <div className="space-y-3 rounded-xl border p-4">
                            <p>请确认创作方案</p>
                            {delivery ? (
                              <>
                                <div className="aspect-video max-w-md overflow-hidden rounded-lg">
                                  <ProjectMediaPreview
                                    projectID={projectID}
                                    id={delivery.assetID}
                                    kind="video"
                                    large
                                    interactive
                                  />
                                </div>
                                <p className="text-sm">
                                  裁剪起点 {delivery.sourceStartMS / 1000} 秒 ·
                                  时长 {delivery.durationMS / 1000} 秒
                                </p>
                                <p className="text-sm">
                                  {delivery.aspectRatio} · {delivery.fps} fps ·{" "}
                                  {delivery.format} 工程
                                </p>
                              </>
                            ) : unreadableDelivery ? (
                              <Alert role="alert">
                                <AlertDescription>
                                  制作方案暂时无法读取，请重新生成方案。
                                </AlertDescription>
                              </Alert>
                            ) : null}
                            <p className="text-sm text-muted-foreground">
                              {delivery
                                ? "采用与导出不新增生成费用"
                                : typeof state.proposal.quote_micros ===
                                    "number"
                                  ? `预计费用 ${money(state.proposal.quote_micros)}`
                                  : "费用以正式报价为准"}
                            </p>
                            {typeof state.proposal.candidate.operation_id ===
                            "string" ? (
                              <ConversationResults
                                projectID={projectID}
                                operationID={
                                  state.proposal.candidate.operation_id
                                }
                              />
                            ) : null}
                            <div className="flex gap-2">
                              <Button
                                disabled={
                                  !writable ||
                                  !latest ||
                                  c.stopping ||
                                  unreadableDelivery
                                }
                                onClick={() => void c.command("approve")}
                              >
                                批准方案
                              </Button>
                              <Button
                                variant="outline"
                                disabled={!writable || !latest || c.stopping}
                                onClick={() => void c.command("reject")}
                              >
                                驳回方案
                              </Button>
                            </div>
                          </div>
                        ) : null}
                        {state.status === "paused" && latest ? (
                          <Button
                            variant="outline"
                            disabled={
                              !writable || c.stopping || !s.execution_available
                            }
                            onClick={() => void c.command("resume")}
                          >
                            继续创作
                          </Button>
                        ) : null}
                        {latest &&
                        (state.status === "needs_reconciliation" ||
                          state.status === "failed") ? (
                          <Button
                            variant="outline"
                            disabled={
                              !writable ||
                              c.stopping ||
                              !s.execution_available ||
                              c.phase !== "idle"
                            }
                            onClick={() => void c.command("pause")}
                          >
                            暂停以核对原任务
                          </Button>
                        ) : null}
                        {!messages.length && retryAction}
                        {s.history_incomplete ? (
                          <p className="text-xs text-muted-foreground">
                            此旧记录可能缺少早期消息，当前仅显示保留的公开内容。
                          </p>
                        ) : null}
                        {state.failure?.code ===
                        "conversation_reference_unavailable" ? (
                          <p className="text-sm text-muted-foreground">
                            历史参考当前不可访问，创作已停止。请核对权限，或新建对话继续。
                          </p>
                        ) : null}
                        {state.failure?.code === "project_memory_changed" ? (
                          <p className="text-sm text-muted-foreground">
                            项目约定已变更。请新建会话，按当前约定继续创作。
                          </p>
                        ) : null}
                        {state.failure?.code === "project_evidence_changed" ? (
                          <p className="text-sm text-muted-foreground">
                            之前使用的项目证据已变更或不可访问。请新建会话并选择当前来源。
                          </p>
                        ) : null}
                        {!s.execution_available ? (
                          <p className="text-xs text-muted-foreground">
                            此记录使用的执行版本已不可用，可以查看历史。
                          </p>
                        ) : null}
                      </MessageContent>
                    </Message>
                  </MessageScrollerItem>
                </Fragment>
              );
            })}
          </MessageScrollerContent>
        </MessageScrollerViewport>
        <MessageScrollerButton className="pointer-coarse:size-11" />
        <span role="status" className="sr-only">
          {notice}
        </span>
      </MessageScroller>
    </MessageScrollerProvider>
  );
}
