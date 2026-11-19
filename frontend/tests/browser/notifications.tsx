/** Browser acceptance of production UI, using explicitly synthetic, local API facts. */
import { createRoot } from "react-dom/client";
import { ThemeProvider } from "next-themes";
import { SessionProvider } from "@/components/auth/session";
import { SpaceProvider } from "@/components/spaces/space-context";
import { BasicLayout } from "@/components/layout/basic-layout";
import { PageLayout } from "@/components/layout/page-layout";
import { TooltipProvider } from "@/components/ui/tooltip";
import { Toaster } from "@/components/ui/sonner";
import { NotificationProvider } from "@/components/notifications/notification-provider";
import { NotificationInboxRoute } from "@/components/notifications/notification-inbox-route";
import { AnnouncementList } from "@/components/notifications/announcement-list";
import { AnnouncementEditor } from "@/components/notifications/announcement-editor";
import { AnnouncementAdministration } from "@/components/notifications/announcement-administration";
import { UsersPage } from "@/app/users/_components/users-page";
import { ModelsPage } from "@/app/models/_components/models-page";
import { ProvidersPage } from "@/app/providers/_components/providers-page";
import { PlatformTeamsPage } from "@/app/platform/teams/_components/platform-teams-page";
import { PlatformSettingsPage } from "@/app/platform/settings/_components/platform-settings-page";
import { Diagnostics } from "@/app/health/_components/diagnostics";
import { AuditEvents } from "@/app/audit/_components/audit-events";
import { Analytics } from "@/app/analytics/_components/analytics";
import { installDevelopmentTransport } from "@/lib/development-transport";
import { RequestError } from "@/lib/request";
import { requireAnnouncement } from "@/components/notifications/contracts";
import { resolveScreen } from "@/components/layout/routes";
import { fixtureURL } from "./notifications-navigation";
import "@/app/globals.css";
const params = new URLSearchParams(location.search);
const route = params.get("route") ?? "/";
const state = params.get("state") ?? "populated";
const role = params.get("role") ?? "admin";
const now = new Date().toISOString();
const uuid = (i: number) =>
  `00000000-0000-4000-8000-${String(i).padStart(12, "0")}`;
