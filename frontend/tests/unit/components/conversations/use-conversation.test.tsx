import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { RequestError } from "@/lib/request";
import { emptyPromptDraft } from "@/components/generation/prompt-document";
import { useConversation } from "@/components/conversations/use-conversation";
const calls = vi.hoisted(() => ({
  create: vi.fn(),
  start: vi.fn(),
  read: vi.fn(),
  run: vi.fn(),
  history: vi.fn(),
  submission: vi.fn(),
  current: vi.fn(),
  control: vi.fn(),
  command: vi.fn(),
  events: vi.fn(),
  models: vi.fn(),
  skills: vi.fn(),
  saveDraft: vi.fn(),
  messages: vi.fn(),
  message: vi.fn(),
}));
vi.mock("@/api/agent", () => ({
  createAgentSession: calls.create,
  createAgentRun: calls.start,
  getAgentSession: calls.read,
  getAgentRun: calls.run,
  listAgentRuns: calls.history,
  getAgentSubmission: calls.submission,
  getCurrentAgentSubmission: calls.current,
  getAgentControlStatus: calls.control,
  commandAgentRun: calls.command,
  subscribeAgentEvents: calls.events,
  listAgentSkills: calls.skills,
  listAgentMessages: calls.messages,
  getAgentMessage: calls.message,
}));
vi.mock("@/api/models", () => ({ listProjectModels: calls.models }));
vi.mock("@/components/conversations/conversation-context", () => ({
  useConversationContext: () => ({
    readDraft: () => undefined,
    saveDraft: calls.saveDraft,
    readSettings: () => undefined,
    saveSettings: () => {},
    changed: () => {},
  }),
}));
vi.mock("@/lib/request", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/request")>()),
  sseAdapter: (handler: unknown) => handler,
}));
const pid = "00000000-0000-4000-8000-000000000001",
  sid = "00000000-0000-4000-8000-000000000002",
  rid = "00000000-0000-4000-8000-000000000003";
