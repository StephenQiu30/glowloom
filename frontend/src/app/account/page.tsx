import { Suspense } from "react";
import { AccountPage } from "@/components/workbench/admin-pages";
import { PreviewBoundary } from "@/components/workbench/workbench-components";
export default function Page() {
  return (
    <Suspense fallback={<p>正在加载页面…</p>}>
      <PreviewBoundary>
        <AccountPage />
      </PreviewBoundary>
    </Suspense>
  );
}
