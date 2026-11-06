"use client";

import { useState, type ComponentProps, type ReactNode } from "react";
import {
  DataTable,
  type DataTableColumn,
} from "@/components/canvas/runtime/components/account/list/data-table";
import { ListPagination } from "@/components/canvas/runtime/components/layout/workspace/list-pagination";
import { ContentState } from "@/components/layout/content-state";
import { Button } from "@/components/ui/button";
import { useIsMobile } from "@/hooks/use-mobile";
import { AdminHeading, SelectionBar } from "./admin-layout";
import { ColumnMenu } from "./column-menu";
import {
  AdminTableFilters,
  type AdminTableFilterConfig,
  type AdminTableFilterValues,
} from "./admin-table-filters";

export type { AdminTableFilterValues } from "./admin-table-filters";

type Row = { id?: string | number };

export type AdminTableColumn<
  T extends Row,
  FilterKey extends string = never,
> = DataTableColumn<T> & {
  id: string;
  header: string;
  filter?: AdminTableFilterConfig<FilterKey>;
  settings?: { defaultVisible?: boolean; hideOnMobile?: boolean };
};

type Selection<T extends Row> = NonNullable<
  ComponentProps<typeof DataTable<T>>["selection"]
> & {
  actions?: ReactNode;
  inToolbar?: boolean;
  clearDisabled?: boolean;
};

type Pagination = Omit<
  Extract<ComponentProps<typeof ListPagination>, { mode: "cursor" }>,
  "mode" | "itemCount"
>;

type Props<T extends Row, FilterKey extends string> = {
  title: string;
  label: string;
  columns: AdminTableColumn<T, FilterKey>[];
  rows: T[];
  actions?: ReactNode;
  toolbarExtra?: ReactNode;
  context?: ReactNode;
  footer?: ReactNode;
  filter?: {
    label: string;
    disabled?: boolean;
    onApply: (values: AdminTableFilterValues<FilterKey>) => void;
  };
  selection?: Selection<T>;
  pagination?: Pagination;
  loading?: string;
  error?: { title: string; description: string; onRetry: () => void };
  emptyTitle?: string;
};

export function AdminTable<T extends Row, FilterKey extends string>({
  title,
  label,
  columns,
  rows,
  actions,
  toolbarExtra,
  context,
  footer,
  filter,
  selection,
  pagination,
  loading,
  error,
  emptyTitle,
}: Props<T, FilterKey>) {
  const mobile = useIsMobile();
  const [visibility, setVisibility] = useState<Record<string, boolean>>({});
  const configurable = columns.filter(({ settings }) => settings);
  const columnVisibility = Object.fromEntries(
    configurable.map(({ id, settings }) => [
      id,
      visibility[id] ?? settings?.defaultVisible ?? true,
    ]),
  );
  const fields = columns.flatMap(({ filter, header }) =>
    filter ? [{ ...filter, label: filter.label ?? header }] : [],
  );
  const ready = !loading && !error;
  const selectionBar =
    ready && selection?.selected.length ? (
      <SelectionBar
        count={selection.selected.length}
        total={selection.inToolbar ? undefined : rows.length}
        disabled={selection.clearDisabled ?? selection.disabled}
        onClear={() => selection.onChange([])}
      >
        {selection.actions}
      </SelectionBar>
    ) : null;
  const selectionInToolbar = selection?.inToolbar && selectionBar;

  return (
    <div className="flex min-w-0 flex-col gap-5">
      <AdminHeading title={title}>
        {toolbarExtra}
        {selectionInToolbar || (
          <>
            {ready && !mobile && rows.length > 0 && configurable.length > 0 ? (
              <ColumnMenu
                columns={configurable.map(({ id, header }) => ({
                  id,
                  label: header,
                }))}
                visibility={columnVisibility}
                onChange={setVisibility}
              />
            ) : null}
            {actions}
          </>
        )}
      </AdminHeading>
      {context}
      {filter && fields.length > 0 ? (
        <AdminTableFilters {...filter} fields={fields} />
      ) : null}
      {loading ? (
        <ContentState state="loading" title={loading} />
      ) : error ? (
        <ContentState
          state="error"
          title={error.title}
          description={error.description}
          actions={<Button onClick={error.onRetry}>重试</Button>}
        />
      ) : (
        <div className="flex min-w-0 flex-col gap-3">
          {selection?.inToolbar ? null : selectionBar}
          <DataTable
            density="comfortable"
            label={label}
            columns={columns}
            data={rows}
            emptyTitle={emptyTitle}
            columnVisibility={Object.fromEntries(
              configurable.map(({ id, settings }) => [
                id,
                columnVisibility[id] && !(mobile && settings?.hideOnMobile),
              ]),
            )}
            selection={selection}
          />
          {pagination ? (
            <ListPagination
              mode="cursor"
              itemCount={rows.length}
              {...pagination}
            />
          ) : null}
          {footer}
        </div>
      )}
    </div>
  );
}
