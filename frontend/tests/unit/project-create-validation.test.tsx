import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it, vi } from "vitest";

import { ProjectCreateDialog } from "@/features/project/project-create-dialog";

it("rejects a whitespace-only project name with an accessible field error", async () => {
  const submit = vi.fn().mockResolvedValue(true);
  render(
    <ProjectCreateDialog
      open
      onOpenChange={vi.fn()}
      onSubmit={submit}
      workspaceId="workspace"
      isSubmitting={false}
    />,
  );
  const user = userEvent.setup();
  await user.type(screen.getByLabelText("项目名称"), "   ");
  await user.click(screen.getByRole("button", { name: "确认创建" }));
  expect(await screen.findByText("请输入项目名称")).toBeVisible();
  expect(screen.getByLabelText("项目名称")).toHaveAttribute("aria-invalid", "true");
  expect(submit).not.toHaveBeenCalled();
});
