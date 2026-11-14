import { mkdtemp, mkdir, writeFile, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { expect, it } from "vitest";
import { assertStaticBoundary } from "../../src/build/boundary";

it("rejects an indirect online import before staging or exporting any static files", async () => {
  const dir = await mkdtemp(join(tmpdir(), "ws-boundary-"));
  try {
    await mkdir(join(dir, "src/app"), { recursive: true });
    await mkdir(join(dir, "src/components"));
    await writeFile(
      join(dir, "src/app/page.tsx"),
      "import '../components/reader'; export default function Page(){return null;}",
    );
    await writeFile(
      join(dir, "src/components/reader.tsx"),
      "import {db} from '@/server/store';",
    );
    await expect(assertStaticBoundary(dir)).rejects.toThrow(
      /reader.tsx|online/,
    );
    await writeFile(
      join(dir, "src/components/reader.tsx"),
      "export const label='readonly';",
    );
    await writeFile(join(dir, "src/app/globals.css"), "body { margin: 0; }");
    await writeFile(
      join(dir, "src/app/page.tsx"),
      "import './globals.css'; import '../components/reader';",
    );
    await expect(assertStaticBoundary(dir)).resolves.toBeUndefined();
  } finally {
    await rm(dir, { recursive: true, force: true });
  }
});
