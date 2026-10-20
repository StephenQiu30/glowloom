import { fireEvent, render, screen } from "@testing-library/react";
import { expect, it, vi } from "vitest";
import { TextIntentInput } from "@/components/generation/text-intent-input";
it("releases the composing parameter owner on unmount", () => {
  const state = vi.fn();
  const view = render(
    <TextIntentInput
      aria-label="negative"
      value=""
      onEdit={() => {}}
      onCompositionState={state}
    />,
  );
  fireEvent.compositionStart(screen.getByLabelText("negative"));
  expect(state).toHaveBeenLastCalledWith(true);
  view.unmount();
  expect(state).toHaveBeenLastCalledWith(false);
});
