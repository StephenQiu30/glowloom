import React from "react";
import { createRoot } from "react-dom/client";
import { ThemeProvider } from "next-themes";
import { CanvasProviders } from "../../src/components/canvas/editor";
import { serviceGraph } from "../../src/components/canvas/service-graph";
import "../../src/app/globals.css";
import CanvasPage from "../../src/components/canvas/editor/canvas-editor";
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
    <CanvasProviders>
      <CanvasPage
        documentIntegration={{
          projectId: project,
          initialState: document,
          viewStateKey: "canvas-ui-fixture",
          mediaEnabled: false,
          canNavigate: true,
          frozen: false,
          onChange: () => {},
          toolbar: () => null,
        }}
      />
    </CanvasProviders>
  </ThemeProvider>,
);
