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

// Sonner follows the viewport media query; jsdom has no browser media engine.
if (typeof window !== "undefined" && !window.matchMedia) {
  window.matchMedia = (query: string) => ({
    matches: false,
    media: query,
    onchange: null,
    addListener: vi.fn(),
    removeListener: vi.fn(),
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    dispatchEvent: vi.fn(),
  });
}

// Pointer capture is a browser layout API used by Sonner swipe handling.
if (
  typeof HTMLElement !== "undefined" &&
  !HTMLElement.prototype.setPointerCapture
) {
  HTMLElement.prototype.setPointerCapture = () => undefined;
  HTMLElement.prototype.releasePointerCapture = () => undefined;
}
