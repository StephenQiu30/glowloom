import {
  expect,
  test,
  type BrowserContext,
  type Page,
  type Response,
} from "@playwright/test";
import { createHash, randomBytes, randomUUID } from "node:crypto";
import { execFile } from "node:child_process";
import { mkdtemp, readFile, stat, writeFile } from "node:fs/promises";
import { basename, join } from "node:path";
import { promisify } from "node:util";
import { deflateSync } from "node:zlib";

// 仅使用本地自创的噪声图片、测试图案视频和正弦波音频，不调用生成供应商。
const projectId = process.env.LV_E2E_PROJECT_ID;
const run = promisify(execFile);
const formats = [
  "jpg",
  "png",
  "webp",
  "mp4",
  "mov",
  "mp3",
  "wav",
  "m4a",
] as const;
type Format = (typeof formats)[number];
type Asset = {
  id: string;
  project_id: string;
  kind: "image" | "video" | "audio";
  file_name: string;
  mime_type: string;
  byte_size: number;
  width?: number;
  height?: number;
  duration_ms?: number;
  revision: number;
};
type UploadResult = { asset: Asset; duplicate_of: string | null };
type CanvasDocument = {
  id: string;
  project_id: string;
  revision: number;
  nodes: {
    id: string;
    node_type: string;
    title: string;
    ref_type?: string;
    ref_id?: string;
    config: Record<string, unknown>;
  }[];
  edges: { id: string }[];
  viewport: { x: number; y: number; zoom: number };
};
type Original = { byteSize: number; sha256: string };
type Fixture = Original & {
  path: string;
  kind: Asset["kind"];
  mime: string;
  width?: number;
  height?: number;
};
let fixtures: Record<Format, Fixture>;
let largePNG: Fixture;

test.skip(!projectId, "需要 LV_E2E_PROJECT_ID 指向明确授权的隔离验收项目");
test.setTimeout(180_000);

function sha256(bytes: Uint8Array) {
  return createHash("sha256").update(bytes).digest("hex");
}
function crc32(bytes: Uint8Array) {
  let crc = 0xffffffff;
  for (const byte of bytes) {
    crc ^= byte;
    for (let bit = 0; bit < 8; bit++)
      crc = (crc >>> 1) ^ (crc & 1 ? 0xedb88320 : 0);
  }
  return (crc ^ 0xffffffff) >>> 0;
}
function chunk(name: string, bytes: Buffer) {
  const type = Buffer.from(name, "ascii"),
    header = Buffer.alloc(4),
    checksum = Buffer.alloc(4);
  header.writeUInt32BE(bytes.length);
  checksum.writeUInt32BE(crc32(Buffer.concat([type, bytes])));
  return Buffer.concat([header, type, bytes, checksum]);
}
async function makePNG(path: string, width: number, height: number) {
  const header = Buffer.alloc(13);
  header.writeUInt32BE(width);
  header.writeUInt32BE(height, 4);
  header[8] = 8;
  header[9] = 2; // RGB，无滤镜的自创随机像素，没有真人素材。
  const pixels = randomBytes((width * 3 + 1) * height);
  for (let row = 0; row < height; row++) pixels[row * (width * 3 + 1)] = 0;
  await writeFile(
    path,
    Buffer.concat([
      Buffer.from("89504e470d0a1a0a", "hex"),
      chunk("IHDR", header),
      chunk("IDAT", deflateSync(pixels)),
      chunk("IEND", Buffer.alloc(0)),
    ]),
  );
}
async function ffmpeg(args: string[]) {
  await run(
    "ffmpeg",
    ["-nostdin", "-hide_banner", "-loglevel", "error", "-y", ...args],
    { timeout: 30_000, maxBuffer: 1 << 20 },
  );
}
async function fixture(
  path: string,
  kind: Asset["kind"],
  mime: string,
  width?: number,
  height?: number,
): Promise<Fixture> {
  const bytes = await readFile(path);
  return {
    path,
    kind,
    mime,
    width,
    height,
    byteSize: bytes.length,
    sha256: sha256(bytes),
  };
}

