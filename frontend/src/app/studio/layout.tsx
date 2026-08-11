import { type ReactNode } from "react";

import { ProtectedRoute } from "@/components/auth/protected-route";

export default function StudioLayout({ children }: { children: ReactNode }) {
  return <ProtectedRoute page="assets">{children}</ProtectedRoute>;
}