const journal: {
  url: string;
  method?: string;
  params?: unknown;
  data?: unknown;
}[] = [];
Object.assign(window, { notificationFixtureJournal: journal });
const session = {
  actor: {
    id: uuid(1),
    email: "notifications@example.test",
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
const items: API.InboxNotification[] = Array.from({ length: 45 }, (_, i) => ({
  id: uuid(100 + i),
  title: `合成互动消息 ${i + 1}`,
  body: `这是用于浏览器布局和交互验收的合成内容 ${i + 1}。`,
  category: "collaboration",
  read: i > 29,
  inbox_seq: String(45 - i),
  occurred_at: now,
  status: "active",
  action_path: "",
}));
const defaultDraft: API.AnnouncementContent = {
  title: "合成维护公告草稿",
  body: "这是隔离验收的合成公告，不会投递给真实账号。",
  placement: "modal",
  audience: "authenticated",
  starts_at: "2099-11-17T01:00:00Z",
  ends_at: "2099-11-18T01:00:00Z",
  endless: false,
  manual_close: false,
  frequency: "once",
  duration_seconds: 5,
  inbox: true,
  priority: 0,
  action_path: "",
};
const displayAnnouncement = {
  id: uuid(700),
  revision: "1",
  notification_id: "",
  title: "合成展示公告",
  body: "隔离合成公告，用于焦点、倒计时及关闭验收。",
  placement: params.get("display") === "popup" ? "modal" : "banner",
  starts_at: now,
  endless: true,
  ends_at: "",
  manual_close: params.has("manual"),
  frequency: "once",
  duration_seconds: 5,
  action_path: "",
};
const sidebarDisplay = params.get("sidebar");
const sidebarAnnouncement =
  sidebarDisplay === "badge" || sidebarDisplay === "promotion"
    ? {
        ...displayAnnouncement,
        title: sidebarDisplay === "badge" ? "New" : "探索创作工具",
        body: "",
        placement: sidebarDisplay === "badge" ? "sidebar_badge" : "sidebar",
        action_path: sidebarDisplay === "badge" ? "/short-drama" : "/ai-tools",
      }
    : undefined;
const saved = sessionStorage.getItem("notification-fixture-announcement");
let announcement: API.Announcement = saved
  ? JSON.parse(saved)
  : {
      id: uuid(700),
      version: "1",
      status: "draft",
      enabled: false,
      draft: defaultDraft,
    };
let mailboxVersion = 1;
const summary = () => ({
  data_epoch: uuid(800),
  mailbox_version: String(mailboxVersion),
  max_inbox_seq: "45",
  unread_count: items.filter((item) => !item.read).length,
});
function persist() {
  sessionStorage.setItem(
    "notification-fixture-announcement",
    JSON.stringify(announcement),
  );
}
installDevelopmentTransport({
  matches: (path) => path === "/tests/browser/notifications.html",
  request: async (url, options) => {
    journal.push({
      url,
      method: options.method,
      params: options.params,
      data: options.data,
    });
    if (url === "/api/session") {
      if (role === "guest") throw new RequestError("unauthenticated", 401);
      return session;
    }
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
    if (url === "/api/announcements/public")
      return { sidebar: sidebarAnnouncement };
    if (url === "/api/notifications/display") {
      if (params.has("displayDelay"))
        await new Promise((resolve) => setTimeout(resolve, 1500));
      return {
        popup:
          params.get("display") === "popup" ? displayAnnouncement : undefined,
        banner:
          params.get("display") === "banner" ? displayAnnouncement : undefined,
        sidebar: sidebarAnnouncement,
      };
    }
    if (url === "/api/notifications/display/ack") return {};
    if (url === `/api/notifications/display/${uuid(700)}/1`)
      return { valid: true };
    if (url === "/api/notifications/summary") return summary();
    if (url === "/api/notifications/read-all") {
      items.forEach((item) => (item.read = true));
      mailboxVersion++;
      return summary();
    }
    if (url === "/api/notifications") {
      if (state === "loading") await new Promise(() => {});
      if (state === "error")
        throw new RequestError("dependency_unavailable", 503);
      const p = options.params ?? {};
      const data =
        state === "empty"
          ? []
          : items
              .filter((item) => !p.unread_only || !item.read)
              .map((item) =>
                p.category === "official"
                  ? {
                      ...item,
                      category: "system",
                      title: item.title?.replace("互动", "官方"),
                    }
                  : item,
              );
      const offset = Number(p.cursor ?? 0);
      return {
        items: data.slice(offset, offset + 20),
        next_cursor: offset + 20 < data.length ? String(offset + 20) : null,
      };
    }
    const notification = url.match(/^\/api\/notifications\/([^/]+)(\/read)?$/);
    if (notification) {
      const item = items.find((item) => item.id === notification[1]);
      if (!item) throw new RequestError("not_found", 404);
      if (notification[2]) {
        item.read = true;
        mailboxVersion++;
        return summary();
      }
      return item;
    }
    if (url === "/api/platform/notifications") {
      if (options.method === "POST") {
        const body = options.data as API.SaveAnnouncementInput;
        announcement = {
          ...announcement,
          draft: body.content,
          status: "draft",
          version: String(Number(announcement.version) + 1),
        };
        persist();
        return announcement;
      }
      if (state === "loading") await new Promise(() => {});
      if (state === "error")
        throw new RequestError("dependency_unavailable", 503);
      return {
        items:
          state === "empty" ||
          sessionStorage.getItem("notification-fixture-deleted")
            ? []
            : [announcement],
        next_cursor: null,
      };
    }
    if (url === `/api/platform/notifications/${uuid(700)}`) return announcement;
    if (url === `/api/platform/notifications/${uuid(700)}/audit`)
      return { items: [{ id: uuid(701), action: "created", created_at: now }] };
    if (url === `/api/platform/notifications/${uuid(700)}/delete`) {
      sessionStorage.setItem("notification-fixture-deleted", "true");
      return announcement;
    }
    if (url === `/api/platform/notifications/${uuid(700)}/publish`) {
      announcement = {
        ...announcement,
        published: announcement.draft,
        status: "active",
        enabled: true,
        version: String(Number(announcement.version) + 1),
      };
      persist();
      return announcement;
    }
    if (url === "/api/platform/users" || url === "/api/platform/teams")
      return { items: [], next_cursor: null };
    if (url === "/api/platform/settings")
      return {
        registration_enabled: true,
        team_creation_enabled: false,
        version: "1",
      };
    const adminReadPaths = [
      "/api/admin/providers",
      "/api/admin/models",
      "/api/admin/capabilities",
      "/api/admin/provider-adapters",
      "/api/admin/provider-presets",
    ];
    if (
      url === "/api/admin/provider-adapters" ||
      url === "/api/admin/capabilities"
    )
      return [];
    if (adminReadPaths.includes(url) && options.method === "GET")
      return { items: [], next_cursor: null };
    if (
      [
        "/api/platform/diagnostics/overview",
        "/api/platform/diagnostics/logs",
        "/api/platform/audit-events",
        "/api/analytics/creation",
      ].includes(url)
    )
      throw new RequestError("dependency_unavailable", 503, { fixture: true });
    throw new Error(`Unhandled fixture API: ${options.method} ${url}`);
  },
});
document.addEventListener(
  "click",
  (event) => {
    const anchor =
      event.target instanceof Element ? event.target.closest("a") : null;
    if (
      anchor &&
      anchor.origin === location.origin &&
      !anchor.pathname.startsWith("/tests/")
    ) {
      event.preventDefault();
      location.assign(fixtureURL(anchor.pathname + anchor.search));
    }
  },
  true,
);
const screen = resolveScreen(route)?.id ?? "home";
let content;
if (route === "/platform/notifications") content = <AnnouncementList />;
else if (route === "/platform/notifications/new")
  content = (
    <PageLayout screen="platform/notifications">
      <AnnouncementEditor />
    </PageLayout>
  );
else if (route.endsWith("/edit"))
  content = (
    <PageLayout screen="platform/notifications">
      <AnnouncementEditor announcement={requireAnnouncement(announcement)} />
    </PageLayout>
  );
else if (route.startsWith("/platform/notifications/"))
  content = (
    <PageLayout screen="platform/notifications">
      <AnnouncementAdministration id={uuid(700)} />
    </PageLayout>
  );
else if (route === "/notifications") content = <NotificationInboxRoute />;
else if (route === "/users") content = <UsersPage />;
else if (route === "/models")
  content = (
    <PageLayout screen="models">
      <ModelsPage />
    </PageLayout>
  );
else if (route === "/providers")
  content = (
    <PageLayout screen="providers">
      <ProvidersPage />
    </PageLayout>
  );
else if (route === "/platform/teams")
  content = (
    <PageLayout screen="platform/teams">
      <PlatformTeamsPage />
    </PageLayout>
  );
else if (route === "/platform/settings")
  content = (
    <PageLayout screen="platform/settings">
      <PlatformSettingsPage />
    </PageLayout>
  );
else if (route === "/health") content = <Diagnostics />;
else if (route === "/audit") content = <AuditEvents />;
else if (route === "/analytics") content = <Analytics />;
else
  content = (
    <PageLayout screen="home">
      <p>合成 API 隔离验收 · 真实源组件</p>
    </PageLayout>
  );
createRoot(document.getElementById("root")!).render(
  <ThemeProvider attribute="class" defaultTheme="dark">
    <TooltipProvider>
      <SessionProvider>
        <NotificationProvider>
          <SpaceProvider>
            <BasicLayout screen={screen} allowGuest>
              {content}
            </BasicLayout>
          </SpaceProvider>
        </NotificationProvider>
      </SessionProvider>
    </TooltipProvider>
    <Toaster />
  </ThemeProvider>,
);