test.beforeAll(async () => {
  const directory = await mkdtemp("/tmp/lanverse-canvas-media-fixtures-");
  const prefix = `self-made-${randomUUID().slice(0, 8)}`;
  const paths = Object.fromEntries(
    formats.map((format) => [format, join(directory, `${prefix}.${format}`)]),
  ) as Record<Format, string>;
  await makePNG(paths.png, 320, 180);
  await Promise.all([
    ffmpeg(["-i", paths.png, "-frames:v", "1", "-q:v", "2", paths.jpg]),
    run("cwebp", ["-quiet", paths.png, "-o", paths.webp], {
      timeout: 30_000,
      maxBuffer: 1 << 20,
    }),
    ffmpeg([
      "-f",
      "lavfi",
      "-i",
      "testsrc2=size=320x180:rate=12",
      "-f",
      "lavfi",
      "-i",
      "sine=frequency=523:sample_rate=44100",
      "-t",
      "1",
      "-c:v",
      "libx264",
      "-preset",
      "ultrafast",
      "-pix_fmt",
      "yuv420p",
      "-c:a",
      "aac",
      "-movflags",
      "+faststart",
      paths.mp4,
    ]),
    ffmpeg([
      "-f",
      "lavfi",
      "-i",
      "sine=frequency=440:sample_rate=44100",
      "-t",
      "1",
      "-c:a",
      "libmp3lame",
      paths.mp3,
    ]),
    ffmpeg([
      "-f",
      "lavfi",
      "-i",
      "sine=frequency=440:sample_rate=44100",
      "-t",
      "1",
      "-c:a",
      "pcm_s16le",
      paths.wav,
    ]),
    ffmpeg([
      "-f",
      "lavfi",
      "-i",
      "sine=frequency=440:sample_rate=44100",
      "-t",
      "1",
      "-c:a",
      "aac",
      "-f",
      "ipod",
      paths.m4a,
    ]),
  ]);
  await ffmpeg(["-i", paths.mp4, "-c", "copy", "-f", "mov", paths.mov]);
  const mimes: Record<Format, string> = {
    jpg: "image/jpeg",
    png: "image/png",
    webp: "image/webp",
    mp4: "video/mp4",
    mov: "video/quicktime",
    mp3: "audio/mpeg",
    wav: "audio/wave",
    m4a: "audio/mp4",
  };
  fixtures = Object.fromEntries(
    await Promise.all(
      formats.map(async (format) => [
        format,
        await fixture(
          paths[format],
          ["jpg", "png", "webp"].includes(format)
            ? "image"
            : ["mp4", "mov"].includes(format)
              ? "video"
              : "audio",
          mimes[format],
          ["jpg", "png", "webp", "mp4", "mov"].includes(format)
            ? 320
            : undefined,
          ["jpg", "png", "webp", "mp4", "mov"].includes(format)
            ? 180
            : undefined,
        ),
      ]),
    ),
  ) as Record<Format, Fixture>;
  const largePath = join(directory, `${prefix}-over-10mib.png`);
  await makePNG(largePath, 2400, 1600);
  largePNG = await fixture(largePath, "image", "image/png", 2400, 1600);
  expect((await stat(largePath)).size).toBeGreaterThan(10 << 20);
  expect(largePNG.byteSize).toBeLessThan(20 << 20);
  await writeFile(
    join(directory, "manifest.json"),
    JSON.stringify({ fixtures, largePNG }, null, 2),
  );
  console.info(
    `自创测试素材：${directory}；大 PNG ${largePNG.byteSize} 字节。`,
  );
});

