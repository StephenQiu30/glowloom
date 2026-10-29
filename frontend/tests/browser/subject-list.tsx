/** Source component + real IntersectionObserver. Synthetic data, no product writes. */
import React, { useState } from "react";
import { createRoot } from "react-dom/client";
import { UnifiedSubjectLibrary } from "../../src/components/subjects/unified-library";
import { installDevelopmentTransport } from "../../src/lib/development-transport";
import {
  Dialog,
  DialogContent,
  DialogTitle,
  DialogDescription,
} from "../../src/components/ui/dialog";
import { Button } from "../../src/components/ui/button";
import "../../src/app/globals.css";
const uuid = (n: number) =>
  `00000000-0000-4000-8000-${String(n).padStart(12, "0")}`;
const scenario = new URLSearchParams(location.search).get("case");
const calls: number[] = [];
const items = Array.from({ length: 45 }, (_, i) => ({
  id: uuid(100 + i),
  owner_scope_id: uuid(2),
  revision: 1,
  version_id: uuid(200 + i),
  name: `合成主体 ${String(i + 1).padStart(2, "0")}`,
  description: "合成回执，仅验证列表加载",
  favorite: false,
  primary: {
    binding_id: uuid(300 + i),
    asset_id: uuid(400 + i),
    asset_revision: 1,
    kind: "image",
    mime_type: "image/svg+xml",
    file_name: "fixture.svg",
  },
  references: [],
  created_at: "2026-10-28T00:00:00Z",
  updated_at: "2026-10-28T00:00:00Z",
}));
let failed = false;
installDevelopmentTransport({
  matches: (path) => path === "/tests/browser/subject-list.html",
  request: async (url, options) => {
    if (options.responseType === "blob")
      return new Blob(
        [
          '<svg xmlns="http://www.w3.org/2000/svg" width="640" height="360"><rect width="640" height="360" fill="#263b4b"/><circle cx="320" cy="180" r="70" fill="#81a8cc"/></svg>',
        ],
        { type: "image/svg+xml" },
      );
    if (url !== "/api/subjects") throw new Error("Unexpected fixture request");
    const params = options.params as {
      page: number;
      page_size: number;
      q?: string;
    };
    calls.push(params.page);
    document.getElementById("requests")!.textContent =
      `请求批次：${calls.join(",")}`;
    await new Promise((resolve) => setTimeout(resolve, 250));
    if (scenario === "error" && params.page === 2 && !failed) {
      failed = true;
      throw new Error("Synthetic tail failure");
    }
    const selected = params.q
      ? items.filter((item) => item.name.includes(params.q!))
      : items;
    return {
      current_actor_id: uuid(1),
      current_scope_id: uuid(2),
      page: params.page,
      page_size: params.page_size,
      total: selected.length,
      items: selected.slice(
        (params.page - 1) * params.page_size,
        params.page * params.page_size,
      ),
      capabilities: {
        intelligent_voice: { available: false, reason: "fixture" },
      },
    };
  },
});
function App() {
  const [dialog, setDialog] = useState(false);
  return (
    <main className="h-screen overflow-hidden bg-background text-foreground">
      <div className="flex h-16 items-center justify-between px-6">
        <h1>源码组件验收 · 合成回执</h1>
        <output id="requests" />
        <Button onClick={() => setDialog(true)}>引用弹窗</Button>
      </div>
      <div
        className="h-[calc(100dvh-64px)] overflow-y-auto p-6"
        data-testid="subject-scroll"
      >
        <UnifiedSubjectLibrary actorID={uuid(1)} scopeID={uuid(2)} />
      </div>
      <Dialog open={dialog} onOpenChange={setDialog}>
        <DialogContent className="max-h-[85dvh] overflow-y-auto sm:max-w-4xl">
          <DialogTitle>引用主体</DialogTitle>
          <DialogDescription>合成回执，仅检查独立滚动容器</DialogDescription>
          <UnifiedSubjectLibrary
            actorID={uuid(1)}
            scopeID={uuid(2)}
            onSelect={() => setDialog(false)}
          />
        </DialogContent>
      </Dialog>
    </main>
  );
}
createRoot(document.getElementById("root")!).render(<App />);
