"use client";
import { Alert, AlertDescription } from "@/components/ui/alert";

import { useEffect, useState } from "react";
import useSWR from "swr";
import {
  adoptScriptVersion,
  confirmScriptEpisodeSplit,
  confirmScriptEpisodeStructure,
  getScriptEpisodeSourceText,
  getScriptEpisodeStructure,
  listScriptEpisodes,
  resplitScriptEpisodes,
  saveScriptEpisodeStructure,
} from "@/api/script";
import { isProjectID } from "@/components/projects/project";
import { Button } from "@/components/ui/button";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
import { Checkbox } from "@/components/ui/checkbox";
import { Field, FieldLabel } from "@/components/ui/field";
import { Textarea } from "@/components/ui/textarea";
import { RequestError } from "@/lib/request";
import { useScriptWrite } from "./use-script-write";

type Scope = {
  owner: string;
  projectID: string;
  head: API.ProjectState;
  writable: boolean;
  onChange: () => Promise<unknown>;
  onLock: (locked: boolean) => void;
  onAssistant: (id: string, title: string, source: string) => void;
  onShot: (text: string) => void;
  onRewrite: (
    episodeID: string,
    lineID: string,
    title: string,
    text: string,
  ) => void;
  suggestion?: { episodeID: string; lineID: string; text: string };
  onSuggestionApplied: () => void;
};

