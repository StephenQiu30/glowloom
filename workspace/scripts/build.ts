import {
  readFile,
  writeFile,
  mkdir,
  cp,
  rm,
  rename,
  readdir,
  open,
  realpath,
} from "node:fs/promises";
import { existsSync } from "node:fs";
import { resolve, dirname, relative } from "node:path";
import { fileURLToPath } from "node:url";
import { execFileSync } from "node:child_process";
import puppeteer from "puppeteer-core";
import { chromium } from "@playwright/test";
import { renderMermaid } from "@mermaid-js/mermaid-cli";
import { loadSnapshot, validateLinks } from "../src/content/catalog";
import { digest } from "../src/content/parse";
import { sealRelease, selectRelease } from "../src/build/release";
import { categoryNames } from "../src/content/runtime";
import { aliasRedirect } from "../src/build/aliases";
import { copyPortableTree } from "../src/build/package";
import { assertStaticBoundary } from "../src/build/boundary";
import { freezeNarration } from "../src/narration/release";

const workspace = resolve(fileURLToPath(new URL("..", import.meta.url)));
const mode = process.argv[2];
if (mode !== "static" && mode !== "online")
  throw new Error("Expected static or online");
const scope = process.argv.includes("--public") ? "public" : "internal";
const basePath = process.env.WORKSPACE_BASE_PATH ?? "";
if (basePath && !/^(?:\/[a-zA-Z0-9_-]+)+$/.test(basePath))
  throw new Error("Invalid base path");
const stage = resolve(workspace, ".build", mode);
await mkdir(dirname(stage), { recursive: true });
const lock = await open(stage + ".lock", "wx");
const uiFiles = [
  "button",
  "badge",
  "input",
  "textarea",
  "dialog",
  "separator",
  "alert",
  "card",
  "empty",
  "label",
  "field",
];
const run = (name: string, args: string[], cwd = stage) =>
  execFileSync(name, args, {
    cwd,
    stdio: "inherit",
    env: { ...process.env, NEXT_TELEMETRY_DISABLED: "1" },
  });
