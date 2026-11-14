export function aliasRedirect(alias: string, id: string, base: string) {
  if (
    !alias.startsWith("docs/") ||
    alias.split("/").some((s) => !s || s === "." || s === "..") ||
    /[\\\\?#<>]/.test(alias) ||
    !/^[a-zA-Z0-9_-]+$/.test(id)
  )
    throw new Error("invalid_alias");
  const url = base + "/read/" + id + "/";
  return `<!doctype html><html lang="zh-CN"><meta charset="utf-8"><title>文档已迁移</title><a href="${url}">打开文档</a><script>location.replace(${JSON.stringify(url)}+location.hash)</script></html>`;
}
