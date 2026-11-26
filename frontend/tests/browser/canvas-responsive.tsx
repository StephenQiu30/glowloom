/** Native canvas and dialogs; all identity, catalog and media data stay in this fixture. */
import { useEffect, useRef, useState } from "react";
import { createRoot } from "react-dom/client";
import { ThemeProvider } from "next-themes";
import { TooltipProvider } from "@/components/ui/tooltip";
import { SessionContext } from "@/components/auth/session";
import { SpaceContext } from "@/components/spaces/space-context";
import { CanvasProviders } from "@/components/canvas/editor";
import { Button } from "@/components/ui/button";
import { Toaster } from "@/components/ui/sonner";
import { CanvasMediaContext } from "@/components/canvas-editor/lib/canvas-media-context.ts";
import { CanvasTopBar } from "@/components/canvas-editor/controls/canvas-top-bar.tsx";
import { ReferenceAssetsPanel } from "@/app/assets/_components/assets-page";
import { CanvasAccountControls } from "@/components/canvas/canvas-account-controls";
import { NotificationProvider } from "@/components/notifications/notification-provider";
import { RequestError } from "@/lib/request";
import { ProjectAssetDialog } from "@/components/media/project-asset-dialog";
import { PersonalLibraryPicker } from "@/components/media/personal-reference-picker";
import { serviceGraph } from "@/components/canvas/service-graph";
import { emptyPromptDraft } from "@/components/generation/prompt-document";
import { installDevelopmentTransport } from "@/lib/development-transport";
import { setCSRFToken, setCreationScope } from "@/lib/request";
import type { CanvasYjsLiveSession } from "@/components/canvas/yjs-live-session";
import CanvasPage from "@/components/canvas-editor/editor/canvas-editor.tsx";
import { CanvasGenerationPanel } from "@/components/canvas-editor/nodes/canvas-generation-panel.tsx";
import "../../src/app/globals.css";

const id = (n: number) =>
  `00000000-0000-4000-8000-${String(n).padStart(12, "0")}`;
const actorID = id(1),
  projectID = id(2),
  canvasID = id(3),
  nodeID = id(4),
  scopeID = id(5);
