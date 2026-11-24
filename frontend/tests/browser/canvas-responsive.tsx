/** Native canvas and dialogs; all identity, catalog and media data stay in this fixture. */
import { useEffect, useRef, useState } from "react";
import { createRoot } from "react-dom/client";
import { ThemeProvider } from "next-themes";
import { TooltipProvider } from "@/components/ui/tooltip";
import { SessionContext } from "@/components/auth/session";
import { SpaceContext } from "@/components/spaces/space-context";
import { CanvasProviders } from "@/components/canvas/editor";
import { Button } from "@/components/ui/button";
import { ProjectAssetDialog } from "@/components/media/project-asset-dialog";
import { PersonalLibraryPicker } from "@/components/media/personal-reference-picker";
import { serviceGraph } from "@/components/canvas/service-graph";
import { emptyPromptDraft } from "@/components/generation/prompt-document";
import { installDevelopmentTransport } from "@/lib/development-transport";
import { setCSRFToken, setCreationScope } from "@/lib/request";
import type { CanvasYjsLiveSession } from "@/components/canvas/yjs-live-session";
import CanvasPage from "@glowloom/canvas/editor/editor/canvas-editor.tsx";
import { CanvasGenerationPanel } from "@glowloom/canvas/editor/nodes/canvas-generation-panel.tsx";
import "../../src/app/globals.css";

const id = (n: number) =>
  `00000000-0000-4000-8000-${String(n).padStart(12, "0")}`;
const actorID = id(1),
  projectID = id(2),
  canvasID = id(3),
  nodeID = id(4),
  scopeID = id(5);
const identity = {
  actor: { id: actorID },
  session_id: id(6),
  csrf_token: "fixture-only",
} as API.EmailSessionView;
const space = { id: scopeID, writable: true } as API.SpaceView;
setCSRFToken("fixture-only");
setCreationScope(scopeID);
installDevelopmentTransport({
  matches: (pathname) => pathname === "/tests/browser/canvas-responsive.html",
  async request(url, options) {
    const params = options.params ?? {};
    if (url === "/api/session") return identity;
    if (url === "/api/me/preferences") return { revision: "1", theme: "dark" };
    if (url === "/api/models") return { items: [] };
    if (url.endsWith("/prompt-submissions/current")) return { epoch: 0 };
    if (url.includes("/library") && !url.endsWith("/preview")) {
      const kind = String(params.kind ?? "image");
      return {
        current_actor_id: actorID,
        current_org_id: scopeID,
        library_id: id(7),
        revision: 1,
        scope:
          params.scope === "personal"
            ? { kind: "personal" }
            : { kind: "project", project_id: projectID },
        page: Number(params.page) || 1,
        page_size: params.page_size ?? 20,
        total: 12,
        folders: [],
        items: Array.from({ length: 12 }, (_, index) => ({
          id: id(20 + index),
          asset_id: id(50 + index),
          revision: 1,
          favorite: false,
          updated_at: "2026-11-16T00:00:00Z",
          title: `LongUnbrokenReferenceAssetNameForResponsiveAcceptance-${index}`,
          kind,
          catalog_state: "active",
          media: {
            id: id(50 + index),
            kind,
            revision: 1,
            file_name: `长素材名称用于验证窄屏-${index}.png`,
          },
        })),
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
            <CanvasProviders>
              <div className="canvas-workspace h-dvh">
                <CanvasPage
                  documentIntegration={{
                    projectId: projectID,
                    initialState: graph,
                    viewStateKey: `responsive-${innerWidth}`,
                    mediaEnabled: false,
                    canNavigate: true,
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
                    toolbar: () => (
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
                    ),
                  }}
                />
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
          </TooltipProvider>
        </SpaceContext>
      </SessionContext>
    </ThemeProvider>
  );
}
createRoot(document.getElementById("root")!).render(<Fixture />);
