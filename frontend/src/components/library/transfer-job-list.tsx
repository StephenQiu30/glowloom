"use client";
import { useRef } from "react";
import { useVirtualizer } from "@tanstack/react-virtual";
import { Button } from "@/components/ui/button";
import type { TransferJob } from "./transfer-model";
import { transferStatusLabels } from "./transfer-progress";
type Props = {
  jobs: TransferJob[];
  focused?: string;
  busy: boolean;
  onSelect: (id: string) => void;
};
export function TransferJobList(props: Props) {
  const scroll = useRef<HTMLDivElement>(null);
  // eslint-disable-next-line react-hooks/incompatible-library -- TanStack Virtual owns DOM measurement; this component is not manually memoized.
  const virtual = useVirtualizer({
    count: props.jobs.length,
    getScrollElement: () => scroll.current,
    estimateSize: () => 110,
    getItemKey: (index) => props.jobs[index].id,
    overscan: 4,
    enabled: props.jobs.length > 100,
  });
  if (props.jobs.length > 100)
    return (
      <div
        ref={scroll}
        role="list"
        aria-label="迁移任务列表"
        className="h-96 overflow-auto"
      >
        <div className="relative" style={{ height: virtual.getTotalSize() }}>
          {virtual.getVirtualItems().map((row) => (
            <div
              role="listitem"
              key={row.key}
              data-index={row.index}
              ref={virtual.measureElement}
              className="absolute top-0 left-0 w-full pb-2"
              style={{ transform: `translateY(${row.start}px)` }}
            >
              <TransferJobButton {...props} job={props.jobs[row.index]} />
            </div>
          ))}
        </div>
      </div>
    );
  return (
    <ul className="max-h-96 space-y-2 overflow-auto" aria-label="迁移任务列表">
      {props.jobs.map((job) => (
        <li key={job.id}>
          <TransferJobButton {...props} job={job} />
        </li>
      ))}
    </ul>
  );
}
function TransferJobButton({
  job,
  focused,
  busy,
  onSelect,
}: Props & { job: TransferJob }) {
  return (
    <Button
      variant={focused === job.id ? "secondary" : "outline"}
      className="h-auto w-full justify-start text-left whitespace-normal"
      aria-label={`查看迁移任务 ${job.id}`}
      disabled={busy}
      onClick={() => onSelect(job.id)}
    >
      <span className="break-all">
        {job.id}
        <span className="block">
          {transferStatusLabels[job.status]} · 尝试 {job.attempt} · 修订{" "}
          {job.revision}
        </span>
      </span>
    </Button>
  );
}
