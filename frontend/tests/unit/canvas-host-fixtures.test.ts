// @vitest-environment node
import { readdirSync, mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { createServer } from "vite";
import { expect, it } from "vitest";
import { canvasHostAliases } from "../../canvas-host";

const fixtures = path.resolve(import.meta.dirname, "../browser");

it("所有浏览器夹具都能解析同一宿主组件和编辑器公共元数据", async () => {
  for (const config of readdirSync(fixtures).filter((name) =>
    name.endsWith(".vite.config.ts"),
  )) {
    const temporary = mkdtempSync(
      path.join(tmpdir(), "glowloom-editor-fixture-"),
    );
    const server = await createServer({
      configFile: path.join(fixtures, config),
      cacheDir: temporary,
      server: { middlewareMode: true, watch: null },
      optimizeDeps: { noDiscovery: true, include: [] },
      logLevel: "silent",
    });
    try {
      const plugins = server.environments.client.pluginContainer;
      const toolbar = await plugins.resolveId(
        "@glowloom/canvas-host/components/ui/toolbar",
      );
      expect(toolbar?.id, config).toBe(
        path.join(
          canvasHostAliases["@glowloom/canvas-host"],
          "components/ui/toolbar.tsx",
        ),
      );
      const metadata = await plugins.resolveId(
        "@glowloom/canvas-host-public/director/catalog/manifest.json",
      );
      expect(metadata?.id, config).toMatch(/^\0canvas-host-public-json:/);
      const loaded = await plugins.load(metadata!.id);
      expect(
        typeof loaded === "string" ? loaded : loaded?.code,
        config,
      ).toContain("export default");
    } finally {
      await server.close();
      rmSync(temporary, { recursive: true, force: true });
    }
  }
}, 30_000);
