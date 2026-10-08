import { Empty, EmptyHeader, EmptyTitle } from "@/components/ui/empty";

export function EmptySearch({
  label = "没有符合条件的结果",
}: {
  label?: string;
}) {
  return (
    <Empty role="status">
      <EmptyHeader>
        <EmptyTitle>{label}</EmptyTitle>
      </EmptyHeader>
    </Empty>
  );
}
