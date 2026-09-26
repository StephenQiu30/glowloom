import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";

import ProjectsError from "./error";

afterEach(cleanup);

it("offers recovery without revealing the internal error", () => {
  const retry = vi.fn();
  render(
    <ProjectsError error={new Error("private backend detail")} retry={retry} />,
  );

  expect(screen.getByRole("alert")).toBeTruthy();
  expect(screen.queryByText("private backend detail")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "重试加载" }));
  expect(retry).toHaveBeenCalledOnce();
});
