import { useState } from "react";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import {
  PageToolbar,
  PageToolbarContext,
} from "@/components/layout/page-toolbar";
import { ProductHeader } from "@/components/layout/product-header";
import { TooltipProvider } from "@/components/ui/tooltip";
vi.mock("next/navigation", () => ({ useRouter: () => ({ push: vi.fn() }) }));
afterEach(cleanup);

function Counter() {
  const [count, setCount] = useState(0);
  return <button onClick={() => setCount(count + 1)}>Filter {count}</button>;
}
function ResponsiveHeader({ mobile }: { mobile: boolean }) {
  const [host, setHost] = useState<HTMLDivElement | null>(null);
  return (
    <TooltipProvider>
      <ProductHeader
        toolbarRef={setHost}
        isMobile={mobile}
        navigationOpen={false}
        onOpenNavigation={vi.fn()}
        onLogin={vi.fn()}
        onLogout={vi.fn()}
        onFeature={vi.fn()}
      />
      <PageToolbarContext.Provider value={host}>
        <PageToolbar inHeader>
          <Counter />
        </PageToolbar>
      </PageToolbarContext.Provider>
    </TooltipProvider>
  );
}
it("changing breakpoint preserves the portal host, child state, and visual keyboard order", () => {
  const view = render(<ResponsiveHeader mobile={false} />);
  const counter = screen.getByRole("button", { name: "Filter 0" });
  const host = counter.closest(".product-header-toolbar");
  const common = view.container.querySelector(".product-header-actions")!;
  expect(
    host!.compareDocumentPosition(common) & Node.DOCUMENT_POSITION_FOLLOWING,
  ).toBeTruthy();
  fireEvent.click(counter);
  view.rerender(<ResponsiveHeader mobile />);
  expect(screen.getByRole("button", { name: "Filter 1" })).toBe(counter);
  expect(counter.closest(".product-header-toolbar")).toBe(host);
  expect(
    common.compareDocumentPosition(host!) & Node.DOCUMENT_POSITION_FOLLOWING,
  ).toBeTruthy();
});
it("standalone consumers fall back inline while a pending shell host waits", () => {
  const view = render(
    <PageToolbar inHeader>
      <button>Inline filter</button>
    </PageToolbar>,
  );
  expect(screen.getByRole("button", { name: "Inline filter" })).toBeTruthy();
  view.rerender(
    <PageToolbarContext.Provider value={null}>
      <PageToolbar inHeader>
        <button>Inline filter</button>
      </PageToolbar>
    </PageToolbarContext.Provider>,
  );
  expect(screen.queryByRole("button", { name: "Inline filter" })).toBeNull();
});
