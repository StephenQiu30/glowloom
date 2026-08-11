export type WorkspaceRole = "owner" | "editor" | "viewer";

export type StudioNavigation =
  | "create"
  | "projects"
  | "assets"
  | "governance"
  | "settings";

const pageRoles: Record<
  StudioNavigation,
  "public" | readonly WorkspaceRole[]
> = {
  create: "public",
  projects: ["owner", "editor", "viewer"],
  assets: ["owner", "editor", "viewer"],
  governance: ["owner", "editor"],
  settings: ["owner", "editor", "viewer"],
};

const primaryNavigation: readonly StudioNavigation[] = [
  "create",
  "projects",
  "assets",
];

export function canAccessPage(
  role: WorkspaceRole | undefined,
  page: StudioNavigation,
): boolean {
  const allowedRoles = pageRoles[page];
  return allowedRoles === "public" || Boolean(role && allowedRoles.includes(role));
}

export function visiblePrimaryNavigation(
  role: WorkspaceRole | undefined,
): StudioNavigation[] {
  return primaryNavigation.filter(
    (page) => (!role || page !== "create") && canAccessPage(role, page),
  );
}