const model = {
  key: "planner",
  status: "active",
  provider_status: "active",
  credential_present: true,
  available: true,
  mode_availability: [{ mode: "text_to_text", available: true }],
  capability: "text.generate",
  input_roles: ["prompt"],
  current_version: { id: "version", limits: {} },
  current_price: { id: "price" },
};
const receipt = {
  root_run_id: rid,
  session_id: sid,
  submission_id: "submission",
  status: "accepted",
  execution_revision: 1,
};
const snapshot = {
  root_run_id: rid,
  session_id: sid,
  submission_id: "submission",
  goal: "写一个故事",
  manifest: {},
  execution_available: true,
  snapshot_cursor: "1:1",
  events: [],
  messages_page: { items: [], through: "1:1" },
  state: { status: "completed", execution_revision: 2 },
};
function draft() {
  const result = emptyPromptDraft();
  result.document.content[0]!.content = [{ type: "text", text: "写一个故事" }];
  return result;
}
beforeEach(() => {
  vi.resetAllMocks();
  calls.models.mockResolvedValue({ items: [model] });
  calls.skills.mockResolvedValue([]);
  calls.create.mockResolvedValue({
    id: sid,
    project_id: pid,
    title: "写一个故事",
    execution_revision: 0,
  });
  calls.start.mockResolvedValue(receipt);
  calls.run.mockResolvedValue(snapshot);
  calls.history.mockResolvedValue({ items: [] });
  calls.current.mockResolvedValue({});
  calls.control.mockResolvedValue({ authorization_revision: 1 });
  calls.events.mockImplementation(() => new Promise(() => {}));
});
afterEach(cleanup);
it("submits a frozen full draft once on double click and clears only after admission", async () => {
  const { result } = renderHook(() =>
    useConversation({
      ownerKey: "actor/space",
      projectID: pid,
      enabled: true,
      initialDraft: draft(),
    }),
  );
  await waitFor(() => expect(result.current.model?.key).toBe("planner"));
  await act(async () => {
    await Promise.all([result.current.send(), result.current.send()]);
  });
  expect(calls.create).toHaveBeenCalledTimes(1);
  expect(calls.start).toHaveBeenCalledTimes(1);
  expect(calls.start.mock.calls[0]![1].input.document).toEqual(
    draft().document,
  );
  expect(result.current.draft.document.content[0]?.content ?? []).toEqual([]);
});
it("finds the original accepted run after a lost response without starting another run", async () => {
  calls.start.mockRejectedValueOnce(new RequestError("network_error"));
  calls.submission.mockResolvedValue(receipt);
  const { result } = renderHook(() =>
    useConversation({
      ownerKey: "actor/space",
      projectID: pid,
      enabled: true,
      initialDraft: draft(),
    }),
  );
  await waitFor(() => expect(result.current.model?.key).toBe("planner"));
  await act(() => result.current.send());
  expect(calls.start).toHaveBeenCalledTimes(1);
  expect(calls.submission).toHaveBeenCalledTimes(1);
  expect(calls.submission.mock.calls[0]![0].requestId).toBe(
    calls.start.mock.calls[0]![1].request_id,
  );
  expect(result.current.activeRunID).toBe(rid);
});
it("does not reuse private session data after the owner changes", async () => {
  calls.read.mockResolvedValue({
    id: sid,
    project_id: pid,
    title: "私有旧对话",
    execution_revision: 2,
    run: snapshot,
  });
  const { result, rerender } = renderHook(
    ({ ownerKey }) =>
      useConversation({
        ownerKey,
        projectID: pid,
        sessionID: sid,
        enabled: true,
      }),
    { initialProps: { ownerKey: "old/space" } },
  );
  await waitFor(() => expect(result.current.session?.title).toBe("私有旧对话"));
  calls.read.mockImplementation(() => new Promise(() => {}));
  rerender({ ownerKey: "new/space" });
  expect(result.current.session).toBeNull();
  expect(result.current.runs).toEqual([]);
});
it("keeps the original request and session when refreshing an unresolved send", async () => {
  calls.start.mockRejectedValueOnce(new RequestError("network_error"));
  calls.submission.mockRejectedValue(
    new RequestError("submission_not_found", 404),
  );
  calls.read.mockResolvedValue({
    id: sid,
    project_id: pid,
    title: "原会话",
    execution_revision: 0,
  });
  const { result } = renderHook(() =>
    useConversation({
      ownerKey: "actor/space",
      projectID: pid,
      enabled: true,
      initialDraft: draft(),
    }),
  );
  await waitFor(() => expect(result.current.model?.key).toBe("planner"));
  await act(() => result.current.send());
  const original = calls.start.mock.calls[0]![1].request_id;
  expect(result.current.phase).toBe("unknown");
  act(() => result.current.refresh());
  await waitFor(() => expect(result.current.loading).toBe(false));
  expect(result.current.phase).toBe("unknown");
  expect(result.current.canRetrySubmission).toBe(true);
  expect(calls.start).toHaveBeenCalledTimes(1);
  expect(calls.submission).toHaveBeenCalledTimes(2);
  calls.start.mockResolvedValue(receipt);
  await act(() => result.current.send());
  expect(calls.create).toHaveBeenCalledTimes(1);
  expect(calls.start.mock.calls[1]![1].request_id).toBe(original);
});
it("preserves the current draft and coalesces double clicks on regenerate", async () => {
  const source = {
    ...snapshot,
    manifest: { model_key: "planner", max_micros: 1000000 },
    input: {
      draft: draft(),
      plain_text: "写一个故事",
      prompt: "写一个故事",
      media_pins: [],
      schema_version: 1,
      input_hash: "a".repeat(64),
    },
  };
  calls.read.mockResolvedValue({
    id: sid,
    project_id: pid,
    title: "会话",
    execution_revision: 2,
    run: source,
  });
  const next = "00000000-0000-4000-8000-000000000004";
  calls.start.mockResolvedValue({ ...receipt, root_run_id: next });
  calls.run.mockResolvedValue({ ...source, root_run_id: next });
  const { result } = renderHook(() =>
    useConversation({
      ownerKey: "actor/space",
      projectID: pid,
      sessionID: sid,
      enabled: true,
      initialDraft: draft(),
    }),
  );
  await waitFor(() => expect(result.current.runs).toHaveLength(1));
  await act(async () => {
    await Promise.all([result.current.retry(rid), result.current.retry(rid)]);
  });
  expect(calls.start).toHaveBeenCalledTimes(1);
  expect(result.current.draft.document).toEqual(draft().document);
  expect(result.current.runs).toHaveLength(2);
});
it("recovers a lost Stop response from Go and never submits a second Stop decision", async () => {
  const running = {
    ...snapshot,
    state: {
      status: "running",
      execution_revision: 1,
      authorization_revision: 1,
    },
  };
  calls.read.mockResolvedValue({
    id: sid,
    project_id: pid,
    title: "会话",
    execution_revision: 1,
    run: running,
  });
  calls.run.mockResolvedValue(running);
  let status: Record<string, unknown> = { authorization_revision: 1 };
  calls.control.mockImplementation(async () => status);
  calls.command.mockImplementation(async () => {
    status = {
      authorization_revision: 2,
      current_decision: {
        root_run_id: rid,
        decision_id: "stop-decision",
        authorization_revision: 2,
        action: "stop",
      },
      delivery_status: "pending",
    };
    throw new RequestError("network_error");
  });
  const { result } = renderHook(() =>
    useConversation({
      ownerKey: "actor/space",
      projectID: pid,
      sessionID: sid,
      enabled: true,
    }),
  );
  await waitFor(() => expect(result.current.runs).toHaveLength(1));
  await act(() => result.current.stop());
  expect(result.current.stopping).toBe(true);
  await act(() => result.current.stop());
  expect(calls.command).toHaveBeenCalledTimes(1);
});
it("reads the final committed snapshot before closing on a terminal event", async () => {
  const running = {
    ...snapshot,
    state: {
      status: "running",
      execution_revision: 1,
      authorization_revision: 1,
    },
  };
  calls.run.mockResolvedValue(running);
  const { result } = renderHook(() =>
    useConversation({
      ownerKey: "actor/space",
      projectID: pid,
      enabled: true,
      initialDraft: draft(),
    }),
  );
  await waitFor(() => expect(result.current.model?.key).toBe("planner"));
  await act(() => result.current.send());
  await waitFor(() => expect(calls.events).toHaveBeenCalled());
  const tail = {
    message_id: "tail",
    run_id: rid,
    conversation_id: 1,
    sequence: "2:2",
    part_index: 0,
    kind: "assistant",
    content: "最终尾部文字",
  };
  calls.run.mockResolvedValue({
    ...snapshot,
    snapshot_cursor: "2:2",
    messages_page: { items: [tail], through: "2:2" },
    state: { status: "completed", execution_revision: 3 },
  });
  await act(async () => {
    calls.events.mock.calls[0]![1].adapter({
      id: "2:1",
      event: "agent.state",
      data: { root_run_id: rid, status: "completed", execution_revision: 3 },
    });
  });
  await waitFor(() =>
    expect(
      result.current.runs[0]?.messages.map((part) => part.content),
    ).toContain("最终尾部文字"),
  );
});
it("submits only the current proposal with Go's authorization revision", async () => {
  const waiting = {
    ...snapshot,
    state: {
      status: "waiting_approval",
      execution_revision: 2,
      authorization_revision: 1,
      proposal: {
        proposal_id: "proposal",
        candidate_hash: "candidate",
        candidate: {},
        authorization_revision: 2,
      },
    },
  };
  calls.read.mockResolvedValue({
    id: sid,
    project_id: pid,
    title: "会话",
    execution_revision: 2,
    run: waiting,
  });
  calls.run.mockResolvedValue(waiting);
  calls.control.mockResolvedValue({ authorization_revision: 2 });
  calls.command.mockResolvedValue({});
  const { result } = renderHook(() =>
    useConversation({
      ownerKey: "actor/space",
      projectID: pid,
      sessionID: sid,
      enabled: true,
    }),
  );
  await waitFor(() => expect(result.current.runs).toHaveLength(1));
  await act(() => result.current.command("approve"));
  expect(calls.command).toHaveBeenCalledWith(
    { pid, sid, rid },
    expect.objectContaining({
      action: "approve",
      proposal_id: "proposal",
      candidate_hash: "candidate",
      expected_authorization_revision: 2,
      request_id: expect.any(String),
    }),
    expect.anything(),
  );
});
it("unmount only closes the event subscription and never sends Stop", async () => {
  const running = {
    ...snapshot,
    state: { status: "running", execution_revision: 1 },
  };
  calls.read.mockResolvedValue({
    id: sid,
    project_id: pid,
    title: "会话",
    execution_revision: 1,
    run: running,
  });
  calls.run.mockResolvedValue(running);
  const { result, unmount } = renderHook(() =>
    useConversation({
      ownerKey: "actor/space",
      projectID: pid,
      sessionID: sid,
      enabled: true,
    }),
  );
  await waitFor(() => expect(result.current.runs).toHaveLength(1));
  unmount();
  expect(calls.command).not.toHaveBeenCalled();
});
it("hands the unsent draft to the accepted session before navigation", async () => {
  let admit!: (value: typeof receipt) => void;
  calls.start.mockImplementation(
    () =>
      new Promise((resolve) => {
        admit = resolve;
      }),
  );
  const navigate = vi.fn();
  const { result } = renderHook(() =>
    useConversation({
      ownerKey: "actor/space",
      projectID: pid,
      enabled: true,
      initialDraft: draft(),
      onAccepted: navigate,
    }),
  );
  await waitFor(() => expect(result.current.model?.key).toBe("planner"));
  let sending!: Promise<void>;
  act(() => {
    sending = result.current.send();
  });
  await waitFor(() => expect(calls.start).toHaveBeenCalledTimes(1));
  const next = draft();
  next.document.content[0]!.content = [
    { type: "text", text: "下一条尚未发送" },
  ];
  act(() => result.current.changeDraft(next));
  await act(async () => {
    admit(receipt);
    await sending;
  });
  expect(calls.saveDraft).toHaveBeenCalledWith(`${pid}/${sid}`, next);
  const handoff =
    calls.saveDraft.mock.invocationCallOrder[
      calls.saveDraft.mock.calls.findIndex(
        (args) => args[0] === `${pid}/${sid}`,
      )
    ]!;
  expect(handoff).toBeLessThan(navigate.mock.invocationCallOrder[0]!);
});
it("renews an expired message cursor from a fresh snapshot", async () => {
  const paged = {
    ...snapshot,
    messages_page: { items: [], through: "5:1", next_cursor: "expired" },
  };
  calls.read.mockResolvedValue({
    id: sid,
    project_id: pid,
    title: "会话",
    execution_revision: 2,
    run: paged,
  });
  calls.messages.mockRejectedValue(new RequestError("cursor_expired", 410));
  calls.run.mockResolvedValue({
    ...paged,
    messages_page: { items: [], through: "6:1", next_cursor: "renewed" },
  });
  const { result } = renderHook(() =>
    useConversation({
      ownerKey: "actor/space",
      projectID: pid,
      sessionID: sid,
      enabled: true,
    }),
  );
  await waitFor(() => expect(result.current.runs).toHaveLength(1));
  await act(() => result.current.loadOlderMessages(rid));
  expect(result.current.runMap[rid]?.older).toEqual({
    before: "renewed",
    through: "6:1",
  });
});
it("retains accepted messages and reads Go control facts while Agent refresh is offline", async () => {
  const running = {
    ...snapshot,
    state: { status: "running", execution_revision: 2 },
  };
  calls.read.mockResolvedValue({
    id: sid,
    project_id: pid,
    title: "原会话",
    execution_revision: 2,
    run: running,
  });
  calls.run.mockResolvedValue(running);
  const { result } = renderHook(() =>
    useConversation({
      ownerKey: "actor/space",
      projectID: pid,
      sessionID: sid,
      enabled: true,
    }),
  );
  await waitFor(() => expect(result.current.runs).toHaveLength(1));
  calls.read.mockRejectedValue(new RequestError("agent_unavailable", 503));
  calls.control.mockResolvedValue({
    authorization_revision: 2,
    current_decision: {
      root_run_id: rid,
      action: "stop",
      decision_id: "original-stop",
      authorization_revision: 2,
    },
    delivery_status: "pending",
  });
  act(() => result.current.refresh());
  await waitFor(() => expect(result.current.error).toBeTruthy());
  expect(result.current.runs).toHaveLength(1);
  await waitFor(() => expect(result.current.stopping).toBe(true));
  expect(calls.command).not.toHaveBeenCalled();
});
it("blocks new input when an existing session cannot be read", async () => {
  calls.read.mockRejectedValue(new RequestError("agent_unavailable", 503));
  const { result } = renderHook(() =>
    useConversation({
      ownerKey: "actor/space",
      projectID: pid,
      sessionID: sid,
      enabled: true,
      initialDraft: draft(),
    }),
  );
  await waitFor(() => expect(result.current.loading).toBe(false));
  expect(result.current.canSend).toBe(false);
  await act(() => result.current.send());
  expect(calls.start).not.toHaveBeenCalled();
});

