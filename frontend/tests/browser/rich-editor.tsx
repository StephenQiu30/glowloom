import { createRoot } from "react-dom/client";
import { RichGeneration } from "../../src/components/generation/rich-generation";
import "../../src/app/globals.css";
const params = new URLSearchParams(location.search);
if (params.get("theme") === "dark")
  document.documentElement.classList.add("dark");
const project = params.get("project");
if (!project) throw new Error("隔离项目 ID 缺失");
createRoot(document.getElementById("root")!).render(
  <main className="mx-auto max-w-5xl p-4 sm:p-8">
    <RichGeneration
      projectID={project}
      draftID={params.get("draft") ?? undefined}
      seed={{
        prompt: "隔离测试：一只猫，中文与 Emoji 🐈",
        capability: "image",
      }}
    />
  </main>,
);
