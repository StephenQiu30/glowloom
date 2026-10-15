import { vi } from "vitest";

// jsdom does not implement layout observation used by Radix form controls.
// Browser validation covers dimensions; these tests cover events and FormData.
vi.stubGlobal(
  "ResizeObserver",
  class {
    observe() {}
    unobserve() {}
    disconnect() {}
  },
);

// cmdk scrolls keyboard-selected options; jsdom has no scrolling layout.
if (
  typeof HTMLElement !== "undefined" &&
  !HTMLElement.prototype.scrollIntoView
) {
  HTMLElement.prototype.scrollIntoView = vi.fn();
}