test.beforeEach(async ({ page, context }) => {
  await context.grantPermissions(["clipboard-read", "clipboard-write"]);
  await page.goto(`/canvas?project=${projectId}`);
});
async function read(page: Page, id: string): Promise<CanvasDocument> {
  const response = await page.request.get(`/api/canvases/${id}`);
  expect(response.status()).toBe(200);
  return response.json();
}
async function create(page: Page) {
  await page
    .getByLabel("新画布名称", { exact: true })
    .fill(`媒体接管验收-${randomUUID().slice(0, 8)}`);
  await page.getByRole("button", { name: "创建画布", exact: true }).click();
  await expect(
    page.getByRole("region", { name: "画布编辑区", exact: true }),
  ).toBeVisible();
  const id = new URL(page.url()).searchParams.get("canvas")!;
  expect(id).toMatch(/^[0-9a-f-]{36}$/);
  await writeFile(
    test.info().outputPath(`本测试画布-${id}.json`),
    JSON.stringify({ projectId, canvasId: id }),
  );
  return id;
}
async function saved(page: Page, id: string, previous: number) {
  await expect
    .poll(async () => (await read(page, id)).revision)
    .toBe(previous + 1);
  const document = await read(page, id);
  await expect(
    page
      .getByRole("status")
      .filter({ hasText: `已保存 · 修订 ${document.revision}` }),
  ).toBeVisible();
  return document;
}
function uploadResponse(response: Response) {
  return (
    new URL(response.url()).pathname ===
    `/api/projects/${projectId}/media/uploads`
  );
}
async function openPicker(page: Page, paths: string[]) {
  await page.getByRole("button", { name: "上传媒体", exact: true }).click();
  const dialog = page.getByRole("dialog", {
    name: "上传媒体到画布",
    exact: true,
  });
  await expect(dialog).toBeVisible();
  await dialog
    .getByLabel("选择本地媒体文件", { exact: true })
    .setInputFiles(paths);
}
async function importConfirmed(
  page: Page,
  id: string,
  document: CanvasDocument,
  count: number,
) {
  const dialog = page.getByRole("dialog", {
    name: "上传媒体到画布",
    exact: true,
  });
  await expect(
    dialog.getByRole("button", { name: "开始上传", exact: true }),
  ).toBeDisabled();
  await dialog
    .getByRole("checkbox", {
      name: "我已检查内容，拥有使用权，且不含需要授权的真人素材",
      exact: true,
    })
    .check();
  const responses: Response[] = [];
  let rejectUpload: (error: Error) => void;
  const failedUpload = new Promise<never>((_, reject) => {
    rejectUpload = reject;
  });
  const capture = (response: Response) => {
    if (uploadResponse(response) && response.request().method() === "POST") {
      responses.push(response);
      if (response.status() !== 201)
        rejectUpload(new Error(`正式媒体上传返回 HTTP ${response.status()}`));
    }
  };
  page.on("response", capture);
  try {
    const command = page.waitForResponse(
      (response) =>
        new URL(response.url()).pathname === `/api/canvases/${id}/commands` &&
        response.request().method() === "POST",
      { timeout: 120_000 },
    );
    const [response] = await Promise.all([
      command,
      Promise.race([
        expect(dialog).toHaveCount(0, { timeout: 120_000 }),
        failedUpload,
      ]),
      dialog.getByRole("button", { name: "开始上传", exact: true }).click(),
    ]);
    expect(response.status()).toBe(200);
    const body = response.request().postDataJSON();
    expect(body.expected_revision).toBe(document.revision);
    expect(body.commands.map((item: { type: string }) => item.type)).toEqual([
      "AddNodes",
    ]);
    expect(responses).toHaveLength(count);
    return {
      document: await saved(page, id, document.revision),
      results: await Promise.all(
        responses.map((item) => item.json() as Promise<UploadResult>),
      ),
    };
  } finally {
    page.off("response", capture);
  }
}
async function verifyOriginal(
  page: Page,
  result: UploadResult,
  original: Original,
  expected?: Fixture,
) {
  const { asset, duplicate_of: duplicateOf } = result;
  expect(duplicateOf === null || duplicateOf === asset.id).toBe(true);
  expect(asset.project_id).toBe(projectId);
  expect(asset.byte_size).toBe(original.byteSize);
  if (expected) {
    expect(asset).toMatchObject({
      kind: expected.kind,
      mime_type: expected.mime,
    });
    // 重复内容复用已有资产及其原文件名；只有首次接管才使用本次选择的名称。
    if (duplicateOf === null)
      expect(asset.file_name).toBe(basename(expected.path));
    if (expected.width) expect(asset.width).toBe(expected.width);
    if (expected.height) expect(asset.height).toBe(expected.height);
    if (expected.kind !== "image") expect(asset.duration_ms).toBeGreaterThan(0);
  }
  // 正式 preview 会重新验证 same-project、ready/passed；DTO 不暴露原始对象键或审核详情。
  const response = await page.request.get(
    `/api/projects/${projectId}/media/${asset.id}/preview`,
  );
  expect(response.status()).toBe(200);
  const preview = (await response.json()) as {
    asset: Asset;
    url: string;
    expires_at: string;
  };
  expect(preview.asset).toEqual(asset);
  expect(Date.parse(preview.expires_at)).toBeGreaterThan(Date.now());
  const object = await page.request.get(preview.url);
  expect(object.status()).toBe(200);
  const bytes = await object.body();
  expect(bytes.length).toBe(original.byteSize);
  expect(sha256(bytes)).toBe(original.sha256);
  // 审阅附件只留自产内容的校验信息，不保留签名 URL。
  await writeFile(
    test.info().outputPath(`原始对象-${asset.id}.json`),
    JSON.stringify(
      {
        asset,
        byteSize: bytes.length,
        sha256: sha256(bytes),
        eligibleThroughFormalPreview: true,
      },
      null,
      2,
    ),
  );
}
async function fit(page: Page, id: string, document: CanvasDocument) {
  await page.getByRole("button", { name: "显示全部节点", exact: true }).click();
  return saved(page, id, document.revision);
}
async function fullPreview(
  page: Page,
  node: CanvasDocument["nodes"][number],
  asset: Asset,
) {
  const canvasNode = page.locator(`[data-node-id="${node.id}"]`);
  await canvasNode.click({ position: { x: 20, y: 20 } });
  const inline =
    asset.kind === "image"
      ? undefined
      : canvasNode.locator(asset.kind === "video" ? "video" : "audio");
  let playing:
    import("@playwright/test").ElementHandle<HTMLMediaElement> | null = null;
  if (inline) {
    await expect
      .poll(() =>
        inline.evaluate((element: HTMLMediaElement) => element.readyState),
      )
      .toBeGreaterThanOrEqual(2);
    playing = (await inline.elementHandle()) as
      import("@playwright/test").ElementHandle<HTMLMediaElement> | null;
    expect(playing).not.toBeNull();
    await inline.evaluate(async (element: HTMLMediaElement) => {
      element.muted = true;
      element.currentTime = 0;
      await element.play();
    });
    await expect
      .poll(() =>
        inline.evaluate((element: HTMLMediaElement) => element.currentTime),
      )
      .toBeGreaterThan(0);
  }
  await page.getByRole("button", { name: "预览素材", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: node.title, exact: true });
  await expect(dialog).toBeVisible();
  if (playing) {
    await expect
      .poll(() =>
        playing!.evaluate((element) => ({
          paused: element.paused,
          source: element.getAttribute("src"),
          connected: element.isConnected,
        })),
      )
      .toEqual({ paused: true, source: null, connected: false });
    await playing.dispose();
  }
  if (asset.kind === "image") {
    const image = dialog.getByRole("img", { name: node.title, exact: true });
    await expect
      .poll(() =>
        image.evaluate((element: HTMLImageElement) => ({
          complete: element.complete,
          width: element.naturalWidth,
          height: element.naturalHeight,
        })),
      )
      .toEqual({ complete: true, width: asset.width, height: asset.height });
  } else {
    const media = dialog.locator(asset.kind === "video" ? "video" : "audio");
    await expect
      .poll(() =>
        media.evaluate((element: HTMLMediaElement) => element.readyState),
      )
      .toBeGreaterThanOrEqual(2);
    await media.evaluate(async (element: HTMLMediaElement) => {
      element.muted = true;
      element.currentTime = 0;
      await element.play();
    });
    await expect
      .poll(() => media.evaluate((element: HTMLMediaElement) => element.ended))
      .toBe(true);
    expect(
      await media.evaluate((element: HTMLMediaElement) => element.error),
    ).toBeNull();
  }
  await dialog.getByRole("button", { name: "Close", exact: true }).click();
  await expect(dialog).toHaveCount(0);
}
async function restored(page: Page, id: string, document: CanvasDocument) {
  await page.reload();
  await expect(
    page.getByRole("region", { name: "画布编辑区", exact: true }),
  ).toBeVisible();
  expect(await read(page, id)).toEqual(document);
  for (const node of document.nodes) {
    expect(node.ref_type).toBe("media_asset");
    expect(node.ref_id).toMatch(/^[0-9a-f-]{36}$/);
    expect(node.config).toEqual({});
    await expect(page.locator(`[data-node-id="${node.id}"]`)).toBeVisible();
  }
  const json = test.info().outputPath(`刷新后正式画布-${id}.json`),
    screenshot = test.info().outputPath(`刷新后媒体画布-${id}.png`);
  await writeFile(json, JSON.stringify(document, null, 2));
  await page
    .getByRole("region", { name: "无限画布编辑器", exact: true })
    .screenshot({ path: screenshot });
  await test.info().attach("刷新后正式媒体画布", {
    path: json,
    contentType: "application/json",
  });
  await test.info().attach("刷新后媒体画布截图", {
    path: screenshot,
    contentType: "image/png",
  });
}
async function dragFile(context: BrowserContext, page: Page, path: string) {
  const viewport = page.locator("[data-canvas-viewport]");
  await viewport.scrollIntoViewIfNeeded();
  const box = await viewport.boundingBox();
  expect(box).not.toBeNull();
  const session = await context.newCDPSession(page);
  try {
    for (const type of ["dragEnter", "dragOver", "drop"] as const)
      await session.send("Input.dispatchDragEvent", {
        type,
        x: box!.x + 80,
        y: box!.y + 160,
        data: { items: [], files: [path], dragOperationsMask: 1 },
      });
  } finally {
    await session.detach();
  }
  await expect(
    page.getByRole("dialog", { name: "上传媒体到画布", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("list", { name: "上传文件列表", exact: true }),
  ).toContainText(basename(path));
}

test("文件选择器接管 JPG/PNG/WebP 和大于10MiB PNG，复用资产与撤销不删媒体", async ({
  page,
}) => {
  const id = await create(page);
  let document = await read(page, id);
  const images = [fixtures.jpg, fixtures.png, fixtures.webp, largePNG];
  await openPicker(
    page,
    images.map((item) => item.path),
  );
  const imported = await importConfirmed(page, id, document, images.length);
  document = imported.document;
  expect(document.nodes).toHaveLength(4);
  for (let index = 0; index < images.length; index++) {
    const asset = imported.results[index].asset;
    expect(
      document.nodes.find((node) => node.ref_id === asset.id),
    ).toBeTruthy();
    await verifyOriginal(
      page,
      imported.results[index],
      images[index],
      images[index],
    );
  }
  document = await fit(page, id, document);
  for (let index = 0; index < images.length; index++)
    await fullPreview(
      page,
      document.nodes.find(
        (node) => node.ref_id === imported.results[index].asset.id,
      )!,
      imported.results[index].asset,
    );

  await openPicker(page, [fixtures.jpg.path]);
  const reused = await importConfirmed(page, id, document, 1);
  document = reused.document;
  expect(reused.results[0].duplicate_of).toBe(imported.results[0].asset.id);
  expect(reused.results[0].asset.id).toBe(imported.results[0].asset.id);
  expect(document.nodes).toHaveLength(5);
  await page.getByRole("button", { name: "撤销", exact: true }).click();
  document = await saved(page, id, document.revision);
  expect(document.nodes).toHaveLength(4);
  await verifyOriginal(page, reused.results[0], fixtures.jpg, fixtures.jpg);
  await page.getByRole("button", { name: "重做", exact: true }).click();
  document = await saved(page, id, document.revision);
  expect(document.nodes).toHaveLength(5);
  await restored(page, id, document);
});

test("真实文件拖入 MP4 与选择器 MOV，完整视频预览后刷新", async ({
  page,
  context,
}) => {
  const id = await create(page);
  let document = await read(page, id);
  await dragFile(context, page, fixtures.mp4.path);
  const mp4 = await importConfirmed(page, id, document, 1);
  document = mp4.document;
  await verifyOriginal(page, mp4.results[0], fixtures.mp4, fixtures.mp4);
  await openPicker(page, [fixtures.mov.path]);
  const mov = await importConfirmed(page, id, document, 1);
  document = mov.document;
  await verifyOriginal(page, mov.results[0], fixtures.mov, fixtures.mov);
  document = await fit(page, id, document);
  await fullPreview(
    page,
    document.nodes.find((node) => node.ref_id === mp4.results[0].asset.id)!,
    mp4.results[0].asset,
  );
  await fullPreview(
    page,
    document.nodes.find((node) => node.ref_id === mov.results[0].asset.id)!,
    mov.results[0].asset,
  );
  await restored(page, id, document);
});

test("MP3/WAV/M4A 完整播放和真正 ClipboardItem 图片接管", async ({ page }) => {
  const id = await create(page);
  let document = await read(page, id);
  const audio = [fixtures.mp3, fixtures.wav, fixtures.m4a];
  await openPicker(
    page,
    audio.map((item) => item.path),
  );
  const imported = await importConfirmed(page, id, document, 3);
  document = imported.document;
  for (let index = 0; index < audio.length; index++)
    await verifyOriginal(
      page,
      imported.results[index],
      audio[index],
      audio[index],
    );
  document = await fit(page, id, document);
  for (let index = 0; index < audio.length; index++)
    await fullPreview(
      page,
      document.nodes.find(
        (node) => node.ref_id === imported.results[index].asset.id,
      )!,
      imported.results[index].asset,
    );

  const bytes = await readFile(fixtures.png.path);
  const original = await page.evaluate(async (values) => {
    const blob = new Blob([new Uint8Array(values)], { type: "image/png" });
    await navigator.clipboard.write([new ClipboardItem({ "image/png": blob })]);
    // 浏览器可以重新编码 PNG；实际系统剪贴板读出的字节才是上传输入。
    const item = (await navigator.clipboard.read())[0],
      actual = await item.getType("image/png"),
      input = await actual.arrayBuffer();
    const digest = await crypto.subtle.digest("SHA-256", input);
    return {
      byteSize: input.byteLength,
      sha256: Array.from(new Uint8Array(digest))
        .map((value) => value.toString(16).padStart(2, "0"))
        .join(""),
    };
  }, Array.from(bytes));
  await page.getByRole("button", { name: "粘贴", exact: true }).click();
  await expect(
    page.getByRole("dialog", { name: "上传媒体到画布", exact: true }),
  ).toBeVisible();
  const image = await importConfirmed(page, id, document, 1);
  document = image.document;
  expect(image.results[0].asset.kind).toBe("image");
  expect(image.results[0].asset.mime_type).toBe("image/png");
  await verifyOriginal(page, image.results[0], original);
  document = await fit(page, id, document);
  await fullPreview(
    page,
    document.nodes.find((node) => node.ref_id === image.results[0].asset.id)!,
    image.results[0].asset,
  );
  await restored(page, id, document);
});

test("冒名文件真实415后同幂等键重试，可仅保存已经成功的文件", async ({
  page,
}) => {
  const id = await create(page);
  let document = await read(page, id);
  await page.getByRole("button", { name: "上传媒体", exact: true }).click();
  const dialog = page.getByRole("dialog", {
    name: "上传媒体到画布",
    exact: true,
  });
  const spoof = {
    name: `self-made-${randomUUID().slice(0, 8)}-mp3-as-png.png`,
    mimeType: "image/png",
    buffer: await readFile(fixtures.mp3.path),
  };
  await dialog.getByLabel("选择本地媒体文件", { exact: true }).setInputFiles([
    spoof,
    {
      name: basename(fixtures.png.path),
      mimeType: "image/png",
      buffer: await readFile(fixtures.png.path),
    },
  ]);
  await dialog
    .getByRole("checkbox", {
      name: "我已检查内容，拥有使用权，且不含需要授权的真人素材",
      exact: true,
    })
    .check();
  const rejected = page.waitForResponse(
    (response) => uploadResponse(response) && response.status() === 415,
  );
  const accepted = page.waitForResponse(
    (response) => uploadResponse(response) && response.status() === 201,
  );
  await dialog.getByRole("button", { name: "开始上传", exact: true }).click();
  const failure = await rejected,
    success = await accepted,
    result = (await success.json()) as UploadResult;
  await expect(
    dialog.getByRole("button", { name: "重试未成功文件", exact: true }),
  ).toBeEnabled();
  expect(await read(page, id)).toEqual(document);
  const rejectedAgain = page.waitForResponse(
    (response) => uploadResponse(response) && response.status() === 415,
  );
  await dialog
    .getByRole("button", { name: "重试未成功文件", exact: true })
    .click();
  const retry = await rejectedAgain;
  expect(retry.request().headers()["idempotency-key"]).toBe(
    failure.request().headers()["idempotency-key"],
  );
  await expect(
    dialog.getByRole("button", { name: "将成功的 1 个加入画布", exact: true }),
  ).toBeEnabled();
  await dialog
    .getByRole("button", { name: "将成功的 1 个加入画布", exact: true })
    .click();
  await expect(dialog).toHaveCount(0);
  document = await saved(page, id, document.revision);
  expect(document.nodes).toHaveLength(1);
  expect(document.nodes[0].ref_id).toBe(result.asset.id);
  await verifyOriginal(page, result, fixtures.png, fixtures.png);
  document = await fit(page, id, document);
  await fullPreview(page, document.nodes[0], result.asset);
  await restored(page, id, document);
});
