"use client";

import { useEffect, useId, useRef, useState } from "react";
import { ArrowDown, ArrowUp, Film, MessageSquarePlus } from "lucide-react";
import { getCanvasStoryboard, writeCanvasStoryboard } from "@/api/canvases";
import { ProjectMemoryDialog } from "@/components/conversations/project-memory-dialog";
import { FeedbackAlert } from "@/components/feedback/feedback-alert";
import { ContentState } from "@/components/layout/content-state";
import { ProjectMediaPreview } from "@/components/media/project-media";
import { isProjectID } from "@/components/projects/project";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { readErrorMessage } from "@/lib/read-error";
import { RequestError } from "@/lib/request";
import { CanvasStoryboardDelivery } from "./canvas-storyboard-delivery";
import {
  shotReference,
  storyboardEdit,
  storyboardMove,
  type CanvasStoryboardShot,
  type ShotDraft,
  type StoryboardField,
} from "./canvas-storyboard-state";

type StoryboardView = {
  revision: number;
  board?: { id: string; shots: CanvasStoryboardShot[] };
};
type StoryboardWrite = {
  board_id: string;
  expected_revision: number;
  edit?: NonNullable<ReturnType<typeof storyboardEdit>>;
  move?: NonNullable<ReturnType<typeof storyboardMove>>;
};
type PendingEdit = {
  key: string;
  body: StoryboardWrite;
};
type ShotDrafts = Partial<Record<StoryboardField, ShotDraft>>;

const editableFields = [
  { field: "title", label: "镜头标题" },
  { field: "action", label: "动作" },
  { field: "duration_ms", label: "计划时长（秒）" },
  { field: "dialogue", label: "对白" },
  { field: "camera", label: "景别与运镜" },
  { field: "sound", label: "声音" },
] as const;

function displayValue(field: StoryboardField, value: string | number) {
  return field === "duration_ms" ? String(Number(value) / 1000) : String(value);
}

function lookupFailure(error: unknown) {
  return (
    error instanceof RequestError && [401, 403, 404].includes(error.status)
  );
}

function readStoryboard(
  receipt: API.StoryboardView,
  canvasID: string,
  projectID: string,
): StoryboardView {
  const revision = receipt?.revision;
  if (
    !receipt ||
    receipt.canvas_id !== canvasID ||
    receipt.project_id !== projectID ||
    typeof revision !== "number" ||
    !Number.isSafeInteger(revision) ||
    revision < 1
  )
    throw new Error("invalid_storyboard_receipt");
  const board = receipt.board;
  if (
    board &&
    (!isProjectID(board.id) ||
      !Array.isArray(board.shots) ||
      new Set(board.shots.map((shot) => shot.id)).size !== board.shots.length ||
      board.shots.some(
        (shot) =>
          !isProjectID(shot.id) ||
          !Number.isSafeInteger(shot.revision) ||
          shot.revision! < 1 ||
          typeof shot.title !== "string" ||
          typeof shot.action !== "string" ||
          !Number.isSafeInteger(shot.duration_ms) ||
          shot.duration_ms! < 1 ||
          (shot.adopted &&
            (!isProjectID(shot.adopted.asset_id) ||
              !isProjectID(shot.adopted.output_id) ||
              !isProjectID(shot.adopted.operation_id) ||
              !Number.isSafeInteger(shot.adopted.asset_version) ||
              !Number.isSafeInteger(shot.adopted.shot_revision))),
      ))
  )
    throw new Error("invalid_storyboard_receipt");
  return {
    revision,
    ...(board ? { board: board as StoryboardView["board"] } : {}),
  };
}