export function EpisodePanel(props: Scope) {
  const { owner, projectID, head, writable, onChange, onLock } = props;
  const version = head.draft_version_id ?? head.adopted_version_id;
  const [selection, setSelection] = useState<string>();
  const [ack, setAck] = useState(false);
  const write = useScriptWrite();
  const [editorLocked, setEditorLocked] = useState(false);
  const locked = write.locked || editorLocked;
  useEffect(() => {
    onLock(locked);
    return () => onLock(false);
  }, [locked, onLock]);
  const result = useSWR(
    version ? [owner, projectID, "episodes", version] : null,
    () => listScriptEpisodes({ pid: projectID, version_id: version! }),
  );
  const candidate = result.data?.candidate;
  const split = result.data?.head;
  const episodes = result.data?.episodes ?? [];
  const current =
    episodes.find((episode) => episode.id === selection) ?? episodes[0];
  const replace = isProjectID(head.adopted_version_id);

  function review(action: "confirm" | "rules" | "adopt") {
    if (
      locked ||
      !writable ||
      !isProjectID(version) ||
      !Number.isSafeInteger(split?.split_revision) ||
      !head.revision
    )
      return;
    if (
      action === "adopt"
        ? !isProjectID(split?.confirmed_split_set_id)
        : !isProjectID(candidate?.id)
    )
      return;
    const key = crypto.randomUUID();
    const pid = { pid: projectID };
    const base = {
      expected_revision: head.revision,
      expected_split_revision: split!.split_revision,
      ack_invalidate: ack,
    };
    void write.submit(async ({ signal }) => {
      const options = { signal, headers: { "Idempotency-Key": key } };
      if (action === "adopt") {
        const receipt = await adoptScriptVersion(
          { ...pid, vid: version },
          base,
          options,
        );
        if (receipt.version_id !== version)
          throw new RequestError("invalid_receipt");
      } else {
        const body = {
          ...base,
          version_id: version,
          candidate_set_id: candidate!.id,
        };
        const receipt =
          action === "confirm"
            ? await confirmScriptEpisodeSplit(
                pid,
                {
                  ...body,
                  boundaries: candidate!.boundaries,
                  preface: candidate!.preface,
                },
                options,
              )
            : await resplitScriptEpisodes(pid, body, options);
        if (!isProjectID(receipt.split_set_id))
          throw new RequestError("invalid_receipt");
      }
      if (signal.aborted) return;
      setAck(false);
      void onChange();
      void result.mutate();
    });
  }

  return (
    <section className="flex flex-col gap-4">
      <h2 className="text-lg font-semibold">分集视频</h2>
      {!version && (
        <p className="text-sm text-muted-foreground">
          导入剧本或保存创作草稿后，即可审阅分集。
        </p>
      )}
      {result.isLoading && <p role="status">正在读取分集…</p>}
      {result.error && (
        <Alert role="alert">
          <AlertDescription>
            无法读取分集。{" "}
            <Button variant="outline" onClick={() => void result.mutate()}>
              重试
            </Button>
          </AlertDescription>
        </Alert>
      )}
      {candidate?.boundaries?.length ? (
        <div className="flex flex-col gap-3 rounded-lg border p-4">
          <h3 className="font-medium">
            候选分集 · {candidate.boundaries.length} 集
          </h3>
          <ol className="max-h-60 overflow-auto text-sm">
            {candidate.boundaries.map((item) => (
              <li key={item.seq_no} className="py-1">
                第 {item.seq_no} 集 · {item.title} ·{" "}
                {(item.span_end ?? 0) - (item.span_start ?? 0)} 字
              </li>
            ))}
          </ol>
          {candidate.warnings?.length ? (
            <p className="text-sm text-muted-foreground">
              分集提示：{candidate.warnings.join("、")}
            </p>
          ) : null}
          {writable && replace && (
            <Field orientation="horizontal">
              <Checkbox
                id="replace-episode-split"
                checked={ack}
                disabled={locked}
                onCheckedChange={(value) => setAck(value === true)}
              />
              <FieldLabel htmlFor="replace-episode-split">
                我已审阅变更，并确认可能影响已有分集和下游引用。
              </FieldLabel>
            </Field>
          )}
          {writable && (
            <div className="flex flex-wrap gap-2">
              <Button
                variant="outline"
                disabled={locked}
                onClick={() => review("rules")}
              >
                按剧本标题重新分集
              </Button>
              <Button
                disabled={locked || (replace && !ack)}
                onClick={() => review("confirm")}
              >
                确认分集
              </Button>
              {isProjectID(split?.confirmed_split_set_id) &&
                head.adopted_version_id !== version && (
                  <Button
                    variant="secondary"
                    disabled={locked || (replace && !ack)}
                    onClick={() => review("adopt")}
                  >
                    采纳为正式剧本
                  </Button>
                )}
            </div>
          )}
        </div>
      ) : null}
      {write.error && (
        <Alert role="alert">
          <AlertDescription>
            {write.error}{" "}
            {write.unknown && (
              <Button
                disabled={write.pending}
                onClick={() => void write.submit()}
              >
                重试原操作
              </Button>
            )}
          </AlertDescription>
        </Alert>
      )}
      {episodes.length > 0 && (
        <div className="grid gap-4 lg:grid-cols-[14rem_minmax(0,1fr)]">
          <div
            className="flex max-h-80 flex-col gap-2 overflow-auto"
            aria-label="已确认剧集"
          >
            {episodes.map((episode) => (
              <Button
                variant={current?.id === episode.id ? "secondary" : "outline"}
                className="justify-start"
                disabled={locked}
                key={episode.id}
                onClick={() => setSelection(episode.id)}
              >
                <span className="truncate">
                  第 {episode.seq_no} 集 · {episode.title}
                </span>
              </Button>
            ))}
          </div>
          {isProjectID(current?.id) && (
            <EpisodeEditor
              key={current.id}
              {...props}
              onLock={setEditorLocked}
              episode={current}
              onSaved={async () => {
                await Promise.all([onChange(), result.mutate()]);
              }}
            />
          )}
        </div>
      )}
    </section>
  );
}

