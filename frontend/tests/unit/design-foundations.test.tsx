import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Alert, AlertTitle } from "@/components/ui/alert";
import { ProjectServerCard } from "@/features/project/project-server-card";

describe("共享视觉基础", () => {
  it("内容卡片无边框且不以轮廓阴影模拟边框", () => {
    render(<Card aria-label="剧本" role="region"><CardContent>剧本正文</CardContent></Card>);
    expect(screen.getByRole("region", { name: "剧本" })).toHaveClass("bg-transparent", "border-0", "shadow-none");
    for (const className of ["shadow-surface", "ring-1", "border"]) {
      expect(screen.getByRole("region", { name: "剧本" })).not.toHaveClass(className);
    }
  });

  it("次级操作使用轻边界并保留链接语义", () => {
    render(<Button asChild variant="outline"><a href="/login">继续制作</a></Button>);
    expect(screen.getByRole("link", { name: "继续制作" })).toHaveClass("shadow-border", "focus-visible:outline-2", "focus-visible:outline-solid");
    expect(screen.getByRole("link")).toHaveAttribute("href", "/login");
  });

  it("表单轻边界不丢失必填、禁用与错误语义", () => {
    render(<><Input aria-label="邮箱" required /><Textarea aria-label="剧本正文" aria-invalid="true" /><Button disabled>保存</Button></>);
    for (const input of screen.getAllByRole("textbox")) {
      expect(input).toHaveClass("shadow-border", "rounded-md", "focus-visible:outline-2", "focus-visible:outline-solid");
    }
    expect(screen.getByRole("textbox", { name: "邮箱" })).toBeRequired();
    expect(screen.getByRole("textbox", { name: "剧本正文" })).toBeInvalid();
    expect(screen.getByRole("button", { name: "保存" })).toBeDisabled();
  });

  it("项目入口不重新加框，也不裁掉键盘焦点", () => {
    render(<ProjectServerCard project={{ id: "project", workspace_id: "workspace", name: "长安夜航", description: "剧本", aspect_ratio: "9:16", language: "zh-CN", visual_style: "水墨", target_duration_ms: 90000, status: "active", revision: 1 }} />);
    const link = screen.getByRole("link", { name: "打开项目 长安夜航" });
    expect(link).toHaveAttribute("href", "/projects/project");
    expect(link.closest('[data-slot="card"]')).toHaveClass("border-0", "shadow-none");
    for (const className of ["border", "overflow-hidden", "shadow-sm", "hover:shadow-md"]) {
      expect(link.closest('[data-slot="card"]')).not.toHaveClass(className);
    }
  });

  it("去掉提示框描边仍保留危险提示语义", () => {
    render(<Alert variant="destructive"><AlertTitle>无法保存</AlertTitle></Alert>);
    expect(screen.getByRole("alert")).toHaveClass("border-0", "text-destructive");
    expect(screen.getByText("无法保存")).toBeInTheDocument();
  });
});
