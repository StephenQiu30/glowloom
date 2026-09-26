// Liveness probe for Compose and the load balancer (OPS-01 §3).
export const dynamic = "force-dynamic";

export function GET(): Response {
  return Response.json({ status: "ok" });
}
