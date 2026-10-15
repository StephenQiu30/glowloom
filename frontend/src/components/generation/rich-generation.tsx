"use client";
import { useCallback, useEffect, useRef, useState } from "react";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import { getSession } from "@/api/identity";
import { listProjectModels } from "@/api/models";
import { getMediaPreview } from "@/api/media";
import {
  createPromptDraft,
  readPromptDraft,
  savePromptDraft,
  listPromptDrafts,
  currentPromptSubmission,
  resolvePromptSubmission,
  validatePromptFragment,
} from "@/api/promptEditor";
import { createFreeQuote, confirmOperationQuote } from "@/api/operations";
import { RequestError } from "@/lib/request";
import { reconcileLabels } from "./draft-resolution";
import { canonical } from "./draft-save";
import { PromptEditor } from "./prompt-editor";
import { ComposerSession, type ComposerSnapshot } from "./composer-session";
import {
  emptyPromptDraft,
  validatePromptDraft,
  type PromptDraft,
} from "./prompt-document";
import {
  DraftSaver,
  draftRequest,
  branchJournal,
  requireSaved,
  type SavedDraft,
  type LocalBranch,
} from "./draft-save";
import {
  ReferenceControls,
  mediaBinding,
  subjectBinding,
} from "./reference-controls";
import { editorIssues, compatibleParams } from "./editor-capabilities";
import { fieldsForMode, requireQuote, money, type Quote } from "./contracts";
import { TaskDetail, type GenerationSeed } from "./generation-dialog";
export type CanvasComposerOwner = {
  contextKey: string;
  draft: PromptDraft;
  onChange: (draft: PromptDraft) => void;
  flush: () => Promise<API.InputSource>;
};
type Props = {
  projectID: string;
  seed: GenerationSeed;
  onLock?: (locked: boolean) => void;
  onCloseGuard?: (guard: (() => Promise<boolean>) | null) => void;
  onAdopt?: (id: string, kind: string) => Promise<void> | void;
  canvas?: CanvasComposerOwner;
  draftID?: string;
};
function plainSeed(seed: GenerationSeed) {
  const draft = emptyPromptDraft();
  const text =
    seed.reuse?.inputs?.find((i) => i.role === "prompt")?.text ?? seed.prompt;
  draft.document = {
    type: "doc",
    content: text.split("\n").map((text) => ({
      type: "paragraph",
      ...(text ? { content: [{ type: "text" as const, text }] } : {}),
    })),
  };
  draft.params = {
    ...(seed.draft?.aspect_ratio
      ? { aspect_ratio: seed.draft.aspect_ratio }
      : {}),
    ...(seed.draft?.negative_prompt
      ? { negative_prompt: seed.draft.negative_prompt }
      : {}),
    ...(seed.reuse?.params ?? {}),
    output_count: seed.reuse?.output_count ?? seed.draft?.candidate_count ?? 1,
  };
  return draft;
}
function errorText(error: unknown) {
  if (error instanceof RequestError)
    return (
      (
        {
          revision_conflict: "另一窗口已保存更新。请读取服务端或保留本地副本。",
          reference_unavailable:
            "某份参考已失效、无权访问或版本变化，请替换后重试。",
          quote_source_stale:
            "草稿已变化，旧报价不可确认。请放弃旧报价并重新检查。",
          submission_unresolved: "存在未处理的报价或确认，请先恢复该提交。",
          submission_epoch_conflict: "其他窗口已处理这次提交，请刷新提交状态。",
          label_allocation_conflict:
            "编号分配冲突，请保留为新副本或采用服务端版本。",
        } as Record<string, string>
      )[error.code] ?? error.message
    );
  return error instanceof Error ? error.message : "请求失败，请重试。";
}
export function RichGeneration(props: Props) {
  const [bootKey] = useState(() => crypto.randomUUID()),
    [retry, setRetry] = useState(0),
    [opened, setOpened] = useState(props.draftID),
    [ready, setReady] = useState<{
      snapshot: SavedDraft;
      actor: string;
      models: API.ModelResponse[];
      current: API.SubmissionCurrent;
    } | null>(null),
    [error, setError] = useState("");
  const bootDraft = useRef<PromptDraft | null>(null);
  useEffect(() => {
    const controller = new AbortController();
    let active = true;
    async function boot() {
      try {
        const [identity, modelsPage] = await Promise.all([
          getSession({ signal: controller.signal }),
          listProjectModels(
            {
              project_id: props.projectID,
              capability: props.seed.capability + ".generate",
              limit: 100,
            },
            { signal: controller.signal },
          ),
        ]);
        if (!identity.actor?.id) throw new RequestError("session_invalid");
        const models = [...(modelsPage.items ?? [])];
        let cursor = modelsPage.next_cursor;
        const cursors = new Set<string>();
        while (cursor) {
          if (cursors.has(cursor)) throw new RequestError("invalid_receipt");
          cursors.add(cursor);
          const page = await listProjectModels(
            {
              project_id: props.projectID,
              capability: props.seed.capability + ".generate",
              limit: 100,
              cursor,
            },
            { signal: controller.signal },
          );
          models.push(...(page.items ?? []));
          cursor = page.next_cursor;
        }
        let snapshot: SavedDraft;
        if (props.canvas) {
          snapshot = {
            draft_id: props.canvas.contextKey,
            revision: 1,
            draft_hash: "canvas",
            input_hash: "canvas",
            draft: props.canvas.draft,
          };
        } else if (opened) {
          snapshot = requireSaved(
            await readPromptDraft(
              { pid: props.projectID, id: opened },
              { signal: controller.signal },
            ),
          );
        } else {
          let draft =
            bootDraft.current ?? props.seed.composer ?? plainSeed(props.seed);
          if (!bootDraft.current) {
            if (props.seed.reuse?.editor_draft) {
              validatePromptDraft(props.seed.reuse.editor_draft);
              draft = structuredClone(props.seed.reuse.editor_draft);
            }
            draft = structuredClone(draft);
            const add = (ref: ReturnType<typeof mediaBinding>) => {
              ref.order = draft.references.length;
              draft.references.push(ref);
              const kind =
                ref.source.kind === "subject" ? "subject" : ref.mediaKind;
              draft.labelState.allocations[ref.id] = {
                labelKind: kind,
                ordinal: ++draft.labelState.highWater[kind],
              };
              draft.document.content[0].content ??= [];
              draft.document.content[0].content.push({
                type: "reference",
                attrs: { referenceId: ref.id },
              });
            };
            if (props.seed.subject)
              add(subjectBinding(props.seed.subject, "subject"));
            const oldMedia = props.seed.reference
              ? [
                  {
                    media_asset_id: props.seed.reference.assetID,
                    role: "reference_" + props.seed.reference.kind,
                  },
                ]
              : props.seed.reuse?.editor_draft
                ? []
                : (props.seed.reuse?.inputs?.filter((i) => i.media_asset_id) ??
                  []);
            for (const ref of oldMedia) {
              const preview = await getMediaPreview(
                { pid: props.projectID, asset_id: ref.media_asset_id! },
                { signal: controller.signal },
              );
              if (preview.asset?.project_id !== props.projectID)
                throw new RequestError("invalid_receipt");
              add(mediaBinding(preview.asset, ref.role ?? "reference_image"));
            }
            bootDraft.current = structuredClone(draft);
          }
          snapshot = requireSaved(
            await createPromptDraft(
              { pid: props.projectID },
              { expected_revision: 0, draft: draftRequest(draft) },
              {
                headers: { "Idempotency-Key": bootKey },
                signal: controller.signal,
              },
            ),
          );
        }
        const source = props.canvas
          ? {
              owner_kind: "canvas_input",
              canvas_id: props.seed.source!.canvas_id,
              node_id: props.seed.source!.node_id,
            }
          : { owner_kind: "prompt_draft", draft_id: snapshot.draft_id };
        const current = await currentPromptSubmission(
          { pid: props.projectID, ...source },
          { signal: controller.signal },
        );
        if (!Number.isSafeInteger(current.epoch))
          throw new RequestError("invalid_receipt");
        if (active) {
          setReady({ snapshot, actor: identity.actor.id, models, current });
          setError("");
        }
      } catch (error) {
        if (active) setError(errorText(error));
      }
    }
    void boot();
    return () => {
      active = false;
      controller.abort();
    };
    // Initial seed and owner are immutable for this mounted workspace.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [props.projectID, opened, retry, bootKey]);
  if (!ready)
    return (
      <div role={error ? "alert" : "status"}>
        {error || "正在核验身份、草稿和未决提交…"}
        {error && <Button onClick={() => setRetry((n) => n + 1)}>重试</Button>}
      </div>
    );
  return (
    <EditorWorkspace
      key={ready.snapshot.draft_id}
      {...props}
      {...ready}
      onOpen={(id) => {
        setReady(null);
        setOpened(id);
        setRetry((n) => n + 1);
      }}
    />
  );
}
function EditorWorkspace({
  projectID,
  seed,
  onLock,
  onCloseGuard,
  onAdopt,
  canvas,
  snapshot,
  actor,
  models: initialModels,
  current: initialCurrent,
  onOpen,
}: Props & {
  snapshot: SavedDraft;
  actor: string;
  models: API.ModelResponse[];
  current: API.SubmissionCurrent;
  onOpen: (id: string) => void;
}) {
  const [models, setModels] = useState(initialModels);
  const [replicaId] = useState(() => crypto.randomUUID()),
    [session, setSession] = useState<ComposerSession | null>(null),
    [draft, setDraft] = useState(snapshot.draft),
    [seq, setSeq] = useState(0),
    [current, setCurrent] = useState(initialCurrent),
    [quote, setQuote] = useState<Quote | null>(null),
    [quoteSeq, setQuoteSeq] = useState(-1),
    [error, setError] = useState(""),
    [pending, setPending] = useState(false),
    [confirmUnknown, setConfirmUnknown] = useState(false),
    [task, setTask] = useState<string | null>(
      initialCurrent.submission?.state === "accepted"
        ? (initialCurrent.submission.operation_id ?? null)
        : null,
    ),
    [full, setFull] = useState(false),
    [revision, setRevision] = useState(snapshot.revision),
    [saveState, setSaveState] = useState("已保存"),
    [suspended, setSuspended] = useState(false),
    [sourceCheck, setSourceCheck] = useState<{
      key: string;
      error?: string;
    } | null>(null),
    [branches, setBranches] = useState<LocalBranch[]>([]),
    [drafts, setDrafts] = useState<API.DraftSnapshot[]>([]),
    [draftCursor, setDraftCursor] = useState<string | undefined>(),
    [conversion, setConversion] = useState<{
      seq: number;
      params: PromptDraft["params"];
    } | null>(null),
    [serverConflict, setServerConflict] = useState<SavedDraft | null>(null);
  const contextKey = `${actor}/${projectID}/${canvas?.contextKey ?? snapshot.draft_id}`;
  const sessionRef = useRef<ComposerSession | null>(null);
  const firstDirtyAt = useRef<number | null>(null);
  const saver = useRef<DraftSaver | null>(null),
    busy = useRef(false),
    alive = useRef(true),
    timer = useRef<ReturnType<typeof setTimeout> | null>(null),
    submission = useRef<LocalBranch["pendingSubmission"]>(undefined),
    latest = useRef<ComposerSnapshot>({
      contextKey,
      replicaId,
      localSeq: 0,
      draft: snapshot.draft,
    });
  const sourceKey = JSON.stringify(draft.references);
  const sourceReady =
    draft.references.length === 0 ||
    (sourceCheck?.key === sourceKey && !sourceCheck.error);
  useEffect(() => {
    if (!latest.current.draft.references.length) return;
    const controller = new AbortController();
    const value = structuredClone(latest.current.draft);
    value.document = { type: "doc", content: [{ type: "paragraph" }] };
    void validatePromptFragment({ pid: projectID }, draftRequest(value), {
      signal: controller.signal,
    })
      .then(() => {
        if (!controller.signal.aborted) setSourceCheck({ key: sourceKey });
      })
      .catch((e) => {
        if (!controller.signal.aborted)
          setSourceCheck({ key: sourceKey, error: errorText(e) });
      });
    return () => controller.abort();
  }, [sourceKey, projectID]);
  const frozen = confirmUnknown;
  const recoveredChanged =
    !quote &&
    !!current.submission?.draft &&
    canonical(draft) !== canonical(current.submission.draft);
  const activeQuote = quote ?? current.submission?.quote;
  const [expiredQuote, setExpiredQuote] = useState<API.EditorQuote | null>(
    null,
  );
  const quoteExpired = !!activeQuote && expiredQuote === activeQuote;
  useEffect(() => {
    if (!activeQuote) return;
    const expires = Date.parse(activeQuote.expires_at ?? "");
    let timeout: ReturnType<typeof setTimeout>;
    function schedule() {
      const remaining = expires - Date.now();
      timeout = setTimeout(
        () => {
          if (!Number.isFinite(remaining) || Date.now() >= expires) {
            setExpiredQuote(activeQuote!);
          } else {
            schedule();
          }
        },
        Number.isFinite(remaining)
          ? Math.max(0, Math.min(remaining, 2_147_483_647))
          : 0,
      );
    }
    schedule();
    return () => clearTimeout(timeout);
  }, [activeQuote]);
  const model = models.find(
    (m) => m.current_version?.id === draft.modelVersion,
  );
  const issues = editorIssues(draft, model);
  const pendingMode = session?.pendingTarget;
  const updateStatus = useCallback(() => {
    const s = saver.current;
    if (!s || !alive.current) return;
    setRevision(s.branch.serverRevision);
    setSaveState(
      s.localError
        ? "本地恢复不可用"
        : (
            {
              dirty: "未保存",
              saving: "保存中…",
              saved: "已保存",
              unknown: "保存结果待确认",
              conflict: "保存冲突",
            } as const
          )[s.status],
    );
  }, []);
  useEffect(() => {
    alive.current = true;
    if (!canvas) {
      saver.current = new DraftSaver({
        snapshot: latest.current,
        saved: snapshot,
        revision: snapshot.revision,
        draftId: snapshot.draft_id,
        send: async (request) =>
          requireSaved(
            await savePromptDraft(
              { pid: projectID, id: request.draftId! },
              {
                expected_revision: request.expectedRevision,
                draft: draftRequest(request.snapshot.draft),
              },
              { headers: { "Idempotency-Key": request.key } },
            ),
          ),
        persist: branchJournal.write,
        changed: updateStatus,
      });
    }
    branchJournal
      .list(contextKey)
      .then((found) => {
        if (alive.current)
          setBranches(
            found.filter(
              (b) =>
                b.replicaId !== replicaId &&
                (b.pendingSave ||
                  b.pendingSubmission ||
                  b.ackedSeq !== b.snapshot.localSeq),
            ),
          );
      })
      .catch(() => {
        if (alive.current) setSaveState("本地恢复不可用");
      });
    return () => {
      alive.current = false;
      if (timer.current) clearTimeout(timer.current);
      void saver.current?.flush().catch(() => {});
      saver.current?.dispose();
    };
  }, [contextKey, replicaId, snapshot, projectID, updateStatus, canvas]);
  useEffect(() => {
    onLock?.(frozen || pending);
    session?.editor.setEditable(!frozen);
    return () => {
      onLock?.(false);
    };
  }, [frozen, pending, onLock, session]);
  useEffect(() => {
    const before = (e: BeforeUnloadEvent) => {
      if (saver.current?.status !== "saved" || confirmUnknown) {
        e.preventDefault();
      }
    };
    window.addEventListener("beforeunload", before);
    return () => window.removeEventListener("beforeunload", before);
  }, [confirmUnknown]);
  useEffect(() => {
    const suspend = () => {
      saver.current?.dispose();
      session?.cancelModeSelection();
      session?.editor.setEditable(false);
      setSuspended(true);
    };
    const check = () => {
      if (document.visibilityState === "visible")
        void getSession()
          .then((identity) => {
            if (identity.actor?.id !== actor) suspend();
          })
          .catch(suspend);
    };
    const channel =
      typeof BroadcastChannel !== "undefined"
        ? new BroadcastChannel("glowloom-private-cache")
        : null;
    if (channel) channel.onmessage = suspend;
    window.addEventListener("glowloom:private-cache-cleared", suspend);
    document.addEventListener("visibilitychange", check);
    return () => {
      channel?.close();
      window.removeEventListener("glowloom:private-cache-cleared", suspend);
      document.removeEventListener("visibilitychange", check);
    };
  }, [actor, session]);
  const changed = useCallback(
    (value: ComposerSnapshot) => {
      const previous = latest.current;
      latest.current = value;
      setDraft(value.draft);
      setSeq(value.localSeq);
      saver.current?.edit(value);
      if (!sessionRef.current?.composing) canvas?.onChange(value.draft);
      if (previous.localSeq === value.localSeq && !firstDirtyAt.current) return;
      firstDirtyAt.current ??= Date.now();
      if (timer.current) clearTimeout(timer.current);
      if (!canvas) {
        const autosave = () => {
          if (sessionRef.current?.composing) {
            timer.current = setTimeout(autosave, 100);
            return;
          }
          void saver.current
            ?.flush()
            .then(() => {
              firstDirtyAt.current = null;
            })
            .catch((e) => {
              if (alive.current) setError(errorText(e));
            });
        };
        timer.current = setTimeout(
          autosave,
          Math.max(
            0,
            Math.min(800, 5000 - (Date.now() - firstDirtyAt.current)),
          ),
        );
      }
    },
    [canvas],
  );
  async function flush() {
    if (session?.composing) throw new Error("请先完成当前输入法组合。");
    if (canvas) return canvas.flush();
    if (!saver.current) throw new Error("保存协调器尚未就绪");
    const saved = await saver.current.flush();
    return {
      kind: "prompt_draft",
      draft_id: saved.draft_id,
      revision: saved.revision,
    } satisfies API.InputSource;
  }
  useEffect(() => {
    onCloseGuard?.(async () => {
      if (confirmUnknown || busy.current) return false;
      try {
        await flush();
        return true;
      } catch (e) {
        setError(errorText(e));
        return false;
      }
    });
    return () => onCloseGuard?.(null);
  });
  async function discover() {
    const source = canvas
      ? {
          owner_kind: "canvas_input",
          canvas_id: seed.source!.canvas_id,
          node_id: seed.source!.node_id,
        }
      : { owner_kind: "prompt_draft", draft_id: snapshot.draft_id };
    const value = await currentPromptSubmission({ pid: projectID, ...source });
    if (!Number.isSafeInteger(value.epoch))
      throw new RequestError("invalid_receipt");
    setCurrent(value);
    if (value.submission?.state === "accepted") {
      setConfirmUnknown(false);
      setTask(value.submission.operation_id ?? null);
    }
    return value;
  }
  async function run(action: () => Promise<void>) {
    if (busy.current) return;
    busy.current = true;
    setPending(true);
    setError("");
    try {
      await action();
    } catch (e) {
      setError(errorText(e));
    } finally {
      busy.current = false;
      if (alive.current) setPending(false);
    }
  }
  async function getQuote() {
    await run(async () => {
      if (issues.length || pendingMode || !sourceReady)
        throw new Error("请先修复当前输入。");
      const captured = latest.current.localSeq;
      const pointer = await flush();
      if (latest.current.localSeq !== captured)
        throw new Error("保存期间输入已更新，请重新检查费用。");
      const recovered = await discover();
      if (recovered.submission) {
        setQuote(null);
        throw new Error("请先处理已有提交，再创建下一报价。");
      }
      const intent = submission.current ?? {
        key: crypto.randomUUID(),
        body: {
          input_source: pointer,
          expected_submission_epoch: recovered.epoch,
          force_regenerate: !!seed.reuse,
        },
      };
      submission.current = intent;
      await saver.current?.setSubmission(intent);
      const result = requireQuote(
        await createFreeQuote({ pid: projectID }, intent.body, {
          headers: { "Idempotency-Key": intent.key },
        }),
      );
      if (!result.confirm_key || !result.submission_id)
        throw new RequestError("invalid_receipt");
      intent.confirmKey = result.confirm_key;
      await saver.current?.setSubmission(intent);
      await discover();
      if (latest.current.localSeq === captured) {
        setQuote(result);
        setQuoteSeq(captured);
      }
    });
  }
  async function confirm() {
    await run(async () => {
      // An unknown confirmation must retain its original key even after expiry.
      if (!confirmUnknown) {
        const expires = Date.parse(activeQuote?.expires_at ?? "");
        if (!Number.isFinite(expires) || Date.now() >= expires)
          throw new Error("报价已过期，请重新检查费用。");
        if (activeQuote?.confirmable !== true)
          throw new Error("当前报价不可确认，请重新检查费用。");
      }
      if (session?.pendingTarget) throw new Error("模式选择尚未完成。");
      const sub = current.submission;
      const confirmKey = sub?.confirm_key ?? quote?.confirm_key;
      const operation = sub?.operation_id ?? quote?.operation_id;
      if (!operation || !confirmKey) throw new Error("请先恢复报价。");
      if (quote && quoteSeq !== latest.current.localSeq)
        throw new Error("输入已变化，请重新报价。");
      if (!quote && !confirmUnknown) {
        const saved = await flush();
        if (!sameSource(saved, sub?.input_source))
          throw new Error("草稿已变化，请放弃旧报价后重新检查。");
      }
      const intent = submission.current ?? {
        key: crypto.randomUUID(),
        body: { input_source: sub?.input_source },
        confirmKey,
      };
      intent.confirmRequested = true;
      submission.current = intent;
      await saver.current?.setSubmission(intent);
      setConfirmUnknown(true);
      try {
        const result = await confirmOperationQuote(
          { id: operation },
          { project_id: projectID },
          { headers: { "Idempotency-Key": confirmKey } },
        );
        if (!result.reservation_id) throw new RequestError("invalid_receipt");
        setConfirmUnknown(false);
        setTask(operation);
        setQuote(null);
        await discover();
      } catch (e) {
        if (e instanceof RequestError && e.status >= 400 && e.status < 500)
          setConfirmUnknown(false);
        throw e;
      }
    });
  }
  async function resolve(action: "discard_quote" | "acknowledge_result") {
    await run(async () => {
      const sub = current.submission;
      if (!sub?.id) throw new Error("没有可处理的提交");
      const key = `${sub.id}:${action}`;
      const storageKey = resolveKeys.current.get(key) ?? crypto.randomUUID();
      resolveKeys.current.set(key, storageKey);
      const value = await resolvePromptSubmission(
        { pid: projectID, id: sub.id },
        { action, expected_epoch: current.epoch },
        { headers: { "Idempotency-Key": storageKey } },
      );
      setCurrent(value);
      setQuote(null);
      setConfirmUnknown(false);
      submission.current = undefined;
      await saver.current?.setSubmission(undefined);
    });
  }
  const resolveKeys = useRef(new Map<string, string>());
  async function selectMode(version: string, mode: string) {
    if (!session || !version || !mode) return;
    const request = session.beginModeSelection({ modelVersion: version, mode });
    try {
      const page = await listProjectModels({
        project_id: projectID,
        capability: seed.capability + ".generate",
        limit: 100,
      });
      let found = page.items?.find((m) => m.current_version?.id === version);
      let cursor = page.next_cursor;
      const seen = new Set<string>();
      while (!found && cursor && !seen.has(cursor)) {
        seen.add(cursor);
        const next = await listProjectModels({
          project_id: projectID,
          capability: seed.capability + ".generate",
          limit: 100,
          cursor,
        });
        found = next.items?.find((m) => m.current_version?.id === version);
        cursor = next.next_cursor;
      }
      if (session.pendingTarget?.selectionEpoch !== request.selectionEpoch)
        return;
      if (!found?.current_version?.modes?.includes(mode))
        throw new Error("该模型模式已不可用");
      if (
        found.status !== "active" ||
        found.provider_status !== "active" ||
        !found.credential_present
      )
        throw new Error("该模型当前不可用");
      setModels((values) =>
        values.map((value) => (value.id === found!.id ? found! : value)),
      );
      session.resolveModeSelection(request);
    } catch (error) {
      if (session.pendingTarget?.selectionEpoch !== request.selectionEpoch)
        return;
      session.cancelModeSelection();
      setError(errorText(error));
    }
  }
  async function recover(branch: LocalBranch) {
    await run(async () => {
      let expectedRevision = branch.serverRevision;
      if (branch.pendingSave) {
        const r = branch.pendingSave;
        const saved = r.draftId
          ? await savePromptDraft(
              { pid: projectID, id: r.draftId },
              {
                expected_revision: r.expectedRevision,
                draft: draftRequest(r.snapshot.draft),
              },
              { headers: { "Idempotency-Key": r.key } },
            )
          : await createPromptDraft(
              { pid: projectID },
              { expected_revision: 0, draft: draftRequest(r.snapshot.draft) },
              { headers: { "Idempotency-Key": r.key } },
            );
        expectedRevision = requireSaved(saved).revision;
      }
      if (branch.pendingSubmission) {
        submission.current = structuredClone(branch.pendingSubmission);
        await saver.current?.setSubmission(submission.current);
        await discover();
        if (branch.pendingSubmission.confirmRequested) setConfirmUnknown(true);
      }
      const saved = await readPromptDraft({
        pid: projectID,
        id: snapshot.draft_id,
      });
      const server = requireSaved(saved);
      if (server.revision !== expectedRevision) {
        setServerConflict(server);
        throw new Error("该本地分支的服务端基线已变化，可将它保留为新副本。");
      }
      saver.current?.rebase(server);
      session?.adoptDraft(branch.snapshot.draft);
    });
  }
  async function copyDraft(value: PromptDraft) {
    value = structuredClone(value);
    for (const ref of value.references) {
      if (ref.source.kind === "pending_upload") {
        ref.bindingGeneration++;
        ref.source = {
          kind: "pending_upload",
          clientUploadId: crypto.randomUUID(),
          uploadAttemptId: crypto.randomUUID(),
        };
      }
    }
    const saved = requireSaved(
      await createPromptDraft(
        { pid: projectID },
        { expected_revision: 0, draft: draftRequest(value) },
        { headers: { "Idempotency-Key": crypto.randomUUID() } },
      ),
    );
    onOpen(saved.draft_id);
  }
  async function resolveDraftConflict(useLocal: boolean) {
    if (!serverConflict) return;
    const result = reconcileLabels(
      useLocal ? draft : serverConflict.draft,
      serverConflict.draft,
      draft,
    );
    const body = {
      expected_revision: serverConflict.revision,
      draft: draftRequest(result.draft),
    };
    const identity = "draft-resolution:" + canonical(body);
    const key = resolveKeys.current.get(identity) ?? crypto.randomUUID();
    resolveKeys.current.set(identity, key);
    await savePromptDraft(
      { pid: projectID, id: serverConflict.draft_id },
      body,
      { headers: { "Idempotency-Key": key } },
    );
    onOpen(serverConflict.draft_id);
  }
  if (suspended)
    return (
      <div role="alert">
        <p>
          身份已变化或无法核验。当前页面已停止私有缓存写入；本地分支在重新核验后恢复。
        </p>
        <Button onClick={() => onOpen(snapshot.draft_id)}>
          重新核验并找回草稿
        </Button>
      </div>
    );
  return (
    <section
      className={
        full
          ? "fixed inset-0 z-50 overflow-y-auto bg-background p-4 sm:p-8"
          : "space-y-4"
      }
      aria-label="多参考创作编辑器"
    >
      <div className="mx-auto max-w-5xl space-y-4">
        <div className="flex flex-wrap items-center gap-2">
          <span role="status" className="text-sm text-muted-foreground">
            {canvas ? "随画布保存" : saveState} · r{revision}
          </span>
          <Button variant="ghost" onClick={() => setFull((v) => !v)}>
            {full ? "退出全屏" : "展开编辑器"}
          </Button>
          <Button
            variant="outline"
            disabled={pending}
            onClick={() =>
              void run(async () => {
                await flush();
              })
            }
          >
            保存草稿
          </Button>
          {!canvas && (
            <Button
              variant="ghost"
              onClick={() =>
                void run(async () => {
                  const page = await listPromptDrafts({ pid: projectID });
                  setDrafts(page.items ?? []);
                  setDraftCursor(page.next_cursor);
                })
              }
            >
              找回草稿
            </Button>
          )}
        </div>
        {drafts.length > 0 && (
          <div className="rounded border p-3" aria-label="服务端草稿">
            {draftCursor && (
              <Button
                onClick={() =>
                  void run(async () => {
                    const page = await listPromptDrafts({
                      pid: projectID,
                      cursor: draftCursor,
                    });
                    setDrafts((v) => [...v, ...(page.items ?? [])]);
                    setDraftCursor(page.next_cursor);
                  })
                }
              >
                更多草稿
              </Button>
            )}
            {drafts.map((d) => (
              <Button
                key={d.draft_id}
                variant="ghost"
                disabled={frozen}
                onClick={() =>
                  void run(async () => {
                    await flush();
                    onOpen(d.draft_id!);
                  })
                }
              >
                草稿 {d.draft_id?.slice(0, 8)} · r{d.revision}
              </Button>
            ))}
          </div>
        )}
        {branches.length > 0 && (
          <div className="space-y-2 rounded border p-3">
            <p>发现 {branches.length} 份本地分支；请选择，不会自动覆盖。</p>
            {branches.map((b) => (
              <div key={b.replicaId} className="flex flex-wrap gap-2">
                <span>
                  分支 {b.replicaId.slice(0, 8)} · 基线 r{b.serverRevision}
                </span>
                <Button disabled={frozen} onClick={() => void recover(b)}>
                  恢复此分支
                </Button>
                <Button
                  disabled={frozen || !!canvas}
                  onClick={() => void run(() => copyDraft(b.snapshot.draft))}
                >
                  保留为副本
                </Button>
                <Button
                  disabled={frozen}
                  variant="ghost"
                  onClick={() =>
                    void run(async () => {
                      if (!(await branchJournal.remove(b)))
                        throw new Error(
                          "此分支刚有新修改，未删除。请重新读取。",
                        );
                      setBranches((v) =>
                        v.filter((x) => x.replicaId !== b.replicaId),
                      );
                    })
                  }
                >
                  丢弃此分支
                </Button>
              </div>
            ))}
          </div>
        )}
        <div className="grid gap-3 sm:grid-cols-2">
          <label className="grid gap-1 text-sm">
            生成模型
            <select
              aria-label="生成模型"
              className="rounded border bg-background p-2"
              disabled={frozen || !session}
              value={draft.modelVersion ?? ""}
              onChange={(e) => {
                const m = models.find(
                  (m) => m.current_version?.id === e.target.value,
                );
                if (m?.current_version?.id)
                  selectMode(
                    m.current_version.id,
                    m.current_version.modes?.[0] ?? "",
                  );
              }}
            >
              <option value="">请选择模型</option>
              {models.map((m) => (
                <option
                  key={m.id}
                  value={m.current_version?.id}
                  disabled={
                    m.status !== "active" ||
                    m.provider_status !== "active" ||
                    !m.credential_present
                  }
                >
                  {m.display_name}
                  {m.status !== "active" || !m.credential_present
                    ? " · 当前不可用"
                    : ""}
                </option>
              ))}
            </select>
          </label>
          <label className="grid gap-1 text-sm">
            生成模式
            <select
              aria-label="生成模式"
              className="rounded border bg-background p-2"
              disabled={frozen || !model || !session}
              value={draft.mode ?? ""}
              onChange={(e) => selectMode(draft.modelVersion!, e.target.value)}
            >
              <option value="">请选择模式</option>
              {model?.current_version?.modes?.map((m) => (
                <option key={m}>{m}</option>
              ))}
            </select>
          </label>
        </div>
        <div className="min-h-32 rounded-lg border p-3 text-base [&_.tiptap]:min-h-24 [&_.tiptap]:outline-none">
          <PromptEditor
            contextKey={contextKey}
            replicaId={replicaId}
            draft={snapshot.draft}
            onReady={(value) => {
              sessionRef.current = value;
              setSession(value);
              canvas?.onChange(value.snapshot().draft);
            }}
            onChange={changed}
            onRequestSave={() =>
              void run(async () => {
                await flush();
              })
            }
            onRequestSubmit={() => void getQuote()}
          />
        </div>
        {session && (
          <>
            <div className="flex gap-2">
              <Button
                variant="ghost"
                disabled={frozen}
                onClick={() => session.undo()}
              >
                撤销
              </Button>
              <Button
                variant="ghost"
                disabled={frozen}
                onClick={() => session.redo()}
              >
                重做
              </Button>
            </div>
            <ReferenceControls
              session={session}
              draft={draft}
              projectID={projectID}
              model={model}
              disabled={frozen}
              authorized={sourceReady}
            />
          </>
        )}
        {model && (
          <div className="grid gap-3 sm:grid-cols-2">
            {(() => {
              try {
                return fieldsForMode(
                  model.current_version?.param_schema,
                  draft.mode ?? "",
                );
              } catch {
                return [];
              }
            })().map((f) => (
              <label key={f.field} className="grid gap-1 text-sm">
                {f.label}
                {f.enum ? (
                  <select
                    className="rounded border bg-background p-2"
                    disabled={frozen}
                    value={String(draft.params[f.field] ?? "")}
                    onChange={(e) =>
                      session?.updateParams({
                        ...draft.params,
                        [f.field]:
                          f.type === "number" || f.type === "integer"
                            ? Number(e.target.value)
                            : e.target.value,
                      })
                    }
                  >
                    <option value="">请选择</option>
                    {f.enum.map((v) => (
                      <option key={String(v)} value={v}>
                        {v}
                      </option>
                    ))}
                  </select>
                ) : f.type === "boolean" ? (
                  <input
                    type="checkbox"
                    disabled={frozen}
                    checked={!!draft.params[f.field]}
                    onChange={(e) =>
                      session?.updateParams({
                        ...draft.params,
                        [f.field]: e.target.checked,
                      })
                    }
                  />
                ) : (
                  <input
                    className="rounded border bg-background p-2"
                    disabled={frozen}
                    type={f.type === "string" ? "text" : "number"}
                    value={String(draft.params[f.field] ?? "")}
                    min={f.min}
                    max={f.max}
                    onChange={(e) =>
                      session?.updateParams({
                        ...draft.params,
                        [f.field]:
                          f.type === "string"
                            ? e.target.value
                            : e.target.value === ""
                              ? null
                              : Number(e.target.value),
                      })
                    }
                  />
                )}
              </label>
            ))}
            <label className="grid gap-1 text-sm">
              候选数量
              <input
                className="rounded border bg-background p-2"
                type="number"
                min={1}
                max={8}
                disabled={frozen}
                value={Number(draft.params.output_count ?? 1)}
                onChange={(e) =>
                  session?.updateParams({
                    ...draft.params,
                    output_count: Number(e.target.value),
                  })
                }
              />
            </label>
          </div>
        )}
        {issues.length > 0 && (
          <div className="rounded border p-3 text-sm">
            <p>输入保留在草稿中，以下问题会阻止生成：</p>
            <ul className="list-inside list-disc">
              {issues.map((issue, i) => (
                <li key={i}>{issue}</li>
              ))}
            </ul>
            {model && (
              <Button
                variant="outline"
                disabled={frozen}
                onClick={() =>
                  setConversion({ seq, params: compatibleParams(draft, model) })
                }
              >
                检查参数转换
              </Button>
            )}
          </div>
        )}
        {error && (
          <p role="alert" className="text-sm text-destructive">
            {error}
          </p>
        )}
        {saveState === "保存冲突" && !canvas && (
          <div className="flex gap-2">
            <Button
              onClick={() =>
                void run(async () =>
                  setServerConflict(
                    requireSaved(
                      await readPromptDraft({
                        pid: projectID,
                        id: snapshot.draft_id,
                      }),
                    ),
                  ),
                )
              }
            >
              读取服务端版本
            </Button>
            <Button onClick={() => void run(() => copyDraft(draft))}>
              保留当前为副本
            </Button>
          </div>
        )}
        {serverConflict && (
          <Dialog open onOpenChange={() => setServerConflict(null)}>
            <DialogContent>
              <DialogTitle>草稿发生冲突</DialogTitle>
              <DialogDescription>
                服务端 r{serverConflict.revision}
                ；本地正文、引用和参数仍保留。采用服务端会打开新的编辑实例；本地分支继续可找回。
              </DialogDescription>
              <pre className="max-h-60 overflow-auto text-xs">
                {JSON.stringify(
                  { local: draft, server: serverConflict.draft },
                  null,
                  2,
                )}
              </pre>
              <Button
                disabled={pending}
                onClick={() => void run(() => resolveDraftConflict(false))}
              >
                采用服务端
              </Button>
              <p className="text-sm">
                覆盖前编号调整：
                {reconcileLabels(draft, serverConflict.draft).changes.join(
                  "；",
                ) || "无需改号"}
                。原本地分支保留。
              </p>
              <Button
                disabled={pending}
                onClick={() => void run(() => resolveDraftConflict(true))}
              >
                确认编号调整并覆盖此修订
              </Button>
              <Button
                variant="outline"
                onClick={() => void run(() => copyDraft(draft))}
              >
                保留本地副本
              </Button>
            </DialogContent>
          </Dialog>
        )}
        {conversion && (
          <Dialog open onOpenChange={() => setConversion(null)}>
            <DialogContent>
              <DialogTitle>确认参数转换</DialogTitle>
              <DialogDescription>
                仅删除当前模式不认识的参数，并补充默认值。参考素材及角色需在卡片中明确修复；不会自动删除。
              </DialogDescription>
              <pre className="max-h-64 overflow-auto text-xs">
                {JSON.stringify(
                  { before: draft.params, after: conversion.params },
                  null,
                  2,
                )}
              </pre>
              <Button
                disabled={seq !== conversion.seq}
                onClick={() => {
                  if (
                    !session?.applyConversion(
                      conversion.seq,
                      conversion.params,
                      draft.references,
                    )
                  )
                    setError("输入已变化，请重新检查转换。");
                  setConversion(null);
                }}
              >
                应用转换
              </Button>
              <Button variant="outline" onClick={() => setConversion(null)}>
                取消
              </Button>
            </DialogContent>
          </Dialog>
        )}
        {current.submission && (
          <div className="space-y-2 rounded border p-3" aria-label="未决提交">
            <p>
              {current.submission.state === "accepted"
                ? "这次生成已接受，可查看任务并确认已知晓结果。"
                : "发现未处理的报价；关闭页面不会自动取消。"}
            </p>
            <p className="text-sm">
              提交 {current.submission.id?.slice(0, 8)} · 代次 {current.epoch}
            </p>
            {quote && (
              <p>
                预计费用：{money(quote.quote_micros)}；余额：
                {money(quote.available_micros)}
              </p>
            )}
            {!quote && current.submission.quote && (
              <p>原报价：{money(current.submission.quote.quote_micros ?? 0)}</p>
            )}
            {current.submission.state !== "accepted" &&
              !confirmUnknown &&
              quoteExpired && <p role="alert">报价已过期，请重新检查费用。</p>}
            <pre className="max-h-36 overflow-auto text-xs whitespace-pre-wrap">
              {quote?.final_prompt ?? current.submission.quote?.final_prompt}
            </pre>
            <p className="text-sm">
              冻结模式：{current.submission.draft?.mode} · 编译：
              {activeQuote?.compiler_version}
            </p>
            {activeQuote?.reference_mapping?.length ? (
              <table className="w-full text-left text-xs">
                <caption>显示标签与实际媒体输入顺序</caption>
                <thead>
                  <tr>
                    <th>引用</th>
                    <th>角色</th>
                    <th>媒体位置</th>
                  </tr>
                </thead>
                <tbody>
                  {activeQuote.reference_mapping.map((m) => (
                    <tr key={m.reference_id}>
                      <td>{m.label}</td>
                      <td>{m.role}</td>
                      <td>{m.media_seq_nos?.join("、") || "仅描述"}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            ) : null}
            <div className="flex flex-wrap gap-2">
              <Button
                variant="outline"
                disabled={pending}
                onClick={() =>
                  void run(async () => {
                    await discover();
                  })
                }
              >
                核对提交状态
              </Button>
              {current.submission.state === "accepted" ? (
                <Button
                  disabled={pending}
                  onClick={() => void resolve("acknowledge_result")}
                >
                  已知晓，继续编辑
                </Button>
              ) : (
                <>
                  <Button
                    disabled={
                      pending ||
                      (!confirmUnknown &&
                        (quoteExpired ||
                          recoveredChanged ||
                          !!pendingMode ||
                          activeQuote?.confirmable !== true ||
                          issues.length > 0 ||
                          !sourceReady ||
                          (quote !== null && quoteSeq !== seq)))
                    }
                    onClick={() => void confirm()}
                  >
                    {confirmUnknown ? "重试原确认" : "确认费用并生成"}
                  </Button>
                  <Button
                    variant="outline"
                    disabled={pending || confirmUnknown}
                    onClick={() => void resolve("discard_quote")}
                  >
                    放弃旧报价并重新检查
                  </Button>
                </>
              )}
            </div>
          </div>
        )}
        {!sourceReady && (
          <p role="status">
            {sourceCheck?.key === sourceKey
              ? "参考已失效或暂时无法核验，请重新选择或重试保存。"
              : "正在核验参考权限与版本…"}
          </p>
        )}
        {!current.submission && (
          <Button
            disabled={
              pending ||
              issues.length > 0 ||
              !!pendingMode ||
              confirmUnknown ||
              !sourceReady
            }
            onClick={() => void getQuote()}
          >
            {pending ? "正在保存并检查…" : "检查费用"}
          </Button>
        )}
        {task && (
          <TaskDetail projectID={projectID} id={task} onAdopt={onAdopt} />
        )}
      </div>
    </section>
  );
}

function sameSource(a: API.InputSource, b?: API.InputSource) {
  return (
    !!b &&
    a.kind === b.kind &&
    (a.kind === "prompt_draft"
      ? a.draft_id === b.draft_id && a.revision === b.revision
      : a.canvas_id === b.canvas_id &&
        a.node_id === b.node_id &&
        a.input_revision === b.input_revision)
  );
}
