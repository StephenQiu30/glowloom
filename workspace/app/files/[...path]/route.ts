import { readAttachment } from "../../../lib/source-files.mjs";

export async function GET(
  _request: Request,
  { params }: { params: Promise<{ path: string[] }> },
) {
  const { path } = await params;
  const original = await readAttachment(path.join("/"));
  if (original === undefined) return new Response("Not found", { status: 404 });
  return new Response(original, {
    headers: {
      "Content-Type": "text/plain; charset=utf-8",
      "Content-Disposition": "inline",
      "X-Content-Type-Options": "nosniff",
    },
  });
}
