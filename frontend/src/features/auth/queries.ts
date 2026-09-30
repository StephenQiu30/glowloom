import { z } from "zod";
import * as auth from "@/gen/api/auth";
import { ApiError } from "@/lib/request";

export const CURRENT_SESSION_KEY = ["identity", "current-user"] as const;
const userSchema = z.object({
  id: z.string().uuid(),
  org_id: z.string().uuid(),
  login_name: z.string(),
  display_name: z.string(),
  role: z.enum(["admin", "producer"]),
  must_change_password: z.boolean(),
});
const sessionSchema = z.object({
  user: userSchema,
  must_change_password: z.boolean(),
});
export type CurrentSession = z.infer<typeof sessionSchema>;
function parse<T>(schema: z.ZodType<T>, value: unknown): T {
  const result = schema.safeParse(value);
  if (!result.success) throw new ApiError(502, "invalid_response");
  return result.data;
}
export async function getCurrentSession(signal?: AbortSignal) {
  return parse(sessionSchema, await auth.currentUser({ signal }));
}
export async function login(login_name: string, password: string) {
  return parse(sessionSchema, await auth.login({ login_name, password }));
}
export async function logout(key: string) {
  await auth.logout({ headers: { "Idempotency-Key": key } });
}
export async function changePassword(
  current_password: string,
  new_password: string,
  key: string,
) {
  return parse(
    z.object({
      must_change_password: z.literal(false),
      revision: z.number().int().positive(),
    }),
    await auth.changePassword(
      { current_password, new_password },
      { headers: { "Idempotency-Key": key } },
    ),
  );
}
