"use client";
import { LineChart, Line, CartesianGrid, XAxis, YAxis } from "recharts";
import {
  ChartContainer,
  ChartTooltip,
  ChartTooltipContent,
  ChartLegend,
  ChartLegendContent,
} from "@/components/ui/chart";
const config = {
  accepted: { label: "已受理任务", color: "var(--foreground)" },
  completed: { label: "已完成任务", color: "var(--muted-foreground)" },
};
export default function CreationChart({ daily }: { daily: API.Day[] }) {
  return (
    <ChartContainer
      config={config}
      className="h-52 w-full"
      aria-label="每日已接受与已完成任务趋势"
    >
      <LineChart data={daily} accessibilityLayer>
        <CartesianGrid vertical={false} />
        <XAxis
          dataKey="date"
          tickLine={false}
          axisLine={false}
          tickFormatter={(v) => String(v).slice(5)}
        />
        <YAxis
          allowDecimals={false}
          tickLine={false}
          axisLine={false}
          width={38}
        />
        <ChartTooltip content={<ChartTooltipContent />} />
        <ChartLegend content={<ChartLegendContent />} />
        <Line
          type="monotone"
          dataKey="accepted"
          stroke="var(--color-accepted)"
          strokeWidth={2}
          dot={false}
          isAnimationActive={false}
        />
        <Line
          type="monotone"
          dataKey="completed"
          stroke="var(--color-completed)"
          strokeWidth={2}
          strokeDasharray="4 4"
          dot={false}
          isAnimationActive={false}
        />
      </LineChart>
    </ChartContainer>
  );
}
