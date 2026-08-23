import { type ReactNode } from "react";

import { ProtectedRoute } from "@/components/auth/protected-route";

export default function ProjectsLayout({ children }: { children: ReactNode }) {
  return <ProtectedRoute page="projects">{children}</ProtectedRoute>;
}
