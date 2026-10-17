import {
  render as testingRender,
  waitFor,
  type RenderOptions,
} from "@testing-library/react";
import { createElement, type ReactNode } from "react";
import { afterEach } from "vitest";
import { toast } from "sonner";
import { Toaster } from "@/components/ui/sonner";

afterEach(() => {
  toast.dismiss();
});

/** Exercise feedback with the same single root Toaster used by the application. */
export function render(ui: ReactNode, options?: RenderOptions) {
  const Wrapper = options?.wrapper;
  return testingRender(ui, {
    ...options,
    wrapper: ({ children }) => (
      <>
        <Toaster />
        {Wrapper ? createElement(Wrapper, null, children) : children}
      </>
    ),
  });
}
export function findFeedback(message?: string) {
  return waitFor(() => {
    const toast = document.querySelector<HTMLElement>(
      '[data-sonner-toast]:not([data-removed="true"])',
    );
    if (!toast || (message && !toast.textContent?.includes(message)))
      throw new Error("Expected a Sonner notification");
    return toast.querySelector<HTMLElement>("[data-title]") ?? toast;
  });
}
