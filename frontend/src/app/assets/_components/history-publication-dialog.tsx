"use client";

import { useEffect, useRef, useState } from "react";
import Link from "next/link";
import { publishHistoryResults } from "@/api/operations";
import { isProjectID } from "@/components/projects/project";
import { FeedbackToast } from "@/components/feedback/feedback-toast";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogTitle,
} from "@/components/ui/dialog";
import { Field, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { RequestError } from "@/lib/request";
import { readErrorMessage } from "@/lib/read-error";
import {
  historyPublicationTargets,
  type HistoryCardTarget,
} from "./history-results";

import { historyCardTitle } from "./history-result-card";

type PublicationResult = NonNullable<
  Awaited<ReturnType<typeof publishHistoryResults>>["items"]
>[number];

export function HistoryPublicationDialog({
  cards,
  onClose,
  onLock,
  onPublished,
}: {
  cards: HistoryCardTarget[];
  onClose: () => void;
  onLock: (locked: boolean) => void;
  onPublished: (failed: string[]) => void;
}) {
  const [title, setTitle] = useState("创作作品");
  const [description, setDescription] = useState("");
  const [publishPrompt, setPublishPrompt] = useState(false);
  const [results, setResults] = useState<PublicationResult[]>([]);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");
  const [intent, setIntent] = useState<{
    key: string;
    body: Parameters<typeof publishHistoryResults>[0];
  } | null>(null);
  const queue = useRef<API.HistoryPublicationTarget[]>([]);
  const busy = useRef(false);
  const flight = useRef<AbortController | null>(null);
  useEffect(
    () => () => {
      flight.current?.abort();
      onLock(false);
    },
    [onLock],
  );
  const allTargets = historyPublicationTargets(cards);
  const failedTargets = results
    .filter((item) => item.status === "failed")
    .map((item) => ({
      operation_id: item.operation_id!,
      output_id: item.output_id!,
    }));
  const targets = results.length ? failedTargets : allTargets;
  const frozen = pending || !!intent;
  async function submit() {
    if (busy.current || (!intent && (!title.trim() || !targets.length))) return;
    let operation = intent ?? {
      key: crypto.randomUUID(),
      body: {
        targets: targets.slice(0, 16),
        title: title.trim(),
        description: description.trim(),
        publish_prompt: publishPrompt,
      },
    };
    if (!intent) queue.current = targets.slice(16);
    const aggregate = new Map(
      results.map((item) => [`${item.operation_id}:${item.output_id}`, item]),
    );
    busy.current = true;
    setPending(true);
    setError("");
    onLock(true);
    const controller = new AbortController();
    flight.current = controller;
    try {
      while (true) {
        setIntent(operation);
        const response = await publishHistoryResults(operation.body, {
          signal: controller.signal,
          headers: { "Idempotency-Key": operation.key },
        });
        const requested = operation.body.targets ?? [];
        if (
          !Array.isArray(response.items) ||
          response.items.length !== requested.length
        )
          throw new RequestError("invalid_receipt");
        const expected = new Set(
          requested.map((item) => `${item.operation_id}:${item.output_id}`),
        );
        for (const item of response.items) {
          if (
            !expected.delete(`${item.operation_id}:${item.output_id}`) ||
            !["published", "failed"].includes(item.status ?? "") ||
            (item.status === "published" && !isProjectID(item.work_id))
          )
            throw new RequestError("invalid_receipt");
          aggregate.set(`${item.operation_id}:${item.output_id}`, item);
        }
        if (controller.signal.aborted) return;
        setResults([...aggregate.values()]);
        if (!queue.current.length) break;
        operation = {
          key: crypto.randomUUID(),
          body: { ...operation.body, targets: queue.current.splice(0, 16) },
        };
      }
      setIntent(null);
      onLock(false);
      const failed = new Set(
        [...aggregate.values()]
          .filter((item) => item.status === "failed")
          .map((item) => `${item.operation_id}:${item.output_id}`),
      );
      onPublished(
        cards
          .filter((card) =>
            card.outputs.some((output) =>
              failed.has(`${card.task.id}:${output.id}`),
            ),
          )
          .map((card) => card.key),
      );
    } catch (reason) {
      if (controller.signal.aborted) return;
      if (
        reason instanceof RequestError &&
        reason.status >= 400 &&
        reason.status < 500 &&
        reason.code !== "invalid_receipt"
      ) {
        for (const target of [
          ...(operation.body.targets ?? []),
          ...queue.current,
        ])
          aggregate.set(`${target.operation_id}:${target.output_id}`, {
            ...target,
            status: "failed",
            error_code: reason.code,
          });
        queue.current = [];
        setResults([...aggregate.values()]);
        setIntent(null);
        onLock(false);
        setError(readErrorMessage(reason));
      } else setError("发布结果尚未确认，请重试原操作。");
    } finally {
      busy.current = false;
      if (!controller.signal.aborted) setPending(false);
    }
  }
  const published = results.filter((item) => item.status === "published");
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !frozen) onClose();
      }}
    >
      <DialogContent
        className="max-h-[90dvh] overflow-y-auto sm:max-w-xl"
        showCloseButton={!frozen}
      >
        <DialogTitle>发布作品</DialogTitle>
        <DialogDescription>
          所选图片和视频将公开展示。较多视频结果将按每批16个分批提交。
        </DialogDescription>
        <form
          className="grid gap-5"
          onSubmit={(event) => {
            event.preventDefault();
            void submit();
          }}
        >
          <section aria-label="所选发布结果" className="grid gap-2 text-sm">
            {cards.map((card) => (
              <p className="break-words" key={card.key}>
                {historyCardTitle(card)} · {card.outputs.length}个结果
              </p>
            ))}
          </section>
          <FieldGroup>
            <Field>
              <FieldLabel htmlFor="history-work-title">标题</FieldLabel>
              <Input
                id="history-work-title"
                maxLength={120}
                value={title}
                disabled={frozen}
                onChange={(event) => setTitle(event.target.value)}
              />
            </Field>
            <Field>
              <FieldLabel htmlFor="history-work-description">描述</FieldLabel>
              <Textarea
                id="history-work-description"
                maxLength={2000}
                value={description}
                disabled={frozen}
                onChange={(event) => setDescription(event.target.value)}
              />
            </Field>
            <Field orientation="horizontal">
              <Checkbox
                id="history-work-prompt"
                checked={publishPrompt}
                disabled={frozen}
                onCheckedChange={(checked) =>
                  setPublishPrompt(checked === true)
                }
              />
              <FieldLabel htmlFor="history-work-prompt">
                同时公开提示词
              </FieldLabel>
            </Field>
          </FieldGroup>
          {results.length > 0 && (
            <section aria-label="发布结果" className="grid gap-2">
              <p>
                已发布{published.length}项，失败
                {results.length - published.length}项。
              </p>
              {results.map((item) => (
                <div
                  key={`${item.operation_id}:${item.output_id}`}
                  className="flex flex-wrap items-center gap-2 text-sm"
                >
                  <span className="break-words">
                    {cards.find(
                      (card) =>
                        card.task.id === item.operation_id &&
                        card.outputs.some(
                          (output) => output.id === item.output_id,
                        ),
                    )?.task.prompt_preview || "生成结果"}{" "}
                    · {item.output_id?.slice(0, 8)}
                  </span>
                  {item.status === "published" ? (
                    <Button asChild variant="link" size="sm">
                      <Link
                        href={`/works/${item.work_id}`}
                        target="_blank"
                        rel="noopener noreferrer"
                      >
                        查看已发布作品
                      </Link>
                    </Button>
                  ) : (
                    <span>该结果未能发布，请重试。</span>
                  )}
                </div>
              ))}
            </section>
          )}
          <DialogFooter>
            <Button
              type="button"
              variant="ghost"
              disabled={frozen}
              onClick={onClose}
            >
              {results.length ? "完成" : "取消"}
            </Button>
            {(targets.length > 0 || intent) && (
              <Button
                type="submit"
                disabled={pending || (!intent && !title.trim())}
              >
                {pending
                  ? "正在发布…"
                  : intent
                    ? "重试原操作"
                    : results.length
                      ? "重试失败项"
                      : "确认发布"}
              </Button>
            )}
          </DialogFooter>
        </form>
        {error && <FeedbackToast message={error} />}
      </DialogContent>
    </Dialog>
  );
}