async function files(dir: string): Promise<string[]> {
  const result: string[] = [];
  for (const entry of await readdir(dir, { withFileTypes: true })) {
    const p = resolve(dir, entry.name);
    if (entry.isDirectory()) result.push(...(await files(p)));
    else if (entry.isFile()) result.push(p);
  }
  return result;
}
function cssBlock(source: string, selector: string) {
  const start = source.indexOf(selector);
  if (start < 0) throw new Error(`Missing shared theme ${selector}`);
  let depth = 0;
  const open = source.indexOf("{", start);
  for (let n = open; n < source.length; n++) {
    if (source[n] === "{") depth++;
    if (source[n] === "}" && --depth === 0) return source.slice(start, n + 1);
  }
  throw new Error("Unbalanced shared theme");
}
async function fingerprint() {
  const sources = (await files(resolve(workspace, "src"))).filter(
    (p) =>
      mode !== "static" ||
      !/^src\/(?:server|online)\//.test(relative(workspace, p)),
  );
  const inputs = sources.concat(
    [
      resolve(workspace, "scripts/build.ts"),
      resolve(workspace, "scripts/prepare-ui.ts"),
      resolve(workspace, "package.json"),
      resolve(workspace, "pnpm-lock.yaml"),
      resolve(workspace, "../frontend/src/app/globals.css"),
    ],
    uiFiles.map((f) =>
      resolve(workspace, "../frontend/src/components/ui", f + ".tsx"),
    ),
  );
  return digest(
    (
      await Promise.all(
        inputs
          .sort()
          .map(
            async (p) =>
              relative(workspace, p) + "\0" + digest(await readFile(p)),
          ),
      )
    ).join("\n"),
  );
}
try {
  if (mode === "static") await assertStaticBoundary(workspace);
  const snapshot = await loadSnapshot({ workspaceRoot: workspace, scope });
  const errors = await validateLinks(snapshot);
  if (errors.length)
    throw new Error("Content links invalid: " + JSON.stringify(errors));
  const codeDigest = await fingerprint();
  const narration = await freezeNarration(
    resolve(workspace, ".local/narration"),
    snapshot.documents,
    scope,
  );
  const releaseId =
    (snapshot.preview ? "preview-" : "") +
    digest(
      [
        snapshot.contentDigest,
        snapshot.repoCommit,
        codeDigest,
        narration.digest,
        mode,
        scope,
        basePath,
      ].join("\0"),
    ).slice(0, 16);
  const final = resolve(workspace, "dist", mode, releaseId);
  if (existsSync(final)) {
    await selectRelease(dirname(final), releaseId, mode);
    console.log(`Existing immutable ${mode} release: ${final}`);
    process.exitCode = 0;
  } else {
    await rm(stage, { recursive: true, force: true });
    await mkdir(resolve(stage, "src"), { recursive: true });
    for (const dir of [
      "app",
      "components",
      "content",
      "narration",
      "workflows",
    ])
      if (existsSync(resolve(workspace, "src", dir)))
        await cp(resolve(workspace, "src", dir), resolve(stage, "src", dir), {
          recursive: true,
        });
    if (mode === "static" && !snapshot.documents.length)
      await rm(resolve(stage, "src/app/(workspace)/read"), {
        recursive: true,
        force: true,
      });
    if (mode === "online") {
      await cp(resolve(workspace, "src/server"), resolve(stage, "src/server"), {
        recursive: true,
      });
      await cp(
        resolve(workspace, "src/online/app"),
        resolve(stage, "src/app"),
        { recursive: true, errorOnExist: true },
      );
      await cp(
        resolve(workspace, "src/online/components"),
        resolve(stage, "src/online/components"),
        { recursive: true },
      );
      await cp(
        resolve(workspace, "src/online/proxy.ts"),
        resolve(stage, "src/proxy.ts"),
      );
    }
    await mkdir(resolve(stage, "src/components/ui"), { recursive: true });
    for (const name of uiFiles)
      await cp(
        resolve(workspace, "../frontend/src/components/ui", name + ".tsx"),
        resolve(stage, "src/components/ui", name + ".tsx"),
      );
    const theme = await readFile(
      resolve(workspace, "../frontend/src/app/globals.css"),
      "utf8",
    );
    const css = await readFile(resolve(stage, "src/app/globals.css"), "utf8");
    await writeFile(
      resolve(stage, "src/app/globals.css"),
      css +
        "\n" +
        ["@theme inline", ":root {", ".dark {"]
          .map((s) => cssBlock(theme, s))
          .join("\n"),
    );
    await writeFile(
      resolve(stage, "postcss.config.mjs"),
      'export default {plugins:{"@tailwindcss/postcss":{}}};\n',
    );
    await cp(
      resolve(workspace, "package.json"),
      resolve(stage, "package.json"),
    );
    await writeFile(
      resolve(stage, "tsconfig.json"),
      JSON.stringify(
        {
          compilerOptions: {
            target: "ES2023",
            lib: ["dom", "dom.iterable", "es2023"],
            strict: true,
            noEmit: true,
            skipLibCheck: true,
            esModuleInterop: true,
            module: "esnext",
            moduleResolution: "bundler",
            resolveJsonModule: true,
            isolatedModules: true,
            jsx: "react-jsx",
            plugins: [{ name: "next" }],
            paths: { "@/*": ["./src/*"] },
          },
          include: [
            "next-env.d.ts",
            "src/**/*.ts",
            "src/**/*.tsx",
            ".next/types/**/*.ts",
          ],
          exclude: ["node_modules"],
        },
        null,
        2,
      ),
    );
    await writeFile(
      resolve(stage, "next.config.mjs"),
      "export default " +
        JSON.stringify({
          output: mode === "static" ? "export" : "standalone",
          trailingSlash: true,
          basePath,
          images: { unoptimized: true },
          experimental: { cpus: 2 },
          outputFileTracingRoot: workspace,
          env: {
            NEXT_PUBLIC_WORKSPACE_MODE: mode,
            NEXT_PUBLIC_WORKSPACE_BASE_PATH: basePath,
          },
        }) +
        ";\n",
    );
    await mkdir(resolve(stage, "public/raw"), { recursive: true });
    await mkdir(resolve(stage, "public/narration"), { recursive: true });
    const narrationIndex: Record<string, string> = {};
    for (const doc of snapshot.documents) {
      await writeFile(
        resolve(
          stage,
          "public/raw",
          doc.id + (doc.path.endsWith(".csv") ? ".csv" : ".md"),
        ),
        doc.source,
      );
      if (basePath)
        doc.html = doc.html.replaceAll('href="/', 'href="' + basePath + "/");
      const frozen = narration.items.find(
        (i) => i.manifest.documentId === doc.id,
      );
      if (frozen) {
        const { manifest, path: manifestPath } = frozen;
        const out = resolve(stage, "public/narration", doc.id);
        await mkdir(out, { recursive: true });
        for (const segment of manifest.segments) {
          const source = resolve(dirname(manifestPath), segment.src);
          if ((await realpath(source)) !== source)
            throw new Error("Audio symlink escape");
          if (digest(await readFile(source)) !== segment.sha256)
            throw new Error("Audio digest mismatch");
          await mkdir(dirname(resolve(out, segment.src)), { recursive: true });
          await cp(source, resolve(out, segment.src));
        }
        await writeFile(
          resolve(out, "manifest.json"),
          JSON.stringify(manifest),
        );
        narrationIndex[doc.id] = doc.id + "/manifest.json";
      }
    }
    await writeFile(
      resolve(stage, "public/narration/index.json"),
      JSON.stringify(narrationIndex),
    );
    const diagrams = new Map(
      snapshot.documents.flatMap((d) =>
        d.diagrams.map((x) => [x.id, x] as const),
      ),
    );
    if (diagrams.size) {
      const executablePath =
        process.env.WORKSPACE_BROWSER_EXECUTABLE ??
        (process.platform === "darwin"
          ? "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
          : chromium.executablePath());
      const browser = await puppeteer.launch({
        executablePath,
        headless: true,
        args: process.env.CI ? ["--no-sandbox"] : [],
      });
      try {
        for (const diagram of diagrams.values()) {
          const { data } = await renderMermaid(browser, diagram.source, "svg", {
            fontEmbed: false,
            mermaidConfig: {
              securityLevel: "strict",
              theme: "neutral",
              htmlLabels: false,
              flowchart: { htmlLabels: false },
              deterministicIds: true,
              deterministicIDSeed: diagram.id,
            },
            svgId: "diagram-" + diagram.id,
          });
          const svg = new TextDecoder().decode(data);
          if (/<script|\son\w+\s*=|javascript:/i.test(svg))
            throw new Error("Unsafe Mermaid output: " + diagram.id);
          const replacement =
            svg + "<figcaption>流程图。对应说明与定义见本文原文。</figcaption>";
          for (const doc of snapshot.documents)
            doc.html = doc.html.replace(
              `<figure data-diagram="${diagram.id}"></figure>`,
              `<figure>${replacement}</figure>`,
            );
        }
      } finally {
        await browser.close();
      }
    }
    await writeFile(
      resolve(stage, "content-snapshot.json"),
      JSON.stringify(snapshot),
    );
    run(
      process.execPath,
      [
        resolve(workspace, "node_modules/next/dist/bin/next"),
        "build",
        "--webpack",
      ],
      stage,
    );
    const indexSite =
      mode === "static"
        ? resolve(stage, "out")
        : resolve(stage, ".local-index");
    if (mode === "online") {
      for (const doc of snapshot.documents) {
        const dir = resolve(indexSite, "read", doc.id);
        await mkdir(dir, { recursive: true });
        await writeFile(
          resolve(dir, "index.html"),
          `<html lang="zh-CN"><body><article data-pagefind-body data-pagefind-filter="category:${categoryNames[doc.category] ?? doc.category}"><h1 data-pagefind-meta="title">${doc.title.replaceAll("<", "&lt;")}</h1>${doc.html}</article></body></html>`,
        );
      }
    }
    if (snapshot.documents.length) {
      run(
        "pnpm",
        [
          "exec",
          "pagefind",
          "--site",
          indexSite,
          "--glob",
          "read/**/*.html",
          "--output-path",
          resolve(stage, "public/pagefind"),
        ],
        workspace,
      );
    } else {
      await mkdir(resolve(stage, "public/pagefind"), { recursive: true });
      await writeFile(
        resolve(stage, "public/pagefind/pagefind.js"),
        "export async function search(){return {results:[]};}\n",
      );
    }
    if (mode === "static")
      for (const doc of snapshot.documents)
        for (const alias of doc.aliases ?? []) {
          const html = aliasRedirect(alias, doc.id, basePath);
          const path = resolve(stage, "out", alias, "index.html");
          await mkdir(dirname(path), { recursive: true });
          await writeFile(path, html);
        }
    const candidate = resolve(workspace, ".build", mode + "-package");
    await rm(candidate, { recursive: true, force: true });
    await mkdir(candidate, { recursive: true });
    const release = {
      mode,
      releaseId,
      repoCommit: snapshot.repoCommit,
      preview: snapshot.preview,
      contentDigest: snapshot.contentDigest,
      codeDigest,
      narrationDigest: narration.digest,
      scope,
      basePath,
      createdAt: new Date().toISOString(),
    };
    if (mode === "static") {
      await cp(resolve(stage, "out"), candidate, { recursive: true });
      await cp(
        resolve(stage, "public/pagefind"),
        resolve(candidate, "pagefind"),
        { recursive: true },
      );
      await writeFile(
        resolve(candidate, "release.json"),
        JSON.stringify(release, null, 2),
      );
    } else {
      await copyPortableTree(resolve(stage, ".next/standalone"), candidate);
      const entry = (await files(candidate)).find((p) =>
        p.endsWith("/.build/online/server.js"),
      );
      if (!entry) throw new Error("Missing standalone server entry");
      const app = dirname(entry);
      await cp(resolve(stage, ".next/static"), resolve(app, ".next/static"), {
        recursive: true,
      });
      await cp(resolve(stage, "public"), resolve(app, "public"), {
        recursive: true,
      });
      await cp(
        resolve(stage, "content-snapshot.json"),
        resolve(app, "content-snapshot.json"),
      );
      await writeFile(
        resolve(candidate, "release.json"),
        JSON.stringify(
          {
            ...release,
            entry: relative(candidate, entry),
            schemaContract: "dev-workspace-v1",
          },
          null,
          2,
        ),
      );
    }
    const metadata = JSON.parse(
      await readFile(resolve(candidate, "release.json"), "utf8"),
    );
    await sealRelease(candidate, metadata);
    const current = await loadSnapshot({ workspaceRoot: workspace, scope });
    if (
      current.contentDigest !== snapshot.contentDigest ||
      (await fingerprint()) !== codeDigest ||
      (
        await freezeNarration(
          resolve(workspace, ".local/narration"),
          current.documents,
          scope,
        )
      ).digest !== narration.digest
    )
      throw new Error("Source changed while building; candidate not published");
    await mkdir(dirname(final), { recursive: true });
    await rename(candidate, final);
    await selectRelease(dirname(final), releaseId, mode);
    console.log(`${mode} release: ${final}`);
  }
} finally {
  await lock.close();
  await rm(stage + ".lock", { force: true });
}
