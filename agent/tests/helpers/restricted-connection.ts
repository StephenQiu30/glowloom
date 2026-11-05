/** An optional dedicated login may SET ROLE; production-role passwords stay untouched. */
export function isRestrictedTestConnection(url: URL) {
  if (!url.pathname.endsWith("_test")) return false;
  return (
    url.username === "glowloom_agent" ||
    (url.username === "glowloom_agent_conversation" &&
      url.pathname.startsWith("/glowloom_conversation_") &&
      url.searchParams.get("options") === "-c role=glowloom_agent")
  );
}
