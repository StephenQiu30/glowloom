function grantURL(value: string) {
  const url = new URL(value, window.location.origin);
  if (
    url.protocol !== "https:" &&
    !(url.protocol === "http:" && url.origin === window.location.origin)
  )
    throw new Error("媒体授权地址无效");
  return url.href;
}
export async function readGrantedText(value: string, signal?: AbortSignal) {
  // eslint-disable-next-line no-restricted-syntax -- Read bytes from an already granted media URL; this does not issue business commands.
  const response = await fetch(grantURL(value), {
    signal,
    credentials: "same-origin",
  });
  if (!response.ok) throw new Error("媒体读取未完成");
  return response.text();
}
export async function uploadGrantedPart(
  value: string,
  body: Blob,
  signal: AbortSignal,
) {
  // eslint-disable-next-line no-restricted-syntax -- PUT only the granted object-storage part; business admission uses the generated API.
  const response = await fetch(grantURL(value), {
    method: "PUT",
    body,
    credentials: "omit",
    signal: AbortSignal.any([signal, AbortSignal.timeout(600000)]),
    headers: { "Content-Type": "application/octet-stream" },
  });
  if (!response.ok) throw new Error("媒体分片上传未完成");
}
