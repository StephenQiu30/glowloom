"use client";
import { Badge } from "@/components/ui/badge";
import { QuoteCostDetail } from "./quote-cost-detail";
import { GenerationCredits, useGenerationCredits } from "./generation-credits";
import dynamic from "next/dynamic";
import { modelsForCreation, creationModeLabel } from "./creation-contract";
import { ComposerSurface } from "./composer-surface";
import { ContentState } from "@/components/layout/content-state";
import { readErrorMessage } from "@/lib/read-error";

import type { Transaction } from "@tiptap/pm/state";
import type { ComposerCollaboration } from "./composer-session";
import { TextIntentInput } from "./text-intent-input";

import { toast } from "@/components/feedback/notification";
import { FeedbackToast } from "@/components/feedback/feedback-toast";
import { Skeleton } from "@/components/ui/skeleton";
import { Field, FieldLabel } from "@/components/ui/field";
import { cn } from "cn";
import {
  ArrowUp,
  AtSign,
  LoaderCircle,
  Maximize2,
  Minimize2,
  MoreHorizontal,
  RefreshCw,
} from "lucide-react";
import { InputGroup, InputGroupAddon } from "@/components/ui/input-group";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  useCallback,
  useEffect,
  useEffectEvent,
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
import {
  Dialog,
  DialogContent,
  DialogFooter,
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
import {
  availableModelModes,
  modelAvailableForMode,
} from "./model-availability";
import { ModelPicker } from "./model-picker";
import { GenerationParameters } from "./generation-parameters";
import { GenerationToolbarParameters } from "./generation-toolbar-parameters";
import {
  ComposerToolbar,
  ComposerToolbarActions,
  ComposerToolbarButton,
  ComposerSelectTrigger,
} from "./composer-toolbar";
import { AvatarTextDrive } from "./avatar-text-drive";
import type { VoiceSelection } from "./voice-library-api";
const VoiceLibrary = dynamic(() =>
  import("./voice-library").then((m) => m.VoiceLibrary),
);
import {
  referenceMode,
  referenceGeneration,
  defaultGenerationDraft,
} from "./reference-mode";
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
import { fieldsForMode, requireQuote, type Quote } from "./contracts";
import { TaskDetail, type GenerationSeed } from "./generation-dialog";
import type {
  CanvasLinkedReferenceSource,
  CanvasReferenceSource,
  CanvasSubjectReferenceSource,
} from "./reference-source";
export type CanvasComposerOwner = {
  readOnly?: boolean;
  contextKey: string;
  draft: PromptDraft;
  onChange: (draft: PromptDraft, transaction?: Transaction) => void;
  collaboration?: ComposerCollaboration;
  onSession?: (
    session: ComposerSession,
    initialDraft: PromptDraft,
  ) => (() => void) | void;
  // Opening the editor must not materialize an empty generation input.
  waitForNode?: () => Promise<void>;
  flush: (draft: PromptDraft) => Promise<API.InputSource>;
  referenceSources?: CanvasReferenceSource[];
  subjectSources?: CanvasSubjectReferenceSource[];
  incomingSources?: CanvasLinkedReferenceSource[];
  onMotion?: () => void;
};
type Props = {
  adoptLabel?: string;
  projectID: string;
  seed: GenerationSeed;
  onLock?: (locked: boolean) => void;
  onCloseGuard?: (guard: (() => Promise<boolean>) | null) => void;
  onAdopt?: (id: string, kind: string) => Promise<void> | void;
  onAdoptText?: (operationID: string, outputID: string) => Promise<void> | void;
  canvas?: CanvasComposerOwner;
  referenceSources?: CanvasReferenceSource[];
  subjectSources?: CanvasSubjectReferenceSource[];
  draftID?: string;
  presentation?: "canvas" | "home" | "assistant";
  onComposerSession?: (session: ComposerSession | null) => void;
  onOperationReady?: (operationID: string, draftID: string) => void;
  onDraftReady?: (draftID: string) => void;
  canvasToolbar?: ReactNode;
  headerActions?: ReactNode;
  projectControl?: ReactNode;
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
  capability: string,
  signal?: AbortSignal,
) {
  const models: API.ModelResponse[] = [];
  const cursors = new Set<string>();
  let cursor: string | undefined;
  do {
    const page = await listProjectModels(
      {
        project_id: projectID,
        capability,
        limit: 100,
        ...(cursor ? { cursor } : {}),
      },
      { signal },
    );
    models.push(...(page.items ?? []));
    cursor = page.next_cursor ?? undefined;
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
            props.seed.reuse?.capability ?? props.seed.capability + ".generate",
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
            await props.canvas.waitForNode?.();
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
            if (
              preferences.prompt_suffix &&
              props.seed.capability !== "audio" &&
              !["avatar", "motion"].includes(props.seed.creationType ?? "")
            )
              draft.document.content.push({
                type: "paragraph",
                content: [{ type: "text", text: preferences.prompt_suffix }],
              });
            const add = (ref: ReturnType<typeof mediaBinding>) => {
              ref.order = draft.references.length;
              draft.references.push(ref);
              const kind =
                ref.source.kind === "subject" ||
                ref.source.kind === "canvas_subject"
                  ? "subject"
                  : ref.mediaKind;
              draft.labelState.allocations[ref.id] = {
                labelKind: kind,
                ordinal: (draft.labelState.highWater[kind] =
                  (draft.labelState.highWater[kind] ?? 0) + 1),
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
        const availableModels = modelsForCreation(
          catalog.models,
          props.seed.creationType,
        );
        if (active) {
          props.onDraftReady?.(snapshot.draft_id);
          setReady({
            snapshot,
            actor: identity.actor.id,
            models: availableModels,
            initialModelError: catalog.error,
            current,
          });
          setError("");
        }
      } catch (error) {
        if (active) setError(readErrorMessage(error));
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
  if (!ready && error && props.presentation)
    return (
      <FeedbackToast
        message={error}
        action={{
          label: "重试",
          onClick: () => {
            setError("");
            setRetry((n) => n + 1);
          },
        }}
      />
    );
  if (!ready)
    return error ? (
      <ContentState
        className={cn(
          "min-h-52",
          props.presentation === "canvas" && "canvas-generator-loading",
        )}
        state="error"
        title="编辑器暂时无法加载"
        description={error}
        actions={
          <>
            {props.headerActions}
            <Button
              onClick={() => {
                setError("");
                setRetry((n) => n + 1);
              }}
            >
              重试
            </Button>
          </>
        }
      />
    ) : (
      <div
        role="status"
        aria-label="正在加载编辑器"
        className={cn(
          "flex min-h-52 flex-col gap-4 p-4",
          props.presentation === "canvas" && "canvas-generator-loading",
        )}
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
  adoptLabel,
  projectID,
  seed,
  onLock,
  onCloseGuard,
  onAdopt,
  onAdoptText,
  canvas,
  referenceSources,
  subjectSources,
  presentation,
  onComposerSession,
  onOperationReady,
  headerActions,
  canvasToolbar,
  projectControl,
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
  const [initialDraft] = useState(() =>
    presentation === "canvas" && !canvas?.readOnly && !initialCurrent.submission
      ? defaultGenerationDraft(snapshot.draft, initialModels)
      : snapshot.draft,
  );
  const formFieldId = useId();
  const [voiceLibraryOpen, setVoiceLibraryOpen] = useState(false);
  const voiceLibraryButton = useRef<HTMLButtonElement>(null);
  const compact =
    presentation === "canvas" ||
    presentation === "home" ||
    presentation === "assistant";
  const [quoteReviewOpen, setQuoteReviewOpen] = useState(
    presentation !== "assistant",
  );

  const [models, setModels] = useState(initialModels);
  const catalog = useRef(initialModels);
  useEffect(() => {
    catalog.current = models;
  }, [models]);
  const resolveReferenceGeneration = useCallback(
    (
      value: PromptDraft,
      previous?: Pick<PromptDraft, "modelVersion" | "mode">,
    ) =>
      referenceGeneration(
        value,
        catalog.current.find(
          (item) => item.current_version?.id === value.modelVersion,
        ),
        previous,
      ),
    [],
  );
  const [modelError, setModelError] = useState(initialModelError);
  const [modelRetryAttempt, setModelRetryAttempt] = useState(0);
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
        seed.reuse?.capability ?? seed.capability + ".generate",
        controller.signal,
      );
      if (!controller.signal.aborted) {
        const available = modelsForCreation(result, seed.creationType);
        catalog.current = available;
        setModels(available);
        setModelError("");
        if (
          presentation === "canvas" &&
          session &&
          !frozen &&
          !current.submission &&
          !session.pendingTarget
        ) {
          const before = session.snapshot().draft;
          const next = defaultGenerationDraft(before, available);
          if (next !== before && next.modelVersion && next.mode) {
            if (!canvasEdited.current) {
              session.applyAuthoritative(
                next,
                session.editor.state.tr.setMeta(
                  "composer-display-defaults",
                  before,
                ),
              );
            } else {
              session.resolveModeSelection(
                session.beginModeSelection({
                  modelVersion: next.modelVersion,
                  mode: next.mode,
                }),
              );
            }
          }
        }
      }
    } catch (error) {
      if (!controller.signal.aborted) {
        setModels([]);
        setModelError(errorText(error));
        setModelRetryAttempt((attempt) => attempt + 1);
      }
    } finally {
      if (!controller.signal.aborted) setReloadingModels(false);
      modelRequest.current = null;
    }
  }
  const [replicaId] = useState(() => crypto.randomUUID()),
    [session, setSession] = useState<ComposerSession | null>(null),
    [draft, setDraft] = useState(initialDraft),
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
  useEffect(() => {
    onComposerSession?.(session);
    return () => onComposerSession?.(null);
  }, [session, onComposerSession]);

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
      draft: initialDraft,
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
  const quoteAmountKnown =
    Number.isSafeInteger(activeQuote?.quote_micros) &&
    typeof activeQuote?.quote_micros === "number" &&
    activeQuote.quote_micros >= 0;
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
  const estimateIssues =
    seed.capability === "image"
      ? editorIssues(draft, model, { allowEmptyPrompt: true })
      : issues;

  const pendingMode = session?.pendingTarget;
  const creditPreview = useGenerationCredits({
    projectID,
    draft,
    enabled:
      !!model &&
      !estimateIssues.length &&
      !pendingMode &&
      sourceReady &&
      !current.submission,
    regenerate: !!seed.reuse,
  });
  const creditDisplay =
    model && !current.submission ? (
      <GenerationCredits preview={creditPreview} />
    ) : null;
  const canvasCanGenerate =
    !!session &&
    !frozen &&
    !issues.length &&
    !pendingMode &&
    sourceReady &&
    creditPreview.status !== "unavailable";
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
    return () => {
      onLock?.(false);
    };
  }, [frozen, pending, onLock]);
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
          .catch((error: unknown) => {
            if (
              error instanceof RequestError &&
              [401, 403].includes(error.status)
            )
              suspend();
          });
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
  const publishedCanvasSeq = useRef(0);
  const canvasEdited = useRef(false);
  const ownerSessionCleanup = useRef<(() => void) | undefined>(undefined);
  const changed = useCallback(
    (value: ComposerSnapshot, transaction?: Transaction) => {
      const previous = latest.current;
      latest.current = value;
      setDraft(value.draft);
      setSeq(value.localSeq);
      saver.current?.edit(value);
      if (transaction?.getMeta("composer-authoritative")) {
        publishedCanvasSeq.current = value.localSeq;
        canvasEdited.current = false;
        return;
      }
      if (canvas) {
        if (
          !canvas.readOnly &&
          publishedCanvasSeq.current !== value.localSeq &&
          (canvas.collaboration || !sessionRef.current?.composing)
        ) {
          publishedCanvasSeq.current = value.localSeq;
          canvasEdited.current = true;
          if (canvas.collaboration) canvas.onChange(value.draft, transaction);
          else canvas.onChange(value.draft);
        }
        return;
      }
      if (previous.localSeq === value.localSeq && !firstDirtyAt.current) return;
      firstDirtyAt.current ??= Date.now();
      if (timer.current) clearTimeout(timer.current);
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
        Math.max(0, Math.min(800, 5000 - (Date.now() - firstDirtyAt.current))),
      );
    },
    [canvas],
  );
  async function flush() {
    if (session?.composing) throw new Error("请先完成当前输入法组合。");
    if (canvas) return canvas.flush(latest.current.draft);
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
    if (compact && (issues.length || pendingMode || !sourceReady)) {
      setError(
        issues[0] ??
          (pendingMode
            ? "正在核验模型，请稍后再试。"
            : "正在核验参考内容，请稍后再试。"),
      );
      return;
    }
    if (presentation === "canvas" && !canvasCanGenerate) return;
    if (compact) setQuoteReviewOpen(presentation !== "canvas");
    await run(() => prepareQuote(presentation === "canvas"));
  }
  async function prepareQuote(submitImmediately = false) {
    if (issues.length || pendingMode || !sourceReady)
      throw new Error(issues[0] ?? "请等待参考内容核验完成。");
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
    const owner = await discover();
    if (submitImmediately) {
      if (!alive.current || !session?.editor.isEditable) return;
      const receipt = owner.submission;
      if (
        receipt?.state !== "quoted" ||
        receipt.id !== result.submission_id ||
        receipt.operation_id !== result.operation_id ||
        receipt.confirm_key !== result.confirm_key ||
        !sameSource(pointer, receipt.input_source)
      )
        throw new RequestError("invalid_receipt");
      if (captured !== latest.current.localSeq)
        throw new Error("输入已变化，请重新生成。");
      if (!result.confirmable)
        throw new Error("当前报价不可用，请检查积分余额或调整生成参数。");
      setQuote(result);
      setQuoteSeq(captured);
      await confirmQuote({
        quote: result,
        seq: captured,
        source: pointer,
      });
      return;
    }
    if (latest.current.localSeq === captured) {
      setQuote(result);
      setQuoteSeq(captured);
    }
  }
  async function refreshQuote() {
    if (confirmUnknown) return;
    await run(async () => {
      await resolveSubmission("discard_quote");
      await prepareQuote();
    });
  }
  async function confirm() {
    await run(async () => confirmQuote());
  }
  async function reconcileConfirmation() {
    if (!confirmUnknown) return;
    await run(async () => {
      const original = current.submission;
      const recovered = await discover();
      if (recovered.submission?.state === "accepted") return;
      if (
        recovered.submission?.state !== "quoted" ||
        recovered.submission.operation_id !== original?.operation_id ||
        recovered.submission.confirm_key !== original?.confirm_key
      )
        throw new Error("原请求仍待核对，请稍后重试。");
      // Replaying the original confirmation recovers its receipt; it never creates a quote.
      await confirmQuote();
    });
  }
  async function confirmQuote(fresh?: {
    quote: Quote;
    seq: number;
    source: API.InputSource;
  }) {
    const checkedQuote = fresh?.quote ?? activeQuote;
    // An unknown confirmation must retain its original key even after expiry.
    if (!confirmUnknown) {
      if (!isUnexpiredQuote(checkedQuote?.expires_at))
        throw new Error("报价已过期，请重新检查费用。");
      if ((!fresh && !quoteAmountKnown) || checkedQuote?.confirmable !== true)
        throw new Error("当前报价不可确认，请重新检查费用。");
    }
    if (session?.pendingTarget) throw new Error("模式选择尚未完成。");
    const sub = current.submission;
    const confirmKey =
      fresh?.quote.confirm_key ?? sub?.confirm_key ?? quote?.confirm_key;
    const operation =
      fresh?.quote.operation_id ?? sub?.operation_id ?? quote?.operation_id;
    if (!operation || !confirmKey) throw new Error("请先恢复报价。");
    if (
      (fresh || quote) &&
      (fresh?.seq ?? quoteSeq) !== latest.current.localSeq
    )
      throw new Error("输入已变化，请重新报价。");
    if (!fresh && !quote && !confirmUnknown) {
      const saved = await flush();
      if (!sameSource(saved, sub?.input_source))
        throw new Error("草稿已变化，请放弃旧报价后重新检查。");
    }
    const intent = submission.current ?? {
      key: crypto.randomUUID(),
      body: { input_source: fresh?.source ?? sub?.input_source },
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
  }
  async function resolve(action: "discard_quote" | "acknowledge_result") {
    await run(() => resolveSubmission(action));
  }
  async function resolveSubmission(
    action: "discard_quote" | "acknowledge_result",
  ) {
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
  }
  const resolveKeys = useRef(new Map<string, string>());
  const reportedOperation = useRef<string | null>(null);
  const reportOperation = useEffectEvent(async (operationID: string) => {
    if (reportedOperation.current === operationID) return;
    reportedOperation.current = operationID;
    if (compact && current.submission?.state === "accepted") {
      try {
        await resolveSubmission("acknowledge_result");
      } catch (cause) {
        toast.error(
          cause instanceof Error ? cause.message : "生成输入暂时无法解锁",
        );
      }
    }
    onOperationReady?.(operationID, snapshot.draft_id);
  });
  useEffect(() => {
    if (
      task &&
      !pending &&
      (!compact || current.submission?.state === "accepted")
    )
      void reportOperation(task);
  }, [task, pending, compact, current.submission?.state]);

  async function selectMode(version: string, mode: string) {
    if (!session || !version) return;
    if (!mode) {
      setError("该模型不支持当前参考组合，请选择其他模型。");
      return;
    }
    const request = session.beginModeSelection({ modelVersion: version, mode });
    try {
      const page = await listProjectModels({
        project_id: projectID,
        capability: seed.reuse?.capability ?? seed.capability + ".generate",
        limit: 100,
      });
      let found = page.items?.find((m) => m.current_version?.id === version);
      let cursor = page.next_cursor ?? undefined;
      const seen = new Set<string>();
      while (!found && cursor && !seen.has(cursor)) {
        seen.add(cursor);
        const next = await listProjectModels({
          project_id: projectID,
          capability: seed.reuse?.capability ?? seed.capability + ".generate",
          limit: 100,
          cursor,
        });
        found = next.items?.find((m) => m.current_version?.id === version);
        cursor = next.next_cursor ?? undefined;
      }
      if (session.pendingTarget?.selectionEpoch !== request.selectionEpoch)
        return;
      // Apply fresh capability facts even when the requested mode is rejected.
      setModels((values) =>
        values.map((value) =>
          value.current_version?.id === version
            ? (found ?? {
                ...value,
                available: false,
                mode_availability: [],
                status: "disabled",
              })
            : value,
        ),
      );
      catalog.current = catalog.current.map((item) =>
        item.current_version?.id === version
          ? (found ?? { ...item, available: false, mode_availability: [] })
          : item,
      );
      if (!found || !modelAvailableForMode(found))
        throw new Error("该模型当前不可用");
      if (!modelAvailableForMode(found, mode))
        throw new Error("该模型模式已不可用");
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
  const textOnlyAudio =
    seed.capability === "audio" && draft.mode !== "omni_audio";
  const motionInput = seed.creationType === "motion";
  const promptControl = (
    <div
      hidden={motionInput}
      aria-hidden={motionInput || undefined}
      className={motionInput ? "hidden" : "contents"}
    >
      <PromptEditor
        inputGroup={compact}
        editable={!suspended && (!frozen || session?.composing === true)}
        className={
          compact
            ? "max-h-80 min-h-16 overflow-y-auto"
            : "max-h-80 min-h-24 overflow-y-auto"
        }
        placeholder={
          seed.creationType === "music"
            ? draft.mode === "music_song" &&
              draft.params.music_input === "lyrics"
              ? "填写歌词。"
              : "描述曲风、情绪与节奏。"
            : seed.creationType === "avatar"
              ? "描述表情、姿态与动作（可选）。"
              : draft.mode === "omni_audio"
                ? "描述角色、台词、音效与时间段，@ 引用音频。"
                : textOnlyAudio
                  ? "输入需要朗读的文字。"
                  : presentation === "assistant"
                    ? "描述任务，@ 引用素材"
                    : compact
                      ? "描述创意，@ 引用素材"
                      : undefined
        }
        resolveGeneration={compact ? resolveReferenceGeneration : undefined}
        contextKey={contextKey}
        replicaId={replicaId}
        draft={initialDraft}
        collaboration={canvas?.collaboration}
        onDispose={() => {
          ownerSessionCleanup.current?.();
          ownerSessionCleanup.current = undefined;
        }}
        onReady={(value) => {
          sessionRef.current = value;
          setSession(value);
          ownerSessionCleanup.current?.();
          ownerSessionCleanup.current =
            canvas?.onSession?.(value, snapshot.draft) || undefined;
          const current = value.snapshot();
          latest.current = current;
          setDraft(current.draft);
          setSeq(current.localSeq);
          publishedCanvasSeq.current = current.localSeq;
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
  );
  async function adoptDrivingAudio(id: string) {
    if (!session || frozen) throw new Error("请先完成当前提交。");
    const preview = await getMediaPreview({ pid: projectID, asset_id: id });
    const asset = preview.asset;
    if (
      !asset ||
      asset.project_id !== projectID ||
      asset.kind !== "audio" ||
      !asset.duration_ms ||
      asset.duration_ms >= 60000
    )
      throw new Error("请选择少于 60 秒的项目音频。");
    const binding = mediaBinding(asset, "driving_audio");
    const old = session
      .snapshot()
      .draft.references.find((r) => r.role === "driving_audio");
    if (old) session.replaceReference(old.id, binding);
    else session.insertReferences([binding]);
    await flush();
  }
  async function useVoice(selection: VoiceSelection) {
    if (!session || frozen) throw new Error("请先完成当前提交。");
    const mode = selection.kind === "system" ? "text2audio" : "omni_audio";
    const before = session.snapshot().draft;
    if (!before.modelVersion && before.mode !== mode) {
      // Choosing an input mode is valid before any supplier model is available.
      // The server still requires a verified release before quoting generation.
      session.adoptDraft({ ...before, mode });
    } else if (before.mode && before.mode !== mode) {
      const next = models.find((m) => modelAvailableForMode(m, mode));
      if (!next?.current_version?.id)
        throw new Error(
          mode === "omni_audio"
            ? "音色已保留。启用全能配音模型后可以引用。"
            : "音色已保留。启用文字配音模型后可以使用。",
        );
      await selectMode(next.current_version.id, mode);
      if (session.snapshot().draft.mode !== mode)
        throw new Error("请先完成配音模式切换。");
    }
    if (selection.kind === "system")
      session.updateParams({
        ...session.snapshot().draft.params,
        speaker: selection.voice.id,
      });
    else {
      const preview = await getMediaPreview({
        pid: projectID,
        asset_id: selection.voice.asset_id,
      });
      if (
        !preview.asset ||
        preview.asset.project_id !== projectID ||
        preview.asset.kind !== "audio" ||
        preview.asset.revision !== selection.voice.asset_revision
      )
        throw new Error("参考音色已变化，请重新创建或选择。");
      const current = session.snapshot().draft.references;
      if (
        current.some((r) => r.mediaKind === "image") ||
        current.filter((r) => r.mediaKind === "audio").length >= 3
      )
        throw new Error("全能配音最多引用三段音频，不能与图片混用。");
      session.insertReferences([
        mediaBinding(preview.asset, "reference_audio"),
      ]);
    }
    await flush();
    setVoiceLibraryOpen(false);
  }
  const voiceControls =
    seed.creationType === "audio" ? (
      <>
        <Button
          variant="outline"
          size="sm"
          disabled={frozen}
          onClick={() => setVoiceLibraryOpen(true)}
          ref={voiceLibraryButton}
        >
          音色库
        </Button>
        {voiceLibraryOpen && (
          <VoiceLibrary
            ownerKey={contextKey}
            key={contextKey}
            projectID={projectID}
            onClose={() => setVoiceLibraryOpen(false)}
            onSelect={useVoice}
            onRestoreFocus={() => voiceLibraryButton.current?.focus()}
          />
        )}
      </>
    ) : seed.creationType === "avatar" ? (
      <AvatarTextDrive
        key={contextKey}
        projectID={projectID}
        contextKey={contextKey}
        disabled={frozen}
        onAdopt={adoptDrivingAudio}
      />
    ) : null;
  const referenceControls =
    session && (!textOnlyAudio || presentation === "canvas") ? (
      <ReferenceControls
        session={session}
        draft={draft}
        projectID={projectID}
        model={model}
        disabled={frozen}
        authorized={sourceReady}
        compact={compact}
        canvasLayout={presentation === "canvas"}
        expanded={full}
        home={presentation === "home"}
        assistant={presentation === "assistant"}
        canvasSources={canvas?.referenceSources ?? referenceSources}
        canvasSubjectSources={canvas?.subjectSources ?? subjectSources}
        incomingSources={canvas?.incomingSources}
        onMotion={canvas?.onMotion}
      />
    ) : null;
  const collaborativeNegativeField =
    canvas?.collaboration && model
      ? (() => {
          try {
            return fieldsForMode(
              model.current_version?.param_schema,
              draft.mode ?? "",
            ).find(
              (f) =>
                f.field === "negative_prompt" && !f.enum && f.type === "string",
            );
          } catch {
            return undefined;
          }
        })()
      : undefined;
  const parameterFields = model
    ? (() => {
        try {
          return fieldsForMode(
            model.current_version?.param_schema,
            draft.mode ?? "",
          );
        } catch {
          return [];
        }
      })()
    : [];
  const durationField =
    compact && seed.capability === "video"
      ? parameterFields.find((f) => f.field === "duration_ms")
      : undefined;
  const parameterControls = model ? (
    <>
      <GenerationParameters
        fields={parameterFields.filter(
          (f) =>
            (!collaborativeNegativeField || f.field !== "negative_prompt") &&
            (!durationField || f.field !== durationField.field),
        )}
        includeOutputCount={!compact}
        params={draft.params}
        maxOutputs={Number(model.current_version?.limits?.max_outputs ?? 1)}
        disabled={frozen}
        compact={compact}
        onChange={(params) => session?.updateParams(params)}
      />
      {collaborativeNegativeField ? (
        <Field data-disabled={frozen}>
          <FieldLabel htmlFor={`${formFieldId}-negative`}>
            {collaborativeNegativeField.label}
          </FieldLabel>
          <TextIntentInput
            id={`${formFieldId}-negative`}
            disabled={frozen}
            value={String(draft.params.negative_prompt ?? "")}
            onEdit={(value, edit) =>
              session?.updateTextParam("negative_prompt", value, edit)
            }
            onHistory={(direction) =>
              canvas?.collaboration?.history[direction]()
            }
            onCompositionState={(active) =>
              canvas?.collaboration?.composition(active, "negative_prompt")
            }
          />
        </Field>
      ) : null}
    </>
  ) : null;
  const issuesPanel =
    issues.length > 0 ? (
      compact ? (
        <FeedbackToast
          message={issues.join("；")}
          action={
            model
              ? {
                  label: "检查参数",
                  onClick: () =>
                    setConversion({
                      seq,
                      params: compatibleParams(draft, model),
                    }),
                }
              : undefined
          }
        />
      ) : (
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
      )
    ) : null;
  const needsQuoteRefresh =
    quoteExpired || recoveredChanged || (quote !== null && quoteSeq !== seq);
  const quotedBudget = activeQuote?.available_micros;
  const insufficientProjectBudget =
    typeof quotedBudget === "number" &&
    Number.isSafeInteger(quotedBudget) &&
    quotedBudget >= 0 &&
    typeof activeQuote?.quote_micros === "number" &&
    quoteAmountKnown &&
    quotedBudget < activeQuote.quote_micros;
  const submissionPanel = current.submission ? (
    <div className="flex flex-col gap-4" aria-label="生成确认">
      {current.submission.state === "accepted" ? (
        <p className="text-sm text-muted-foreground">
          已提交生成，可查看任务或继续创作。
        </p>
      ) : activeQuote ? (
        <QuoteCostDetail
          amount={activeQuote.quote_micros}
          detail={activeQuote.quote_detail}
        />
      ) : null}
      {confirmUnknown && (
        <p role="status" className="text-sm text-muted-foreground">
          核对原请求并恢复结果。
        </p>
      )}
      {!confirmUnknown &&
        current.submission.state !== "accepted" &&
        activeQuote?.confirmable === false && (
          <p role="status" className="text-sm text-muted-foreground">
            {insufficientProjectBudget
              ? "项目预算不足，请调整数量或联系管理员。"
              : activeQuote.quote_detail?.wallet_block ===
                  "credits_insufficient"
                ? "适用于所选模型的积分不足，请调整生成量或联系付款管理员。"
                : activeQuote.quote_detail?.wallet_block ===
                    "commercial_price_unavailable"
                  ? "该模型的商业价格尚未发布，请选择其他模型。"
                  : activeQuote.quote_detail?.wallet_block === "payer_changed"
                    ? "项目付款账户已变更，请联系管理员。"
                    : "当前报价暂不可确认，请检查项目预算或更新费用。"}
          </p>
        )}
      {current.submission.state !== "accepted" &&
        !confirmUnknown &&
        quoteExpired && (
          <FeedbackToast message="报价已过期，请重新检查费用。" />
        )}
      {!confirmUnknown &&
        needsQuoteRefresh &&
        current.submission.state !== "accepted" && (
          <p role="status" className="text-sm text-muted-foreground">
            {quoteExpired
              ? "费用已过期，请更新后再生成。"
              : "内容已更改，请更新费用。"}
          </p>
        )}
      <div className="flex flex-wrap justify-end gap-2">
        {current.submission.state === "accepted" ? (
          <Button
            disabled={pending}
            onClick={() => void resolve("acknowledge_result")}
          >
            继续创作
          </Button>
        ) : confirmUnknown ? (
          <Button
            disabled={pending}
            onClick={() => void reconcileConfirmation()}
          >
            核对状态
          </Button>
        ) : error ? (
          <Button
            disabled={pending}
            onClick={() =>
              void run(async () => {
                await discover();
              })
            }
          >
            核对状态
          </Button>
        ) : (
          <>
            <Button
              variant="ghost"
              disabled={pending}
              onClick={() => void resolve("discard_quote")}
            >
              取消
            </Button>
            <Button
              disabled={
                pending ||
                !!pendingMode ||
                issues.length > 0 ||
                !sourceReady ||
                (!needsQuoteRefresh &&
                  (activeQuote?.confirmable !== true || !quoteAmountKnown))
              }
              onClick={() =>
                void (needsQuoteRefresh ? refreshQuote() : confirm())
              }
            >
              {needsQuoteRefresh ? "更新费用" : "确认生成"}
            </Button>
          </>
        )}
      </div>
    </div>
  ) : null;
  if (suspended && compact)
    return (
      <FeedbackToast
        message="请重新核验登录状态，本地草稿已保留"
        action={{ label: "重新核验", onClick: () => onOpen(snapshot.draft_id) }}
      />
    );
  if (suspended)
    return (
      <ContentState
        state="error"
        title="请重新核验登录状态"
        description="本地草稿已保留。"
        actions={
          <Button onClick={() => onOpen(snapshot.draft_id)}>
            重新核验并找回草稿
          </Button>
        }
      />
    );
  const modelControl = modelError ? (
    <Button
      variant="ghost"
      size="toolbar"
      aria-label="重试模型目录"
      disabled={reloadingModels || frozen}
      onClick={() => void retryModels()}
    >
      <RefreshCw data-icon="inline-start" />
      {reloadingModels ? "正在读取模型…" : "重试模型"}
    </Button>
  ) : (
    <ModelPicker
      mode={draft.mode}
      models={models}
      value={draft.modelVersion}
      disabled={frozen || !session}
      onSelect={(target) => {
        if (target.current_version?.id)
          void selectMode(
            target.current_version.id,
            referenceMode(
              {
                ...draft,
                modelVersion: target.current_version.id,
              },
              target,
            ) ?? "",
          );
      }}
    />
  );

  const toolbarParameters = (
    <GenerationToolbarParameters
      enabled={!!model}
      disabled={!session || frozen}
      params={draft.params}
      maxOutputs={Number(model?.current_version?.limits?.max_outputs ?? 1)}
      duration={durationField}
      onChange={(params) => session?.updateParams(params)}
    >
      {parameterControls}
    </GenerationToolbarParameters>
  );

  const editorLabel =
    seed.creationType === "music"
      ? "音乐创作编辑器"
      : seed.creationType === "avatar"
        ? "数字人创作编辑器"
        : seed.creationType === "motion"
          ? "动作模仿创作编辑器"
          : textOnlyAudio
            ? "音频创作编辑器"
            : "多参考创作编辑器";
  return (
    <ComposerSurface
      expanded={full}
      onExpandedChange={(open) => {
        if (!open && sessionRef.current?.composing) return;
        setFull(open);
      }}
      title={editorLabel}
    >
      <div
        role="region"
        aria-label={editorLabel}
        data-expanded={full || undefined}
        data-presentation={presentation}
        className={cn(
          "mx-auto flex w-full max-w-5xl flex-col gap-4",
          compact && "gap-2",
          full &&
            (compact ? "min-h-0 flex-1" : "max-h-full overflow-y-auto p-4"),
        )}
      >
        {canvasToolbar}
        {!compact && (
          <div className="flex flex-wrap items-center gap-2">
            <span role="status" className="text-sm text-muted-foreground">
              {canvas ? "随画布保存" : saveState} · r{revision}
            </span>
            <Button variant="ghost" onClick={() => setFull((v) => !v)}>
              {full ? "收起编辑器" : "展开编辑器"}
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
        {presentation === "assistant" ? (
          <InputGroup
            variant="composer"
            className="creation-input-surface canvas-assistant-composer"
          >
            <div
              data-slot="input-group-control"
              className="canvas-assistant-prompt"
            >
              {promptControl}
            </div>
            <ComposerToolbar
              aria-label="生成工具"
              className="canvas-assistant-tools"
            >
              {referenceControls}
              {voiceControls}
              {headerActions}
              <ComposerToolbarButton
                aria-label="引用参考"
                size="toolbar-icon"
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
              </ComposerToolbarButton>
              {modelControl}
              {toolbarParameters}
              <ComposerToolbarActions>
                {creditDisplay}
                <ComposerToolbarButton
                  className="rounded-full max-md:size-11"
                  size="toolbar-icon"
                  variant="default"
                  aria-label={
                    current.submission
                      ? current.submission.state === "accepted"
                        ? "继续创作"
                        : "查看报价"
                      : "发送消息"
                  }
                  title={
                    current.submission?.state === "accepted"
                      ? "继续创作"
                      : current.submission
                        ? "查看费用"
                        : "检查费用并发送"
                  }
                  disabled={
                    pending ||
                    confirmUnknown ||
                    (!current.submission &&
                      (!session ||
                        frozen ||
                        !draft.document.content.some((p) =>
                          p.content?.some(
                            (n) =>
                              n.type === "reference" ||
                              (n.type === "text" && n.text.trim()),
                          ),
                        )))
                  }
                  onClick={() =>
                    current.submission
                      ? current.submission.state === "accepted"
                        ? void resolve("acknowledge_result")
                        : setQuoteReviewOpen(true)
                      : void getQuote()
                  }
                >
                  {pending ? (
                    <LoaderCircle className="animate-spin" />
                  ) : (
                    <ArrowUp />
                  )}
                </ComposerToolbarButton>
              </ComposerToolbarActions>
            </ComposerToolbar>
          </InputGroup>
        ) : compact ? (
          <InputGroup
            variant="composer"
            className={cn(
              presentation === "home"
                ? "creation-input-surface creation-composer creation-composer-ready"
                : "flex-col items-stretch gap-1",
              full && "min-h-0 flex-1",
            )}
          >
            <div
              className={cn(
                presentation === "home" ? "creation-ready-body" : "min-w-0",
                full &&
                  presentation !== "home" &&
                  "flex min-h-0 flex-1 flex-col",
              )}
            >
              <InputGroupAddon
                align="block-start"
                className={
                  presentation === "home"
                    ? "creation-reference-tools"
                    : "items-start px-4 pt-4"
                }
              >
                {referenceControls}
                {voiceControls}
                {presentation !== "home" && headerActions}
                {presentation !== "home" && (
                  <ComposerToolbarButton
                    aria-label={full ? "收起编辑器" : "展开编辑器"}
                    size="toolbar-icon"
                    className="ml-auto"
                    onClick={() => setFull((value) => !value)}
                  >
                    {full ? <Minimize2 /> : <Maximize2 />}
                  </ComposerToolbarButton>
                )}
              </InputGroupAddon>
              <div
                data-slot="input-group-control"
                className={cn(
                  presentation === "home"
                    ? "creation-ready-prompt"
                    : "min-h-20 w-full flex-1 px-4 py-2 [&_.tiptap]:min-h-16 [&_.tiptap]:outline-none",
                  full &&
                    "min-h-0 flex-1 overflow-y-auto [&_.tiptap]:max-h-none [&_.tiptap]:min-h-full [&_.tiptap]:overflow-visible",
                )}
              >
                {promptControl}
              </div>
            </div>
            <ComposerToolbar
              aria-label="生成工具"
              className={
                presentation === "home"
                  ? "creation-tools"
                  : "flex-wrap gap-1 px-3 pt-1 pb-3"
              }
            >
              {presentation === "home" && (
                <DropdownMenu>
                  <DropdownMenuTrigger asChild>
                    <ComposerToolbarButton
                      aria-label="更多工具"
                      size="toolbar-icon"
                      disabled={pending || frozen}
                    >
                      <MoreHorizontal />
                    </ComposerToolbarButton>
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
              {presentation === "home" && (
                <ComposerToolbarButton
                  aria-label={full ? "收起编辑器" : "展开编辑器"}
                  size="toolbar-icon"
                  onClick={() => setFull((value) => !value)}
                >
                  {full ? <Minimize2 /> : <Maximize2 />}
                </ComposerToolbarButton>
              )}
              <div
                className={
                  presentation === "home" ? "creation-option-tools" : "contents"
                }
              >
                <div
                  className={
                    presentation === "home"
                      ? "creation-primary-tools"
                      : "contents"
                  }
                >
                  {presentation === "home" && headerActions}
                  {modelControl}
                </div>
                {toolbarParameters}
                {seed.creationType === "music" && model ? (
                  <Select
                    value={draft.mode ?? "music_song"}
                    disabled={frozen || !session}
                    onValueChange={(mode) =>
                      selectMode(draft.modelVersion!, mode)
                    }
                  >
                    <ComposerSelectTrigger
                      aria-label="音乐模式"
                      className="w-auto"
                    >
                      <SelectValue />
                    </ComposerSelectTrigger>
                    <SelectContent>
                      <SelectGroup>
                        {availableModelModes(model).map((mode) => (
                          <SelectItem key={mode} value={mode}>
                            {creationModeLabel(mode)}
                          </SelectItem>
                        ))}
                      </SelectGroup>
                    </SelectContent>
                  </Select>
                ) : seed.creationType === "music" ? (
                  <span className="px-2 text-xs text-muted-foreground">
                    {creationModeLabel(draft.mode ?? "music_song")}
                  </span>
                ) : seed.creationType === "audio" ? (
                  <span className="px-2 text-xs text-muted-foreground">
                    {creationModeLabel(draft.mode ?? "text2audio")}
                  </span>
                ) : (
                  presentation !== "home" &&
                  draft.mode && (
                    <Badge variant="secondary">
                      {creationModeLabel(draft.mode)}
                    </Badge>
                  )
                )}
                {!textOnlyAudio &&
                  !["avatar", "motion"].includes(seed.creationType ?? "") && (
                    <ComposerToolbarButton
                      aria-label="提及素材或主体"
                      size="toolbar-icon"
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
                    </ComposerToolbarButton>
                  )}
                {presentation === "home" && projectControl}
              </div>
              <ComposerToolbarActions>
                {creditDisplay}
                {current.submission ? (
                  <ComposerToolbarButton
                    size="toolbar"
                    className="shrink-0"
                    variant="default"
                    disabled={pending}
                    onClick={() =>
                      current.submission?.state === "accepted"
                        ? void resolve("acknowledge_result")
                        : setQuoteReviewOpen(true)
                    }
                  >
                    {current.submission.state === "accepted"
                      ? "继续创作"
                      : presentation === "canvas"
                        ? "确认生成"
                        : "查看报价"}
                  </ComposerToolbarButton>
                ) : (
                  <ComposerToolbarButton
                    className="rounded-full max-md:size-11"
                    size="toolbar-icon"
                    variant="default"
                    aria-label={presentation === "canvas" ? "生成" : "检查费用"}
                    title={
                      presentation === "canvas" ? "生成" : "检查费用并继续"
                    }
                    disabled={
                      pending ||
                      confirmUnknown ||
                      (presentation === "canvas" && !canvasCanGenerate)
                    }
                    onClick={() => void getQuote()}
                  >
                    {pending ? (
                      <LoaderCircle className="animate-spin" />
                    ) : (
                      <ArrowUp className="size-5" />
                    )}
                  </ComposerToolbarButton>
                )}
              </ComposerToolbarActions>
            </ComposerToolbar>
          </InputGroup>
        ) : (
          <>
            <div className="grid gap-3 sm:grid-cols-2">
              <Field data-disabled={frozen || !session}>
                <FieldLabel htmlFor={`${formFieldId}-field-1`}>
                  <span>生成模型</span>
                </FieldLabel>
                <Select
                  disabled={frozen || !session}
                  value={draft.modelVersion ?? ""}
                  onValueChange={(selectedValue) => {
                    const m = models.find(
                      (m) => m.current_version?.id === selectedValue,
                    );
                    if (m?.current_version?.id)
                      selectMode(
                        m.current_version.id,
                        availableModelModes(m)[0] ?? "",
                      );
                  }}
                >
                  <SelectTrigger
                    id={`${formFieldId}-field-1`}
                    aria-label="生成模型"
                    className="w-full"
                  >
                    <SelectValue placeholder={"请选择模型"} />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectGroup>
                      {models
                        .filter((m) => modelAvailableForMode(m, draft.mode))
                        .map((m) =>
                          m.current_version?.id ? (
                            <SelectItem
                              key={m.id}
                              value={m.current_version?.id}
                              disabled={!modelAvailableForMode(m, draft.mode)}
                            >
                              {m.display_name}
                              {!modelAvailableForMode(m, draft.mode)
                                ? " · 当前不可用"
                                : ""}
                            </SelectItem>
                          ) : null,
                        )}
                    </SelectGroup>
                  </SelectContent>
                </Select>
              </Field>
              <Field data-disabled={frozen || !model || !session}>
                <FieldLabel htmlFor={`${formFieldId}-field-2`}>
                  <span>生成模式</span>
                </FieldLabel>
                <Select
                  disabled={frozen || !model || !session}
                  value={draft.mode ?? ""}
                  onValueChange={(selectedValue) =>
                    selectMode(draft.modelVersion!, selectedValue)
                  }
                >
                  <SelectTrigger
                    id={`${formFieldId}-field-2`}
                    aria-label="生成模式"
                    className="w-full"
                  >
                    <SelectValue placeholder={"请选择模式"}>
                      {draft.mode ? creationModeLabel(draft.mode) : undefined}
                    </SelectValue>
                  </SelectTrigger>
                  <SelectContent>
                    <SelectGroup>
                      {availableModelModes(model).map((m) => (
                        <SelectItem key={m} value={m}>
                          {creationModeLabel(m)}
                        </SelectItem>
                      ))}
                    </SelectGroup>
                  </SelectContent>
                </Select>
              </Field>
            </div>
            <InputGroup
              variant="composer"
              className="h-auto min-h-32 p-3 [&_.tiptap]:min-h-24"
            >
              {promptControl}
            </InputGroup>
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
                {voiceControls}
              </>
            )}
            {parameterControls}
          </>
        )}
        {modelError && (
          <FeedbackToast
            message="模型暂不可用"
            attempt={modelRetryAttempt}
            action={{ label: "重试", onClick: () => void retryModels() }}
          />
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
            open={
              quoteReviewOpen &&
              !!current.submission &&
              current.submission.state !== "accepted"
            }
            onOpenChange={(open) => {
              if (!pending && !confirmUnknown) setQuoteReviewOpen(open);
            }}
          >
            <DialogContent
              {...(presentation === "canvas"
                ? { "aria-describedby": undefined }
                : {})}
              data-canvas-no-zoom={presentation === "assistant" || undefined}
              data-canvas-shortcuts-ignore={
                presentation === "assistant" || undefined
              }
              showCloseButton={
                presentation !== "canvas" && !pending && !confirmUnknown
              }
              className="max-h-[85dvh] overflow-y-auto sm:max-w-[480px]"
            >
              <DialogTitle>确认生成</DialogTitle>
              {presentation === "canvas" ? (
                <DialogFooter>
                  <Button
                    variant="ghost"
                    disabled={pending || confirmUnknown}
                    onClick={() => void resolve("discard_quote")}
                  >
                    取消
                  </Button>
                  <Button
                    disabled={
                      pending ||
                      (!confirmUnknown &&
                        (frozen ||
                          !sourceReady ||
                          !!pendingMode ||
                          issues.length > 0 ||
                          (!needsQuoteRefresh &&
                            (!quoteAmountKnown ||
                              activeQuote?.confirmable !== true))))
                    }
                    onClick={() => {
                      if (confirmUnknown) void reconcileConfirmation();
                      else if (needsQuoteRefresh)
                        void run(async () => {
                          await resolveSubmission("discard_quote");
                          await prepareQuote(true);
                        });
                      else void confirm();
                    }}
                  >
                    确认
                  </Button>
                </DialogFooter>
              ) : (
                <>
                  <DialogDescription>
                    确认本次消耗后开始生成。
                  </DialogDescription>
                  {submissionPanel}
                </>
              )}
            </DialogContent>
          </Dialog>
        ) : (
          submissionPanel
        )}
        {!sourceReady && compact && sourceCheck?.key === sourceKey && (
          <FeedbackToast
            message="参考暂时无法核验"
            action={{
              label: "重新核验",
              onClick: () => setSourceEpoch((epoch) => epoch + 1),
            }}
          />
        )}
        {!sourceReady && !compact && (
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
          <div className="flex items-center justify-end gap-2">
            {creditDisplay}
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
          </div>
        )}
        {task && presentation !== "canvas" && presentation !== "assistant" && (
          <TaskDetail
            projectID={projectID}
            id={task}
            presentation={compact ? "inline" : "default"}
            onAdopt={onAdopt}
            adoptLabel={adoptLabel}
            onAdoptText={onAdoptText}
          />
        )}
      </div>
    </ComposerSurface>
  );
}

function isUnexpiredQuote(expiresAt?: string) {
  return Date.parse(expiresAt ?? "") > Date.now();
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
