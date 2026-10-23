"use client";

import {
  columnVisibilityFeature,
  rowSelectionFeature,
  tableFeatures,
  useTable,
  type ColumnDef,
  type RowData,
} from "@tanstack/react-table";
import type { Dispatch, SetStateAction } from "react";
import { Checkbox } from "@/components/ui/checkbox";
import { Empty, EmptyHeader, EmptyTitle } from "@/components/ui/empty";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";

// The API owns pagination and filtering; only render the returned page.
const features = tableFeatures({
  columnVisibilityFeature,
  rowSelectionFeature,
});
export type DataTableColumn<TData extends RowData> = ColumnDef<
  typeof features,
  TData
>;

type Props<TData extends { id?: string | number }> = {
  columns: DataTableColumn<TData>[];
  data: TData[];
  label: string;
  loading?: boolean;
  columnVisibility?: Record<string, boolean>;
  emptyTitle?: string;
  selection?: {
    selected: TData[];
    onChange: Dispatch<SetStateAction<TData[]>>;
    getLabel: (row: TData) => string;
    pageLabel: string;
    disabled?: boolean;
  };
};

export function DataTable<TData extends { id?: string | number }>({
  columns,
  data,
  label,
  loading = false,
  columnVisibility,
  emptyTitle = "暂无记录",
  selection,
}: Props<TData>) {
  const selectionDisabled = loading || selection?.disabled;
  const tableColumns: DataTableColumn<TData>[] = selection
    ? [
        {
          id: "selection",
          enableHiding: false,
          header: ({ table }) => (
            <Checkbox
              aria-label={selection.pageLabel}
              disabled={selectionDisabled || !data.length}
              checked={
                table.getIsAllPageRowsSelected() ||
                (table.getIsSomePageRowsSelected() && "indeterminate")
              }
              onCheckedChange={(checked) =>
                table.toggleAllPageRowsSelected(checked === true)
              }
            />
          ),
          cell: ({ row }) => (
            <Checkbox
              aria-label={`选择 ${selection.getLabel(row.original)}`}
              checked={row.getIsSelected()}
              disabled={selectionDisabled}
              onCheckedChange={(checked) =>
                row.toggleSelected(checked === true)
              }
            />
          ),
        },
        ...columns,
      ]
    : columns;
  const table = useTable({
    features,
    data,
    columns: tableColumns,
    getRowId: (row) => String(row.id),
    enableRowSelection: Boolean(selection) && !selectionDisabled,
    state: {
      columnVisibility,
      rowSelection: Object.fromEntries(
        selection?.selected.map((row) => [String(row.id), true]) ?? [],
      ),
    },
    onRowSelectionChange: (updater) =>
      selection?.onChange((previous) => {
        const previousState = Object.fromEntries(
          previous.map((row) => [String(row.id), true as const]),
        );
        const nextState =
          typeof updater === "function" ? updater(previousState) : updater;
        const snapshots = new Map(previous.map((row) => [String(row.id), row]));
        // Keep the revision captured when selected, even after a background read.
        return data
          .filter((row) => nextState[String(row.id)])
          .map((row) => snapshots.get(String(row.id)) ?? row);
      }),
  });

  return (
    <div
      className="min-w-0 shrink-0 overflow-hidden rounded-md border"
      aria-busy={loading}
    >
      <Table aria-label={label}>
        <TableHeader>
          {table.getHeaderGroups().map((headerGroup) => (
            <TableRow key={headerGroup.id}>
              {headerGroup.headers.map((header) => (
                <TableHead key={header.id} scope="col">
                  {header.isPlaceholder ? null : (
                    <table.FlexRender header={header} />
                  )}
                </TableHead>
              ))}
            </TableRow>
          ))}
        </TableHeader>
        <TableBody>
          {loading ? (
            <TableRow>
              {table.getVisibleLeafColumns().map((column) => (
                <TableCell key={column.id}>
                  <Skeleton className="h-4 w-full" />
                </TableCell>
              ))}
            </TableRow>
          ) : table.getRowModel().rows.length ? (
            table.getRowModel().rows.map((row) => (
              <TableRow
                key={row.id}
                data-state={row.getIsSelected() ? "selected" : undefined}
              >
                {row.getVisibleCells().map((cell) => (
                  <TableCell key={cell.id}>
                    <table.FlexRender cell={cell} />
                  </TableCell>
                ))}
              </TableRow>
            ))
          ) : (
            <TableRow>
              <TableCell colSpan={table.getVisibleLeafColumns().length}>
                <Empty>
                  <EmptyHeader>
                    <EmptyTitle>{emptyTitle}</EmptyTitle>
                  </EmptyHeader>
                </Empty>
              </TableCell>
            </TableRow>
          )}
        </TableBody>
      </Table>
    </div>
  );
}