const fixtureParams = new URLSearchParams(location.search);
const inspectToolbar = fixtureParams.has("toolbar");
const syncing = fixtureParams.has("syncing");
const identity = {
  actor: {
    id: actorID,
    display_name: "画布验收账号",
    email: "canvas@example.test",
    platform_role: "creator",
  },
  session_id: id(6),
  csrf_token: "fixture-only",
} as API.EmailSessionView;
const space = { id: scopeID, writable: true } as API.SpaceView;
const notificationSummary = {
  data_epoch: "canvas-fixture",
  mailbox_version: "1",
  max_inbox_seq: "1",
  unread_count: 1,
};
const notification = {
  id: id(9),
  inbox_seq: "1",
  title: "画布验收通知",
  body: "隔离浏览器验收消息。",
  category: "collaboration",
  read: false,
  occurred_at: new Date().toISOString(),
  status: "active",
  action_path: "",
};
setCSRFToken("fixture-only");
setCreationScope(scopeID);
installDevelopmentTransport({
  matches: (pathname) => pathname === "/tests/browser/canvas-responsive.html",
  async request(url, options) {
    const params = options.params ?? {};
    if (url === "/api/session") return identity;
    if (url === "/api/billing/wallet") {
      if (fixtureParams.has("walletForbidden"))
        throw new RequestError("forbidden", 403);
      return {
        enabled: true,
        general_credits: "0",
        held_credits: "2",
        payer: "personal",
      };
    }
    if (url === "/api/notifications/summary") return notificationSummary;
    if (url === "/api/notifications/display") return {};
    if (url === "/api/notifications")
      return { items: [notification], ...notificationSummary };
    if (url.endsWith("/operations")) return { items: [] };
    if (url === "/api/me/preferences") return { revision: "1", theme: "dark" };
    if (url === "/api/models") return { items: [] };
    if (url.endsWith("/prompt-submissions/current")) return { epoch: 0 };
    if (url.includes("/library") && !url.endsWith("/preview")) {
      const kind = String(params.kind ?? "image");
      const page = Number(params.page) || 1;
      const pageSize = Number(params.page_size) || 20;
      const total = fixtureParams.has("assetPages") ? pageSize + 2 : 12;
      const offset = (page - 1) * pageSize;
      const count = Math.max(0, Math.min(pageSize, total - offset));
      return {
        current_actor_id: actorID,
        current_org_id: scopeID,
        library_id: id(7),
        revision: 1,
        scope:
          params.scope === "personal"
            ? { kind: "personal" }
            : { kind: "project", project_id: projectID },
        page,
        page_size: pageSize,
        total,
        folders: [],
        items: Array.from({ length: count }, (_, index) => {
          const position = offset + index;
          return {
            id: id(20 + position),
            asset_id: id(50 + position),
            revision: 1,
            favorite: false,
            updated_at: "2026-11-16T00:00:00Z",
            title: `LongUnbrokenReferenceAssetNameForResponsiveAcceptance-${position}`,
            kind,
            catalog_state: "active",
            media: {
              id: id(50 + position),
              kind,
              revision: 1,
              file_name: `长素材名称用于验证窄屏-${position}.png`,
            },
          };
        }),
      };
    }
    if (url.endsWith("/preview"))
      return {
        asset: {
          id: id(
            50 + Number(url.match(/items\/.+?(\d{12})\//)?.[1] ?? 20) - 20,
          ),
          kind: "image",
          revision: 1,
        },
        url: `${location.origin}/tests/browser/canvas-reference.svg`,
        expires_at: new Date(Date.now() + 3600000).toISOString(),
      };
    if (url.includes("/subjects"))
      return {
        current_actor_id: actorID,
        current_scope_id: scopeID,
        items: [],
        page: 1,
        page_size: 20,
        total: 0,
      };
    if (url.includes("/canvases")) return { items: [] };
    throw new Error(`Unexpected isolated request: ${url}`);
  },
});
const initial = serviceGraph(
  {
    id: canvasID,
    project_id: projectID,
    revision: "1",
    schema_version: 1,
    operation_version: 1,
    permissions: { read: true, write: true },
    nodes: [
      {
        id: nodeID,
        node_type: "image",
        node_action: "resource",
        content_version: "1",
        title: "响应式图片节点",
        x: 100,
        y: innerHeight < 400 ? 56 : 160,
        width: 240,
        height: innerHeight < 400 ? 80 : 180,
        config: {},
      },
    ],
    edges: [],
  },
  canvasID,
  projectID,
);
const empty = emptyPromptDraft();
empty.document.content[0].content = [
  { type: "text", text: "展开前后保留这段创作内容。" },
];
function Fixture() {
  const [graph, setGraph] = useState(initial);
  const [picker, setPicker] = useState<"project" | "personal" | null>(null);
  const listeners = useRef(new Set<() => void>());
  const snapshot = useRef({
    document: graph,
    writable: true,
    migration: false,
    inputDrafts: { [nodeID]: { draft: empty, reason: "" } },
  });
  useEffect(() => {
    snapshot.current = { ...snapshot.current, document: graph };
    listeners.current.forEach((listener) => listener());
  }, [graph]);
  const client = useRef({
    subscribe: (listener: () => void) => {
      listeners.current.add(listener);
      return () => listeners.current.delete(listener);
    },
    getSnapshot: () => snapshot.current,
    getDocumentEpoch: () => "isolated-responsive",
    canAccept: () => true,
    change: (next: typeof initial) => {
      snapshot.current = { ...snapshot.current, document: next };
      setGraph(next);
      listeners.current.forEach((listener) => listener());
    },
    changeInput: () => true,
    bindInputComposer: () => () => {},
    waitForSaved: async (check?: () => void) => check?.(),
    prepareInput: async () => {
      throw new Error("隔离布局验收不创建生成任务");
    },
  } as unknown as CanvasYjsLiveSession).current;
  const Shell = inspectToolbar ? NotificationProvider : PassThrough;
  return (
    <ThemeProvider attribute="class" forcedTheme="dark">
      <SessionContext value={identity}>
        <SpaceContext
          value={{
            spaces: [space],
            selected: space,
            pending: false,
            error: "",
            refresh: async () => {},
            select: async () => {},
          }}
        >
          <TooltipProvider>
            <Shell>
              <CanvasProviders>
                <div className="canvas-workspace h-dvh">
                  <CanvasMediaContext.Provider
                    value={{
                      canvasId: canvasID,
                      projectId: projectID,
                      bindingRevision: "1",
                    }}
                  >
                    <CanvasPage
                      documentIntegration={{
                        projectId: projectID,
                        initialState: graph,
                        viewStateKey: `responsive-${innerWidth}`,
                        mediaEnabled: false,
                        renderReferenceAssets: (refreshKey) => (
                          <ReferenceAssetsPanel refreshKey={refreshKey} />
                        ),
                        canNavigate: !syncing,
                        frozen: false,
                        onChange: setGraph,
                        renderGenerationPanel: () => (
                          <CanvasGenerationPanel
                            client={client}
                            nodeID={nodeID}
                            canvasID={canvasID}
                            projectID={projectID}
                          />
                        ),
                        toolbar: (controls) =>
                          inspectToolbar ? (
                            <CanvasTopBar
                              {...controls}
                              title="画布导航验收"
                              projectID={projectID}
                              canvasID={canvasID}
                              canSwitchCanvas={!syncing}
                              onRenameProject={() => {}}
                              onDeleteProject={() => {}}
                              renderAccount={(compact) => (
                                <CanvasAccountControls
                                  scopeID={scopeID}
                                  compact={compact}
                                  canNavigate={!syncing}
                                  onHome={controls.onHome}
                                />
                              )}
                            />
                          ) : (
                            <>
                              <div className="flex flex-wrap gap-2 p-2">
                                <span title="隔离布局验收，无保存与生成请求">
                                  隔离验收
                                </span>
                                <Button onClick={() => setPicker("project")}>
                                  项目素材
                                </Button>
                                <Button onClick={() => setPicker("personal")}>
                                  个人素材
                                </Button>
                              </div>
                              {controls.renderMore?.()}
                            </>
                          ),
                      }}
                    />
                  </CanvasMediaContext.Provider>
                </div>
                {picker === "project" && (
                  <ProjectAssetDialog
                    projectID={projectID}
                    canvasPresentation
                    onClose={() => setPicker(null)}
                    onAdd={() => {}}
                  />
                )}
                {picker === "personal" && (
                  <PersonalLibraryPicker
                    owner={{ actorId: actorID, scopeId: scopeID }}
                    onClose={() => setPicker(null)}
                    onSelect={() => {}}
                  />
                )}
              </CanvasProviders>
              {inspectToolbar && <Toaster />}
            </Shell>
          </TooltipProvider>
        </SpaceContext>
      </SessionContext>
    </ThemeProvider>
  );
}
function PassThrough({ children }: { children: React.ReactNode }) {
  return children;
}

createRoot(document.getElementById("root")!).render(<Fixture />);
