import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { ServiceCanvasWorkspace } from "@/components/canvas/service-workspace";
vi.mock("next/navigation", () => ({
  useRouter: () => ({ replace: vi.fn() }),
  useSearchParams: () =>
    new URLSearchParams({
      canvas_id: "10000000-0000-4000-8000-000000000001",
      project_id: "10000000-0000-4000-8000-000000000002",
    }),
}));
vi.mock("@/components/auth/session", () => ({
  useOptionalSession: () => ({ actor: { id: "actor" } }),
  SessionGate: ({ children }: { children: React.ReactNode }) => children,
}));
vi.mock("@/components/spaces/space-context", () => ({
  useSpace: () => ({
    selected: { id: "10000000-0000-4000-8000-000000000003" },
    spaces: [{ id: "10000000-0000-4000-8000-000000000003" }],
    pending: false,
    error: "",
  }),
  SpaceGate: ({ children }: { children: React.ReactNode }) => children,
}));
vi.mock("@/api/canvases", () => ({
  locateCanvas: async () => ({
    canvas_id: "10000000-0000-4000-8000-000000000001",
    project_id: "10000000-0000-4000-8000-000000000002",
    scope_id: "10000000-0000-4000-8000-000000000003",
  }),
}));
vi.mock("@/components/canvas/yjs-workspace", () => ({
  YjsCanvasWorkspace: ({
    actorID,
    scopeID,
  }: {
    actorID: string;
    scopeID: string;
  }) => (
    <div>
      Yjs {actorID} {scopeID}
    </div>
  ),
}));
afterEach(() => {
  cleanup();
  vi.unstubAllEnvs();
});
it.each(["false", "true", ""])(
  "always uses the Yjs workspace, including obsolete flag %s",
  async (value) => {
    vi.stubEnv("NEXT_PUBLIC_CANVAS_YJS", value);
    render(<ServiceCanvasWorkspace />);
    expect(
      await screen.findByText("Yjs actor 10000000-0000-4000-8000-000000000003"),
    ).toBeTruthy();
  },
);
