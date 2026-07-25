import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";

import { Providers } from "@/app/providers";
import { useAppDispatch, useAppSelector } from "@/store/hooks";
import { makeStore } from "@/store/make-store";
import { setSidebarOpen } from "@/store/slices/ui-slice";

function SidebarProbe() {
  const dispatch = useAppDispatch();
  const open = useAppSelector((state) => state.ui.sidebarOpen);

  return (
    <button onClick={() => dispatch(setSidebarOpen(!open))} type="button">
      {open ? "导航已展开" : "导航已收起"}
    </button>
  );
}

describe("Redux application boundary", () => {
  it("creates an isolated store for each application instance", () => {
    const first = makeStore();
    const second = makeStore();

    first.dispatch(setSidebarOpen(false));

    expect(first).not.toBe(second);
    expect(first.getState().ui.sidebarOpen).toBe(false);
    expect(second.getState().ui.sidebarOpen).toBe(true);
  });

  it("provides typed Redux hooks through the root provider", async () => {
    const user = userEvent.setup();
    render(
      <Providers>
        <SidebarProbe />
      </Providers>,
    );

    await user.click(screen.getByRole("button", { name: "导航已展开" }));

    expect(screen.getByRole("button", { name: "导航已收起" })).toBeInTheDocument();
  });
});
