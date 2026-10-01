import { useState } from "react";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { MediaDepthDialog } from "./media-depth-dialog";
vi.mock("./media-depth-panel", () => ({ MediaDepthPanel: () => null }));
afterEach(cleanup);
it("程序打开的深度工具关闭后将键盘焦点交还来源视频节点", async () => {
  function Owner() {
    const [open, setOpen] = useState(true);
    return (
      <>
        <div data-testid="source" role="group" tabIndex={0}>
          来源视频
        </div>
        {open ? (
          <MediaDepthDialog
            projectId="b580ad59-17a5-4ed2-985b-cb0cdd04b4c4"
            canvasId="5db0c65d-d391-45a0-ab1a-d7f6eb4c868d"
            nodeId="0c6e50ee-0912-4da2-96b3-c84e27b61389"
            onClose={() => setOpen(false)}
            restoreFocus={() => screen.getByTestId("source").focus()}
          />
        ) : null}
      </>
    );
  }
  render(<Owner />);
  fireEvent.click(screen.getByRole("button", { name: "关闭深度任务" }));
  await waitFor(() =>
    expect(document.activeElement).toBe(screen.getByTestId("source")),
  );
});
