import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { useState } from "react";
import { ProjectCoverField } from "./project-cover-field";
vi.mock("./project-cover-preview", () => ({
  ProjectCoverPreview: () => <span>已选主图</span>,
}));
vi.mock("./project-cover-picker", () => ({
  ProjectCoverPicker: ({
    onSelected,
  }: {
    onSelected: (id: string) => void;
  }) => (
    <button onClick={() => onSelected("c13b18f1-cd43-4f35-bef4-f06801746458")}>
      选择图片
    </button>
  ),
}));
const pid = "9817c918-e49d-4dc8-b8b6-c92833b135e4";
const scope = {
  origin: window.location.origin,
  actorId: pid,
  orgId: "c13b18f1-cd43-4f35-bef4-f06801746458",
};
afterEach(cleanup);
it("Picker 选择退出并解锁后焦点回原按钮，390px可继续键盘操作", async () => {
  function Harness() {
    const [value, setValue] = useState<string | null>(null),
      [busy, setBusy] = useState(false);
    return (
      <ProjectCoverField
        projectId={pid}
        scope={scope}
        value={value}
        savedAssetId={null}
        unavailable={false}
        disabled={busy}
        onChange={setValue}
        onBusyChange={setBusy}
      />
    );
  }
  render(<Harness />);
  const trigger = screen.getByRole("button", { name: "选择项目图片" });
  trigger.focus();
  fireEvent.click(trigger);
  const select = await screen.findByRole("button", { name: "选择图片" });
  select.focus();
  fireEvent.click(select);
  await waitFor(() => expect(document.activeElement).toBe(trigger));
});
