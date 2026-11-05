/** Isolated UI/API fixture; no provider or production service connection. */
import { createRoot } from "react-dom/client";
import { ConversationWorkspace } from "../../src/components/conversations/conversation-workspace";
import { emptyPromptDraft } from "../../src/components/generation/prompt-document";
import { TooltipProvider } from "../../src/components/ui/tooltip";
import { Toaster } from "../../src/components/ui/sonner";
import { setCSRFToken, setCreationScope } from "../../src/lib/request";
import "../../src/app/globals.css";
setCSRFToken("fixture-only");
setCreationScope("11111111-1111-4111-8111-111111111113");
const params = new URLSearchParams(location.search);
const welcome = params.has("welcome");
if (params.get("theme") === "dark")
  document.documentElement.classList.add("dark");
const draft = emptyPromptDraft();
draft.document.content[0]!.content = [
  { type: "text", text: "请根据参考写一个分镜故事" },
];
for (let i = 1; i <= 2; i++) {
  const id = `reference-${i}`;
  draft.references.push({
    id,
    source: {
      kind: "media",
      sourceId: `11111111-1111-4111-8111-11111111112${i}`,
      sourceVersion: "1",
    },
    bindingGeneration: 1,
    mediaKind: "image",
    role: "reference_image",
    order: i - 1,
  });
  draft.labelState.allocations[id] = { labelKind: "image", ordinal: i };
}
draft.labelState.highWater.image = 2;
function AuditControls() {
  return (
    <div className="mb-4 flex flex-wrap gap-2" aria-label="隔离测试状态控制">
      {(["completed", "failed", "submitted"] as const).map((outcome) => (
        <button
          key={outcome}
          className="rounded border px-3 py-2 text-xs"
          onClick={async () => {
            const response = await fetch("/api/fixture/conversation-outcome", {
              method: "POST",
              headers: { "Content-Type": "application/json" },
              body: JSON.stringify({ outcome }),
            });
            if (!response.ok) return;
            const url = new URL(location.href);
            url.searchParams.set("history", "1");
            location.assign(url);
          }}
        >
          隔离切换：{outcome}
        </button>
      ))}
    </div>
  );
}
createRoot(document.getElementById("root")!, {
  onUncaughtError: (error) =>
    console.error(String(error), error instanceof Error ? error.stack : ""),
}).render(
  <TooltipProvider>
    <Toaster />
    <main className="mx-auto flex h-dvh max-w-5xl flex-col px-4 pt-20 pb-5">
      {!welcome && (
        <>
          <p className="mb-4 text-xs text-muted-foreground">
            隔离 fixture · 不调用真实生成服务
          </p>
          <AuditControls />
        </>
      )}
      <ConversationWorkspace
        ownerKey="fixture/space"
        projectID="11111111-1111-4111-8111-111111111114"
        sessionID={
          params.get("history")
            ? "11111111-1111-4111-8111-111111111115"
            : undefined
        }
        enabled
        initialDraft={welcome ? emptyPromptDraft() : draft}
        onChooseProject={() => {}}
      />
    </main>
  </TooltipProvider>,
);
