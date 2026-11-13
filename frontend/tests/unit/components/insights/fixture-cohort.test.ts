import { expect, it } from "vitest";
import {
  fixtureDaily,
  fixtureTotals,
  fixtureStatuses,
} from "../../../browser/insights-data";
it("the isolated sample uses one accepted cohort across cards, trend and breakdown", () => {
  const daily = fixtureDaily(Date.parse("2026-11-13T10:00:00Z"));
  for (const key of [
    "accepted",
    "completed",
    "failed",
    "cancelled",
    "unknown",
  ] as const)
    expect(daily.reduce((sum, day) => sum + day[key], 0)).toBe(
      fixtureTotals[key],
    );
  expect(fixtureStatuses.reduce((sum, row) => sum + row.operations, 0)).toBe(
    fixtureTotals.accepted,
  );
  expect(
    fixtureTotals.completed +
      fixtureTotals.failed +
      fixtureTotals.cancelled +
      fixtureTotals.unknown +
      fixtureTotals.running,
  ).toBe(fixtureTotals.accepted);
  expect(fixtureTotals.partial).toBeLessThanOrEqual(fixtureTotals.unknown);
  expect(fixtureTotals.rate).toBe(
    fixtureTotals.completed /
      (fixtureTotals.completed +
        fixtureTotals.failed +
        fixtureTotals.cancelled),
  );
});

it.each([true, false])(
  "fixture diagnostic pages reconcile counts, identity and cursor exhaustion (empty=%s)",
  async (empty) => {
    const { fixtureLogSource } = await import("../../../browser/insights-data");
    const data = fixtureLogSource(
      "2026-11-13T10:00:00Z",
      empty ? "empty" : "populated",
    );
    const { overview } = data;
    const first = data.logs(undefined);
    const second = first.next_cursor ? data.logs(first.next_cursor) : null;
    const rows = [...first.items, ...(second?.items ?? [])];
    expect(rows.length).toBe(overview.requests);
    expect(overview.coverage.retained).toBe(rows.length);
    expect(overview.client_errors).toBe(
      rows.filter((r) => r.status >= 400 && r.status < 500).length,
    );
    expect(overview.server_errors).toBe(
      rows.filter((r) => r.status >= 500).length,
    );
    expect(overview.panics).toBe(rows.filter((r) => r.panic).length);
    expect(overview.rate_limited).toBe(
      rows.filter((r) => r.status === 429).length,
    );
    expect(overview.server_error_rate).toBe(
      empty ? null : overview.server_errors / rows.length,
    );
    expect(data.logs(first.first_cursor)).toEqual(first);
    expect(second?.next_cursor ?? null).toBeNull();
    for (const row of rows) expect(data.detail(row.id)?.record).toEqual(row);
    expect(data.detail("missing")).toBeNull();
    if (empty) {
      expect(first.next_cursor).toBeNull();
      expect(overview.p95_ms).toBeNull();
      expect(overview.p50_ms).toBeNull();
      expect(overview.coverage.oldest_at).toBeNull();
    } else {
      expect(second?.items.length).toBeGreaterThan(0);
      const elapsed = rows.map((r) => r.elapsed_ms).sort((a, b) => a - b);
      expect(overview.p95_ms).toBe(elapsed[Math.ceil(0.95 * rows.length) - 1]);
    }
    const audit = data.audit(undefined);
    expect(data.audit(audit.first_cursor)).toEqual(audit);
    if (empty) expect(audit.next_cursor).toBeNull();
    else {
      const next = data.audit(audit.next_cursor!);
      expect(next.items[0].id).not.toBe(audit.items[0].id);
      expect(next.next_cursor).toBeNull();
    }
  },
);
