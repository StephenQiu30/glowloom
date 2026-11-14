import { NextResponse, type NextRequest } from "next/server";
import { getSnapshot } from "@/content/runtime";
import { verifySession, cookieName } from "@/server/auth/session";
export async function proxy(request: NextRequest) {
  const path = request.nextUrl.pathname;
  if (
    path === "/login/" ||
    path === "/login" ||
    path === "/api/session/" ||
    path === "/api/session" ||
    path.startsWith("/_next/static/") ||
    path === "/favicon.ico"
  )
    return NextResponse.next();
  const secret = process.env.GLOWLOOM_WORKSPACE_SESSION_SECRET ?? "";
  if (verifySession(request.cookies.get(cookieName)?.value ?? "", secret)) {
    if (path.startsWith("/docs/")) {
      const doc = (await getSnapshot()).documents.find((d) =>
        d.aliases?.includes(
          decodeURIComponent(path).replace(/^\//, "").replace(/\/$/, ""),
        ),
      );
      if (doc) {
        const target = request.nextUrl.clone();
        target.pathname = "/read/" + doc.id + "/";
        return NextResponse.redirect(target, 308);
      }
    }
    return NextResponse.next();
  }
  if (
    path.startsWith("/api/") ||
    path.startsWith("/raw/") ||
    path.startsWith("/pagefind/") ||
    path.startsWith("/narration/")
  )
    return NextResponse.json({ error: "unauthorized" }, { status: 401 });
  const url = request.nextUrl.clone();
  url.pathname = "/login/";
  url.search = "";
  url.searchParams.set("next", path);
  return NextResponse.redirect(url);
}
export const config = { matcher: ["/:path*"] };
