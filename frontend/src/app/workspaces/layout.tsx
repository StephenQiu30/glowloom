import { type ReactNode } from "react";

import { ProtectedRoute } from "@/components/auth/protected-route";

export default function WorkspacesLayout({ children }: { children: ReactNode }) {
  return <ProtectedRoute page="settings">{children}</ProtectedRoute>;
}