export function CanvasStoryboard({
  active,
  canvasID,
  projectID,
  commitSeq,
  writable,
  canReference,
  onDiscuss,
  onReference,
}: {
  active: boolean;
  canvasID: string;
  projectID: string;
  commitSeq: string;
  writable: boolean;
  canReference: boolean;
  onDiscuss: () => void;
  onReference: (reference: string) => void;
}) {
  const [data, setData] = useState<StoryboardView>();
  const [selectedID, setSelectedID] = useState("");
  const [memoryShot, setMemoryShot] = useState<CanvasStoryboardShot>();
  const [drafts, setDrafts] = useState<Record<string, ShotDrafts>>({});
  const [readError, setReadError] = useState("");
  const [writeError, setWriteError] = useState("");
  const [refresh, setRefresh] = useState(0);
  const [pending, setPending] = useState<PendingEdit>();
  const write = useRef<AbortController | null>(null);
  const [busy, setBusy] = useState(false);
  useEffect(() => () => write.current?.abort(), []);

  useEffect(() => {
    if (!active) return;
    const controller = new AbortController();
    void getCanvasStoryboard({ id: canvasID }, { signal: controller.signal })
      .then((response) => {
        if (controller.signal.aborted) return;
        const next = readStoryboard(response, canvasID, projectID);
        setData((previous) =>
          previous && previous.revision > next.revision ? previous : next,
        );
        setReadError("");
      })
      .catch((error) => {
        if (controller.signal.aborted) return;
        if (lookupFailure(error)) setData(undefined);
        setReadError(readErrorMessage(error));
      });
    return () => controller.abort();
  }, [active, canvasID, projectID, commitSeq, refresh]);

  async function save(command: PendingEdit) {
    if (write.current || !writable) return;
    const controller = new AbortController();
    write.current = controller;
    setPending(command);
    setBusy(true);
    setWriteError("");
    try {
      const response = await writeCanvasStoryboard(
        { id: canvasID },
        // Swagger 2 models RawMessage as an object. The edit field accepts the checked scalar above.
        command.body as unknown as API.StoryboardCommand,
        {
          signal: controller.signal,
          headers: {
            "Content-Type": "application/json",
            "Idempotency-Key": command.key,
          },
        },
      );
      if (controller.signal.aborted) return;
      const next = readStoryboard(response, canvasID, projectID);
      setData((previous) =>
        previous && previous.revision > next.revision ? previous : next,
      );
      const edit = command.body.edit;
      if (edit)
        setDrafts((previous) => ({
          ...previous,
          [edit.shot_id]: {
            ...previous[edit.shot_id],
            [edit.field]: undefined,
          },
        }));
      setPending(undefined);
    } catch (error) {
      if (controller.signal.aborted) return;
      if (error instanceof RequestError && error.status === 409) {
        setPending(undefined);
        setWriteError("故事板已有更新，正在读取当前版本。");
        setRefresh((current) => current + 1);
      } else if (
        error instanceof RequestError &&
        error.status >= 400 &&
        error.status < 500 &&
        error.status !== 408 &&
        error.status !== 429
      ) {
        setPending(undefined);
        if (lookupFailure(error)) {
          setData(undefined);
          setRefresh((current) => current + 1);
        }
        setWriteError(readErrorMessage(error));
      } else {
        setWriteError("保存结果待确认，草稿已保留。");
      }
    } finally {
      if (!controller.signal.aborted) {
        write.current = null;
        setBusy(false);
      }
    }
  }

  const board = data?.board;
  const shots = board?.shots;
  const selected = shots?.find((shot) => shot.id === selectedID);
  const index = selected && shots ? shots.indexOf(selected) : -1;

  function apply(change: Pick<StoryboardWrite, "edit" | "move">) {
    if (!board || !data || pending) return;
    void save({
      key: crypto.randomUUID(),
      body: { board_id: board.id, expected_revision: data.revision, ...change },
    });
  }

  return (
    <div className="space-y-3 p-3 sm:p-4">
      {memoryShot && (
        <ProjectMemoryDialog
          key={memoryShot.id}
          projectID={projectID}
          initialStatement=""
          shot={memoryShot}
          onClose={() => setMemoryShot(undefined)}
        />
      )}
      {readError ? (
        <FeedbackAlert
          tone="error"
          title={readError}
          actions={
            <Button
              variant="outline"
              size="feedback"
              onClick={() => setRefresh((current) => current + 1)}
            >
              重新读取
            </Button>
          }
        />
      ) : null}
      {writeError ? (
        <FeedbackAlert
          tone={pending ? "info" : "error"}
          title={writeError}
          actions={
            pending && writable && !busy ? (
              <Button
                variant="outline"
                size="feedback"
                onClick={() => void save(pending)}
              >
                重试保存
              </Button>
            ) : undefined
          }
        />
      ) : null}
      {!data ? (
        !readError ? (
          <ContentState state="loading" title="正在读取故事板…" />
        ) : null
      ) : !board || !shots?.length ? (
        <ContentState
          title="还没有镜头"
          icon={Film}
          actions={
            canReference ? (
              <Button variant="outline" onClick={onDiscuss}>
                用 AI 规划
              </Button>
            ) : undefined
          }
        />
      ) : (
        <ol aria-label="镜头顺序" className="space-y-2">
          {shots.map((shot, position) => {
            const isSelected = selected?.id === shot.id;
            return (
              <li key={shot.id} className="overflow-hidden rounded-lg border">
                <Button
                  type="button"
                  variant="ghost"
                  aria-expanded={isSelected}
                  className="grid h-auto min-h-20 w-full grid-cols-[auto_minmax(0,1fr)_auto] items-center gap-3 rounded-none p-3 text-left font-normal whitespace-normal hover:bg-muted/60 focus-visible:ring-inset aria-expanded:bg-muted/60"
                  onClick={() => setSelectedID(isSelected ? "" : shot.id)}
                >
                  <span
                    aria-label={`第 ${position + 1} 镜`}
                    className="text-xs text-muted-foreground tabular-nums"
                  >
                    {String(position + 1).padStart(2, "0")}
                  </span>
                  <span className="min-w-0">
                    <span className="block truncate font-medium">
                      {shot.title}
                    </span>
                    <span className="line-clamp-2 text-muted-foreground">
                      {shot.action}
                    </span>
                  </span>
                  <span className="space-y-1 text-right text-xs text-muted-foreground tabular-nums">
                    <span className="block">{shot.duration_ms / 1000} 秒</span>
                    <span className="block">
                      {shot.adopted?.shot_revision === shot.revision
                        ? "已采用"
                        : shot.adopted
                          ? "旧版已采用"
                          : "待制作"}
                    </span>
                  </span>
                </Button>
                {isSelected && (
                  <div className="space-y-4 border-t p-3">
                    {shot.adopted ? (
                      <div className="relative aspect-video max-h-56 overflow-hidden rounded-md bg-muted">
                        <ProjectMediaPreview
                          projectID={projectID}
                          id={shot.adopted.asset_id}
                          kind="video"
                          fit="contain"
                          interactive
                        />
                      </div>
                    ) : null}
                    <div className="flex flex-wrap gap-2">
                      <Button
                        variant="outline"
                        size="sm"
                        disabled={!canReference}
                        onClick={() =>
                          onReference(
                            shotReference(board.id, shot, position + 1),
                          )
                        }
                      >
                        <MessageSquarePlus data-icon="inline-start" />
                        加入对话
                      </Button>
                      <Button
                        variant="outline"
                        size="sm"
                        onClick={() => setMemoryShot(shot)}
                      >
                        镜头约定
                      </Button>
                      {writable ? (
                        <>
                          <Button
                            variant="outline"
                            size="sm"
                            disabled={!!pending || index <= 0}
                            onClick={() => {
                              const move = storyboardMove(shots, shot.id, -1);
                              if (move) apply({ move });
                            }}
                          >
                            <ArrowUp data-icon="inline-start" /> 上移
                          </Button>
                          <Button
                            variant="outline"
                            size="sm"
                            disabled={!!pending || index >= shots.length - 1}
                            onClick={() => {
                              const move = storyboardMove(shots, shot.id, 1);
                              if (move) apply({ move });
                            }}
                          >
                            <ArrowDown data-icon="inline-start" /> 下移
                          </Button>
                        </>
                      ) : null}
                    </div>
                    {writable
                      ? editableFields.map(({ field, label }) => (
                          <ShotField
                            key={field}
                            field={field}
                            label={label}
                            shot={shot}
                            draft={drafts[shot.id]?.[field]}
                            disabled={!!pending}
                            onChange={(draft) =>
                              setDrafts((previous) => ({
                                ...previous,
                                [shot.id]: {
                                  ...previous[shot.id],
                                  [field]: draft,
                                },
                              }))
                            }
                            onSave={(edit) => apply({ edit })}
                          />
                        ))
                      : null}
                    {!writable &&
                    (shot.dialogue || shot.camera || shot.sound) ? (
                      <dl className="space-y-2">
                        {shot.dialogue ? (
                          <div>
                            <dt className="text-muted-foreground">对白</dt>
                            <dd className="whitespace-pre-wrap">
                              {shot.dialogue}
                            </dd>
                          </div>
                        ) : null}
                        {shot.camera ? (
                          <div>
                            <dt className="text-muted-foreground">
                              景别与运镜
                            </dt>
                            <dd className="whitespace-pre-wrap">
                              {shot.camera}
                            </dd>
                          </div>
                        ) : null}
                        {shot.sound ? (
                          <div>
                            <dt className="text-muted-foreground">声音</dt>
                            <dd className="whitespace-pre-wrap">
                              {shot.sound}
                            </dd>
                          </div>
                        ) : null}
                      </dl>
                    ) : null}
                    <CanvasStoryboardDelivery
                      active={active}
                      projectID={projectID}
                      canvasID={canvasID}
                      boardID={board.id}
                      revision={data.revision}
                      shot={shot}
                      writable={writable && !pending}
                      onAdopted={(value) => {
                        const next = readStoryboard(value, canvasID, projectID);
                        setData((previous) =>
                          previous && previous.revision > next.revision
                            ? previous
                            : next,
                        );
                      }}
                      onStale={() => setRefresh((current) => current + 1)}
                    />
                  </div>
                )}
              </li>
            );
          })}
        </ol>
      )}
    </div>
  );
}

