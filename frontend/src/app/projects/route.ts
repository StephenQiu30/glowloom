/** Compatibility navigation runs before React renders the asset workspace. */
export function GET(request: Request) {
  const incoming = new URL(request.url);
  const destination = new URL("/assets", incoming);
  destination.searchParams.set("view", "canvas");
  const query = incoming.searchParams.get("q")?.trim();
  if (query) destination.searchParams.set("q", query);
  // A relative Location preserves the public origin behind the local/prod proxy.
  return new Response(null, {
    status: 307,
    headers: {
      Location: `${destination.pathname}${destination.search}`,
      "Cache-Control": "no-store",
    },
  });
}
