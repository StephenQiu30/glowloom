import { type ReactNode } from "react";

import { ProtectedRoute } from "@/features/identity/protected-route";

export default function ProjectsLayout({ children }: { children: ReactNode }) {
  return <ProtectedRoute page="projects">{children}</ProtectedRoute>;
}