function ShotField({
  field,
  label,
  shot,
  draft,
  disabled,
  onChange,
  onSave,
}: {
  field: StoryboardField;
  label: string;
  shot: CanvasStoryboardShot;
  draft?: ShotDraft;
  disabled: boolean;
  onChange: (draft?: ShotDraft) => void;
  onSave: (edit: NonNullable<ReturnType<typeof storyboardEdit>>) => void;
}) {
  const id = useId();
  const baseline = shot[field] ?? "";
  const committed = displayValue(field, baseline);
  const conflict = !!draft && draft.base !== baseline;
  const edit = draft ? storyboardEdit(shot, field, draft) : null;
  const input = {
    id,
    value: draft?.value ?? committed,
    disabled,
    onChange: (
      event: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>,
    ) => {
      const value = event.target.value;
      onChange(
        value === committed
          ? undefined
          : { base: draft?.base ?? baseline, value },
      );
    },
  };
  return (
    <form
      className="space-y-1.5"
      onSubmit={(event) => {
        event.preventDefault();
        if (edit && !disabled) onSave(edit);
      }}
    >
      <Label htmlFor={id}>{label}</Label>
      <div className="flex items-start gap-2">
        {field === "action" ? (
          <Textarea {...input} rows={3} />
        ) : field === "duration_ms" ? (
          <Input {...input} type="number" min="0.001" step="0.001" />
        ) : (
          <Input {...input} />
        )}
        {draft && !conflict ? (
          <Button
            type="submit"
            variant="secondary"
            disabled={disabled || !edit}
          >
            保存
          </Button>
        ) : null}
      </div>
      {conflict ? (
        <div className="space-y-1.5 rounded-md border bg-muted/50 p-2">
          <p className="wrap-anywhere whitespace-pre-wrap">
            当前版本：{committed}
          </p>
          <div className="flex flex-wrap gap-1">
            <Button
              type="button"
              variant="outline"
              size="sm"
              disabled={disabled}
              onClick={() => onChange(undefined)}
            >
              使用当前版本
            </Button>
            <Button
              type="button"
              variant="ghost"
              size="sm"
              disabled={disabled}
              onClick={() => onChange({ base: baseline, value: draft.value })}
            >
              保留我的草稿
            </Button>
          </div>
        </div>
      ) : null}
    </form>
  );
}
