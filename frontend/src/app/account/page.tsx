import { Suspense } from "react";
import { AccountPage } from "@/features/workbench/admin-pages";
import { PreviewBoundary } from "@/features/workbench/poc-components";
export default function Page() {
  return (
    <Suspense fallback={<p>正在加载页面…</p>}>
      <PreviewBoundary>
        <AccountPage />
      </PreviewBoundary>
    </Suspense>
  );
}
