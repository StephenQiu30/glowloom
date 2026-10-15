import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { BoundAvatar } from "./avatar-editor";
vi.mock("@/components/ui/avatar", () => ({
  Avatar: ({ children }: { children: React.ReactNode }) => (
    <span>{children}</span>
  ),
  AvatarImage: ({ src, alt }: { src: string; alt: string }) => (
    <span role="img" aria-label={alt} data-src={src} />
  ),
  AvatarFallback: ({ children }: { children: React.ReactNode }) => (
    <span>{children}</span>
  ),
}));
afterEach(cleanup);
it("成员头像从已绑定变为空时立即清除旧URL", async () => {
  const load = vi.fn().mockResolvedValue({
    url: "https://example.com/old",
    expires_at: new Date(Date.now() + 60000).toISOString(),
  });
  const view = render(<BoundAvatar assetID="old" load={load} />);
  const { waitFor } = await import("@testing-library/react");
  await waitFor(() =>
    expect(screen.getByRole("img")).toHaveProperty(
      "dataset.src",
      "https://example.com/old",
    ),
  );
  view.rerender(<BoundAvatar assetID="" load={load} />);
  expect(screen.getByRole("img").getAttribute("data-src")).toBe("");
});
