/** Browser fixture identity; selected through Vite alias only, never a product auth path. */
export function useSession() {
  return {
    actor: {
      id: "11111111-1111-4111-8111-111111111111",
      email: "fixture@example.test",
      display_name: "浏览器测试管理员",
      platform_role: new URLSearchParams(location.search).has("creator")
        ? "creator"
        : "platform_admin",
    },
  };
}

export const useOptionalSession = useSession;
export function useSessionUpdate() {
  return () => undefined;
}
export function SessionGate({
  children,
}: {
  children: import("react").ReactNode;
}) {
  return children;
}
export const SessionProvider = SessionGate;
export function clearPrivateResources() {
  return undefined;
}
export async function clearPrivateSession() {
  return undefined;
}
export async function revokeSession() {
  return undefined;
}
