"use client";
import { useEffect, useEffectEvent } from "react";
import { getAgentRun, subscribeAgentEvents } from "@/api/agent";
import { sseAdapter, type ServerEvent } from "@/lib/request";
import { terminalStatuses } from "./conversation-state";

export function useConversationEvents({
  identity,
  projectID,
  sessionID,
  run,
  refreshKey,
  onSnapshot,
  onEvent,
  onConnection,
}: {
  identity: string;
  projectID: string;
  sessionID: string | null;
  run: API.RunSnapshot | undefined;
  refreshKey: number;
  onSnapshot: (snapshot: API.RunSnapshot) => void;
  onEvent: (event: ServerEvent) => void;
  onConnection: (state: "connected" | "recovering" | "offline") => void;
}) {
  const current = useEffectEvent(() => run);
  const receiveSnapshot = useEffectEvent(onSnapshot),
    receiveEvent = useEffectEvent(onEvent),
    connection = useEffectEvent(onConnection);
  const rid = run?.root_run_id;
  useEffect(() => {
    const snapshot = current();
    if (
      !sessionID ||
      !rid ||
      !snapshot ||
      terminalStatuses.has(snapshot.state.status)
    )
      return;
    const controller = new AbortController();
    let cursor = snapshot.snapshot_cursor,
      failures = 0;
    let timer: ReturnType<typeof setTimeout> | undefined;
    const seen = new Set<string>();
    let finishing = false;
    async function reload() {
      const next = await getAgentRun(
        { pid: projectID, sid: sessionID!, rid: rid! },
        { signal: controller.signal },
      );
      if (controller.signal.aborted) return false;
      receiveSnapshot(next);
      cursor = next.snapshot_cursor;
      return terminalStatuses.has(next.state.status);
    }
    const receive = (event: ServerEvent) => {
      if (
        controller.signal.aborted ||
        seen.has(event.id) ||
        !/^\d+:\d+$/.test(event.id)
      )
        return;
      seen.add(event.id);
      if (seen.size > 10000) seen.delete(seen.values().next().value!);
      cursor = event.id;
      receiveEvent(event);
      if (
        event.event === "agent.state" &&
        event.data &&
        typeof event.data === "object" &&
        "status" in event.data &&
        typeof event.data.status === "string" &&
        terminalStatuses.has(event.data.status) &&
        !finishing
      ) {
        finishing = true;
        // A terminal frame can precede text in the same committed batch. Read the
        // complete atomic snapshot before closing this subscription.
        void reload()
          .then(() => {
            if (!controller.signal.aborted) connection("connected");
            controller.abort();
          })
          .catch(() => {
            finishing = false;
            connection("recovering");
          });
      }
    };
    const connect = async () => {
      if (controller.signal.aborted) return;
      try {
        connection("connected");
        await subscribeAgentEvents(
          { pid: projectID, sid: sessionID, rid },
          {
            signal: controller.signal,
            headers: { "Last-Event-ID": cursor },
            adapter: sseAdapter(receive),
          },
        );
      } catch {
        if (controller.signal.aborted) return;
      }
      if (controller.signal.aborted) return;
      connection("recovering");
      try {
        if (await reload()) {
          connection("connected");
          controller.abort();
          return;
        }
        failures = 0;
      } catch {
        failures++;
      }
      if (controller.signal.aborted) return;
      if (failures >= 5) {
        connection("offline");
        return;
      }
      timer = setTimeout(() => void connect(), 2000);
    };
    void connect();
    return () => {
      controller.abort();
      if (timer) clearTimeout(timer);
    };
  }, [identity, projectID, sessionID, rid, refreshKey]);
}
