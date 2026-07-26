import { expect, test } from "@playwright/test";

test("creates an empty-state project and enters its Story workspace", async ({ page }) => {
  let intentKey = "";
  await page.route("http://127.0.0.1:8000/v1/**", async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    const headers = { "access-control-allow-origin": "*", "content-type": "application/json" };
    if (path === "/v1/projects" && request.method() === "POST") {
      intentKey = request.headers()["idempotency-key"] ?? "";
      expect(request.postDataJSON()).toEqual({ title: "雾中来信" });
      await route.fulfill({
        headers,
        json: {
          project: {
            id: "project-1",
            title: "雾中来信",
            status: "active",
            production_spec: {
              aspect_ratio: "9:16",
              width: 720,
              height: 1280,
              fps: 24,
              timebase: 90000,
              target_min_ticks: 2700000,
              target_max_ticks: 5400000,
            },
            created_at: "2030-07-25T12:00:00Z",
            updated_at: "2030-07-25T12:00:00Z",
          },
          episode: {
            id: "episode-1",
            project_id: "project-1",
            target_min_ticks: 2700000,
            target_max_ticks: 5400000,
            current_source_revision_id: null,
            created_at: "2030-07-25T12:00:00Z",
            updated_at: "2030-07-25T12:00:00Z",
          },
        },
      });
      return;
    }
    await route.fulfill({ headers, json: { items: [] } });
  });

  await page.goto("/projects");
  await expect(page.getByText("还没有短剧项目")).toBeVisible();
  await page.getByRole("textbox", { name: "项目标题" }).fill("雾中来信");
  await page.getByRole("button", { name: "创建项目" }).click();

  await expect(page).toHaveURL(/\/episodes\/episode-1\/story$/);
  await expect(page.getByRole("heading", { name: "故事与分镜" })).toBeVisible();
  expect(intentKey).toMatch(/^create-project:[0-9a-f-]{36}$/);
});
