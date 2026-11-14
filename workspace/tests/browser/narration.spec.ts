import { test, expect } from "@playwright/test";
import { readFile } from "node:fs/promises";
// Explicit silent PCM fixture: validates native media behavior, not TTS quality or speech alignment.
function silentWav(seconds: number) {
  const count = 8000 * seconds;
  const b = Buffer.alloc(44 + count * 2);
  b.write("RIFF");
  b.writeUInt32LE(b.length - 8, 4);
  b.write("WAVEfmt ", 8);
  b.writeUInt32LE(16, 16);
  b.writeUInt16LE(1, 20);
  b.writeUInt16LE(1, 22);
  b.writeUInt32LE(8000, 24);
  b.writeUInt32LE(16000, 28);
  b.writeUInt16LE(2, 32);
  b.writeUInt16LE(16, 34);
  b.write("data", 36);
  b.writeUInt32LE(count * 2, 40);
  return b;
}
test("plays existing audio with native seek, rate, paragraph cues, and manual follow control", async ({
  page,
}) => {
  const snapshot = JSON.parse(
    await readFile(".build/static/content-snapshot.json", "utf8"),
  );
  const doc = snapshot.documents.find(
    (d: { id: string }) => d.id === "design-43",
  );
  const blocks = doc.blocks.slice(0, 2);
  await page.route("**/narration/index.json", (r) =>
    r.fulfill({ json: { "design-43": "design-43/manifest.json" } }),
  );
  await page.route("**/narration/design-43/manifest.json", (r) =>
    r.fulfill({
      json: {
        documentId: doc.id,
        revision: doc.revision,
        scope: "internal",
        configDigest: "fixture-silent-pcm",
        precision: "paragraph",
        segments: [
          {
            id: "fixture",
            src: "fixture.wav",
            durationMs: 4000,
            sha256: "a".repeat(64),
            cues: blocks.map((b: { id: string; text: string }, i: number) => ({
              startMs: i * 2000,
              endMs: (i + 1) * 2000,
              blockId: b.id,
              textStart: 0,
              textEnd: b.text.length,
            })),
          },
        ],
      },
    }),
  );
  await page.route("**/narration/design-43/fixture.wav", (r) => {
    const bytes = silentWav(4),
      range = r
        .request()
        .headers()
        .range?.match(/bytes=(\d+)-(\d*)/);
    const start = range ? Number(range[1]) : 0,
      end = range && range[2] ? Number(range[2]) : bytes.length - 1;
    return r.fulfill({
      status: range ? 206 : 200,
      contentType: "audio/wav",
      headers: {
        "accept-ranges": "bytes",
        ...(range
          ? { "content-range": `bytes ${start}-${end}/${bytes.length}` }
          : {}),
      },
      body: bytes.subarray(start, end + 1),
    });
  });
  await page.goto("/read/design-43/");
  const audio = page.locator("audio");
  await expect(audio).toBeVisible();
  await expect
    .poll(() => audio.evaluate((e: HTMLAudioElement) => e.readyState))
    .toBeGreaterThanOrEqual(4);
  await audio.evaluate((e: HTMLAudioElement) => {
    e.currentTime = 2.3;
  });
  await expect(page.locator(`[id="${blocks[1].id}"]`)).toHaveAttribute(
    "data-speaking",
    "true",
  );
  await page.getByLabel("播放速度").selectOption("1.5");
  expect(await audio.evaluate((e: HTMLAudioElement) => e.playbackRate)).toBe(
    1.5,
  );
  await page.getByLabel("跟随阅读").check();
  await page.mouse.wheel(0, 200);
  await expect(page.getByLabel("跟随阅读")).not.toBeChecked();
  await page.getByRole("button", { name: "回到朗读位置" }).click();
  await expect(page.getByLabel("跟随阅读")).toBeChecked();
  await expect(page.getByLabel("朗读范围")).toBeVisible();
  await page
    .getByLabel("朗读范围")
    .selectOption(
      doc.headings.find((h: { depth: number }) => h.depth === 2).id,
    );
  expect(
    await audio.evaluate((e: HTMLAudioElement) => e.currentTime),
  ).toBeGreaterThanOrEqual(1.9);
  await audio.evaluate((e: HTMLAudioElement) => e.play());
  await expect
    .poll(() => audio.evaluate((e: HTMLAudioElement) => e.paused))
    .toBe(false);
  await page.goto("/read/prd-29/");
  await expect(page.locator("audio")).toHaveCount(0);
});
