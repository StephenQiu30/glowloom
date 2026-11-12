"use client";

import { modelAvailableForMode } from "@/components/generation/model-availability";
import {
  useCallback,
  useEffect,
  useEffectEvent,
  useRef,
  useState,
} from "react";
import {
  createAgentSession,
  createAgentRun,
  getAgentSession,
  getAgentRun,
  listAgentRuns,
  getAgentSubmission,
  getCurrentAgentSubmission,
  getAgentControlStatus,
  commandAgentRun,
  listAgentSkills,
  listAgentMessages,
  getAgentMessage,
} from "@/api/agent";
import { listProjectModels } from "@/api/models";
import { RequestError, type ServerEvent } from "@/lib/request";
import { readErrorMessage } from "@/lib/read-error";
import { canonical } from "@/components/generation/draft-save";
import {
  emptyPromptDraft,
  validatePromptDraft,
  type PromptDraft,
} from "@/components/generation/prompt-document";
import {
  useConversationContext,
  type ConversationSettings,
} from "./conversation-context";
import {
  budgetMicros,
  canSubmitConversation,
  draftText,
  mergeMessages,
  mergeRun,
  readMessageParts,
  terminalStatuses,
  type ConversationRun,
} from "./conversation-state";
import { useConversationEvents } from "./use-conversation-events";
export interface ConversationOptions {
  ownerKey: string;
  projectID: string;
  canvasID?: string;
  sessionID?: string;
  enabled: boolean;
  initialDraft?: PromptDraft;
  goalContract?: API.GoalContract;
  onDraftChange?: (draft: PromptDraft) => void;
  onAccepted?: (sessionID: string) => void;
}
export type ConversationController = ReturnType<typeof useConversation>;
type Summary = API.RunPage["items"][number];
type Phase = "idle" | "submitting" | "unknown";
interface View {
  identity: string;
  session: API.SessionView | null;
  sessionID: string | null;
  summaries: Summary[];
  runs: Record<string, ConversationRun>;
  models: API.ModelResponse[];
  skills: API.SkillVersion[];
  activeRunID: string | null;
  control: API.ControlStatus | null;
  loading: boolean;
  error: string;
  phase: Phase;
  connection: "connected" | "recovering" | "offline";
  unresolved: string[];
  olderRuns?: string;
  denied: boolean;
}
interface PendingInput {
  identity: string;
  sessionKey: string;
  requestKey: string;
  sessionID?: string;
  draft: PromptDraft;
  quote?: API.QuoteRef;
  body: API.StartRequest;
  title: string;
  sent: boolean;
}
const defaultSettings = (): ConversationSettings => ({
  budget: "1",
  skills: [],
});
function initialView(identity: string, sessionID?: string): View {
  return {
    identity,
    sessionID: sessionID ?? null,
    session: null,
    summaries: [],
    runs: {},
    models: [],
    skills: [],
    activeRunID: null,
    control: null,
    loading: true,
    error: "",
    phase: "idle",
    connection: "connected",
    unresolved: [],
    denied: false,
  };
}
function message(error: unknown) {
  if (error instanceof RequestError) {
    if (error.code === "conversation_reference_unavailable")
      return "参考暂不可用，请检查上传、访问权限和模型支持的格式。";
    if (error.code === "agent_authorization_blocked")
      return "原任务尚有结果待核对，请先查看原任务。";
    if (error.code === "agent_budget_exceeded")
      return "当前预算不足，请核对预算后再操作。";
    if (error.code.includes("conflict"))
      return "会话已发生变化，请刷新后核对原记录。";
  }
  return readErrorMessage(error);
}
function denied(error: unknown) {
  return (
    error instanceof RequestError &&
    (error.status === 403 || error.status === 404)
  );
}
function definiteRejection(error: unknown) {
  return (
    error instanceof RequestError &&
    error.status >= 400 &&
    error.status < 500 &&
    error.status !== 408 &&
    error.status !== 429
  );
}
function requireRun(value: API.RunSnapshot, sid: string) {
  if (
    !value ||
    value.session_id !== sid ||
    !value.root_run_id ||
    !Number.isSafeInteger(value.state?.execution_revision) ||
    !value.messages_page ||
    !Array.isArray(value.messages_page.items) ||
    !/^\d+:\d+$/.test(value.snapshot_cursor)
  )
    throw new Error("conversation_contract_unavailable");
  return value;
}

