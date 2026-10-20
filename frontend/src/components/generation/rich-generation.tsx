"use client";

import type { Transaction } from "@tiptap/pm/state";
import type { ComposerCollaboration } from "./composer-session";
import { TextIntentInput } from "./text-intent-input";
import {
  Empty,
  EmptyHeader,
  EmptyTitle,
  EmptyDescription,
  EmptyContent,
} from "@/components/ui/empty";

import { FeedbackToast } from "@/components/feedback/feedback-toast";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Table,
  TableCaption,
  TableHeader,
  TableRow,
  TableHead,
  TableBody,
  TableCell,
} from "@/components/ui/table";
import { Field, FieldGroup, FieldLabel } from "@/components/ui/field";
import { cn } from "cn";
import {
  AtSign,
  SlidersHorizontal,
  Maximize2,
  Minimize2,
  Undo2,
  Redo2,
  Save,
} from "lucide-react";
import {
  InputGroup,
  InputGroupAddon,
  InputGroupButton,
} from "@/components/ui/input-group";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import {
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectGroup,
  SelectItem,
} from "@/components/ui/select";
import { Input } from "@/components/ui/input";
import { Checkbox } from "@/components/ui/checkbox";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import {
  NativeSelect,
  NativeSelectOption,
} from "@/components/ui/native-select";
import {
  useCallback,
  useEffect,
  useRef,
  useState,
  useId,
  type ReactNode,
} from "react";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
} from "@/components/ui/dropdown-menu";
import { Alert, AlertTitle, AlertDescription } from "@/components/ui/alert";
import {
  Dialog,
  DialogContent,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import { getSession, getMyPreferences } from "@/api/identity";
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
import {
  RequestError,
  getSessionGeneration,
  getScopeGeneration,
} from "@/lib/request";
import { reconcileLabels } from "./draft-resolution";
import { canonical } from "./draft-save";
import { PromptEditor } from "./prompt-editor";
import { ModelPicker } from "./model-picker";
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
import type { CanvasReferenceSource } from "./reference-source";
export type CanvasComposerOwner = {
  readOnly?: boolean;
  contextKey: string;
  draft: PromptDraft;
  onChange: (draft: PromptDraft, transaction?: Transaction) => void;
  collaboration?: ComposerCollaboration;
  onSession?: (session: ComposerSession) => (() => void) | void;
  flush: () => Promise<API.InputSource>;
  referenceSources?: CanvasReferenceSource[];
  incomingSources?: (CanvasReferenceSource & { edgeID: string })[];
};
type Props = {
  projectID: string;
  seed: GenerationSeed;
  onLock?: (locked: boolean) => void;
  onCloseGuard?: (guard: (() => Promise<boolean>) | null) => void;
  onAdopt?: (id: string, kind: string) => Promise<void> | void;
  canvas?: CanvasComposerOwner;
  draftID?: string;
  presentation?: "canvas" | "home";
  onDraftReady?: (draftID: string) => void;
  headerActions?: ReactNode;
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
async function readModelCatalog(
  projectID: string,
  capability: GenerationSeed["capability"],
  signal?: AbortSignal,
) {
  const models: API.ModelResponse[] = [];
  const cursors = new Set<string>();
  let cursor: string | undefined;
  do {
    const page = await listProjectModels(
      {
        project_id: projectID,
        capability: capability + ".generate",
        limit: 100,
        ...(cursor ? { cursor } : {}),
      },
      { signal },
    );
    models.push(...(page.items ?? []));
    cursor = page.next_cursor;
    if (cursor) {
      if (cursors.has(cursor)) throw new RequestError("invalid_receipt");
      cursors.add(cursor);
    }
  } while (cursor);
  return models;
}
export function RichGeneration(props: Props) {
  const [bootKey] = useState(() => crypto.randomUUID()),
    [retry, setRetry] = useState(0),
    [opened, setOpened] = useState(props.draftID),
    [ready, setReady] = useState<{
      snapshot: SavedDraft;
      actor: string;
      models: API.ModelResponse[];
      initialModelError: string;
      current: API.SubmissionCurrent;
    } | null>(null),
    [error, setError] = useState("");
  const bootDraft = useRef<PromptDraft | null>(null);
  useEffect(() => {
    const controller = new AbortController();
    let active = true;
    async function boot() {
      try {
        const [identity, catalog, preferences] = await Promise.all([
          getSession({ signal: controller.signal }),
          readModelCatalog(
            props.projectID,
            props.seed.capability,
            controller.signal,
          )
            .then((models) => ({ models, error: "" }))
            .catch((error) => ({
              models: [] as API.ModelResponse[],
              error: errorText(error),
            })),
          getMyPreferences({ signal: controller.signal }),
        ]);
        if (!identity.actor?.id) throw new RequestError("session_invalid");
        let snapshot: SavedDraft;
        if (props.canvas) {
          if (props.presentation === "canvas" && !props.canvas.readOnly)
            await props.canvas.flush();
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
            if (preferences.prompt_suffix)
              draft.document.content.push({
                type: "paragraph",
                content: [{ type: "text", text: preferences.prompt_suffix }],
              });
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
          props.onDraftReady?.(snapshot.draft_id);
          setReady({
            snapshot,
            actor: identity.actor.id,
            models: catalog.models,
            initialModelError: catalog.error,
            current,
          });
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
    return error ? (
      <Empty className="min-h-52">
        <EmptyHeader>
          <EmptyTitle>编辑器暂时无法加载</EmptyTitle>
          <EmptyDescription>{error}</EmptyDescription>
        </EmptyHeader>
        <EmptyContent>
          <Button variant="outline" onClick={() => setRetry((n) => n + 1)}>
            重试
          </Button>
        </EmptyContent>
      </Empty>
    ) : (
      <div
        role="status"
        aria-label="正在加载编辑器"
        className="flex min-h-52 flex-col gap-4 p-4"
      >
        <span className="sr-only">正在核验身份、草稿和未决提交…</span>
        <Skeleton className="size-14" />
        <Skeleton className="h-16 w-full" />
        <Skeleton className="h-7 w-2/3" />
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
  presentation,
  headerActions,
  snapshot,
  actor,
  models: initialModels,
  initialModelError,
  current: initialCurrent,
  onOpen,
}: Props & {
  snapshot: SavedDraft;
  actor: string;
  models: API.ModelResponse[];
  initialModelError: string;
  current: API.SubmissionCurrent;
  onOpen: (id: string) => void;
}) {
  const formFieldId = useId();
  const compact = presentation === "canvas" || presentation === "home";
  const [quoteReviewOpen, setQuoteReviewOpen] = useState(true);

  const [models, setModels] = useState(initialModels);
  const [modelError, setModelError] = useState(initialModelError);
  const [reloadingModels, setReloadingModels] = useState(false);
  const modelRequest = useRef<AbortController | null>(null);
  useEffect(() => () => modelRequest.current?.abort(), []);
  async function retryModels() {
    if (modelRequest.current) return;
    const controller = new AbortController();
    modelRequest.current = controller;
    setReloadingModels(true);
    try {
      const result = await readModelCatalog(
        projectID,
        seed.capability,
        controller.signal,
      );
      if (!controller.signal.aborted) {
        setModels(result);
        setModelError("");
      }
    } catch (error) {
      if (!controller.signal.aborted) {
        setModels([]);
        setModelError(errorText(error));
      }
    } finally {
      if (!controller.signal.aborted) setReloadingModels(false);
      modelRequest.current = null;
    }
  }
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
    [draftsLoaded, setDraftsLoaded] = useState(false),
    [draftCursor, setDraftCursor] = useState<string | undefined>(),
    [conversion, setConversion] = useState<{
      seq: number;
      params: PromptDraft["params"];
    } | null>(null),
    [serverConflict, setServerConflict] = useState<SavedDraft | null>(null);
  const workspaceRef = useRef<HTMLElement>(null);
  useEffect(() => {
    const section = workspaceRef.current;
    if (!full || !section) return;
    const previous = document.activeElement as HTMLElement | null;
    const siblings: [HTMLElement, boolean][] = [];
    let child: HTMLElement = section;
    while (child.parentElement) {
      for (const sibling of child.parentElement.children) {
        if (sibling !== child && sibling instanceof HTMLElement) {
          siblings.push([sibling, sibling.inert]);
          sibling.inert = true;
        }
      }
      child = child.parentElement;
      if (child === document.body) break;
    }
    section.querySelector<HTMLElement>('[aria-label="退出全屏"]')?.focus();
    return () => {
      for (const [element, inert] of siblings) element.inert = inert;
      if (previous?.isConnected) previous.focus({ preventScroll: true });
    };
  }, [full]);
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
  const [sourceEpoch, setSourceEpoch] = useState(0);
  const sourceKey = JSON.stringify([draft.references, sourceEpoch]);
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
  const frozen = confirmUnknown || canvas?.readOnly === true;
  const recoveredChanged =
    !quote &&
    !!current.submission?.draft &&
    (canonical(draft) !== canonical(current.submission.draft) ||
      (!canvas &&
        current.submission.input_source?.kind === "prompt_draft" &&
        current.submission.input_source.revision !== revision));
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
        persist:
          presentation === "canvas" ? async () => {} : branchJournal.write,
        changed: updateStatus,
      });
    }
    if (!canvas && presentation !== "canvas")
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
  }, [
    contextKey,
    replicaId,
    snapshot,
    projectID,
    updateStatus,
    canvas,
    presentation,
  ]);
  useEffect(() => {
    onLock?.(frozen || pending);
    session?.editor.setEditable(
      !suspended && (!frozen || session.composing),
      false,
    );
    return () => {
      onLock?.(false);
    };
  }, [frozen, pending, onLock, session, suspended, draft]);
  useEffect(() => {
    if (canvas) return;
    const before = (e: BeforeUnloadEvent) => {
      if (saver.current?.status !== "saved" || confirmUnknown) {
        e.preventDefault();
      }
    };
    window.addEventListener("beforeunload", before);
    return () => window.removeEventListener("beforeunload", before);
  }, [confirmUnknown, canvas]);
  useEffect(() => {
    const suspend = () => {
      saver.current?.dispose();
      session?.cancelModeSelection();
      session?.editor.setEditable(false);
      setSuspended(true);
    };
    const check = () => {
      if (document.visibilityState === "visible") {
        setSourceEpoch((epoch) => epoch + 1);
        void getSession()
          .then((identity) => {
            if (identity.actor?.id !== actor) suspend();
          })
          .catch(suspend);
      }
    };
    const channel =
      typeof BroadcastChannel !== "undefined"
        ? new BroadcastChannel("glowloom-private-cache")
        : null;
    if (channel) channel.onmessage = suspend;
    window.addEventListener("glowloom:private-cache-cleared", suspend);
    document.addEventListener("visibilitychange", check);
    window.addEventListener("online", check);
    return () => {
      channel?.close();
      window.removeEventListener("glowloom:private-cache-cleared", suspend);
      document.removeEventListener("visibilitychange", check);
      window.removeEventListener("online", check);
    };
  }, [actor, session]);
  const ownerSessionCleanup = useRef<(() => void) | undefined>(undefined);
  const changed = useCallback(
    (value: ComposerSnapshot, transaction?: Transaction) => {
      const previous = latest.current;
      latest.current = value;
      setDraft(value.draft);
      setSeq(value.localSeq);
      saver.current?.edit(value);
      if (transaction?.getMeta("composer-authoritative")) return;
      if (canvas?.collaboration || !sessionRef.current?.composing)
        if (canvas?.collaboration) canvas.onChange(value.draft, transaction);
        else canvas?.onChange(value.draft);
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
  async function loadDrafts() {
    await run(async () => {
      const page = await listPromptDrafts({ pid: projectID });
      setDrafts(page.items ?? []);
      setDraftCursor(page.next_cursor);
      setDraftsLoaded(true);
    });
  }
  async function getQuote() {
    if (compact) setQuoteReviewOpen(true);
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
      // Apply fresh capability facts even when the requested mode is rejected.
      setModels((values) =>
        values.map((value) =>
          value.current_version?.id === version
            ? (found ?? { ...value, status: "disabled" })
            : value,
        ),
      );
      if (!found?.current_version?.modes?.includes(mode))
        throw new Error("该模型模式已不可用");
      if (
        found.status !== "active" ||
        found.provider_status !== "active" ||
        !found.credential_present
      )
        throw new Error("该模型当前不可用");
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
      if (branch.inputRecovery) {
        const editor = sessionRef.current;
        const generation = getSessionGeneration();
        const scope = getScopeGeneration();
        const sequence = latest.current.localSeq;
        const fresh = (await branchJournal.list(contextKey)).find(
          (value) =>
            value.replicaId === branch.replicaId && value.inputRecovery,
        );
        if (
          !fresh ||
          !editor ||
          !alive.current ||
          editor !== sessionRef.current ||
          generation !== getSessionGeneration() ||
          scope !== getScopeGeneration() ||
          sequence !== latest.current.localSeq
        )
          throw new Error("恢复文字已变化或无权访问，请重新核验。");
        // No await between the final authorization/fence check and adoption.
        editor.adoptDraft(fresh.snapshot.draft);
        return;
      }
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
  const promptControl = (
    <PromptEditor
      inputGroup={compact}
      className={
        compact ? "max-h-80 min-h-16 overflow-y-auto outline-none" : undefined
      }
      placeholder={
        compact
          ? seed.capability === "video"
            ? "上传参考视频、输入文字或 @ 主体，描述你想生成的视频"
            : seed.capability === "image"
              ? "上传参考图、输入文字或 @ 主体，描述你想生成的图片"
              : "输入文字或 @ 主体，描述你的创作需求"
          : undefined
      }
      contextKey={contextKey}
      replicaId={replicaId}
      draft={snapshot.draft}
      collaboration={canvas?.collaboration}
      onDispose={() => {
        ownerSessionCleanup.current?.();
        ownerSessionCleanup.current = undefined;
      }}
      onReady={(value) => {
        sessionRef.current = value;
        setSession(value);
        ownerSessionCleanup.current?.();
        ownerSessionCleanup.current = canvas?.onSession?.(value) || undefined;
        const current = value.snapshot();
        latest.current = current;
        setDraft(current.draft);
        setSeq(current.localSeq);
        canvas?.onChange(current.draft);
      }}
      onChange={changed}
      onRequestSave={() =>
        void run(async () => {
          await flush();
        })
      }
      onRequestSubmit={() => void getQuote()}
    />
  );
  const referenceControls = session ? (
    <ReferenceControls
      session={session}
      draft={draft}
      projectID={projectID}
      model={model}
      disabled={frozen}
      authorized={sourceReady}
      compact={compact}
      canvasSources={canvas?.referenceSources}
      incomingSources={canvas?.incomingSources}
    />
  ) : null;
  const parameterControls = model ? (
    <FieldGroup className="grid gap-3 sm:grid-cols-2">
      {(() => {
        try {
          return fieldsForMode(
            model.current_version?.param_schema,
            draft.mode ?? "",
          );
        } catch {
          return [];
        }
      })().map((f, fieldIndex0) => (
        <Field key={f.field} data-disabled={frozen}>
          <FieldLabel htmlFor={`${formFieldId}-field-3-${fieldIndex0}`}>
            <span>{f.label}</span>
          </FieldLabel>
          {f.enum && compact && f.enum.length <= 7 ? (
            <ToggleGroup
              id={`${formFieldId}-field-3-${fieldIndex0}`}
              aria-label={f.label}
              type="single"
              value={String(draft.params[f.field] ?? "")}
              disabled={frozen}
              className="flex-wrap justify-start"
              onValueChange={(value) => {
                if (value)
                  session?.updateParams({
                    ...draft.params,
                    [f.field]:
                      f.type === "number" || f.type === "integer"
                        ? Number(value)
                        : value,
                  });
              }}
            >
              {f.enum.map((value) => (
                <ToggleGroupItem key={String(value)} value={String(value)}>
                  {value}
                </ToggleGroupItem>
              ))}
            </ToggleGroup>
          ) : f.enum ? (
            <NativeSelect
              id={`${formFieldId}-field-3-${fieldIndex0}`}

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
              <NativeSelectOption value="">请选择</NativeSelectOption>
              {f.enum.map((v) => (
                <NativeSelectOption key={String(v)} value={v}>
                  {v}
                </NativeSelectOption>
              ))}
            </NativeSelect>
          ) : f.type === "boolean" ? (
            <Checkbox
              id={`${formFieldId}-field-3-${fieldIndex0}`}

              disabled={frozen}
              checked={!!draft.params[f.field]}
              onCheckedChange={(e) =>
                session?.updateParams({
                  ...draft.params,
                  [f.field]: e === true,
                })
              }
            />
          ) : canvas?.collaboration && f.field === "negative_prompt" ? (
            <TextIntentInput
              id={`${formFieldId}-field-3-${fieldIndex0}`}
              disabled={frozen}
              value={String(draft.params[f.field] ?? "")}
              onEdit={(value, edit) =>
                session?.updateTextParam(f.field, value, edit)
              }
              onHistory={(direction) =>
                canvas.collaboration?.history[direction]()
              }
              onCompositionState={(active) =>
                canvas.collaboration?.composition(active, "negative_prompt")
              }
            />
          ) : (
            <Input
              id={`${formFieldId}-field-3-${fieldIndex0}`}

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
        </Field>
      ))}
      <Field data-disabled={frozen}>
        <FieldLabel htmlFor={`${formFieldId}-field-4`}>
          <span>候选数量</span>
        </FieldLabel>
        <Input
          id={`${formFieldId}-field-4`}

          type="number"
          min={1}
          max={Math.min(
            8,
            Number(model.current_version?.limits?.max_outputs ?? 1),
          )}
          disabled={frozen}
          value={Number(draft.params.output_count ?? 1)}
          onChange={(e) =>
            session?.updateParams({
              ...draft.params,
              output_count: Number(e.target.value),
            })
          }
        />
      </Field>
    </FieldGroup>
  ) : null;
  const issuesPanel =
    issues.length > 0 ? (
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
    ) : null;
  const submissionPanel = current.submission ? (
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
        quoteExpired && (
          <FeedbackToast message="报价已过期，请重新检查费用。" />
        )}
      <pre className="max-h-36 overflow-auto text-xs whitespace-pre-wrap">
        {quote?.final_prompt ?? current.submission.quote?.final_prompt}
      </pre>
      <p className="text-sm">
        冻结模式：{current.submission.draft?.mode} · 编译：
        {activeQuote?.compiler_version}
      </p>
      {activeQuote?.reference_mapping?.length ? (
        <Table className="w-full text-left text-xs">
          <TableCaption>显示标签与实际媒体输入顺序</TableCaption>
          <TableHeader>
            <TableRow>
              <TableHead>引用</TableHead>
              <TableHead>角色</TableHead>
              <TableHead>媒体位置</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {activeQuote.reference_mapping.map((m) => (
              <TableRow key={m.reference_id}>
                <TableCell>{m.label}</TableCell>
                <TableCell>{m.role}</TableCell>
                <TableCell>{m.media_seq_nos?.join("、") || "仅描述"}</TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
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
  ) : null;
  if (suspended)
    return (
      <Empty>
        <EmptyHeader>
          <EmptyTitle>请重新核验登录状态</EmptyTitle>
          <EmptyDescription>本地草稿已保留。</EmptyDescription>
        </EmptyHeader>
        <EmptyContent>
          <Button onClick={() => onOpen(snapshot.draft_id)}>
            重新核验并找回草稿
          </Button>
        </EmptyContent>
      </Empty>
    );
  return (
    <section
      ref={workspaceRef}
      className={cn(
        "flex min-w-0 flex-col gap-4",
        full &&
          "fixed inset-0 z-50 items-center justify-center overflow-y-auto bg-black/60 p-4 sm:p-8",
      )}
      onKeyDownCapture={(event) => {
        if (full && event.key === "Tab") {
          const focusable = Array.from(
            event.currentTarget.querySelectorAll<HTMLElement>(
              'button:not(:disabled),input:not(:disabled),[contenteditable="true"],[tabindex="0"]',
            ),
          ).filter((n) => n.getClientRects().length);
          const first = focusable[0],
            last = focusable.at(-1);
          if (event.shiftKey && document.activeElement === first) {
            event.preventDefault();
            last?.focus();
          }
          if (!event.shiftKey && document.activeElement === last) {
            event.preventDefault();
            first?.focus();
          }
        }
        if (
          full &&
          event.key === "Escape" &&
          !(event.target as HTMLElement).closest(
            '[data-slot="popover-content"]',
          )
        ) {
          event.preventDefault();
          event.stopPropagation();
          setFull(false);
        }
      }}
      data-expanded={full || undefined}
      data-presentation={presentation}
      role={full ? "dialog" : undefined}
      aria-modal={full || undefined}
      aria-label="多参考创作编辑器"
    >
      <div
        className={cn(
          "mx-auto flex w-full max-w-5xl flex-col gap-4",
          compact && "gap-2",
          full &&
            "max-h-[90dvh] overflow-y-auto rounded-3xl bg-card p-4 sm:p-6",
        )}
      >
        {!compact && (
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
                disabled={pending || frozen}
                onClick={() => void loadDrafts()}
              >
                找回草稿
              </Button>
            )}
          </div>
        )}
        {draftsLoaded && (
          <div className="rounded border p-3" aria-label="服务端草稿">
            <div className="flex items-center justify-between gap-2">
              <p className="text-sm text-muted-foreground">
                {drafts.length ? "选择要恢复的草稿" : "暂无保存草稿"}
              </p>
              <Button
                variant="ghost"
                size="sm"
                onClick={() => setDraftsLoaded(false)}
              >
                收起草稿
              </Button>
            </div>
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
                disabled={frozen || pending}
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
                  {b.inputRecovery
                    ? "登录过期前的文字（参考素材与生成设置需重新选择）"
                    : `分支 ${b.replicaId.slice(0, 8)} · 基线 r${b.serverRevision}`}
                </span>
                <Button disabled={frozen} onClick={() => void recover(b)}>
                  恢复此分支
                </Button>
                <Button
                  disabled={frozen || !!canvas || b.inputRecovery}
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
        {compact ? (
          <InputGroup
            variant="composer"
            className={cn(
              "min-h-[208px] flex-col items-stretch gap-1",
              presentation === "home" && "min-h-[248px]",
            )}
          >
            <InputGroupAddon
              align="block-start"
              className="items-start px-4 pt-4"
            >
              {referenceControls}
              {headerActions}
              {presentation === "home" && (
                <DropdownMenu>
                  <DropdownMenuTrigger asChild>
                    <InputGroupButton
                      className="ml-auto"
                      disabled={pending || frozen}
                    >
                      更多工具
                    </InputGroupButton>
                  </DropdownMenuTrigger>
                  <DropdownMenuContent align="end" sideOffset={8}>
                    <DropdownMenuGroup>
                      <DropdownMenuItem
                        onSelect={() =>
                          void run(async () => {
                            await flush();
                          })
                        }
                      >
                        保存草稿
                      </DropdownMenuItem>
                      <DropdownMenuItem onSelect={() => void loadDrafts()}>
                        找回草稿
                      </DropdownMenuItem>
                    </DropdownMenuGroup>
                  </DropdownMenuContent>
                </DropdownMenu>
              )}
              <InputGroupButton
                aria-label={full ? "退出全屏" : "展开编辑器"}
                size="icon-sm"
                className={cn(presentation !== "home" && "ml-auto")}
                onClick={() => setFull((value) => !value)}
              >
                {full ? <Minimize2 /> : <Maximize2 />}
              </InputGroupButton>
            </InputGroupAddon>
            <div
              data-slot="input-group-control"
              className={cn(
                "min-h-20 w-full flex-1 px-4 py-2 [&_.tiptap]:min-h-16 [&_.tiptap]:outline-none",
                full && "[&_.tiptap]:min-h-64",
              )}
            >
              {promptControl}
            </div>
            <InputGroupAddon
              align="block-end"
              className="flex-wrap gap-1 px-4 pb-4"
            >
              <ModelPicker
                models={models}
                value={draft.modelVersion}
                disabled={frozen || !session}
                onSelect={(target) => {
                  if (target.current_version?.id)
                    void selectMode(
                      target.current_version.id,
                      target.current_version.modes?.[0] ?? "",
                    );
                }}
              />
              <Select
                value={draft.mode ?? ""}
                disabled={frozen || !model || !session}
                onValueChange={(mode) =>
                  void selectMode(draft.modelVersion!, mode)
                }
              >
                <SelectTrigger aria-label="生成模式" size="sm" variant="quiet">
                  <SelectValue placeholder="选择模式" />
                </SelectTrigger>
                <SelectContent
                  position="popper"
                  side="bottom"
                  align="start"
                  sideOffset={8}
                  collisionPadding={16}
                >
                  <SelectGroup>
                    {model?.current_version?.modes?.map((mode) => (
                      <SelectItem key={mode} value={mode}>
                        {generationModeLabel(mode)}
                      </SelectItem>
                    ))}
                  </SelectGroup>
                </SelectContent>
              </Select>
              <Popover>
                <PopoverTrigger asChild>
                  <InputGroupButton
                    aria-label="生成参数"
                    disabled={!model || !session || frozen}
                  >
                    <SlidersHorizontal data-icon="inline-start" />
                    {[draft.params.aspect_ratio, draft.params.resolution]
                      .filter((value) => typeof value === "string" && value)
                      .join(" · ") || "参数"}{" "}
                    · {Number(draft.params.output_count ?? 1)}个
                  </InputGroupButton>
                </PopoverTrigger>
                <PopoverContent
                  side="bottom"
                  align="start"
                  sideOffset={8}
                  collisionPadding={16}
                  className="max-h-[70dvh] w-80 max-w-[calc(100vw-32px)] overflow-y-auto"
                  aria-label="生成参数"
                >
                  {parameterControls}
                  <div className="flex flex-wrap gap-1">
                    <Button
                      variant="ghost"
                      size="icon-sm"
                      aria-label="撤销"
                      disabled={frozen || !session}
                      onClick={() => session?.undo()}
                    >
                      <Undo2 />
                    </Button>
                    <Button
                      variant="ghost"
                      size="icon-sm"
                      aria-label="重做"
                      disabled={frozen || !session}
                      onClick={() => session?.redo()}
                    >
                      <Redo2 />
                    </Button>
                    <Button
                      variant="ghost"
                      size="sm"
                      disabled={pending}
                      onClick={() =>
                        void run(async () => {
                          await flush();
                        })
                      }
                    >
                      <Save data-icon="inline-start" />
                      保存草稿
                    </Button>
                  </div>
                  <p role="status" className="text-xs text-muted-foreground">
                    {canvas ? "随画布保存" : saveState} · r{revision}
                  </p>
                </PopoverContent>
              </Popover>
              <InputGroupButton
                aria-label="提及素材或主体"
                size="icon-sm"
                disabled={frozen || !session}
                onClick={() =>
                  session?.editor
                    .chain()
                    .focus(undefined, { scrollIntoView: false })
                    .insertContent("@")
                    .run()
                }
              >
                <AtSign />
              </InputGroupButton>
              {issues.length > 0 ? (
                <Popover>
                  <PopoverTrigger asChild>
                    <InputGroupButton>需要调整</InputGroupButton>
                  </PopoverTrigger>
                  <PopoverContent
                    side="top"
                    className="max-h-[70dvh] w-80 overflow-y-auto"
                  >
                    {issuesPanel}
                  </PopoverContent>
                </Popover>
              ) : null}
              {current.submission ? (
                <InputGroupButton
                  className="ml-auto"
                  variant="default"
                  onClick={() => setQuoteReviewOpen(true)}
                >
                  {current.submission.state === "accepted"
                    ? "查看任务"
                    : "查看报价"}
                </InputGroupButton>
              ) : (
                <InputGroupButton
                  className="ml-auto"
                  variant="default"
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
                </InputGroupButton>
              )}
            </InputGroupAddon>
          </InputGroup>
        ) : (
          <>
            <div className="grid gap-3 sm:grid-cols-2">
              <Field data-disabled={frozen || !session}>
                <FieldLabel htmlFor={`${formFieldId}-field-1`}>
                  <span>生成模型</span>
                </FieldLabel>
                <NativeSelect
                  id={`${formFieldId}-field-1`}
                  aria-label="生成模型"

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
                  <NativeSelectOption value="">请选择模型</NativeSelectOption>
                  {models.map((m) => (
                    <NativeSelectOption
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
                    </NativeSelectOption>
                  ))}
                </NativeSelect>
              </Field>
              <Field data-disabled={frozen || !model || !session}>
                <FieldLabel htmlFor={`${formFieldId}-field-2`}>
                  <span>生成模式</span>
                </FieldLabel>
                <NativeSelect
                  id={`${formFieldId}-field-2`}
                  aria-label="生成模式"

                  disabled={frozen || !model || !session}
                  value={draft.mode ?? ""}
                  onChange={(e) =>
                    selectMode(draft.modelVersion!, e.target.value)
                  }
                >
                  <NativeSelectOption value="">请选择模式</NativeSelectOption>
                  {model?.current_version?.modes?.map((m) => (
                    <NativeSelectOption key={m}>{m}</NativeSelectOption>
                  ))}
                </NativeSelect>
              </Field>
            </div>
            <div className="min-h-32 rounded-lg border p-3 text-base [&_.tiptap]:min-h-24 [&_.tiptap]:outline-none">
              {promptControl}
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
                {referenceControls}
              </>
            )}
            {parameterControls}
          </>
        )}
        {modelError && (
          <Alert>
            <AlertTitle>模型目录暂不可用</AlertTitle>
            <AlertDescription>
              {modelError}。正文和参考仍可编辑、保存；恢复模型目录前不能生成。
            </AlertDescription>
            <Button
              variant="outline"
              size="sm"
              disabled={reloadingModels}
              onClick={() => void retryModels()}
            >
              {reloadingModels ? "正在读取模型…" : "重试模型目录"}
            </Button>
          </Alert>
        )}
        {!compact && issuesPanel}
        {error && <FeedbackToast message={error} />}
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
        {compact ? (
          <Dialog
            open={quoteReviewOpen && !!current.submission}
            onOpenChange={(open) => {
              if (!pending && !confirmUnknown) setQuoteReviewOpen(open);
            }}
          >
            <DialogContent
              showCloseButton={!pending && !confirmUnknown}
              className="max-h-[85dvh] overflow-y-auto sm:max-w-3xl"
            >
              <DialogTitle>报价与生成任务</DialogTitle>
              <DialogDescription>
                确认当前冻结输入与费用后开始生成。
              </DialogDescription>
              {submissionPanel}
              {task && (
                <TaskDetail projectID={projectID} id={task} onAdopt={onAdopt} />
              )}
            </DialogContent>
          </Dialog>
        ) : (
          submissionPanel
        )}
        {!sourceReady && (
          <div>
            <p role="status">
              {sourceCheck?.key === sourceKey
                ? "参考已失效或暂时无法核验，请重新选择或重新核验。"
                : "正在核验参考权限与版本…"}
            </p>
            {sourceCheck?.key === sourceKey && (
              <Button
                variant="outline"
                onClick={() => setSourceEpoch((epoch) => epoch + 1)}
              >
                重新核验参考
              </Button>
            )}
          </div>
        )}
        {!compact && !current.submission && (
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
        {!compact && task && (
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

function generationModeLabel(mode: string) {
  return (
    (
      {
        text_to_image: "文生图",
        text2image: "文生图",
        image_to_image: "图生图",
        image2image: "图生图",
        text_to_video: "文生视频",
        text2video: "文生视频",
        image_to_video: "图生视频",
        image2video: "图生视频",
        omni_reference: "全能参考",
        text_to_text: "文字创作",
      } as Record<string, string>
    )[mode] ?? mode
  );
}
