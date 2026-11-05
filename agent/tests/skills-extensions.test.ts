import { mkdtemp, mkdir, writeFile, chmod, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { createHash } from "node:crypto";
import { expect, it } from "vitest";
import { loadTrustedExtensions } from "../src/harness/trusted-extensions.js";
const source = `export default pi => pi.registerTool({name:'approved_echo',label:'Echo',description:'Reviewed echo',parameters:{type:'object',properties:{value:{type:'string'}},required:['value']},execute:async (_id,args)=>({content:[{type:'text',text:args.value}],details:{reviewed:true}})});`;
async function fixture(content = source) {
  const root = await mkdtemp(join(tmpdir(), "glowloom-trusted-test-"));
  const path = join(root, "approved.mjs");
  await writeFile(path, content);
  await chmod(path, 0o444);
  await chmod(root, 0o555);
  return {
    root,
    path,
    entries: [
      {
        path: "approved.mjs",
        sha256: createHash("sha256").update(content).digest("hex"),
      },
    ],
    close: async () => {
      await chmod(root, 0o755);
      await rm(root, { recursive: true, force: true });
    },
  };
}
it("uses the public SDK loader to actually load and invoke only a pinned reviewed local tool", async () => {
  const f = await fixture();
  try {
    const loaded = await loadTrustedExtensions({
      imageRoot: f.root,
      entries: f.entries,
    });
    try {
      expect(loaded.tools.map((t) => t.name)).toEqual(["approved_echo"]);
      expect(
        (
          await loaded.tools[0]!.invoke(
            "id",
            { value: "actual SDK output" },
            undefined,
          )
        ).content,
      ).toEqual([{ type: "text", text: "actual SDK output" }]);
    } finally {
      await loaded.close();
    }
  } finally {
    await f.close();
  }
});
it("rejects SDK hooks instead of silently dropping unsupported lifecycle behavior", async () => {
  const hook = await fixture(
    `export default pi => pi.on('before_agent_start',()=>({systemPrompt:'unsafe'}));`,
  );
  try {
    await expect(
      loadTrustedExtensions({ imageRoot: hook.root, entries: hook.entries }),
    ).rejects.toThrow(/unsupported.*hook/i);
  } finally {
    await hook.close();
  }
});
it("rejects writable entries, hash drift, network sources and paths outside the reviewed image root", async () => {
  const f = await fixture();
  try {
    await expect(
      loadTrustedExtensions({
        imageRoot: f.root,
        entries: [
          { path: "https://evil.invalid/plugin.js", sha256: "a".repeat(64) },
        ],
      }),
    ).rejects.toThrow();
    await expect(
      loadTrustedExtensions({
        imageRoot: f.root,
        entries: [{ path: "../plugin.mjs", sha256: "a".repeat(64) }],
      }),
    ).rejects.toThrow();
    await expect(
      loadTrustedExtensions({
        imageRoot: f.root,
        entries: [{ ...f.entries[0]!, sha256: "a".repeat(64) }],
      }),
    ).rejects.toThrow(/hash/i);
    await chmod(f.path, 0o644);
    await expect(
      loadTrustedExtensions({ imageRoot: f.root, entries: f.entries }),
    ).rejects.toThrow(/read.only/i);
  } finally {
    await f.close();
  }
});
it("ignores tenant cwd extension discovery and allows an empty image allowlist", async () => {
  const root = await mkdtemp(join(tmpdir(), "glowloom-upload-test-"));
  try {
    await mkdir(join(root, ".pi", "extensions"), { recursive: true });
    await writeFile(
      join(root, ".pi", "extensions", "tenant.mjs"),
      `throw new Error('tenant extension was executed');`,
    );
    const loaded = await loadTrustedExtensions({
      imageRoot: root,
      entries: [],
    });
    expect(loaded.tools).toEqual([]);
    await loaded.close();
  } finally {
    await rm(root, { recursive: true, force: true });
  }
});
it("rejects SDK provider registration that would otherwise be silently ignored by a tool-only bridge", async () => {
  const f = await fixture(
    `export default pi => pi.registerProvider('outside',{baseUrl:'https://outside.invalid',apiKey:'none',api:'openai-completions',models:[]});`,
  );
  try {
    await expect(
      loadTrustedExtensions({ imageRoot: f.root, entries: f.entries }),
    ).rejects.toThrow(/unsupported.*provider/i);
  } finally {
    await f.close();
  }
});
