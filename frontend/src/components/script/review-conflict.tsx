"use client";
import { useState } from "react";
import { useForm } from "react-hook-form";
import { Button } from "@/components/ui/button";
import { getWorkspace, requireScriptScope } from "./source-queries";
import { getEpisodes, getStructure } from "./review-queries";
import {
  reviewCommandSchema,
  reviewImpactSchema,
  type ReviewCommand,
  type ReviewIntent,
} from "./review-intent";
import type { ScriptScope } from "./source-intent";
import type { useReviewWriter } from "./use-review-writer";
export function ReviewConflict({
  scope,
  writer,
  onDiscard,
}: {
  scope: ScriptScope;
  writer: ReturnType<typeof useReviewWriter>;
  onDiscard?: () => void;
}) {
  const [fresh, setFresh] = useState<{
    command: ReviewCommand;
    facts: unknown;
  }>();
  const [reading, setReading] = useState(false);
  const [error, setError] = useState<string>();
  const form = useForm<{ consent: boolean }>({
    defaultValues: { consent: false },
  });
  const rejected = writer.rejected;
  if (!rejected) return null;
  const impact =
    rejected.cause.code === "confirmation_required"
      ? reviewImpactSchema.safeParse(rejected.cause.meta)
      : undefined;
  async function readLatest(original: ReviewIntent) {
    setReading(true);
    setError(undefined);
    setFresh(undefined);
    form.reset({ consent: false });
    try {
      const head = await getWorkspace(scope.projectId);
      requireScriptScope(head, scope);
      let command: ReviewCommand;
      let facts: unknown;
      if (
        original.action === "resplit" ||
        original.action === "confirm_split" ||
        original.action === "adopt"
      ) {
        const version =
          original.action === "adopt"
            ? original.versionId
            : original.body.version_id;
        const view = await getEpisodes(
          scope,
          version,
          original.action === "confirm_split" ? original.charCount : undefined,
        );
        if (original.action === "adopt") {
          if (
            view.head.confirmed_split_set_id !== original.splitSetId ||
            view.episodes.map((episode) => episode.id).join(":") !==
              original.episodeIds.join(":")
          )
            throw new Error(
              "正式分集已经改变，请关闭此被拒绝意图并完整阅读当前分集后重新采纳。",
            );
          command = {
            action: "adopt",
            versionId: original.versionId,
            splitSetId: original.splitSetId,
            episodeIds: original.episodeIds,
            body: {
              ...original.body,
              expected_revision: head.state.revision,
              expected_split_revision: view.head.split_revision,
            },
          };
        } else {
          if (head.state.draft_version_id !== original.body.version_id)
            throw new Error(
              "当前草稿已改为另一不可变版本。旧分集稿仍保留，请返回原版本阅读后明确选择新的操作。",
            );
          const body = {
            ...original.body,
            expected_revision: head.state.revision,
            expected_split_revision: view.head.split_revision,
            candidate_set_id: view.head.candidate_split_set_id,
          };
          command =
            original.action === "resplit"
              ? { action: "resplit", body }
              : {
                  action: "confirm_split",
                  charCount: original.charCount,
                  body: {
                    ...body,
                    boundaries: original.body.boundaries,
                    ...(original.body.preface
                      ? { preface: original.body.preface }
                      : {}),
                  },
                };
        }
        facts = {
          script_revision: head.state.revision,
          head: view.head,
          candidate: view.candidate,
          episodes: view.episodes,
        };
      } else {
        if (!head.state.draft_version_id)
          throw new Error("当前项目没有可保存结构的草稿版本。");
        const view = await getEpisodes(scope, head.state.draft_version_id);
        const episode = view.episodes.find(
          (item) => item.id === original.episodeId,
        );
        if (!episode || episode.is_delete)
          throw new Error("原正式集已移除，原结构稿保留，不能自动换集保存。");
        const current = episode.current_structure_id
          ? await getStructure(scope, original.episodeId)
          : null;
        if (
          original.action === "confirm_structure" &&
          current?.structure.id !== original.structureId
        )
          throw new Error(
            "当前结构已被替换。请完整阅读新的候选后再明确确认，不能把旧请求改成确认新正文。",
          );
        const body = {
          ...original.body,
          expected_revision: head.state.revision,
          expected_episode_revision: episode.revision,
          base_structure_version_no: current?.structure.version_no ?? 0,
        };
        command =
          original.action === "save_structure"
            ? {
                action: "save_structure",
                episodeId: original.episodeId,
                spanStart: episode.span_start,
                spanEnd: episode.span_end,
                body: { ...body, document: original.body.document },
              }
            : {
                action: "confirm_structure",
                episodeId: original.episodeId,
                structureId: original.structureId,
                body: { ...body, ack_invalidate: original.body.ack_invalidate },
              };
        facts = { episode, structure: current?.structure };
      }
      setFresh({ command: reviewCommandSchema.parse(command), facts });
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "最新事实读取失败。");
    } finally {
      setReading(false);
    }
  }
  return (
    <section
      className="space-y-3 rounded-lg border border-amber-500 p-3"
      aria-label="审核冲突与影响确认"
    >
      <p>
        原审核被确定拒绝，完整草稿仍保留。读取当前事实后才能明确另行提交新意图。
      </p>
      <p role="alert">{writer.error ?? rejected.cause.message}</p>
      {impact &&
        (impact.success ? (
          <ul className="space-y-2">
            {impact.data.affected_episodes.map((episode) => (
              <li key={episode.episode_id} className="text-sm break-all">
                受影响集 {episode.episode_id}；原确认结构{" "}
                {episode.confirmed_structure_id}；原因 {episode.reason}
              </li>
            ))}
          </ul>
        ) : (
          <p role="alert">服务端影响清单无效，不能确认未知影响。</p>
        ))}
      <Button
        type="button"
        variant="outline"
        disabled={reading || writer.busy}
        onClick={() => void readLatest(rejected.intent)}
      >
        读取最新审核事实并保留原稿
      </Button>
      {fresh && (
        <>
          <details>
            <summary className="cursor-pointer">
              完整查看当前服务端分集或结构
            </summary>
            <pre className="max-h-64 overflow-auto text-xs break-all whitespace-pre-wrap">
              {JSON.stringify(fresh.facts, null, 2)}
            </pre>
          </details>
          <form
            className="space-y-3"
            onSubmit={form.handleSubmit(async ({ consent }) => {
              if (!consent || (impact && !impact.success)) return;
              const command = fresh.command;
              const approved =
                impact?.success && "ack_invalidate" in command.body
                  ? reviewCommandSchema.parse({
                      ...command,
                      body: { ...command.body, ack_invalidate: true },
                    })
                  : command;
              writer.acknowledgeLatest();
              await writer.submit(approved);
            })}
          >
            <label className="flex items-start gap-2 text-sm">
              <input
                type="checkbox"
                disabled={writer.busy}
                {...form.register("consent")}
                required
              />
              我已审阅当前事实与上述真实影响，明确按最新修订另行提交原稿
              {impact?.success ? "并确认旧结构失效" : ""}。
            </label>
            <Button
              type="submit"
              disabled={
                writer.busy || (impact !== undefined && !impact.success)
              }
            >
              按最新修订明确另行提交原稿
            </Button>
          </form>
        </>
      )}
      {error && <p role="alert">{error}</p>}
      {onDiscard && (
        <Button
          type="button"
          variant="destructive"
          disabled={
            reading ||
            writer.busy ||
            Boolean(writer.intent || writer.storageError)
          }
          onClick={onDiscard}
        >
          明确放弃被拒绝的审核草稿
        </Button>
      )}
    </section>
  );
}