function EpisodeEditor({
  episode,
  owner,
  projectID,
  head,
  writable,
  onSaved,
  onAssistant,
  onLock,
  onShot,
  onRewrite,
  suggestion,
  onSuggestionApplied,
}: Scope & { episode: API.Episode; onSaved: () => Promise<void> }) {
  const id = episode.id!;
  const start = episode.span_start ?? 0;
  const end = episode.span_end ?? start;
  const stop = Math.min(end, start + 65_536);
  const write = useScriptWrite();
  useEffect(() => {
    onLock(write.locked);
    return () => onLock(false);
  }, [write.locked, onLock]);
  const source = useSWR(
    stop > start ? [owner, projectID, "episode-source", id, start, stop] : null,
    () => getScriptEpisodeSourceText({ eid: id, from: start, to: stop }),
  );
  const saved = useSWR(
    [owner, projectID, "episode-structure", id, episode.revision],
    async () => {
      try {
        return await getScriptEpisodeStructure({ eid: id });
      } catch (error) {
        if (
          error instanceof RequestError &&
          error.status === 404 &&
          !isProjectID(episode.current_structure_id)
        )
          return null;
        throw error;
      }
    },
  );
  const [edited, setEdited] = useState<API.StructureDocument>();
  const persisted = saved.data?.structure?.document;
  const document = edited ?? persisted;
  const current = saved.data?.episode ?? episode;
  const versionNo = saved.data?.structure?.version_no ?? 0;

  function updateItem(sceneIndex: number, itemIndex: number, value: string) {
    if (!document?.scenes) return;
    setEdited({
      ...document,
      scenes: document.scenes.map((row, i) =>
        i !== sceneIndex
          ? row
          : {
              ...row,
              items: row.items?.map((item, j) =>
                j === itemIndex ? { ...item, content: value } : item,
              ),
            },
      ),
    });
  }

  function draftFromScript() {
    if (!source.data?.text || stop !== end) return;
    let offset = start;
    const items = source.data.text.split("\n").flatMap((line) => {
      const span_start = offset;
      const span_end = offset + Array.from(line).length;
      offset = span_end + 1;
      return line.trim()
        ? [
            {
              type: "action" as const,
              line_key: crypto.randomUUID(),
              content: line,
              span_start,
              span_end,
            },
          ]
        : [];
    });
    if (items.length)
      setEdited({
        scenes: [
          {
            scene_key: crypto.randomUUID(),
            seq_no: 1,
            heading: episode.title,
            span_start: start,
            span_end: end,
            items,
          },
        ],
      });
  }

  function save(confirm: boolean) {
    if (
      write.locked ||
      !writable ||
      !head.revision ||
      !current.revision ||
      (!confirm && !edited)
    )
      return;
    const key = crypto.randomUUID();
    const base = {
      expected_revision: head.revision,
      expected_episode_revision: current.revision,
      base_structure_version_no: versionNo,
    };
    void write.submit(async ({ signal }) => {
      const options = { signal, headers: { "Idempotency-Key": key } };
      const receipt = confirm
        ? await confirmScriptEpisodeStructure({ eid: id }, base, options)
        : await saveScriptEpisodeStructure(
            { eid: id },
            { ...base, document: edited },
            options,
          );
      if (!isProjectID(receipt.structure_id))
        throw new RequestError("invalid_receipt");
      if (signal.aborted) return;
      setEdited(undefined);
      void Promise.all([saved.mutate(), onSaved()]);
    });
  }

  return (
    <article className="flex min-w-0 flex-col gap-4 rounded-lg border p-4">
      <header className="flex flex-wrap items-center justify-between gap-2">
        <div>
          <h3 className="font-semibold">
            第 {episode.seq_no} 集 · {episode.title}
          </h3>
          {persisted && !edited && (
            <p className="text-xs text-muted-foreground">
              {isProjectID(current.confirmed_structure_id) &&
              current.confirmed_structure_id === current.current_structure_id
                ? "分镜已确认"
                : "分镜草稿已保存"}
            </p>
          )}
        </div>
        <Button
          variant="outline"
          disabled={!writable || !source.data?.text || write.locked}
          onClick={() =>
            onAssistant(
              id,
              episode.title ?? "本集",
              Array.from(source.data?.text ?? "")
                .slice(0, 1200)
                .join(""),
            )
          }
        >
          故事板助手
        </Button>
      </header>
      {source.error ? (
        <Alert role="alert">
          <AlertDescription>无法读取分集正文。</AlertDescription>
        </Alert>
      ) : (
        <Collapsible>
          <CollapsibleTrigger asChild>
            <Button variant="outline" size="sm">
              剧本正文{stop < end ? "（节选）" : ""}
            </Button>
          </CollapsibleTrigger>
          <CollapsibleContent>
            <pre className="mt-2 max-h-72 overflow-auto font-sans text-sm whitespace-pre-wrap">
              {source.data?.text ?? "正在读取…"}
            </pre>
          </CollapsibleContent>
        </Collapsible>
      )}
      {saved.error && (
        <Alert role="alert">
          <AlertDescription>无法读取分镜草稿。</AlertDescription>
        </Alert>
      )}
      {saved.isLoading && <p role="status">正在读取分镜草稿…</p>}
      {document?.scenes?.map((scene, sceneIndex) => (
        <section key={scene.scene_key} className="flex flex-col gap-2">
          <h4 className="text-sm font-medium">
            {scene.heading || `场景 ${sceneIndex + 1}`}
          </h4>
          {scene.items?.map((item, itemIndex) => (
            <Field key={item.line_key}>
              <FieldLabel htmlFor={item.line_key}>
                片段 {itemIndex + 1}
                {item.speaker_text ? ` · ${item.speaker_text}` : ""}
              </FieldLabel>
              <Textarea
                id={item.line_key}
                value={item.content ?? ""}
                disabled={!writable || write.locked}
                onChange={(event) =>
                  updateItem(sceneIndex, itemIndex, event.target.value)
                }
              />
              {writable && item.line_key && item.content?.trim() && (
                <div className="flex flex-wrap gap-2">
                  <Button
                    variant="outline"
                    size="sm"
                    disabled={write.locked}
                    onClick={() =>
                      onRewrite(
                        id,
                        item.line_key!,
                        episode.title ?? "本集",
                        item.content!,
                      )
                    }
                  >
                    请助手改写片段
                  </Button>
                  {!edited && (
                    <Button
                      variant="outline"
                      size="sm"
                      disabled={write.locked}
                      onClick={() => onShot(item.content!)}
                    >
                      生成片段
                    </Button>
                  )}
                  {suggestion?.episodeID === id &&
                    suggestion.lineID === item.line_key && (
                      <Button
                        variant="secondary"
                        size="sm"
                        disabled={write.locked}
                        onClick={() => {
                          updateItem(sceneIndex, itemIndex, suggestion.text);
                          onSuggestionApplied();
                        }}
                      >
                        应用助手建议
                      </Button>
                    )}
                </div>
              )}
            </Field>
          ))}
        </section>
      ))}
      {writable && !saved.error && !saved.isLoading && (
        <div className="flex flex-wrap gap-2">
          {!document && (
            <Button
              variant="outline"
              disabled={write.locked || !source.data?.text || stop !== end}
              onClick={draftFromScript}
            >
              从剧本正文拆出片段草稿
            </Button>
          )}
          {edited && (
            <Button
              disabled={
                write.locked ||
                !edited.scenes?.every((scene) =>
                  scene.items?.every((item) => !!item.content?.trim()),
                )
              }
              onClick={() => save(false)}
            >
              保存分镜草稿
            </Button>
          )}
          {persisted &&
            !edited &&
            current.current_structure_id !== current.confirmed_structure_id && (
              <Button
                disabled={
                  write.locked ||
                  head.adopted_version_id !== episode.script_version_id
                }
                onClick={() => save(true)}
              >
                确认分镜
              </Button>
            )}
        </div>
      )}
      {write.error && (
        <Alert role="alert">
          <AlertDescription>
            {write.error}{" "}
            {write.unknown && (
              <Button
                disabled={write.pending}
                onClick={() => void write.submit()}
              >
                重试原操作
              </Button>
            )}
          </AlertDescription>
        </Alert>
      )}
    </article>
  );
}
