import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { afterEach, expect, it, vi } from "vitest";

import { Providers } from "./providers";

afterEach(cleanup);

it("shares a query between children and refetches it after invalidation", async () => {
  let revision = 0;
  const queryFn = vi.fn(async () => ++revision);

  function Reader({ label }: { label: string }) {
    const { data } = useQuery({ queryKey: ["projects"], queryFn });
    return <p>{`${label}: ${data ?? "loading"}`}</p>;
  }

  function Refresh() {
    const client = useQueryClient();
    return (
      <button
        onClick={() =>
          void client.invalidateQueries({ queryKey: ["projects"] })
        }
      >
        刷新
      </button>
    );
  }

  render(
    <Providers>
      <Reader label="one" />
      <Reader label="two" />
      <Refresh />
    </Providers>,
  );

  await waitFor(() => expect(screen.getByText("two: 1")).toBeTruthy());
  expect(screen.getByText("one: 1")).toBeTruthy();
  expect(queryFn).toHaveBeenCalledTimes(1);

  fireEvent.click(screen.getByRole("button", { name: "刷新" }));
  await waitFor(() => expect(screen.getByText("two: 2")).toBeTruthy());
  expect(screen.getByText("one: 2")).toBeTruthy();
  expect(queryFn).toHaveBeenCalledTimes(2);
});
