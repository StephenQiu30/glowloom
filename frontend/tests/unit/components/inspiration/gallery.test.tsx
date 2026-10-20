import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { InspirationGallery } from "@/components/inspiration/gallery";

afterEach(cleanup);

it("returns focus to the exact card that opened the preview", async () => {
  render(<InspirationGallery />);
  expect(
    screen.getByRole("tab", { name: "精选" }).getAttribute("aria-controls"),
  ).toBe(screen.getByRole("tabpanel", { name: "精选" }).id);
  const card = screen.getByRole("button", { name: /^光影之间/ });
  card.focus();
  fireEvent.click(card);
  expect(await screen.findByRole("dialog", { name: "光影之间" })).toBeTruthy();
  fireEvent.keyDown(document, { key: "Escape" });
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  await waitFor(() => expect(document.activeElement).toBe(card));
});
