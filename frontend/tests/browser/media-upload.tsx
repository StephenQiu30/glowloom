/** Isolated source-component acceptance. Synthetic transport; never sends files to a service. */
import React, { useState } from "react";
import { createRoot } from "react-dom/client";
import { UploadDialog } from "../../src/components/media/upload-dialog";
import { Button } from "../../src/components/ui/button";
import { Toaster } from "../../src/components/ui/sonner";
import { installDevelopmentTransport } from "../../src/lib/development-transport";
import { RequestError } from "../../src/lib/request";
import "../../src/app/globals.css";
const params = new URLSearchParams(location.search);
const scenario = params.get("case") ?? "ready";
document.documentElement.classList.toggle(
  "dark",
  params.get("theme") !== "light",
);
installDevelopmentTransport({
  matches: (path) => path === "/tests/browser/media-upload.html",
  request: async (url, options) => {
    if (!url.includes("/media/")) throw new Error("Unexpected fixture route");
    options.onUploadProgress?.({ loaded: 40, total: 100 } as never);
    await new Promise((resolve) => setTimeout(resolve, 800));
    if (scenario === "unknown") throw new Error("Fixture network failure");
    if (scenario === "denied") throw new RequestError("forbidden", 403);
    return {
      asset: {
        id: "11111111-1111-4111-8111-111111111111",
        revision: 1,
        kind: "image",
        mime_type: "image/png",
        file_name: "验收参考.png",
      },
    };
  },
});
function App() {
  const [open, setOpen] = useState(true);
  const [done, setDone] = useState(false);
  const file = params.get("file")
    ? new File(
        ["fixture"],
        "上传区域验收-20261028-这是一个用于检查窄屏省略效果的长文件名.png",
        { type: "image/png" },
      )
    : undefined;
  return (
    <main className="flex min-h-screen flex-col items-center justify-center gap-4 bg-background p-6 text-foreground">
      <Toaster theme={params.get("theme") === "light" ? "light" : "dark"} />
      <p>源码组件验收 · 合成回执</p>
      <Button onClick={() => setOpen(true)}>
        {done ? "再次上传" : "继续上传"}
      </Button>
      {done ? <p role="status">已采纳合成参考文件</p> : null}
      {open ? (
        <UploadDialog
          initialFile={file}
          onClose={() => setOpen(false)}
          onUploaded={() => {
            setDone(true);
          }}
        />
      ) : null}
    </main>
  );
}
createRoot(document.getElementById("root")!).render(<App />);
