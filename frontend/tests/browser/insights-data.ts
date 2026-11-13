/** Synthetic acceptance cohort, confined to the explicit browser fixture. */
const accepted = [12, 14, 16, 18, 20, 22, 26];
const completed = [10, 12, 14, 16, 18, 18, 22];
const failed = [1, 1, 1, 1, 1, 1, 2];
const cancelled = [0, 0, 0, 1, 1, 1, 1];
const unknown = [0, 1, 1, 0, 0, 1, 1];
export const fixtureTotals = {
  accepted: 128,
  completed: 110,
  failed: 8,
  cancelled: 4,
  unknown: 4,
  partial: 2,
  running: 2,
  rate: 110 / 122,
};
export const fixtureStatuses = [
  { key: "completed", operations: 110 },
  { key: "failed", operations: 8 },
  { key: "cancelled", operations: 4 },
  { key: "unknown", operations: 4 },
  { key: "running", operations: 2 },
];
export function fixtureDaily(now: number): API.Day[] {
  return accepted.map((count, i) => ({
    date: new Date(now - (6 - i) * 86400000).toLocaleDateString("en-CA", {
      timeZone: "Asia/Shanghai",
    }),
    accepted: count,
    completed: completed[i],
    failed: failed[i],
    cancelled: cancelled[i],
    unknown: unknown[i],
  }));
}

export const fixtureUUID = (n: number) =>
  `00000000-0000-4000-8000-${String(n).padStart(12, "0")}`;

/** Tiny, deterministic pages exercise real page controls with synthetic data. */
export function fixtureLogSource(asOf: string, state: string) {
  const at = (secondsAgo: number) =>
    new Date(Date.parse(asOf) - secondsAgo * 1000).toISOString();
  const records: API.DiagnosticRecord[] =
    state === "empty"
      ? []
      : [503, 429].map((status, i) => ({
          elapsed_ms: i === 0 ? 842 : 120,
          error_class: i === 0 ? "server_error" : "rate_limited",
          event: "http.request.completed",
          id: fixtureUUID(10 + 2 * i),
          level: i === 0 ? "error" : "warn",
          method: "GET",
          panic: false,
          release: "fixture-local",
          request_id: fixtureUUID(11 + 2 * i),
          role: "api",
          route: "/api/operations",
          service: "backend",
          status,
          timestamp: at(i + 1),
          trace_id: "0123456789abcdef0123456789abcdef",
        }));
  const coverage: API.ObservabilityCoverage = {
    capacity: 4096,
    evicted: state === "partial" ? 180 : 0,
    missing_services: ["agent", "canvas", "frontend"],
    oldest_at: records.at(-1)?.timestamp ?? null,
    restart_loss: true,
    retained: records.length,
    retention_seconds: 86400,
    source: "process_memory",
    started_at: at(86400),
    status: "partial",
  };
  const requests = records.length;
  const elapsed = records.map((row) => row.elapsed_ms).sort((a, b) => a - b);
  const serverErrors = records.filter((row) => row.status >= 500).length;
  const overview: API.Overview = {
    as_of: asOf,
    client_errors: records.filter(
      (row) => row.status >= 400 && row.status < 500,
    ).length,
    coverage,
    from: at(86400),
    to: asOf,
    p50_ms: elapsed[Math.ceil(requests * 0.5) - 1] ?? null,
    p95_ms: elapsed[Math.ceil(requests * 0.95) - 1] ?? null,
    panics: records.filter((row) => row.panic).length,
    rate_limited: records.filter((row) => row.status === 429).length,
    requests,
    server_error_rate: requests ? serverErrors / requests : null,
    server_errors: serverErrors,
    unavailable: ["time_series", "queue", "resource"],
  };
  const events =
    state === "empty"
      ? []
      : [20, 23].map((id, i) => ({
          id: fixtureUUID(id),
          action: "user.disabled",
          actor_id: fixtureUUID(1),
          created_at: at(i + 1),
          reason: "合成安全事件，用于验证只读摘要",
          request_id: fixtureUUID(id + 1),
          result: "success",
          safe_diff: { status: "active → disabled" },
          target_user_id: fixtureUUID(id + 2),
          team_id: "",
        }));
  function page<T>(rows: T[], cursor: string | undefined, scope: string) {
    const first = `fixture-${scope}-first`,
      next = `fixture-${scope}-next`;
    if (cursor && cursor !== first && cursor !== next)
      throw new Error("Invalid fixture cursor");
    const offset = cursor === next ? 1 : 0;
    return {
      as_of: asOf,
      first_cursor: first,
      next_cursor: rows.length > offset + 1 ? next : null,
      items: rows.slice(offset, offset + 1),
    };
  }
  return {
    overview,
    logs: (cursor?: string) => ({
      ...page(records, cursor, "log"),
      from: overview.from,
      to: overview.to,
      coverage,
    }),
    audit: (cursor?: string) => page(events, cursor, "audit"),
    detail: (id: string) => {
      const record = records.find((row) => row.id === id);
      return record ? { record, coverage } : null;
    },
  };
}
