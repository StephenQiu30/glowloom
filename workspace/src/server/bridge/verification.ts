export function verificationCommand(
  project: string,
  id: string,
): { binary: string; args: string[] } {
  if (
    !["workspace", "frontend", "backend", "agent", "canvas"].includes(project)
  )
    throw new Error("verification_command_not_configured");
  if (id === "typecheck")
    return project === "frontend"
      ? { binary: "pnpm", args: ["exec", "tsc", "--noEmit"] }
      : project === "backend"
        ? { binary: "go", args: ["vet", "./..."] }
        : { binary: "pnpm", args: ["typecheck"] };
  if (project === "workspace" && id === "content")
    return { binary: "pnpm", args: ["check:content"] };
  if (project === "workspace" && id === "unit")
    return {
      binary: "pnpm",
      args: ["exec", "vitest", "run", "--reporter=json"],
    };
  throw new Error(
    id === "unit" || id === "content"
      ? "verification_command_not_configured"
      : "unknown_command",
  );
}
