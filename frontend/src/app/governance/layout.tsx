import { type ReactNode } from "react";

import { ProtectedRoute } from "@/components/auth/protected-route";

export default function GovernanceLayout({ children }: { children: ReactNode }) {
  return <ProtectedRoute page="governance">{children}</ProtectedRoute>;
}
