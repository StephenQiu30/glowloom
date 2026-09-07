import { type ReactNode } from "react";

import { ProtectedRoute } from "@/features/identity/protected-route";

export default function StudioLayout({ children }: { children: ReactNode }) {
  return <ProtectedRoute page="assets">{children}</ProtectedRoute>;
}
