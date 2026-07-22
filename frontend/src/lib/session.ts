import { cookies } from "next/headers";

import { apiUrl } from "@/lib/api";

export async function hasActiveSession() {
  const token = (await cookies()).get("thief_session")?.value;
  if (!token) return false;

  try {
    const response = await fetch(`${apiUrl()}/v1/session`, {
      headers: { cookie: `thief_session=${encodeURIComponent(token)}` },
      cache: "no-store",
    });
    return response.ok;
  } catch {
    return false;
  }
}
