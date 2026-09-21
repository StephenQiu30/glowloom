import { type ReactNode } from "react";

import { ProtectedRoute } from "@/components/identity/protected-route";

export default function WorkspacesLayout({ children }: { children: ReactNode }) {
  return <ProtectedRoute page="settings">{children}</ProtectedRoute>;
}
