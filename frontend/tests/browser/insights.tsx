/** Isolated source-page acceptance. All identities and facts below are synthetic. */
import {
  fixtureDaily,
  fixtureTotals,
  fixtureStatuses,
  fixtureLogSource,
  fixtureUUID as uuid,
} from "./insights-data";
import { createRoot } from "react-dom/client";
import { ThemeProvider } from "next-themes";
import { SessionProvider } from "@/components/auth/session";
import { SpaceProvider } from "@/components/spaces/space-context";
import { BasicLayout } from "@/components/layout/basic-layout";
import { TooltipProvider } from "@/components/ui/tooltip";
import { Toaster } from "@/components/ui/sonner";
import { installDevelopmentTransport } from "@/lib/development-transport";
import { RequestError } from "@/lib/request";
import { Diagnostics } from "@/app/health/_components/diagnostics";
import { AuditEvents } from "@/app/audit/_components/audit-events";
import { Analytics } from "@/app/analytics/_components/analytics";
import "@/app/globals.css";
const params = new URLSearchParams(location.search),
  page = params.get("page") ?? "diagnostics",
  role = params.get("role") ?? "admin",
  state = params.get("state") ?? "populated";
const now = new Date().toISOString(),
  from = new Date(Date.now() - 7 * 86400000).toISOString();
const session = {
  actor: {
    id: uuid(1),
    email: "insights@example.test",
    display_name: "合成验收账号",
    platform_role: role === "admin" ? "platform_admin" : "creator",
    status: "active",
    revision: "1",
    capabilities: [],
  },
  session_id: uuid(2),
  csrf_token: "fixture-only",
  credential_epoch: "1",
  persistent: false,
  server_time: now,
  session_protocol: "v1",
  upgrade_available: false,
  last_active_at: now,
  idle_expires_at: new Date(Date.now() + 3600000).toISOString(),
  absolute_expires_at: new Date(Date.now() + 3600000).toISOString(),
};
const source = fixtureLogSource(now, state);
let calls = 0;
installDevelopmentTransport({
  matches: (path) => path === "/tests/browser/insights.html",
  request: async (url, options) => {
    if (url === "/api/session") return session;
    if (url === "/api/me/preferences")
      return {
        revision: "1",
        theme: params.get("theme") ?? "dark",
        canvas_background: "dots",
        prompt_suffix: "",
      };
    if (url === "/api/me/spaces")
      return {
        items: [
          {
            id: uuid(5),
            name: "合成个人空间",
            kind: "personal",
            role: "owner",
            writable: true,
          },
        ],
      };
    if (url === "/api/projects") return { items: [], next_cursor: null };
    if (
      url.includes("diagnostics") ||
      url.includes("audit-events") ||
      url.includes("analytics")
    ) {
      calls++;
      if (state === "loading")
        await new Promise((resolve) => setTimeout(resolve, 3600000));
      await new Promise((resolve) => setTimeout(resolve, 180));
      if (state === "error")
        throw new RequestError("dependency_unavailable", 503, {
          fixture: true,
        });
      if (role !== "admin" && !url.endsWith("/creation"))
        throw new RequestError("forbidden", 403, { fixture: true });
    }
    if (url === "/api/platform/diagnostics/overview") return source.overview;
    if (url === "/api/platform/diagnostics/logs")
      return source.logs(options.params?.cursor);
    if (url.startsWith("/api/platform/diagnostics/logs/")) {
      const detail = source.detail(
        url.slice("/api/platform/diagnostics/logs/".length),
      );
      if (!detail) throw new RequestError("not_found", 404, { fixture: true });
      return detail;
    }
    if (url === "/api/platform/audit-events")
      return source.audit(options.params?.cursor);
    if (url === "/api/analytics/creation") {
      const empty = state === "empty";
      return {
        accepted_operations: empty ? 0 : 128,
        as_of: now,
        attempts: empty ? 0 : 152,
        cancelled_operations: empty ? 0 : 4,
        completed_operations: empty ? 0 : 110,
        cost_micros: null,
        coverage: {
          capabilities: [],
          max_operations: 10000,
          source: "operation_facts",
          status: state === "partial" ? "partial" : "available",
          truncated: state === "partial",
        },
        daily: empty ? [] : fixtureDaily(Date.now()),
        entries: empty
          ? []
          : [
              { key: "canvas", operations: 100 },
              { key: "agent", operations: 28 },
            ],
        failed_operations: empty ? 0 : 8,
        from,
        models: empty
          ? []
          : [
              {
                model_id: uuid(30),
                model_name: "合成视频模型",
                operations: 128,
              },
            ],
        outputs: empty ? 0 : 140,
        partial_operations: empty ? 0 : 2,
        profit_micros: null,
        revenue_micros: null,
        scope: options.params?.scope ?? "personal",
        statuses: empty ? [] : fixtureStatuses,
        terminal_success_rate: empty ? null : fixtureTotals.rate,
        timezone: "Asia/Shanghai",
        to: now,
        unknown_operations: empty ? 0 : 4,
      };
    }
    if (url === "/api/analytics/business")
      return {
        as_of: now,
        capabilities: [],
        cost_micros: null,
        profit_micros: null,
        reason: "finance_authority_unconnected",
        refund_micros: null,
        revenue_micros: null,
        status: "unavailable",
      };
    throw new Error(`Unhandled fixture API: ${url}`);
  },
});
// For the business screenshot exercise the actual public tab after the page is mounted.
if (page === "business")
  new MutationObserver((_, observer) => {
    const tab = Array.from(
      document.querySelectorAll<HTMLElement>("[role=tab]"),
    ).find((t) => t.textContent === "经营");
    if (tab) {
      tab.dispatchEvent(
        new MouseEvent("mousedown", { bubbles: true, button: 0 }),
      );
      observer.disconnect();
    }
  }).observe(document.body, { subtree: true, childList: true });
const screen =
  page === "audit"
    ? "audit"
    : page === "creation" || page === "business"
      ? "analytics"
      : "health";
createRoot(document.getElementById("root")!).render(
  <ThemeProvider attribute="class" defaultTheme="dark">
    <TooltipProvider>
      <SessionProvider>
        <SpaceProvider>
          <BasicLayout screen={screen}>
            {page === "audit" ? (
              <AuditEvents />
            ) : screen === "analytics" ? (
              <Analytics />
            ) : (
              <Diagnostics />
            )}
          </BasicLayout>
        </SpaceProvider>
      </SessionProvider>
    </TooltipProvider>
    <Toaster />
  </ThemeProvider>,
);
export const fixtureCalls = () => calls;
