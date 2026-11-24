"use client";

import { useState } from "react";
import { Copy } from "lucide-react";
import { Checkbox } from "@/components/ui/checkbox";
import { IconAction } from "@/components/ui/icon-action";
import { toast } from "@/components/feedback/notification";
import { SelectionBar } from "./admin-layout";

/** Read-only directories can copy selected references without changing their records. */
export function usePageSelection<T extends { id: string }>(
  rows: readonly T[],
  scope: string,
  label: string,
  getReference: (row: T) => string,
) {
  const [state, setState] = useState({
    scope,
    values: new Map<string, string>(),
  });
  if (state.scope !== scope) setState({ scope, values: new Map() });
  const selected = state.values;
  const pageCount = rows.filter((row) => selected.has(row.id)).length;
  function toggle(targets: readonly T[], checked: boolean) {
    setState((previous) => {
      const values = new Map(previous.values);
      for (const row of targets) {
        if (checked) values.set(row.id, getReference(row));
        else values.delete(row.id);
      }
      return { scope, values };
    });
  }
  const selectAll = (
    <Checkbox
      aria-label={`全选本页${label}`}
      disabled={!rows.length}
      checked={
        pageCount === 0
          ? false
          : pageCount === rows.length
            ? true
            : "indeterminate"
      }
      onCheckedChange={(checked) => toggle(rows, checked === true)}
    />
  );
  function selectRow(row: T) {
    return (
      <Checkbox
        aria-label={`选择 ${getReference(row)}`}
        checked={selected.has(row.id)}
        onCheckedChange={(checked) => toggle([row], checked === true)}
      />
    );
  }
  async function copy() {
    try {
      await navigator.clipboard.writeText(
        Array.from(selected.values()).join("\n"),
      );
      toast.success(`已复制 ${selected.size} 项`);
    } catch {
      toast.error("无法复制，请检查浏览器剪贴板权限。");
    }
  }
  const actions = (
    <SelectionBar
      count={selected.size}
      onClear={() => setState({ scope, values: new Map() })}
    >
      <IconAction label="复制所选条目" icon={Copy} onClick={copy} />
    </SelectionBar>
  );
  return { selectAll, selectRow, actions };
}
