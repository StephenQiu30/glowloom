import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { useState } from "react";
import { afterEach, expect, it } from "vitest";
import { ConnectionCreateMenu } from "@/components/canvas/controls/canvas-create-menus";

afterEach(cleanup);
it("returns creation focus to the new node instead of dismissing its composer via the canvas", async () => {
  function Harness() {
    const [created, setCreated] = useState(false);
    return (
      <>
        <div data-canvas-viewport tabIndex={-1} />
        {created ? (
          <div data-node-id="created-node" tabIndex={-1}>
            新节点
          </div>
        ) : (
          <ConnectionCreateMenu
            pending={{
              position: { x: 0, y: 0 },
              connection: { nodeId: "source", handleType: "source" },
            }}
            existingType="text"
            onClose={() => {}}
            onCreate={() => {
              setCreated(true);
              return "created-node";
            }}
          />
        )}
      </>
    );
  }
  render(<Harness />);
  fireEvent.click(screen.getByRole("option", { name: "图片" }));
  await waitFor(() =>
    expect(document.activeElement).toBe(screen.getByText("新节点")),
  );
});
