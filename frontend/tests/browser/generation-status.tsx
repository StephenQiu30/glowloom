import { useEffect, useRef, useState } from "react";
import { createRoot } from "react-dom/client";
import { SWRConfig } from "swr";
import { ThemeProvider } from "next-themes";
import { TooltipProvider } from "@/components/ui/tooltip";
import { Toaster } from "@/components/ui/sonner";
import { CanvasProviders } from "@/components/canvas/editor";
import CanvasPage from "@/components/canvas/editor/canvas-editor";
import { CanvasTopBar } from "@/components/canvas/controls/canvas-top-bar";
import { CanvasOperationStatus } from "@/components/canvas/nodes/canvas-operation-status";
import { CanvasMediaContext } from "@/components/canvas/lib/canvas-media-context";
import { applyCanvasOperationResult } from "@/components/canvas/lib/canvas-operation-result";
import { serviceGraph } from "@/components/canvas/service-graph";
import type { CanvasYjsLiveSession } from "@/components/canvas/yjs-live-session";
import { installDevelopmentTransport } from "@/lib/development-transport";
import "../../src/app/globals.css";

const pid = "11111111-1111-4111-8111-111111111111",
  cid = "22222222-2222-4222-8222-222222222222",
  nid = "33333333-3333-4333-8333-333333333333",
  aid = "44444444-4444-4444-8444-444444444444",
  oid = "55555555-5555-4555-8555-555555555555";
const params = new URLSearchParams(location.search);
const kind =
  params.get("kind") === "video"
    ? "video"
    : params.get("kind") === "audio"
      ? "audio"
      : "image";
const ids =
  params.get("kind") === "batch"
    ? [aid, "77777777-7777-4777-8777-777777777777"]
    : [aid];
let status = "submitted";
const imageURL = "/tests/browser/canvas-reference.svg";
installDevelopmentTransport({
  matches: (path) => path === "/tests/browser/generation-status.html",
  async request(url) {
    if (url.includes("/operations/"))
      return {
        id: oid,
        project_id: pid,
        target_content_version: "1",
        status,
        source: { canvas_id: cid, node_id: nid },
        outputs:
          status === "completed"
            ? ids.map((id, sequence) => ({
                id,
                kind,
                sequence,
                moderation_status: "passed",
                media_asset_id: id,
              }))
            : [],
      };
    if (url.endsWith("/preview"))
      return {
        asset: {
          id: ids.find((id) => url.includes(id)),
          project_id: pid,
          kind,
          revision: 1,
          width: 720,
          height: 540,
        },
        url:
          kind === "image"
            ? imageURL
            : "/tests/browser/fixtures/generation-result." +
              (kind === "video" ? "mp4" : "wav"),
      };
    if (url.includes("/files/") || url.includes("/media/"))
      return { url: imageURL, revision: "1", file_id: aid };
    throw new Error(`Unexpected fixture request: ${url}`);
  },
});
const initial = serviceGraph(
  {
    id: cid,
    project_id: pid,
    revision: "1",
    schema_version: 1,
    operation_version: 1,
    permissions: { read: true, write: true },
    nodes: [
      {
        id: nid,
        node_type: kind,
        node_action: "resource",
        content_version: "1",
        title: { image: "图片素材", video: "视频素材", audio: "音频素材" }[
          kind
        ],
        x: Math.max(16, (window.innerWidth - 320) / 2),
        y: 180,
        width: 320,
        height: 320,
        config: {},
      },
    ],
    edges: [],
  },
  cid,
  pid,
);
function Fixture() {
  const [graph, setGraph] = useState(initial),
    [active, setActive] = useState(true);
  const state = useRef(graph);
  useEffect(() => {
    state.current = graph;
  }, [graph]);
  const client = useRef({
    getSnapshot: () => ({
      document: state.current,
      writable: true,
      migration: false,
    }),
    getDocumentEpoch: () => "fixture",
    canAccept: () => true,
    change: (next: typeof graph) => {
      state.current = next;
      setGraph(next);
    },
    waitForSaved: async (check?: () => void) => check?.(),
  } as unknown as CanvasYjsLiveSession).current;
  useEffect(() => {
    const complete = () => {
      status = "completed";
      window.dispatchEvent(new Event("focus"));
    };
    window.addEventListener("fixture-complete", complete);
    return () => window.removeEventListener("fixture-complete", complete);
  }, []);
  return (
    <ThemeProvider
      attribute="class"
      forcedTheme={
        new URLSearchParams(location.search).get("theme") === "light"
          ? "light"
          : "dark"
      }
    >
      <SWRConfig value={{ refreshWhenHidden: true }}>
        <TooltipProvider>
          <Toaster />
          <CanvasProviders>
            <CanvasMediaContext.Provider
              value={{ projectId: pid, canvasId: cid, bindingRevision: "1" }}
            >
              <div className="canvas-workspace h-dvh">
                <CanvasPage
                  documentIntegration={{
                    projectId: pid,
                    initialState: graph,
                    viewStateKey: `generation-acceptance-${window.innerWidth}`,
                    mediaEnabled: true,
                    canNavigate: true,
                    frozen: false,
                    onChange: setGraph,
                    renderToolNode: (id) =>
                      active ? (
                        <CanvasOperationStatus
                          projectID={pid}
                          canvasID={cid}
                          nodeID={id}
                          operationID={oid}
                          identity="fixture"
                          onCompleted={async (_, __, task) => {
                            await applyCanvasOperationResult(
                              client,
                              pid,
                              cid,
                              nid,
                              task,
                            );
                            setActive(false);
                          }}
                        />
                      ) : null,
                    renderGenerationPanel: () => null,
                    toolbar: (controls) => (
                      <CanvasTopBar
                        {...controls}
                        title="未命名画布"
                        nodes={graph.nodes}
                        saveControl={<span>已保存</span>}
                        onRenameProject={() => {}}
                        onDeleteProject={() => {}}
                      />
                    ),
                  }}
                />
              </div>
            </CanvasMediaContext.Provider>
          </CanvasProviders>
        </TooltipProvider>
      </SWRConfig>
    </ThemeProvider>
  );
}
createRoot(document.getElementById("root")!).render(<Fixture />);
