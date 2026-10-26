/** Test-only browser entry. Uses product components; never contacts a real account. */
import React from "react";
import { createRoot } from "react-dom/client";
import { ThemeProvider } from "next-themes";
import {
  SessionProvider,
  SessionGate,
} from "../../src/components/auth/session";
import { PersonalSpace } from "../../src/app/account/_components/personal-space";
import { installDevelopmentTransport } from "../../src/lib/development-transport";
import { RequestError } from "../../src/lib/request";
import { Toaster } from "../../src/components/ui/sonner";
import "../../src/app/globals.css";
let actor = {
  id: "11111111-1111-4111-8111-111111111111",
  display_name: "织光创作者",
  email: "profile@example.test",
  bio: "",
  revision: "1",
  status: "active",
  platform_role: "creator",
  capabilities: [],
};
const session = {
  actor,
  session_id: "22222222-2222-4222-8222-222222222222",
  csrf_token: "fixture-only",
  credential_epoch: "1",
  persistent: false,
  last_active_at: new Date().toISOString(),
  absolute_expires_at: new Date(Date.now() + 3600000).toISOString(),
};
const scenario = new URLSearchParams(location.search).get("state");
installDevelopmentTransport({
  matches: (p) => p === "/tests/browser/personal-space.html",
  request: async (url, options) => {
    if (scenario === "loading")
      await new Promise((resolve) => setTimeout(resolve, 1200));
    if (url === "/api/session") return { ...session, actor };
    if (url === "/api/me/preferences")
      return {
        revision: "1",
        theme: "dark",
        canvas_background: "dots",
        prompt_suffix: "",
      };
    if (url === "/api/me") {
      if (scenario === "error")
        throw new RequestError("dependency_unavailable", 503);
      if (scenario === "forbidden") throw new RequestError("forbidden", 403);
      if (options.method === "PATCH") {
        const b = options.data;
        actor = {
          ...actor,
          display_name: b.display_name ?? actor.display_name,
          bio: b.bio ?? actor.bio,
          revision: String(Number(actor.revision) + 1),
        };
      }
      return actor;
    }
    if (url === "/api/me/avatar") throw new RequestError("not_found", 404);
    if (url.includes("library"))
      return {
        current_actor_id: actor.id,
        current_org_id: "33333333-3333-4333-8333-333333333333",
        library_id: "44444444-4444-4444-8444-444444444444",
        scope: { kind: "personal" },
        revision: 1,
        total: 0,
        page: 1,
        page_size: 12,
        items: [],
        folders: [],
      };
    if (url === "/api/projects") return { items: [], next_cursor: null };
    return { items: [], next_cursor: null, as_of: new Date().toISOString() };
  },
});
createRoot(document.getElementById("root")!).render(
  <ThemeProvider attribute="class" defaultTheme="dark">
    <SessionProvider>
      <SessionGate>
        <PersonalSpace />
      </SessionGate>
    </SessionProvider>
    <Toaster />
  </ThemeProvider>,
);