it("pins the production obligation on the original request across an uncertain response", async () => {
  calls.start.mockRejectedValue(new RequestError("network_error"));
  calls.submission.mockResolvedValue(undefined);
  const original = { kind: "production", required_deliveries: 1 } as const;
  const { result, rerender } = renderHook(
    ({ goalContract }) =>
      useConversation({
        ownerKey: "actor/space",
        projectID: pid,
        enabled: true,
        canvasID: "00000000-0000-4000-8000-000000000099",
        initialDraft: draft(),
        goalContract,
      }),
    { initialProps: { goalContract: original as API.GoalContract } },
  );
  await waitFor(() => expect(result.current.model?.key).toBe("planner"));
  await act(() => result.current.send());
  expect(calls.start.mock.calls[0]![1].goal_contract).toEqual(original);
  expect(calls.create.mock.calls[0]![1].canvas_id).toBe(
    "00000000-0000-4000-8000-000000000099",
  );
  expect(calls.start.mock.calls[0]![1].canvas_id).toBe(
    "00000000-0000-4000-8000-000000000099",
  );
  rerender({ goalContract: { kind: "production", required_deliveries: 2 } });
  await act(() => result.current.send());
  for (const [, body] of calls.start.mock.calls) {
    expect(body.goal_contract).toEqual(original);
    expect(body.request_id).toBe(calls.start.mock.calls[0]![1].request_id);
  }
});
