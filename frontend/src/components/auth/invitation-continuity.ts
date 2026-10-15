// Opaque invitation proof remains in process memory across same-app navigation only.
let token = "";
export function pendingInvitation() {
  return token;
}
export function clearInvitation() {
  token = "";
}
export function readInvitationFragment() {
  if (typeof window === "undefined") return token;
  if (!window.location.hash) return token;
  const fragment = new URLSearchParams(window.location.hash.slice(1));
  const values = fragment.getAll("token");
  window.history.replaceState(
    window.history.state,
    "",
    window.location.pathname + window.location.search,
  );
  token =
    values.length === 1 && /^[A-Za-z0-9_-]{20,512}$/.test(values[0])
      ? values[0]
      : "";
  return token;
}
