"use client";
import { useState } from "react";
import { useInfiniteQuery, useQuery } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { EpisodeSplitForm, type SplitBase } from "./episode-split-form";
import { StructureEditForm, type StructureBase } from "./structure-edit-form";
import {
  getEpisodes,
  getStructure,
  listStructureVersions,
  reviewScopeKey,
} from "./review-queries";
import {
  findVersion,
  getConfirmation,
  listConfirmations,
} from "./version-queries";
import {
  readEpisodeView,
  type EpisodeView,
  type ScriptEpisode,
  type ScriptVersionSummary,
  type StructureDocument,
} from "./review-model";
import type { ScriptWorkspace } from "./source-model";
import type { ScriptScope } from "./source-intent";
import type { ReviewCommand } from "./review-intent";
import type { useReviewWriter } from "./use-review-writer";
import { ReviewRecovery } from "./review-recovery";
import { ReviewConflict } from "./review-conflict";
import { VersionTextInspector } from "./version-text-inspector";
type Reviewer = ReturnType<typeof useReviewWriter>;
type Frame =
  | {
      type: "split";
      base: SplitBase;
      view: EpisodeView;
      version: ScriptVersionSummary;
    }
  | {
      type: "control";
      command: ReviewCommand;
      title: string;
      view?: EpisodeView;
      version?: ScriptVersionSummary;
    };
