export const base64 = (bytes: Uint8Array): string => {
  let text = "";
  for (const byte of bytes) text += String.fromCharCode(byte);
  return btoa(text);
};
export const unbase64 = (text: string): Uint8Array => {
  const bytes = Uint8Array.from(atob(text), (c) => c.charCodeAt(0));
  if (base64(bytes) !== text) throw new Error("INVALID_BASE64");
  return bytes;
};
export async function sha256(bytes: Uint8Array): Promise<string> {
  return [...new Uint8Array(await crypto.subtle.digest("SHA-256", bytes as any))]
    .map((byte) => byte.toString(16).padStart(2, "0"))
    .join("");
}
