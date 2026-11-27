"use client";
import { createPortal } from "react-dom";
import { IconAction } from "@/components/ui/icon-action";
import { X } from "lucide-react";
import type { AdminSection } from "@/components/layout/routes";
import { createContext, useContext, useState, type ReactNode } from "react";
import { HeaderLinks } from "@/components/layout/header-navigation";
import { PageHeader } from "@/components/layout/page-header";
import { Toolbar, ToolbarGroup } from "@/components/ui/toolbar";

import { adminDestinations } from "./admin-destinations";
import "./admin-workspace.css";

const AdminNavigationTitle = createContext<string | undefined>(undefined);

export const SelectionToolbarSlot = createContext<HTMLDivElement | null>(null);
const RegisterSelectionToolbar = createContext<
  ((node: HTMLDivElement | null) => void) | undefined
>(undefined);

/** Selected actions occupy the same space as the default page actions. */
export function SelectionToolbar({ children }: { children: ReactNode }) {
  const register = useContext(RegisterSelectionToolbar);
  return (
    <div className="ml-auto flex min-w-0 flex-wrap items-center justify-end gap-2">
      <div ref={register} className="peer empty:hidden" />
      <div className="flex flex-wrap items-center gap-2 peer-[:not(:empty)]:hidden">
        {children}
      </div>
    </div>
  );
}

function useAdminHeadingInNavigation(title: string) {
  return title === useContext(AdminNavigationTitle);
}

export function AdminNavigation({ active }: { active: AdminSection }) {
  return (
    <HeaderLinks
      active={active}
      label="管理页面"
      links={adminDestinations}
      className="admin-navigation"
    />
  );
}
/** Owned by the persistent application shell, outside route loading boundaries. */
export function AdminWorkspace({
  active,
  children,
}: {
  active: AdminSection;
  children: ReactNode;
}) {
  const [slot, register] = useState<HTMLDivElement | null>(null);
  const link = adminDestinations.find((item) => item.id === active);
  return (
    <div className="admin-workspace">
      {link && <AdminNavigation active={active} />}
      <AdminNavigationTitle.Provider value={link?.label}>
        <RegisterSelectionToolbar.Provider value={register}>
          <SelectionToolbarSlot.Provider value={slot}>
            {children}
          </SelectionToolbarSlot.Provider>
        </RegisterSelectionToolbar.Provider>
      </AdminNavigationTitle.Provider>
    </div>
  );
}
export function AdminHeading({
  title,
  description,
  children,
  detail = false,
}: {
  title: string;
  description?: ReactNode;
  children?: ReactNode;
  detail?: boolean;
}) {
  const inNavigation = useAdminHeadingInNavigation(title);
  return (
    <PageHeader
      title={title}
      className="admin-heading"
      description={description}
      hideTitle={inNavigation && !detail}
    >
      {children ? (
        <ToolbarGroup
          appearance="plain"
          className="max-w-full flex-wrap"
          aria-label="页面操作"
        >
          {children}
        </ToolbarGroup>
      ) : null}
    </PageHeader>
  );
}
export function SelectionBar({
  count,
  total,
  disabled,
  onClear,
  children,
}: {
  count: number;
  total?: number;
  disabled?: boolean;
  onClear: () => void;
  children: ReactNode;
}) {
  const slot = useContext(SelectionToolbarSlot);
  if (!count) return null;
  const actions = (
    <ToolbarGroup
      appearance="plain"
      className="flex-wrap"
      aria-label="批量操作"
    >
      <span className="text-sm" aria-live="polite">
        已选 {count}
        {total === undefined ? "" : ` / ${total}`} 项
      </span>
      <IconAction
        variant="ghost"
        disabled={disabled}
        onClick={onClear}
        icon={X}
        label="清除选择"
      />
      <Toolbar className="flex-wrap sm:ml-auto">{children}</Toolbar>
    </ToolbarGroup>
  );
  return slot ? createPortal(actions, slot) : actions;
}
