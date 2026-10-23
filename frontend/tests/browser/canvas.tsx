import React from "react";
import { createRoot } from "react-dom/client";
import { ThemeProvider } from "next-themes";
import { CanvasEditor } from "../../src/components/canvas/editor";
import { serviceGraph } from "../../src/components/canvas/service-graph";
import "../../src/app/globals.css";
const id = "11111111-1111-4111-8111-111111111111",
  project = "22222222-2222-4222-8222-222222222222";
const document = serviceGraph(
  {
    id,
    project_id: project,
    revision: "0",
    schema_version: 1,
    operation_version: 1,
    permissions: { read: true, write: true },
    nodes: [],
    edges: [],
  },
  id,
  project,
);
createRoot(window.document.getElementById("root")!).render(
  <ThemeProvider attribute="class">
    <CanvasEditor
      service={{
        document,
        canvasID: id,
        projectID: project,
        revision: "0",
        name: "画布交互夹具",
        status: "本地交互验证",
        writable: true,
        viewStateKey: "canvas-ui-fixture",
        commit: async () => {
          throw new Error("此夹具不提供服务端保存，请在正式画布验证保存回执");
        },
        undo: {
          canUndo: false,
          canRedo: false,
          undo: () => {},
          redo: () => {},
        },
      }}
    />
  </ThemeProvider>,
);
