import "@testing-library/jest-dom/vitest";
import { cleanup } from "@testing-library/react";
import { afterEach, beforeEach } from "vitest";

import { clearAccessToken } from "@/lib/auth-session";

class ResizeObserverMock implements ResizeObserver {
  disconnect(): void {}

  observe(): void {}

  unobserve(): void {}
}

globalThis.ResizeObserver = ResizeObserverMock;
HTMLElement.prototype.scrollIntoView = () => undefined;
HTMLElement.prototype.hasPointerCapture = () => false;
HTMLElement.prototype.setPointerCapture = () => undefined;
HTMLElement.prototype.releasePointerCapture = () => undefined;

beforeEach(() => {
  clearAccessToken();
});

afterEach(() => {
  clearAccessToken();
  cleanup();
});
