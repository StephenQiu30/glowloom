import path from "node:path";
import { REPOSITORY_ROOT, isAttachmentPath } from "./source-files.mjs";

/**
 * Convert only ordinary relative Markdown links; original files stay unchanged.
 * @param {string} sourceFile Absolute path provided by Nextra's Markdown loader.
 * @param {string} href
 * @param {{repositoryRoot?: string, contentRoot?: string}} [options]
 */
export function resolveSourceLink(sourceFile, href, options = {}) {
  if (
    !href ||
    href.startsWith("#") ||
    href.startsWith("/") ||
    /^[a-z][a-z0-9+.-]*:/i.test(href)
  )
    return href;
  const repositoryRoot = path.resolve(
    options.repositoryRoot ?? REPOSITORY_ROOT,
  );
  const contentRoot = path.resolve(
    options.contentRoot ?? path.join(repositoryRoot, "workspace/content"),
  );
  const hashAt = href.indexOf("#");
  const fragment = hashAt < 0 ? "" : href.slice(hashAt);
  const withoutHash = hashAt < 0 ? href : href.slice(0, hashAt);
  const queryAt = withoutHash.indexOf("?");
  const query = queryAt < 0 ? "" : withoutHash.slice(queryAt);
  const linkPath = decodeURIComponent(
    queryAt < 0 ? withoutHash : withoutHash.slice(0, queryAt),
  );
  if (!linkPath) return `${query}${fragment}`;
  if (
    linkPath.includes("\\") ||
    linkPath.includes("\0") ||
    path.isAbsolute(linkPath)
  )
    throw new Error(`无效原文链接：${href}`);
  const target = path.resolve(path.dirname(sourceFile), linkPath);
  const repositoryPath = path
    .relative(repositoryRoot, target)
    .split(path.sep)
    .join("/");
  if (
    repositoryPath === ".." ||
    repositoryPath.startsWith("../") ||
    path.isAbsolute(repositoryPath)
  )
    throw new Error(`原文链接越出仓库：${href}`);
  const suffix = `${query}${fragment}`;
  if (isAttachmentPath(repositoryPath)) {
    // Nextra strips .md from internal links after custom remark plugins run.
    // Encoding the extension's dot preserves this original-file endpoint.
    const attachmentPath = repositoryPath
      .split("/")
      .map(encodeURIComponent)
      .join("/")
      .replace(/\.md$/, "%2Emd");
    return `/files/${attachmentPath}${suffix}`;
  }
  const contentPath = path
    .relative(contentRoot, target)
    .split(path.sep)
    .join("/");
  if (
    contentPath === ".." ||
    contentPath.startsWith("../") ||
    path.isAbsolute(contentPath)
  )
    throw new Error(`原文链接不在附件白名单：${repositoryPath}`);
  if (path.extname(contentPath) && !/\.mdx?$/i.test(contentPath))
    throw new Error(`原文链接不在附件白名单：${repositoryPath}`);
  const route = contentPath.replace(/\.mdx?$/i, "").replace(/(^|\/)index$/, "");
  return `/${route.split("/").filter(Boolean).map(encodeURIComponent).join("/")}${suffix}`;
}

/** @param {{repositoryRoot?: string, contentRoot?: string}} [options] */
export default function remarkSourceLinks(options = {}) {
  return (tree, file) => {
    if (!file.path) throw new Error("Markdown 原文路径缺失，无法解析相对链接");
    const visit = (node) => {
      if (node.type === "link" || node.type === "definition")
        node.url = resolveSourceLink(file.path, node.url, options);
      if (Array.isArray(node.children)) node.children.forEach(visit);
    };
    visit(tree);
  };
}
