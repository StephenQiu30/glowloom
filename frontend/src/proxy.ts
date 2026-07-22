import { type NextRequest, NextResponse } from "next/server";

export function proxy(request: NextRequest) {
  if (request.cookies.get("thief_session")) {
    return NextResponse.next();
  }

  const login = new URL("/login", request.url);
  const returnTo = `${request.nextUrl.pathname}${request.nextUrl.search}`;
  login.searchParams.set("returnTo", returnTo);
  return NextResponse.redirect(login);
}

export const config = {
  matcher: ["/create/:path*", "/works/:path*", "/admin/:path*"],
};