export function useConversation({
  ownerKey,
  projectID,
  canvasID,
  sessionID,
  enabled,
  initialDraft,
  goalContract,
  onDraftChange,
  onAccepted,
}: ConversationOptions) {
  const memory = useConversationContext();
  const identity = `${ownerKey}/${projectID}/${sessionID ?? "new"}`;
  const draftKey = `${projectID}/${sessionID ?? "new"}`;
  const [view, setView] = useState(() => initialView(identity, sessionID));
  const state =
    view.identity === identity ? view : initialView(identity, sessionID);
  const currentRunForRecovery = useEffectEvent(() => state.activeRunID);
  const [localDraft, setLocalDraft] = useState(() => ({
    identity,
    draft: initialDraft ?? memory?.readDraft(draftKey) ?? emptyPromptDraft(),
  }));
  const controlled = !!onDraftChange && !!initialDraft;
  const draft = controlled
    ? initialDraft!
    : localDraft.identity === identity
      ? localDraft.draft
      : emptyPromptDraft();
  const latestDraft = useRef(draft);
  useEffect(() => {
    latestDraft.current = draft;
  }, [draft]);
  const [settingsRecord, setSettingsRecord] = useState(() => ({
    identity,
    settings: memory?.readSettings(projectID) ?? defaultSettings(),
  }));
  const settings =
    settingsRecord.identity === identity
      ? settingsRecord.settings
      : defaultSettings();
  const [quote, setQuote] = useState<{
    identity: string;
    ref: API.QuoteRef;
    text: string;
  } | null>(null);
  const [refreshKey, setRefreshKey] = useState(0);
  const signal = useRef<AbortSignal | null>(null),
    activeIdentity = useRef(identity),
    sid = useRef<string | null>(sessionID ?? null);
  const pending = useRef<PendingInput | null>(null),
    flight = useRef(false);
  const runRequests = useRef(new Map<string, Promise<API.RunSnapshot>>());
  const commandKeys = useRef(
    new Map<
      string,
      {
        request_id: string;
        action: API.DecisionInput["action"];
        expected_authorization_revision: number;
        proposal_id?: string;
        candidate_hash?: string;
      }
    >(),
  );
  const model = settings.modelKey
    ? state.models.find((item) => item.key === settings.modelKey)
    : state.models[0];
  const activeRun = state.activeRunID
    ? state.runs[state.activeRunID]
    : undefined;
  const stopping =
    state.control?.current_decision?.root_run_id === state.activeRunID &&
    state.control.current_decision.action === "stop" &&
    state.control.delivery_status !== "applied";
  const active =
    (!!state.activeRunID &&
      (!activeRun || !terminalStatuses.has(activeRun.snapshot.state.status))) ||
    stopping;
  const allowed = canSubmitConversation({
    draft,
    planningModel: model,
    authorized: enabled && !!projectID && !state.denied,
    budget: settings.budget,
    busy:
      state.loading ||
      state.phase !== "idle" ||
      state.unresolved.length > 0 ||
      (!!state.sessionID && !state.session),
    active,
  });
  const current = () =>
    activeIdentity.current === identity && !signal.current?.aborted;
  const patch = useCallback(
    (change: (old: View) => View) =>
      setView((old) => (old.identity === identity ? change(old) : old)),
    [identity],
  );
  function changeDraft(value: PromptDraft) {
    latestDraft.current = value;
    if (controlled) onDraftChange!(value);
    else setLocalDraft({ identity, draft: value });
    memory?.saveDraft(draftKey, value);
  }
  function changeSettings(value: ConversationSettings) {
    setSettingsRecord({ identity, settings: value });
    memory?.saveSettings(projectID, value);
  }
  const applyRun = useCallback(
    (value: API.RunSnapshot, resetHistory = false) => {
      const run = requireRun(value, sid.current ?? "");
      patch((old) => {
        const found = old.summaries.find(
          (item) => item.root_run_id === run.root_run_id,
        );
        const summary: Summary = {
          root_run_id: run.root_run_id,
          session_id: run.session_id,
          accepted_revision:
            found?.accepted_revision ?? run.state.execution_revision,
          created_at: found?.created_at ?? new Date().toISOString(),
          status: run.state.status,
          input_excerpt: run.input?.plain_text ?? run.goal,
          ...(run.retry_of_run_id
            ? { retry_of_run_id: run.retry_of_run_id }
            : {}),
        };
        return {
          ...old,
          runs: {
            ...old.runs,
            [run.root_run_id]: mergeRun(
              old.runs[run.root_run_id],
              run,
              resetHistory,
            ),
          },
          summaries: [
            ...old.summaries.filter(
              (item) => item.root_run_id !== run.root_run_id,
            ),
            summary,
          ].sort((a, b) => a.accepted_revision - b.accepted_revision),
        };
      });
    },
    [patch],
  );
  async function loadRun(runID: string) {
    const session = sid.current;
    if (!session) throw new Error("session_required");
    const key = `${identity}/${runID}`;
    const existing = runRequests.current.get(key);
    if (existing) return existing;
    const operation = getAgentRun(
      { pid: projectID, sid: session, rid: runID },
      { signal: signal.current ?? undefined },
    )
      .then((value) => {
        if (current()) applyRun(value);
        return value;
      })
      .finally(() => runRequests.current.delete(key));
    runRequests.current.set(key, operation);
    return operation;
  }
  useEffect(() => {
    const controller = new AbortController();
    const sameScope = activeIdentity.current === identity;
    const lookupSession = sessionID ?? (sameScope ? sid.current : null);
    signal.current = controller.signal;
    activeIdentity.current = identity;
    sid.current = lookupSession;
    if (!sameScope) {
      pending.current = null;
      commandKeys.current.clear();
      flight.current = false;
    }
    setView((old) =>
      old.identity === identity
        ? {
            ...old,
            loading: true,
            error: "",
            phase: pending.current ? "unknown" : old.phase,
          }
        : initialView(identity, sessionID),
    );
    if (!enabled || !projectID) {
      setView({ ...initialView(identity, sessionID), loading: false });
      return () => controller.abort();
    }
    const options = { signal: controller.signal };
    void Promise.allSettled([
      listProjectModels(
        {
          project_id: projectID,
          capability: "text.generate",
          mode: "text_to_text",
          limit: 200,
        },
        options,
      ),
      listAgentSkills({ pid: projectID }, options),
      lookupSession
        ? Promise.all([
            getAgentSession({ pid: projectID, sid: lookupSession }, options),
            listAgentRuns({ pid: projectID, sid: lookupSession }, options),
            getCurrentAgentSubmission(
              { pid: projectID, sid: lookupSession },
              options,
            ),
          ])
        : Promise.resolve(null),
    ])
      .then(async ([catalog, skills, history]) => {
        if (controller.signal.aborted) return;
        patch((old) => ({
          ...old,
          models:
            catalog.status === "fulfilled"
              ? (catalog.value.items ?? []).filter(
                  (item) =>
                    item.capability === "text.generate" &&
                    modelAvailableForMode(item, "text_to_text"),
                )
              : [],
          skills: skills.status === "fulfilled" ? skills.value : [],
          error: catalog.status === "rejected" ? message(catalog.reason) : "",
          loading: false,
        }));
        if (history.status === "rejected") {
          const inaccessible = denied(history.reason);
          if (inaccessible) setQuote(null);
          patch((old) =>
            inaccessible
              ? {
                  ...old,
                  error: message(history.reason),
                  denied: true,
                  summaries: [],
                  runs: {},
                  session: null,
                  control: null,
                  activeRunID: null,
                }
              : {
                  ...old,
                  error: message(history.reason),
                  connection: "offline",
                },
          );
          // Go decisions remain independently readable when execution is offline.
          const knownRun = currentRunForRecovery();
          if (!inaccessible && sameScope && lookupSession && knownRun) {
            try {
              const control = await getAgentControlStatus(
                { pid: projectID, sid: lookupSession, rid: knownRun },
                options,
              );
              if (!controller.signal.aborted)
                patch((old) => ({ ...old, control }));
            } catch {
              /* Keep the last acknowledged business facts. */
            }
          }
          return;
        }
        if (!history.value) return;
        const [session, page, submission] = history.value;
        if (session.id !== lookupSession || session.project_id !== projectID)
          throw new Error("session_scope_mismatch");
        sid.current = session.id;
        patch((old) => ({
          ...old,
          session,
          sessionID: session.id,
          summaries: page.items,
          olderRuns: page.next_cursor,
          activeRunID:
            session.run?.root_run_id ??
            submission.active_run?.root_run_id ??
            null,
          unresolved: submission.unresolved_run_ids ?? [],
        }));
        if (session.run) {
          applyRun(session.run);
          try {
            const control = await getAgentControlStatus(
              { pid: projectID, sid: session.id, rid: session.run.root_run_id },
              options,
            );
            if (!controller.signal.aborted)
              patch((old) => ({ ...old, control }));
          } catch {
            /* Agent read remains usable when the business receipt is still synchronizing. */
          }
        }
      })
      .catch((error) => {
        if (!controller.signal.aborted)
          patch((old) => ({ ...old, error: message(error), loading: false }));
      });
    return () => controller.abort();
  }, [identity, projectID, sessionID, enabled, refreshKey, patch, applyRun]);

  const receiveEvent = useCallback(
    (event: ServerEvent) => {
      const data = event.data;
      if (!data || typeof data !== "object") return;
      patch((old) => {
        const id = old.activeRunID;
        if (
          !id ||
          !(id in old.runs) ||
          !("root_run_id" in data) ||
          data.root_run_id !== id
        )
          return old;
        const run = old.runs[id]!;
        if (
          event.event === "agent.state" &&
          "execution_revision" in data &&
          typeof data.execution_revision === "number" &&
          Number.isSafeInteger(data.execution_revision) &&
          data.execution_revision >= run.snapshot.state.execution_revision &&
          "status" in data &&
          typeof data.status === "string"
        )
          return {
            ...old,
            runs: {
              ...old.runs,
              [id]: {
                ...run,
                snapshot: {
                  ...run.snapshot,
                  state: data as API.RunSnapshot["state"],
                },
              },
            },
          };
        const parts =
          "message_parts" in data
            ? readMessageParts(data.message_parts, id)
            : [];
        return parts.length
          ? {
              ...old,
              runs: {
                ...old.runs,
                [id]: { ...run, messages: mergeMessages(run.messages, parts) },
              },
            }
          : old;
      });
    },
    [patch],
  );
  const connection = useCallback(
    (value: View["connection"]) =>
      patch((old) => ({ ...old, connection: value })),
    [patch],
  );
  useConversationEvents({
    identity,
    projectID,
    sessionID: state.sessionID,
    run: activeRun?.snapshot,
    refreshKey,
    onSnapshot: applyRun,
    onEvent: receiveEvent,
    onConnection: connection,
  });

  async function accepted(command: PendingInput, receipt: API.AcceptedRun) {
    if (!current()) return;
    if (!receipt.root_run_id || receipt.session_id !== command.sessionID)
      throw new Error("invalid_admission_receipt");
    pending.current = null;
    patch((old) => ({
      ...old,
      phase: "idle",
      sessionID: command.sessionID!,
      activeRunID: receipt.root_run_id,
      error: "",
      control: null,
    }));
    if (!command.body.retry_of_run_id) {
      if (canonical(latestDraft.current) === canonical(command.draft))
        changeDraft(emptyPromptDraft());
      setQuote(null);
    }
    const remaining =
      !command.body.retry_of_run_id &&
      canonical(latestDraft.current) === canonical(command.draft)
        ? emptyPromptDraft()
        : latestDraft.current;
    memory?.saveDraft(`${projectID}/${command.sessionID!}`, remaining);
    if (onAccepted && draftKey.endsWith("/new"))
      memory?.saveDraft(draftKey, emptyPromptDraft());
    memory?.changed();
    onAccepted?.(command.sessionID!);
    try {
      await loadRun(receipt.root_run_id);
    } catch {
      if (current())
        patch((old) => ({
          ...old,
          error: "需求已受理，正在等待消息同步。请重试连接以读取原任务。",
          connection: "offline",
        }));
    }
  }
  async function submitPending(command: PendingInput, allowReplay: boolean) {
    if (!current() || command.identity !== identity) return;
    if (!command.sessionID) {
      const session = await createAgentSession(
        { pid: projectID },
        {
          request_id: command.sessionKey,
          title: command.title,
          ...(command.body.canvas_id
            ? { canvas_id: command.body.canvas_id }
            : {}),
        },
        { signal: signal.current ?? undefined },
      );
      if (!current()) return;
      if (!session.id || session.project_id !== projectID)
        throw new Error("invalid_session_receipt");
      command.sessionID = session.id;
      sid.current = session.id;
      command.body.expected_execution_revision = session.execution_revision;
      patch((old) => ({ ...old, session, sessionID: session.id }));
    }
    if (command.sent) {
      try {
        const receipt = await getAgentSubmission(
          {
            pid: projectID,
            sid: command.sessionID,
            requestId: command.requestKey,
          },
          { signal: signal.current ?? undefined },
        );
        await accepted(command, receipt);
        return;
      } catch (error) {
        if (!(error instanceof RequestError && error.status === 404))
          throw error;
        if (!allowReplay) {
          patch((old) => ({
            ...old,
            phase: "unknown",
            error: "尚未找到受理回执。可核对并重试原发送，不会创建新的尝试。",
          }));
          return;
        }
      }
    }
    command.sent = true;
    const receipt = await createAgentRun(
      { pid: projectID, sid: command.sessionID },
      command.body,
      { signal: signal.current ?? undefined },
    );
    await accepted(command, receipt);
  }
  async function send() {
    if (flight.current || !current()) return;
    let command = pending.current;
    if (!command) {
      if (!allowed.allowed || !model) return;
      const frozen = structuredClone(draft),
        requestKey = crypto.randomUUID();
      const frozenCanvasID = state.session ? state.session.canvas_id : canvasID;
      const selectedQuote =
        quote?.identity === identity ? quote.ref : undefined;
      command = {
        identity,
        sessionKey: crypto.randomUUID(),
        requestKey,
        ...(sid.current ? { sessionID: sid.current } : {}),
        draft: frozen,
        ...(selectedQuote ? { quote: selectedQuote } : {}),
        title:
          Array.from(draftText(frozen).trim()).slice(0, 60).join("") ||
          "图片创作",
        sent: false,
        body: {
          request_id: requestKey,
          ...(frozenCanvasID ? { canvas_id: frozenCanvasID } : {}),
          ...(goalContract
            ? { goal_contract: structuredClone(goalContract) }
            : {}),
          input: frozen,
          expected_execution_revision:
            activeRun?.snapshot.state.execution_revision ??
            state.session?.execution_revision ??
            0,
          budget_micros: budgetMicros(settings.budget)!,
          manifest: {
            model_key: model.key,
            max_micros: budgetMicros(settings.budget)!,
            params: {},
            skills: settings.skills,
          },
          ...(selectedQuote ? { quoted_from: selectedQuote } : {}),
        },
      };
      pending.current = command;
    }
    flight.current = true;
    patch((old) => ({ ...old, phase: "submitting", error: "" }));
    try {
      await submitPending(command, true);
    } catch (error) {
      if (!current()) return;
      if (definiteRejection(error)) {
        pending.current = null;
        patch((old) => ({
          ...old,
          phase: "idle",
          error: message(error),
          ...(error instanceof RequestError &&
          error.code === "agent_authorization_blocked"
            ? { unresolved: ["original"] }
            : {}),
        }));
      } else {
        patch((old) => ({
          ...old,
          phase: "unknown",
          error: "发送回执暂未确认，正在核对原请求。",
        }));
        try {
          await submitPending(command, false);
        } catch {
          if (current())
            patch((old) => ({
              ...old,
              phase: "unknown",
              error: "连接暂时中断，请核对原发送。原输入仍保留。",
            }));
        }
      }
    } finally {
      flight.current = false;
    }
  }
  async function loadOlderRuns() {
    if (!sid.current || !state.olderRuns) return;
    try {
      const page = await listAgentRuns(
        { pid: projectID, sid: sid.current, cursor: state.olderRuns },
        { signal: signal.current ?? undefined },
      );
      if (current())
        patch((old) => ({
          ...old,
          summaries: [
            ...new Map(
              [...page.items, ...old.summaries].map((item) => [
                item.root_run_id,
                item,
              ]),
            ).values(),
          ].sort((a, b) => a.accepted_revision - b.accepted_revision),
          olderRuns: page.next_cursor,
        }));
    } catch (error) {
      if (current()) patch((old) => ({ ...old, error: message(error) }));
    }
  }
  async function loadOlderMessages(runID: string) {
    const older = state.runs[runID]?.older;
    if (!sid.current || !older) return;
    try {
      const page = await listAgentMessages(
        {
          pid: projectID,
          sid: sid.current,
          rid: runID,
          before: older.before,
          through: older.through,
        },
        { signal: signal.current ?? undefined },
      );
      if (current())
        patch((old) => {
          const run = old.runs[runID];
          if (!run) return old;
          const rest = { ...run };
          delete rest.older;
          return {
            ...old,
            runs: {
              ...old.runs,
              [runID]: {
                ...rest,
                messages: mergeMessages(run.messages, page.items),
                ...(page.next_cursor
                  ? {
                      older: {
                        before: page.next_cursor,
                        through: page.through,
                      },
                    }
                  : {}),
              },
            },
          };
        });
    } catch (error) {
      if (
        error instanceof RequestError &&
        error.status === 410 &&
        sid.current &&
        current()
      ) {
        try {
          const snapshot = await getAgentRun(
            { pid: projectID, sid: sid.current, rid: runID },
            { signal: signal.current ?? undefined },
          );
          if (current()) applyRun(requireRun(snapshot, sid.current), true);
          return;
        } catch (reloadError) {
          error = reloadError;
        }
      }
      if (current()) patch((old) => ({ ...old, error: message(error) }));
    }
  }
  async function quoteMessage(runID: string, messageID: string) {
    if (!sid.current) return;
    try {
      const value = await getAgentMessage(
        { pid: projectID, sid: sid.current, rid: runID, messageId: messageID },
        { signal: signal.current ?? undefined },
      );
      if (current())
        setQuote({
          identity,
          ref: { run_id: runID, message_id: messageID },
          text: value.content,
        });
    } catch (error) {
      if (current()) patch((old) => ({ ...old, error: message(error) }));
    }
  }
  async function command(action: API.DecisionInput["action"]) {
    const runID = state.activeRunID,
      session = sid.current;
    if (!runID || !session || flight.current || !current()) return;
    flight.current = true;
    let requested: API.DecisionInput | undefined;
    try {
      const status = await getAgentControlStatus(
        { pid: projectID, sid: session, rid: runID },
        { signal: signal.current ?? undefined },
      );
      if (!current()) return;
      if (action === "stop" && status.current_decision?.action === "stop") {
        patch((old) => ({ ...old, control: status, error: "" }));
        await loadRun(runID);
        return;
      }
      const revision =
        (status as API.ControlStatus & { authorization_revision?: number })
          .authorization_revision ??
        status.current_decision?.authorization_revision ??
        activeRun?.snapshot.state.authorization_revision ??
        1;
      const key = `${runID}/${action}/${revision}`;
      let body = commandKeys.current.get(key);
      if (!body) {
        const proposal = activeRun?.snapshot.state.proposal;
        body = {
          request_id: crypto.randomUUID(),
          action,
          expected_authorization_revision: revision,
          ...((action === "approve" || action === "reject") && proposal
            ? {
                proposal_id: proposal.proposal_id,
                candidate_hash: proposal.candidate_hash,
              }
            : {}),
        };
        commandKeys.current.set(key, body);
      }
      requested = body;
      await commandAgentRun(
        { pid: projectID, sid: session, rid: runID },
        body,
        { signal: signal.current ?? undefined },
      );
      const control = await getAgentControlStatus(
        {
          pid: projectID,
          sid: session,
          rid: runID,
          request_id: body.request_id,
        },
        { signal: signal.current ?? undefined },
      );
      if (current()) patch((old) => ({ ...old, control, error: "" }));
      await loadRun(runID);
    } catch (error) {
      if (requested && !definiteRejection(error) && current()) {
        try {
          const control = await getAgentControlStatus(
            {
              pid: projectID,
              sid: session,
              rid: runID,
              request_id: requested.request_id,
            },
            { signal: signal.current ?? undefined },
          );
          if (current())
            patch((old) => ({
              ...old,
              control,
              error: control.current_decision
                ? ""
                : "控制回执暂未确认，请核对原任务。",
            }));
          await loadRun(runID);
          return;
        } catch {
          /* Keep the same command identity for an explicit retry. */
        }
      }
      if (current()) patch((old) => ({ ...old, error: message(error) }));
    } finally {
      flight.current = false;
    }
  }
  useEffect(() => {
    const control = state.control,
      runID = state.activeRunID,
      session = state.sessionID;
    if (
      !control?.current_decision ||
      control.delivery_status !== "pending" ||
      !runID ||
      !session
    )
      return;
    const controller = new AbortController();
    let timer: ReturnType<typeof setTimeout> | undefined,
      attempts = 0;
    const poll = async () => {
      try {
        const status = await getAgentControlStatus(
          { pid: projectID, sid: session, rid: runID },
          { signal: controller.signal },
        );
        if (controller.signal.aborted) return;
        patch((old) => ({ ...old, control: status }));
        await loadRun(runID);
        if (
          status.delivery_status === "applied" ||
          status.delivery_status === "superseded"
        )
          return;
      } catch {
        if (controller.signal.aborted) return;
      }
      if (++attempts < 30) timer = setTimeout(() => void poll(), 1000);
      else
        patch((old) => ({
          ...old,
          error: "控制请求已受理，执行回执仍待同步。可刷新核对原任务。",
        }));
    };
    timer = setTimeout(() => void poll(), 1000);
    return () => {
      controller.abort();
      if (timer) clearTimeout(timer);
    };
    // A new decision owns a new poll; snapshot updates must not restart its timer.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [
    identity,
    projectID,
    state.sessionID,
    state.activeRunID,
    state.control?.current_decision?.decision_id,
    state.control?.delivery_status,
    patch,
  ]);
  async function retry(runID: string) {
    const previous = state.runs[runID]?.snapshot;
    if (
      !previous?.input ||
      !terminalStatuses.has(previous.state.status) ||
      active ||
      flight.current ||
      !sid.current
    )
      return;
    flight.current = true;
    try {
      const currentState = await getCurrentAgentSubmission(
        { pid: projectID, sid: sid.current },
        { signal: signal.current ?? undefined },
      );
      if (!current()) return;
      if (currentState.unresolved_run_ids?.length) {
        patch((old) => ({
          ...old,
          unresolved: currentState.unresolved_run_ids!,
          error: "请先核对原任务的结果，再重新生成。",
        }));
        return;
      }
      const original = structuredClone(previous.input.draft);
      validatePromptDraft(original);
      const frozen: PendingInput = {
        identity,
        sessionKey: crypto.randomUUID(),
        sessionID: sid.current,
        requestKey: crypto.randomUUID(),
        draft: original,
        title: "",
        sent: false,
        body: {
          request_id: "",
          input: original,
          retry_of_run_id: runID,
          expected_execution_revision:
            activeRun?.snapshot.state.execution_revision ??
            state.session?.execution_revision ??
            previous.state.execution_revision,
          budget_micros:
            typeof previous.manifest.max_micros === "number"
              ? previous.manifest.max_micros
              : budgetMicros(settings.budget)!,
          manifest: previous.manifest,
          ...(previous.manifest.goal_contract
            ? {
                goal_contract: structuredClone(previous.manifest.goal_contract),
              }
            : {}),
          ...(previous.input.quote
            ? {
                quoted_from: {
                  run_id: previous.input.quote.run_id,
                  message_id: previous.input.quote.message_id,
                },
              }
            : {}),
        },
      };
      frozen.body.request_id = frozen.requestKey;
      pending.current = frozen;
      flight.current = true;
      patch((old) => ({ ...old, phase: "submitting", error: "" }));
      await submitPending(frozen, true);
    } catch (error) {
      if (current()) {
        if (definiteRejection(error)) pending.current = null;
        patch((old) => ({
          ...old,
          error: message(error),
          phase: definiteRejection(error) ? "idle" : "unknown",
        }));
      }
    } finally {
      flight.current = false;
    }
  }
  return {
    draft,
    changeDraft,
    quote: quote?.identity === identity ? quote : null,
    clearQuote: () => setQuote(null),
    settings,
    changeSettings,
    models: state.models,
    model,
    skills: state.skills,
    session: state.session,
    sessionID: state.sessionID,
    summaries: state.summaries,
    runs: state.summaries
      .map((item) => state.runs[item.root_run_id])
      .filter((item): item is ConversationRun => !!item),
    runMap: state.runs,
    activeRunID: state.activeRunID,
    active: !!active,
    stopping,
    control: state.control,
    phase: state.phase,
    loading: state.loading,
    error: state.error,
    connection: state.connection,
    unresolved: state.unresolved,
    denied: state.denied,
    canSend: allowed.allowed,
    sendReason: allowed.reason,
    hasOlderRuns: !!state.olderRuns,
    send,
    stop: () => command("stop"),
    command,
    retry,
    loadRun: async (runID: string) => {
      try {
        await loadRun(runID);
      } catch (error) {
        if (current()) patch((old) => ({ ...old, error: message(error) }));
      }
    },
    loadOlderRuns,
    loadOlderMessages,
    quoteMessage,
    refresh: () => setRefreshKey((n) => n + 1),
  };
}
