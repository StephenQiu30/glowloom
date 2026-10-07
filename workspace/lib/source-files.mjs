import { lstat, readFile, realpath } from "node:fs/promises";
import path from "node:path";
// Next starts from the workspace package; bundling changes module locations.
export const REPOSITORY_ROOT = path.resolve(process.cwd(), "..");

// Only these existing references are served. Adding a file is an explicit edit.
export const ATTACHMENT_PATHS = Object.freeze([
  "AGENTS.md",
  "PROJECT.md",
  "DESIGN.md",
  "BACKLOG.md",
  "README.md",
  "THIRD_PARTY_NOTICES.md",
  "design-qa.md",
  "CHANGELOG.md",
  "LICENSE",
  "backend/db/schema.sql",
  ".github/workflows/ci.yml",
  "workspace/content/licenses/CC-BY-4.0.txt",
  "workspace/content/licenses/LicenseRef-KhronosSpecCopyright.txt",
  "workspace/content/licenses/beeftv.txt",
  "workspace/content/licenses/infinite-canvas.txt",
  "workspace/content/licenses/lingji-cut.txt",
  "workspace/content/licenses/noto-cjk-ofl.txt",
  "workspace/content/licenses/video-depth-anything.txt",
]);

/** @param {string} sourcePath */
export function isAttachmentPath(sourcePath) {
  return ATTACHMENT_PATHS.includes(sourcePath);
}

/**
 * Read one allowed original as plain text. No user path is joined before the
 * allowlist check, and symlinks are rejected at every level before reading.
 * @param {string} sourcePath
 * @param {string} [repositoryRoot]
 * @returns {Promise<string | undefined>}
 */
export async function readAttachment(
  sourcePath,
  repositoryRoot = REPOSITORY_ROOT,
) {
  if (!isAttachmentPath(sourcePath)) return undefined;
  const root = await realpath(repositoryRoot);
  let target = root;
  try {
    for (const segment of sourcePath.split("/")) {
      target = path.join(target, segment);
      if ((await lstat(target)).isSymbolicLink()) return undefined;
    }
    const resolved = await realpath(target);
    const relative = path.relative(root, resolved);
    if (relative.startsWith(`..${path.sep}`) || path.isAbsolute(relative))
      return undefined;
    if (!(await lstat(resolved)).isFile()) return undefined;
    return await readFile(resolved, "utf8");
  } catch (error) {
    if (
      error &&
      typeof error === "object" &&
      "code" in error &&
      (error.code === "ENOENT" || error.code === "ENOTDIR")
    )
      return undefined;
    throw error;
  }
}
