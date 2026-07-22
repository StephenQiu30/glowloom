import type { NextRequest } from "next/server";

import { apiUrl } from "@/lib/api";

export async function GET(request: NextRequest) {
  return forward(request, "GET");
}

export async function POST(request: NextRequest) {
  return forward(request, "POST");
}

export async function DELETE(request: NextRequest) {
  return forward(request, "DELETE");
}

async function forward(request: NextRequest, method: string) {
  const headers = new Headers({ accept: "application/json" });
  const cookie = request.headers.get("cookie");
  const csrfToken = request.headers.get("x-csrf-token");
  if (cookie) headers.set("cookie", cookie);
  if (csrfToken) headers.set("x-csrf-token", csrfToken);

  let body: string | undefined;
  if (method === "POST") {
    headers.set("content-type", "application/json");
    body = await request.text();
  }

  const upstream = await fetch(`${apiUrl()}/v1/session`, {
    method,
    headers,
    body,
    cache: "no-store",
  });
  const responseHeaders = new Headers();
  const contentType = upstream.headers.get("content-type");
  if (contentType) responseHeaders.set("content-type", contentType);
  for (const value of upstream.headers.getSetCookie()) {
    responseHeaders.append("set-cookie", value);
  }

  return new Response(upstream.status === 204 ? null : await upstream.text(), {
    status: upstream.status,
    headers: responseHeaders,
  });
}
