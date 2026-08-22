import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

vi.mock("@/api/approveScriptAnalysis", () => ({ approveScriptAnalysis: vi.fn() }));
vi.mock("@/api/createProject", () => ({ createProject: vi.fn() }));
vi.mock("@/api/createScriptRevision", () => ({ createScriptRevision: vi.fn() }));
vi.mock("@/api/createWorkspace", () => ({ createWorkspace: vi.fn() }));
vi.mock("@/api/getAnalysisDraft", () => ({ getAnalysisDraft: vi.fn() }));
vi.mock("@/api/getOperation", () => ({ getOperation: vi.fn() }));
vi.mock("@/api/queueScriptAnalysis", () => ({ queueScriptAnalysis: vi.fn() }));

import { ScriptAnalysisWorkspace } from "@/features/script-analysis/views/script-analysis-workspace";

describe("ScriptAnalysisWorkspace", () => {
  it("shows the manual fact-line entry point", () => {
    render(<ScriptAnalysisWorkspace />);

    expect(screen.getByRole("heading", { name: "先把整本剧本，变成可核对的事实。" })).toBeInTheDocument();
    const script = (screen.getByLabelText("剧本文本") as HTMLTextAreaElement).value;
    expect(script).toContain("第1集 归途");
    expect(script).toContain("第3集 终局");
    expect(screen.getByTestId("analyze-button")).toHaveTextContent("提交解析任务");
    expect(screen.getByTestId("phase-status")).toHaveTextContent("等待导入");
  });
});
