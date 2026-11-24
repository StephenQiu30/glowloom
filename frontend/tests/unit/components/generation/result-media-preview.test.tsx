import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import {
  ResultFrame,
  ResultMediaPreview,
} from "@/components/generation/result-media-preview";

vi.mock("@/components/media/project-media", () => ({
  ProjectMediaPreview: ({
    id,
    playOnOpen,
  }: {
    id: string;
    playOnOpen?: boolean;
  }) => <video aria-label={id} autoPlay={playOnOpen} />,
}));

afterEach(cleanup);

it("reserves the requested portrait frame and mounts the player only after activation", () => {
  const { container } = render(
    <ResultFrame kind="video" ratio="9:16">
      <ResultMediaPreview projectID="project" assetID="clip" kind="video" />
    </ResultFrame>,
  );
  expect((container.firstChild as HTMLElement).style.aspectRatio).toBe(
    "0.5625 / 1",
  );
  expect(screen.queryByLabelText("clip")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "播放生成视频" }));
  expect(screen.getByLabelText<HTMLVideoElement>("clip").autoplay).toBe(true);
});
