"use client";
import { useLayoutEffect, useRef, useState } from "react";
import { ArrowDown, Copy, Quote, RefreshCw } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Collapsible,
  CollapsibleTrigger,
  CollapsibleContent,
} from "@/components/ui/collapsible";
import { MarkdownContent } from "@/components/canvas/runtime/components/editor/markdown-content";
import { ProjectMediaPreview } from "@/components/media/project-media";
import { money } from "@/components/generation/contracts";
import type { ConversationController } from "./use-conversation";
import { terminalStatuses } from "./conversation-state";
import { ConversationResults } from "./conversation-result";
const labels: Record<string, string> = {
  accepted: "已受理",
  running: "正在创作",
  waiting_approval: "等待确认方案",
  paused: "已暂停",
  failed: "本轮未完成",
  stopped: "已停止",
  completed: "本轮执行结束",
  needs_reconciliation: "原任务结果待核对",
  rejected: "方案已驳回",
};
export function ConversationThread({
  conversation: c,
  projectID,
}: {
  conversation: ConversationController;
  projectID: string;
}) {
  const box = useRef<HTMLDivElement>(null),
    content = useRef<HTMLDivElement>(null),
    bottom = useRef<HTMLDivElement>(null),
    near = useRef(true),
    observedHeight = useRef(0),
    anchor = useRef<{ height: number; top: number } | null>(null);
  const [reading, setReading] = useState(false),
    [notice, setNotice] = useState("");
  useLayoutEffect(() => {
    const el = box.current,
      body = content.current;
    if (!el || !body) return;
    const observer = new ResizeObserver(() => {
      observedHeight.current = el.scrollHeight;
      // Images and lazy result cards can grow after the message snapshot commits.
      if (near.current && !anchor.current) el.scrollTop = el.scrollHeight;
    });
    observer.observe(body);
    observer.observe(el);
    return () => observer.disconnect();
  }, []);
  useLayoutEffect(() => {
    const el = box.current;
    if (!el) return;
    if (anchor.current) {
      el.scrollTop =
        anchor.current.top + el.scrollHeight - anchor.current.height;
      anchor.current = null;
    } else if (near.current) {
      el.scrollTop = el.scrollHeight;
    }
  }, [c.runMap, c.summaries]);
  async function older(action: () => Promise<unknown>) {
    const el = box.current;
    if (el) anchor.current = { height: el.scrollHeight, top: el.scrollTop };
    await action();
  }
  return (
    <div className="relative min-h-0 flex-1">
      <div
        ref={box}
        className="h-full overflow-y-auto overscroll-contain px-4 pb-6"
        aria-label="对话消息"
        onScroll={() => {
          const el = box.current;
          if (!el) return;
          // Layout-induced scroll events can precede the resize notification.
          if (near.current && el.scrollHeight !== observedHeight.current)
            return;
          near.current = el.scrollHeight - el.scrollTop - el.clientHeight < 96;
          setReading(!near.current);
        }}
      >
        <div ref={content} className="mx-auto max-w-[960px] space-y-8">
          {c.hasOlderRuns ? (
            <Button variant="ghost" onClick={() => void older(c.loadOlderRuns)}>
              加载更早对话
            </Button>
          ) : null}
          {c.summaries.map((summary) => {
            const run = c.runMap[summary.root_run_id];
            if (!run)
              return (
                <div
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
                </div>
              );
            const s = run.snapshot,
              state = s.state,
              latest = c.activeRunID === s.root_run_id;
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
            return (
              <section
                key={s.root_run_id}
                aria-label={`第${summary.accepted_revision}轮创作`}
                className="space-y-5"
              >
                <div className="ml-auto max-w-[85%] rounded-2xl bg-muted px-4 py-3">
                  {s.retry_of_run_id ? (
                    <p className="mb-2 text-xs text-muted-foreground">
                      重新生成 · 保留原尝试
                    </p>
                  ) : null}
                  {(s.input?.media_pins.length ?? 0) > 0 ? (
                    <div className="mb-2 flex gap-2">
                      {s.input!.media_pins.map((pin, index) => (
                        <div
                          key={`${pin.bindingId ?? pin.sourceId}/${index}`}
                          className="size-16 overflow-hidden rounded-md"
                        >
                          <ProjectMediaPreview
                            projectID={projectID}
                            id={pin.sourceId}
                            kind={pin.mediaKind}
                          />
                        </div>
                      ))}
                    </div>
                  ) : null}
                  <p className="break-words whitespace-pre-wrap">
                    {s.input ? s.input.plain_text || "参考创作" : s.goal}
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
                </div>
                <div className="space-y-4">
                  {run.older ? (
                    <Button
                      variant="ghost"
                      onClick={() =>
                        void older(() => c.loadOlderMessages(s.root_run_id))
                      }
                    >
                      加载本轮较早消息
                    </Button>
                  ) : null}
                  {messages.map((message) => (
                    <article key={message.id} className="group min-w-0">
                      <MarkdownContent
                        value={message.text}
                        className="break-words"
                      />
                      <div className="mt-2 flex gap-1">
                        <Button
                          variant="ghost"
                          size="sm"
                          aria-label="复制回复"
                          onClick={() => {
                            void navigator.clipboard
                              .writeText(message.text)
                              .then(
                                () => setNotice("已复制回复"),
                                () => setNotice("复制未完成，请选择文字复制"),
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
                          <Quote className="size-3.5" />
                          引用
                        </Button>
                      </div>
                    </article>
                  ))}
                  <ConversationResults
                    projectID={projectID}
                    messages={run.messages}
                    onReuse={(draft) => c.changeDraft(draft)}
                  />
                  <p
                    role={latest ? "status" : undefined}
                    className="text-sm text-muted-foreground"
                  >
                    {latest && c.stopping
                      ? "正在停止，等待执行回执"
                      : (labels[state.status] ?? "正在同步状态")}
                  </p>
                  {state.plan?.length || state.delegations?.length ? (
                    <Collapsible className="rounded-xl border p-3 text-sm">
                      <CollapsibleTrigger asChild>
                        <Button variant="ghost">创作进度</Button>
                      </CollapsibleTrigger>
                      <CollapsibleContent>
                        <ol className="mt-3 space-y-2">
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
                      </CollapsibleContent>
                    </Collapsible>
                  ) : null}
                  {state.status === "waiting_approval" && state.proposal ? (
                    <div className="space-y-3 rounded-xl border p-4">
                      <p>请确认创作方案</p>
                      <p className="text-sm text-muted-foreground">
                        {typeof state.proposal.quote_micros === "number"
                          ? `预计费用 ${money(state.proposal.quote_micros)}`
                          : "费用以正式报价为准"}
                      </p>
                      {typeof state.proposal.candidate.operation_id ===
                      "string" ? (
                        <ConversationResults
                          projectID={projectID}
                          operationID={state.proposal.candidate.operation_id}
                        />
                      ) : null}
                      <div className="flex gap-2">
                        <Button
                          disabled={!latest || c.stopping}
                          onClick={() => void c.command("approve")}
                        >
                          批准方案
                        </Button>
                        <Button
                          variant="outline"
                          disabled={!latest || c.stopping}
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
                      disabled={c.stopping || !s.execution_available}
                      onClick={() => void c.command("resume")}
                    >
                      继续创作
                    </Button>
                  ) : null}
                  {terminalStatuses.has(state.status) && s.input ? (
                    <Button
                      variant="outline"
                      disabled={
                        c.active ||
                        c.unresolved.length > 0 ||
                        !s.execution_available ||
                        c.phase !== "idle"
                      }
                      onClick={() => void c.retry(s.root_run_id)}
                    >
                      <RefreshCw className="size-4" />
                      重新生成
                    </Button>
                  ) : null}
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
                  {!s.execution_available ? (
                    <p className="text-xs text-muted-foreground">
                      此记录使用的执行版本已不可用，可以查看历史。
                    </p>
                  ) : null}
                </div>
              </section>
            );
          })}
          <div ref={bottom} />
        </div>
      </div>
      {reading ? (
        <Button
          variant="secondary"
          className="absolute right-4 bottom-3 rounded-full shadow-sm"
          onClick={() => {
            near.current = true;
            box.current?.scrollTo({
              top: box.current.scrollHeight,
              behavior: "smooth",
            });
            setReading(false);
          }}
        >
          <ArrowDown className="size-4" />
          回到底部
        </Button>
      ) : null}
      <span role="status" className="sr-only">
        {notice}
      </span>
    </div>
  );
}
