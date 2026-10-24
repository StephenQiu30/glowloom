import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { CanvasLocationGate } from "@/components/canvas/canvas-location-gate";
import { RequestError } from "@/lib/request";

const mock = vi.hoisted(() => ({
  request: vi.fn(),
  select: vi.fn(),
  refresh: vi.fn(),
  replace: vi.fn(),
  selected: "personal",
  pending: false,
  error: "",
  spaces: [] as { id: string }[],
  query: "",
}));
vi.mock("@/api/canvases", () => ({ locateCanvas: mock.request }));
vi.mock("@/components/spaces/space-context", () => ({
  useSpace: () => ({
    selected: { id: mock.selected },
    pending: mock.pending,
    error: mock.error,
    spaces: mock.spaces,
    select: mock.select,
    refresh: mock.refresh,
  }),
}));
vi.mock("next/navigation", () => ({
  useRouter: () => ({ replace: mock.replace }),
  useSearchParams: () => new URLSearchParams(mock.query),
}));
const canvas = "d0485d98-a222-4d57-917b-4297b43144ee",
  project = "a5a402d7-fb34-40c7-b5b9-47473cafc3d2",
  team = "f4144ca8-22e1-4f1d-a88a-ba862f43ee78";
const destination = { canvas_id: canvas, project_id: project, scope_id: team };
const content = () => (
  <CanvasLocationGate canvasID={canvas}>
    {(location) => <div data-testid="editor">{location.scope_id}</div>}
  </CanvasLocationGate>
);
beforeEach(() => {
  vi.clearAllMocks();
  mock.selected = "personal";
  mock.pending = false;
  mock.error = "";
  mock.spaces = [{ id: "personal" }, { id: team }];
  mock.query = `project_id=${project}&canvas_id=${canvas}`;
  mock.select.mockResolvedValue(undefined);
  mock.request.mockResolvedValue(destination);
});
afterEach(cleanup);
it("resolves old links before connecting and canonicalizes only after selecting the authorized team", async () => {
  const view = render(content());
  expect(screen.queryByTestId("editor")).toBeNull();
  await waitFor(() => expect(mock.select).toHaveBeenCalledWith(team));
  expect(mock.replace).not.toHaveBeenCalled();
  mock.pending = true;
  view.rerender(content());
  expect(screen.queryByTestId("editor")).toBeNull();
  mock.pending = false;
  mock.selected = team;
  view.rerender(content());
  expect(screen.getByTestId("editor").textContent).toBe(team);
  await waitFor(() =>
    expect(mock.replace).toHaveBeenCalledWith(
      `/canvas?project_id=${project}&canvas_id=${canvas}&scope_id=${team}`,
      { scroll: false },
    ),
  );
  expect(mock.select).toHaveBeenCalledTimes(1);
});
it("ignores tampered scope hints and uses server ownership", async () => {
  mock.query += "&scope_id=foreign";
  render(content());
  await waitFor(() => expect(mock.select).toHaveBeenCalledWith(team));
  expect(screen.queryByText("foreign")).toBeNull();
});
it("renders a compact permission state without opening another space", async () => {
  mock.request.mockRejectedValue(new RequestError("not_found", 404));
  render(content());
  await screen.findByText("链接已失效或暂无访问权限。");
  expect(mock.select).not.toHaveBeenCalled();
  expect(screen.queryByTestId("editor")).toBeNull();
  expect(screen.queryByText("not_found")).toBeNull();
  expect(screen.queryByRole("button", { name: "重试" })).toBeNull();
});
it("recovers from dependency failure without exposing internal details", async () => {
  mock.request.mockRejectedValueOnce(new Error("/internal/private path"));
  render(content());
  fireEvent.click(await screen.findByRole("button", { name: "重试" }));
  await waitFor(() => expect(mock.select).toHaveBeenCalledWith(team));
  expect(screen.queryByText("/internal/private path")).toBeNull();
});
it.each([null, { ...destination, scope_id: "javascript:alert(1)" }])(
  "rejects malformed routing responses: %j",
  async (invalid) => {
    mock.request.mockResolvedValue(invalid);
    render(content());
    await screen.findByRole("button", { name: "重试" });
    expect(mock.select).not.toHaveBeenCalled();
    expect(mock.replace).not.toHaveBeenCalled();
  },
);
it("fails closed when current space membership is absent", async () => {
  mock.spaces = [{ id: "personal" }];
  render(content());
  await screen.findByText("链接已失效或暂无访问权限。");
  expect(mock.select).not.toHaveBeenCalled();
});
it("aborts an unmounted identity-bound lookup", async () => {
  mock.request.mockImplementation(() => new Promise(() => {}));
  const view = render(content());
  const signal = mock.request.mock.calls[0][1].signal;
  view.unmount();
  expect(signal.aborted).toBe(true);
});