export function ScriptEpisodePanel({
  scope,
  versionId,
  head,
  disabled,
  historical,
  writer,
  onBlockedChange,
}: {
  scope: ScriptScope;
  versionId: string;
  head: ScriptWorkspace;
  disabled: boolean;
  historical: boolean;
  writer: Reviewer;
  onBlockedChange: (value: boolean) => void;
}) {
  const [selected, setSelected] = useState<string>();
  const [frame, setFrame] = useState<Frame>();
  const [discard, setDiscard] = useState(false);
  const [splitHistory, setSplitHistory] = useState(false);
  const version = useQuery({
    queryKey: [...reviewScopeKey(scope), "version-summary", versionId],
    queryFn: ({ signal }) => findVersion(scope, versionId, signal),
    staleTime: Infinity,
  });
  const episodes = useQuery({
    queryKey: [...reviewScopeKey(scope), "episodes", versionId],
    queryFn: ({ signal }) => getEpisodes(scope, versionId, undefined, signal),
    staleTime: 0,
  });
  let view: EpisodeView | undefined;
  let validationError: string | undefined;
  if (version.data && episodes.data) {
    try {
      view = readEpisodeView(
        episodes.data,
        scope,
        versionId,
        version.data.char_count,
      );
    } catch {
      validationError = "候选分集与不可变剧本长度不一致，请重新读取。";
    }
  }
  const chosen =
    view?.episodes.find((episode) => episode.id === selected) ??
    view?.episodes[0];
  const locked = disabled || writer.locked;
  const canEdit = !locked && !historical;
  const base = view
    ? {
        version_id: versionId,
        expected_revision: head.state.revision,
        expected_split_revision: view.head.split_revision,
        candidate_set_id: view.head.candidate_split_set_id,
      }
    : undefined;
  function open(next: Frame) {
    setFrame(next);
    setDiscard(false);
    onBlockedChange(true);
  }
  function close() {
    if (writer.intent || writer.busy || writer.storageError) return;
    setFrame(undefined);
    setDiscard(false);
    onBlockedChange(false);
  }
  const feedback = (
    <>
      <ReviewRecovery writer={writer} />
      <ReviewConflict
        key={writer.rejected?.intent.key}
        scope={scope}
        writer={writer}
        onDiscard={() => {
          writer.acknowledgeLatest();
          setFrame(undefined);
          setDiscard(false);
          onBlockedChange(false);
        }}
      />
    </>
  );
  return (
    <section
      className="space-y-4 rounded-xl border p-4"
      aria-label="剧本分集与正式结构"
    >
      <h2 className="font-semibold">分集与正式结构</h2>
      {(version.isPending || episodes.isPending) && (
        <p role="status">正在独立读取版本摘要与当前分集…</p>
      )}
      {(version.error || episodes.error || validationError) && (
        <>
          <p role="alert">
            {validationError ??
              version.error?.message ??
              episodes.error?.message}
          </p>
          <Button
            type="button"
            variant="outline"
            onClick={() => {
              void version.refetch();
              void episodes.refetch();
            }}
          >
            重新读取分集事实
          </Button>
        </>
      )}
      {!frame && feedback}
      {view && version.data && base && (
        <>
          <p className="text-sm break-all">
            不可变剧本 {version.data.version_no} ·{" "}
            {version.data.char_count.toLocaleString("zh-CN")} 字符 · 分集修订{" "}
            {view.head.split_revision}。候选来自
            {view.candidate.origin === "rules" ? "规则" : "来源边界"}
            ；正式集以人工确认的集合为准。
          </p>
          <p className="text-xs break-all">
            候选集 {view.head.candidate_split_set_id}；正式集集合{" "}
            {view.head.confirmed_split_set_id ?? "尚未确认"}；当前采纳版本{" "}
            {head.state.adopted_version_id ?? "尚未采纳"}。
          </p>
          {view.candidate.warnings?.map((warning, index) => (
            <p key={index} className="text-sm text-amber-700">
              {warning}
            </p>
          ))}
          <div className="flex flex-wrap gap-2">
            <Button
              type="button"
              variant="outline"
              disabled={!canEdit || !version.data.char_count}
              onClick={() =>
                open({
                  type: "split",
                  base,
                  view: view!,
                  version: version.data!,
                })
              }
            >
              编辑并确认完整分集
            </Button>
            <Button
              type="button"
              variant="outline"
              disabled={!canEdit || !version.data.char_count}
              onClick={() =>
                open({
                  type: "control",
                  title: "按规则重新生成分集候选",
                  command: {
                    action: "resplit",
                    body: { ...base, ack_invalidate: false },
                  },
                })
              }
            >
              按规则重新分集
            </Button>
            <Button
              type="button"
              variant="outline"
              onClick={() => setSplitHistory((value) => !value)}
            >
              {splitHistory ? "收起分集确认历史" : "完整分集确认历史"}
            </Button>
            <Button
              type="button"
              disabled={
                locked ||
                !view.head.confirmed_split_set_id ||
                !view.episodes.length
              }
              onClick={() =>
                open({
                  type: "control",
                  title: "明确采纳已确认剧本",
                  view,
                  version: version.data,
                  command: {
                    action: "adopt",
                    versionId,
                    splitSetId: view!.head.confirmed_split_set_id!,
                    episodeIds: view!.episodes.map((episode) => episode.id),
                    body: {
                      expected_revision: head.state.revision,
                      expected_split_revision: view!.head.split_revision,
                      ack_invalidate: false,
                    },
                  },
                })
              }
            >
              采纳此已确认版本
            </Button>
          </div>
          <VersionTextInspector
            key={versionId}
            scope={scope}
            version={version.data}
          />
          {splitHistory && (
            <SplitHistory
              scope={scope}
              version={version.data}
              writer={writer}
              scriptRevision={head.state.revision}
            />
          )}
          {view.episodes.length ? (
            <div className="grid min-w-0 gap-4 lg:grid-cols-[260px_minmax(0,1fr)]">
              <ul className="max-h-96 space-y-2 overflow-y-auto">
                {view.episodes.map((episode) => (
                  <li key={episode.id}>
                    <Button
                      type="button"
                      variant={
                        episode.id === chosen?.id ? "secondary" : "outline"
                      }
                      className="h-auto w-full justify-start whitespace-normal"
                      onClick={() => setSelected(episode.id)}
                    >
                      第{episode.seq_no}集 · {episode.title} · [
                      {episode.span_start}, {episode.span_end})
                    </Button>
                  </li>
                ))}
              </ul>
              {chosen && (
                <EpisodeStructure
                  key={chosen.id}
                  scope={scope}
                  episode={chosen}
                  version={version.data}
                  scriptRevision={head.state.revision}
                  disabled={disabled || historical}
                  writer={writer}
                  onBlockedChange={onBlockedChange}
                />
              )}
            </div>
          ) : (
            <p>尚无正式集。先完整阅读候选与规范原文，再人工确认分集。</p>
          )}
        </>
      )}
      {frame && (
        <Dialog
          open
          onOpenChange={(open) => {
            if (!open && !writer.locked) setDiscard(true);
          }}
        >
          <DialogContent
            showCloseButton={!writer.locked}
            className="max-h-[calc(100dvh-2rem)] overflow-y-auto sm:max-w-4xl"
            onEscapeKeyDown={(event) => {
              if (writer.locked) event.preventDefault();
            }}
            onInteractOutside={(event) => {
              if (writer.locked) event.preventDefault();
            }}
          >
            <DialogHeader>
              <DialogTitle>
                {frame.type === "split" ? "完整分集人工审核" : frame.title}
              </DialogTitle>
              <DialogDescription>
                此表单冻结所见版本。后端原子核验脚本、分集及下游影响；回执尚未确认时保留原键和完整原体。
              </DialogDescription>
            </DialogHeader>
            {feedback}
            {frame.type === "split" ? (
              <>
                <VersionTextInspector scope={scope} version={frame.version} />
                <EpisodeSplitForm
                  base={frame.base}
                  initialBoundaries={frame.view.candidate.boundaries}
                  initialPreface={frame.view.candidate.preface}
                  charCount={frame.version.char_count}
                  locked={locked}
                  onDirty={() => onBlockedChange(true)}
                  onSubmit={(body) =>
                    writer.submit({
                      action: "confirm_split",
                      charCount: frame.version.char_count,
                      body,
                    })
                  }
                />
              </>
            ) : (
              <>
                {frame.version && (
                  <VersionTextInspector scope={scope} version={frame.version} />
                )}
                {frame.view && (
                  <ol className="space-y-2 text-sm">
                    {frame.view.episodes.map((episode) => (
                      <li key={episode.id} className="break-all">
                        第{episode.seq_no}集 {episode.title} · [
                        {episode.span_start}, {episode.span_end}) · {episode.id}
                      </li>
                    ))}
                  </ol>
                )}
                <ExplicitControl
                  command={frame.command}
                  disabled={locked}
                  onSubmit={() => writer.submit(frame.command)}
                />
              </>
            )}
            {discard && !writer.locked && (
              <section className="space-y-2 rounded-lg border p-3">
                <p>
                  未提交的分集修改仍在此表单。明确关闭后丢弃本地草稿，原历史与正式内容保留。
                </p>
                <div className="flex flex-wrap gap-2">
                  <Button
                    type="button"
                    variant="outline"
                    onClick={() => setDiscard(false)}
                  >
                    继续审阅分集
                  </Button>
                  <Button type="button" variant="destructive" onClick={close}>
                    明确放弃分集草稿并关闭
                  </Button>
                </div>
              </section>
            )}
            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                disabled={writer.locked}
                onClick={() => setDiscard(true)}
              >
                保留历史并关闭审核
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      )}
    </section>
  );
}
function ExplicitControl({
  command,
  disabled,
  onSubmit,
}: {
  command: ReviewCommand;
  disabled: boolean;
  onSubmit: () => void | Promise<void>;
}) {
  const [checked, setChecked] = useState(false);
  return (
    <div className="space-y-3">
      <p>
        {command.action === "resplit"
          ? "按规范正文重新运行规则，只生成新候选。正式分集仍需人工确认。"
          : command.action === "adopt"
            ? "采纳此不可变版本及其当前完整正式分集。已有采纳版本或结构的影响必须由服务端真实验证。"
            : "确认所见完整结构候选，当前候选与已确认版本分别保留。"}
      </p>
      <details>
        <summary className="cursor-pointer">完整原提交正文</summary>
        <pre className="max-h-52 overflow-auto text-xs break-all whitespace-pre-wrap">
          {JSON.stringify(command.body, null, 2)}
        </pre>
      </details>
      <label className="flex items-start gap-2 text-sm">
        <input
          type="checkbox"
          disabled={disabled}
          checked={checked}
          onChange={(event) => setChecked(event.target.checked)}
        />
        我已阅读此完整候选和版本事实，明确执行上述操作。
      </label>
      <Button
        type="button"
        disabled={disabled || !checked}
        onClick={() => void onSubmit()}
      >
        明确提交此审核
      </Button>
    </div>
  );
}
function SplitHistory({
  scope,
  version,
  writer,
  scriptRevision,
}: {
  scope: ScriptScope;
  version: ScriptVersionSummary;
  writer: Reviewer;
  scriptRevision: number;
}) {
  const [selected, setSelected] = useState<string>();
  const [episodeID, setEpisodeID] = useState<string>();
  const list = useInfiniteQuery({
    queryKey: [...reviewScopeKey(scope), "split-history", version.id],
    initialPageParam: 0,
    queryFn: ({ pageParam, signal }) =>
      listConfirmations(scope, version.id, pageParam, signal),
    getNextPageParam: (page) => page.next_revision,
    staleTime: 0,
  });
  const detail = useQuery({
    queryKey: [
      ...reviewScopeKey(scope),
      "split-confirmation",
      version.id,
      selected,
    ],
    queryFn: ({ signal }) => getConfirmation(scope, version, selected!, signal),
    enabled: Boolean(selected),
    staleTime: Infinity,
  });
  const items = list.data?.pages.flatMap((page) => page.items) ?? [];
  const duplicates =
    new Set(items.map((item) => item.id)).size !== items.length;
  return (
    <section
      aria-label="不可变分集确认历史"
      className="space-y-3 rounded-lg border p-3"
    >
      {list.isPending && <p role="status">正在读取分集确认摘要…</p>}
      {(list.error || duplicates) && (
        <p role="alert">
          {duplicates ? "确认分页身份重复，请刷新事实。" : list.error?.message}
        </p>
      )}
      <ul className="max-h-52 space-y-2 overflow-auto">
        {!duplicates &&
          items.map((item) => (
            <li key={item.id}>
              <Button
                type="button"
                variant="outline"
                className="h-auto whitespace-normal"
                onClick={() => {
                  setSelected(item.id);
                  setEpisodeID(undefined);
                }}
              >
                查看分集确认修订 {item.revision} · {item.episode_count} 集 ·{" "}
                {new Date(item.created_at).toLocaleString("zh-CN")}
              </Button>
            </li>
          ))}
      </ul>
      {(list.hasNextPage || list.isError) && (
        <Button
          type="button"
          variant="outline"
          disabled={list.isFetching}
          onClick={() =>
            void (list.isError ? list.refetch() : list.fetchNextPage())
          }
        >
          读取更早确认或重试
        </Button>
      )}
      {detail.isFetching && <p role="status">正在读取完整原分集确认…</p>}
      {detail.error && (
        <>
          <p role="alert">{detail.error.message}</p>
          <Button
            type="button"
            variant="outline"
            onClick={() => void detail.refetch()}
          >
            重试读取该确认
          </Button>
        </>
      )}
      {detail.data && (
        <>
          <p className="text-xs break-all">
            确认 {detail.data.id}；原候选 {detail.data.candidate_set_id}
            ；原正式集集合 {detail.data.formal_set_id}；序言{" "}
            {detail.data.preface
              ? `[${detail.data.preface.start}, ${detail.data.preface.end})`
              : "无"}
            。
          </p>
          <ol className="space-y-2">
            {detail.data.episodes.map((episode) => (
              <li key={episode.id} className="text-sm break-all">
                第{episode.seq_no}集 {episode.title} · [{episode.span_start},{" "}
                {episode.span_end}) · 原正式ID {episode.id}
                <Button
                  type="button"
                  variant="outline"
                  className="ml-2"
                  onClick={() => setEpisodeID(episode.id)}
                >
                  查看此历史集的全部结构
                </Button>
              </li>
            ))}
          </ol>
          {detail.data.episodes.find((episode) => episode.id === episodeID) && (
            <EpisodeStructure
              key={`${detail.data.id}:${episodeID}`}
              scope={scope}
              episode={detail.data.episodes.find(
                (episode) => episode.id === episodeID,
              )!}
              version={version}
              scriptRevision={scriptRevision}
              disabled
              writer={writer}
              onBlockedChange={() => {}}
            />
          )}
        </>
      )}
    </section>
  );
}
function EpisodeStructure({
  scope,
  episode,
  version,
  scriptRevision,
  disabled,
  writer,
  onBlockedChange,
}: {
  scope: ScriptScope;
  episode: ScriptEpisode;
  version: ScriptVersionSummary;
  scriptRevision: number;
  disabled: boolean;
  writer: Reviewer;
  onBlockedChange: (value: boolean) => void;
}) {
  const [history, setHistory] = useState(false);
  const [selectedVersion, setSelectedVersion] = useState<number>();
  const [edit, setEdit] = useState<{
    base: StructureBase;
    document: StructureDocument;
    key: string;
  }>();
  const [confirm, setConfirm] = useState<{
    command: ReviewCommand;
    document: StructureDocument;
    id: string;
    versionNo: number;
  }>();
  const [discard, setDiscard] = useState(false);
  const current = useQuery({
    queryKey: [
      ...reviewScopeKey(scope),
      "structure",
      episode.id,
      selectedVersion ?? "current",
    ],
    queryFn: ({ signal }) =>
      getStructure(scope, episode.id, selectedVersion, signal),
    enabled: Boolean(selectedVersion || episode.current_structure_id),
    staleTime: 0,
  });
  const versions = useInfiniteQuery({
    queryKey: [...reviewScopeKey(scope), "structure-versions", episode.id],
    initialPageParam: 0,
    queryFn: ({ pageParam, signal }) =>
      listStructureVersions(scope, episode.id, pageParam, signal),
    getNextPageParam: (page) => page.next_version_no,
    enabled: history,
    staleTime: 0,
  });
  const items = versions.data?.pages.flatMap((page) => page.items) ?? [];
  const duplicates =
    new Set(items.map((item) => item.id)).size !== items.length;
  const body = current.data?.structure;
  const currentEpisode = current.data?.episode ?? episode;
  const currentOnly = selectedVersion === undefined;
  const invalidSource = Boolean(
    body &&
    (body.source_hash !== version.content_hash ||
      currentEpisode.script_version_id !== version.id ||
      currentEpisode.span_start !== episode.span_start ||
      currentEpisode.span_end !== episode.span_end),
  );
  const locked = disabled || writer.locked;
  const canEdit =
    !locked &&
    currentOnly &&
    !currentEpisode.is_delete &&
    !invalidSource &&
    !current.isFetching &&
    !current.isError;
  const base = {
    expected_revision: scriptRevision,
    expected_episode_revision: currentEpisode.revision,
    base_structure_version_no: body?.version_no ?? 0,
  };
  const feedback = (
    <>
      <ReviewRecovery writer={writer} />
      <ReviewConflict
        key={writer.rejected?.intent.key}
        scope={scope}
        writer={writer}
        onDiscard={() => {
          writer.acknowledgeLatest();
          setEdit(undefined);
          setConfirm(undefined);
          setDiscard(false);
          onBlockedChange(false);
        }}
      />
    </>
  );
  function close() {
    if (writer.locked) return;
    setEdit(undefined);
    setConfirm(undefined);
    setDiscard(false);
    onBlockedChange(false);
  }
  return (
    <section className="min-w-0 space-y-3" aria-label="选中正式集结构">
      <h3 className="font-medium">
        第{episode.seq_no}集 · {episode.title}
      </h3>
      <p className="text-xs break-all">
        正式集 {episode.id} · 集修订 {currentEpisode.revision} · 当前候选{" "}
        {currentEpisode.current_structure_id ?? "无"} · 已确认结构{" "}
        {currentEpisode.confirmed_structure_id ?? "无"} · 继承状态{" "}
        {currentEpisode.inherit_status}。
      </p>
      <VersionTextInspector
        key={episode.id}
        scope={scope}
        version={version}
        start={episode.span_start}
        end={episode.span_end}
      />
      <div className="flex flex-wrap gap-2">
        <Button
          type="button"
          variant="outline"
          onClick={() => setHistory((value) => !value)}
        >
          {history ? "收起结构历史" : "完整结构版本历史"}
        </Button>
        {!currentOnly && (
          <Button
            type="button"
            variant="outline"
            onClick={() => setSelectedVersion(undefined)}
          >
            返回当前结构候选
          </Button>
        )}
        <Button
          type="button"
          disabled={!canEdit}
          onClick={() => {
            setEdit({
              base,
              document: body?.document ?? { scenes: [], unassigned_lines: [] },
              key: crypto.randomUUID(),
            });
            setDiscard(false);
            onBlockedChange(true);
          }}
        >
          手工编辑结构候选
        </Button>
        <Button
          type="button"
          variant="outline"
          disabled={
            !canEdit ||
            !body ||
            Boolean(body.document.unassigned_lines.length) ||
            currentEpisode.current_structure_id !== body.id
          }
          onClick={() => {
            if (!body) return;
            setConfirm({
              id: body.id,
              versionNo: body.version_no,
              document: body.document,
              command: {
                action: "confirm_structure",
                episodeId: episode.id,
                structureId: body.id,
                body: { ...base, ack_invalidate: false },
              },
            });
            setDiscard(false);
            onBlockedChange(true);
          }}
        >
          人工确认当前结构
        </Button>
      </div>
      {body?.document.unassigned_lines.length ? (
        <p>
          当前候选仍有 {body.document.unassigned_lines.length}{" "}
          条未归属行，处理完成后才能确认。
        </p>
      ) : null}
      {current.isFetching && <p role="status">正在读取选中结构正文…</p>}
      {current.error && (
        <>
          <p role="alert">{current.error.message}</p>
          <Button
            type="button"
            variant="outline"
            onClick={() => void current.refetch()}
          >
            重试读取所选结构
          </Button>
        </>
      )}
      {invalidSource && (
        <p role="alert">
          结构正文与本集不可变来源不一致，请重新读取，当前禁止保存或确认。
        </p>
      )}
      {!body && !current.isFetching && !current.isError && (
        <p>尚无结构候选，可按规范原文手工创建完整结构。</p>
      )}
      {body && (
        <details>
          <summary className="cursor-pointer">
            完整阅读{currentOnly ? "当前" : "历史"}结构版本 {body.version_no}
          </summary>
          <p className="text-xs break-all">
            结构 {body.id} · 原文 SHA {body.source_hash} ·{" "}
            {new Date(body.created_at).toLocaleString("zh-CN")}
          </p>
          <StructureEditForm
            key={body.id}
            scope={scope}
            base={base}
            initialDocument={body.document}
            episodeStart={episode.span_start}
            episodeEnd={episode.span_end}
            locked
            onDirty={() => {}}
            onSubmit={() => {}}
          />
        </details>
      )}
      {history && (
        <section aria-label="结构历史摘要" className="space-y-2">
          {versions.isPending && <p role="status">正在读取结构历史摘要…</p>}
          {(versions.error || duplicates) && (
            <p role="alert">
              {duplicates
                ? "结构分页出现重复身份，请重新读取。"
                : versions.error?.message}
            </p>
          )}
          <ul className="max-h-52 space-y-2 overflow-auto">
            {!duplicates &&
              items.map((item) => (
                <li key={item.id}>
                  <Button
                    type="button"
                    variant="outline"
                    className="h-auto whitespace-normal"
                    onClick={() => setSelectedVersion(item.version_no)}
                  >
                    读取结构版本 {item.version_no} ·{" "}
                    {new Date(item.created_at).toLocaleString("zh-CN")}
                  </Button>
                </li>
              ))}
          </ul>
          {(versions.hasNextPage || versions.isError) && (
            <Button
              type="button"
              variant="outline"
              disabled={versions.isFetching}
              onClick={() =>
                void (versions.isError
                  ? versions.refetch()
                  : versions.fetchNextPage())
              }
            >
              读取更早结构或重试
            </Button>
          )}
        </section>
      )}
      {(edit || confirm) && (
        <Dialog
          open
          onOpenChange={(open) => {
            if (!open && !writer.locked) setDiscard(true);
          }}
        >
          <DialogContent
            showCloseButton={!writer.locked}
            className="max-h-[calc(100dvh-2rem)] overflow-y-auto sm:max-w-4xl"
            onEscapeKeyDown={(event) => {
              if (writer.locked) event.preventDefault();
            }}
            onInteractOutside={(event) => {
              if (writer.locked) event.preventDefault();
            }}
          >
            <DialogHeader>
              <DialogTitle>
                {edit ? "完整手工结构编辑" : "正式结构人工确认"}
              </DialogTitle>
              <DialogDescription>
                原场景与台词身份保持稳定；保存生成新的不可变候选，确认不会替换候选正文。需要角色或下游影响信息时，依赖不可用会保持原事实。
              </DialogDescription>
            </DialogHeader>
            {feedback}
            <VersionTextInspector
              scope={scope}
              version={version}
              start={episode.span_start}
              end={episode.span_end}
            />
            {edit && (
              <StructureEditForm
                key={edit.key}
                scope={scope}
                base={edit.base}
                initialDocument={edit.document}
                episodeStart={episode.span_start}
                episodeEnd={episode.span_end}
                locked={locked}
                onDirty={() => onBlockedChange(true)}
                onSubmit={(body) =>
                  writer.submit({
                    action: "save_structure",
                    episodeId: episode.id,
                    spanStart: episode.span_start,
                    spanEnd: episode.span_end,
                    body,
                  })
                }
              />
            )}
            {confirm && (
              <>
                <p>
                  完整候选版本 {confirm.versionNo} · ID {confirm.id}
                </p>
                {body && (
                  <pre className="max-h-72 overflow-auto text-xs break-all whitespace-pre-wrap">
                    {JSON.stringify(confirm.document, null, 2)}
                  </pre>
                )}
                <ExplicitControl
                  command={confirm.command}
                  disabled={locked}
                  onSubmit={() => writer.submit(confirm.command)}
                />
              </>
            )}
            {discard && !writer.locked && (
              <section className="space-y-2 rounded-lg border p-3">
                <p>
                  未提交的结构草稿仍在此表单。关闭只丢弃本地草稿，服务端历史保持。
                </p>
                <div className="flex flex-wrap gap-2">
                  <Button
                    type="button"
                    variant="outline"
                    onClick={() => setDiscard(false)}
                  >
                    继续编辑结构
                  </Button>
                  <Button type="button" variant="destructive" onClick={close}>
                    明确放弃结构草稿并关闭
                  </Button>
                </div>
              </section>
            )}
            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                disabled={writer.locked}
                onClick={() => setDiscard(true)}
              >
                保留历史并关闭结构审核
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      )}
    </section>
  );
}
