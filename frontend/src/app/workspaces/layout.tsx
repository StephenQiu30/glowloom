import { type ReactNode } from "react";

import { ProtectedRoute } from "@/features/identity/protected-route";

export default function WorkspacesLayout({ children }: { children: ReactNode }) {
  return <ProtectedRoute page="settings">{children}</ProtectedRoute>;
}
