"use client";

import { useRef, useState, type ReactNode, type SetStateAction } from "react";
import { Filters, type FilterField } from "@/components/insights/shared";
import { SelectionBar } from "@/components/governance/admin-layout";
import { RequestError } from "@/lib/request";
import {
  AlertDialog,
  AlertDialogContent,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogAction,
  AlertDialogCancel,
} from "@/components/ui/alert-dialog";
import { Copy, Trash2 } from "lucide-react";
import { toast } from "@/components/feedback/notification";
import { DataTable } from "@/components/canvas/runtime/components/account/list/data-table";
import { ListPagination } from "@/components/canvas/runtime/components/layout/workspace/list-pagination";
import { DEFAULT_PAGE_SIZE, paginationRange } from "@/lib/pagination";
import { GatewayAction } from "./gateway-action";
import { Toolbar } from "@/components/ui/toolbar";

export type GatewayTableRow = {
  id: string;
  label: string;
  deleteID?: string;
  terms?: string[];
  status?: string;
  cells: ReactNode[];
};

export function GatewayTable({
  label,
  columns,
  rows,
  empty,
  onDelete,
  externalFilterKey,
  toolbarActions,
  filters,
  summary,
}: {
  label: string;
  columns: string[];
  rows: GatewayTableRow[];
  empty: string;
  onDelete?: (ids: string[]) => Promise<void>;
  externalFilterKey?: string;
  toolbarActions?: ReactNode;
  filters?: ReactNode;
  summary?: ReactNode;
}) {
  const [filter, setFilter] = useState<Record<string, string>>({});
  const statuses = [
    ...new Set(rows.flatMap((row) => (row.status ? [row.status] : []))),
  ];
  const fields: FilterField[] = [{ key: "keyword", label: "关键词" }];
  if (statuses.length)
    fields.push({
      key: "status",
      label: "状态",
      options: [
        ["", "全部状态"],
        ...statuses.map((status) => [status, status] as const),
      ],
    });
  const keyword = filter.keyword?.toLowerCase();
  const matching = rows.filter(
    (row) =>
      (!filter.status || row.status === filter.status) &&
      (!keyword ||
        [row.label, row.id, ...(row.terms ?? [])].some((value) =>
          value.toLowerCase().includes(keyword),
        )),
  );
  const [targets, setTargets] = useState<GatewayTableRow[]>([]);
  const [deleting, setDeleting] = useState(false);
  const pending = useRef(false);
  const [error, setError] = useState("");
  const [current, setCurrent] = useState(1);
  const [pageSize, setPageSize] = useState<number>(DEFAULT_PAGE_SIZE);
  const [selectedIds, setSelectedIds] = useState<Set<string>>(() => new Set());
  const [filterKey, setFilterKey] = useState(externalFilterKey);
  const filterChanged = filterKey !== externalFilterKey;
  if (filterChanged) {
    setFilterKey(externalFilterKey);
    setCurrent(1);
    setSelectedIds(new Set());
  }
  const selected = rows.filter((row) => selectedIds.has(row.id));
  if (!filterChanged && selected.length !== selectedIds.size) {
    setSelectedIds(new Set(selected.map((row) => row.id)));
  }
  const { page, start, end } = paginationRange(
    matching.length,
    current,
    pageSize,
  );
  if (!filterChanged && page !== current) setCurrent(page);
  const visible = matching.slice(Math.max(0, start - 1), end);

  function updateSelection(update: SetStateAction<GatewayTableRow[]>) {
    setSelectedIds((previous) => {
      const pageSelection = visible.filter((row) => previous.has(row.id));
      const nextPage =
        typeof update === "function" ? update(pageSelection) : update;
      const next = new Set(previous);
      visible.forEach((row) => next.delete(row.id));
      nextPage.forEach((row) => next.add(row.id));
      return next;
    });
  }

  async function copySelected() {
    try {
      await navigator.clipboard.writeText(
        selected.map((row) => row.label).join("\n"),
      );
      toast.success(`已复制 ${selected.length} 项`);
    } catch {
      toast.error("无法复制，请检查浏览器剪贴板权限。");
    }
  }

  function confirmDelete(rows: GatewayTableRow[]) {
    setError("");
    setTargets(rows);
  }

  async function remove() {
    if (!onDelete || pending.current || !targets.length) return;
    const ids = targets.flatMap((row) => (row.deleteID ? [row.deleteID] : []));
    if (ids.length !== targets.length) return;
    pending.current = true;
    setDeleting(true);
    setError("");
    try {
      await onDelete(ids);
      setSelectedIds((previous) => {
        const next = new Set(previous);
        targets.forEach((row) => next.delete(row.id));
        return next;
      });
      setTargets([]);
      toast.success(`已删除 ${ids.length} 个渠道`);
    } catch (cause) {
      setError(
        cause instanceof RequestError &&
          cause.code === "route_verification_in_progress"
          ? "所选渠道正在验证，请在验证结束后再删除。"
          : cause instanceof RequestError && cause.status === 403
            ? "当前账号没有删除权限。"
            : "删除未完成，请刷新渠道列表后重试。",
      );
    } finally {
      pending.current = false;
      setDeleting(false);
    }
  }

  const actions =
    selected.length > 0 ? (
      <SelectionBar
        count={selected.length}
        disabled={deleting}
        onClear={() => setSelectedIds(new Set())}
      >
        <GatewayAction
          label="复制所选条目"
          icon={Copy}
          onClick={copySelected}
        />
        {onDelete && (
          <GatewayAction
            label="删除所选渠道"
            icon={Trash2}
            variant="destructive"
            disabled={deleting || selected.some((row) => !row.deleteID)}
            onClick={() => confirmDelete(selected)}
          />
        )}
      </SelectionBar>
    ) : null;

  return (
    <div className="flex min-w-0 flex-col gap-3">
      <Toolbar role="group" aria-label={`${label}工具栏`} className="flex-wrap">
        {externalFilterKey === undefined ? (
          <Filters
            fields={fields}
            onApply={(next) => {
              setFilter(next);
              setCurrent(1);
              setSelectedIds(new Set());
            }}
          />
        ) : (
          filters
        )}
        {(actions || toolbarActions) && (
          <div className="ml-auto flex flex-wrap items-center gap-1">
            {actions || toolbarActions}
          </div>
        )}
      </Toolbar>
      {summary}
      <DataTable<GatewayTableRow>
        label={label}
        data={visible}
        emptyTitle={Object.keys(filter).length ? "没有符合条件的记录" : empty}
        columns={columns.map((title, index) => ({
          id: title,
          header: () => (
            <span className={title === "操作" ? "block text-right" : undefined}>
              {title}
            </span>
          ),
          cell: ({ row }) => (
            <div
              className={
                title === "操作"
                  ? "flex justify-end gap-2"
                  : "wrap-anywhere whitespace-normal max-md:w-max max-md:max-w-64"
              }
            >
              {row.original.cells[index]}
              {title === "操作" && onDelete && row.original.deleteID && (
                <GatewayAction
                  label={`删除渠道 ${row.original.label}`}
                  icon={Trash2}
                  onClick={() => confirmDelete([row.original])}
                />
              )}
            </div>
          ),
        }))}
        selection={{
          selected,
          pageLabel: `全选本页${label}`,
          getLabel: (row) => row.label,
          onChange: updateSelection,
        }}
      />
      {targets.length > 0 && (
        <AlertDialog
          open
          onOpenChange={(open) => {
            if (!open && !pending.current) setTargets([]);
          }}
        >
          <AlertDialogContent>
            <AlertDialogHeader>
              <AlertDialogTitle>
                {targets.length === 1
                  ? `删除渠道「${targets[0].label}」？`
                  : `删除选中的 ${targets.length} 个渠道？`}
              </AlertDialogTitle>
              <AlertDialogDescription>
                删除后停止接收新请求，历史账单和已发出的请求仍可查询。
              </AlertDialogDescription>
            </AlertDialogHeader>
            {error && (
              <p role="alert" className="text-sm text-destructive">
                {error}
              </p>
            )}
            <AlertDialogFooter>
              <AlertDialogCancel disabled={deleting}>取消</AlertDialogCancel>
              <AlertDialogAction
                variant="destructive"
                disabled={deleting}
                onClick={(event) => {
                  event.preventDefault();
                  void remove();
                }}
              >
                确认删除
              </AlertDialogAction>
            </AlertDialogFooter>
          </AlertDialogContent>
        </AlertDialog>
      )}
      <ListPagination
        label={`${label}分页`}
        total={matching.length}
        current={page}
        pageSize={pageSize}
        onPageChange={setCurrent}
        onPageSizeChange={(size) => {
          setPageSize(size);
          setCurrent(1);
        }}
      />
    </div>
  );
}
