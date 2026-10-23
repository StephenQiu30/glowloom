import { Scan, Square } from "lucide-react";

export function MediaRatioIcon({
  width,
  height,
}: {
  width: number;
  height: number;
}) {
  const longSide = Math.max(width, height);

  return (
    <span
      aria-hidden="true"
      className="grid h-7 w-9 shrink-0 place-items-center"
    >
      {width > 0 && height > 0 ? (
        <Square
          preserveAspectRatio="none"
          style={{
            width: (width / longSide) * 24,
            height: (height / longSide) * 24,
          }}
        />
      ) : (
        <Scan className="size-6" />
      )}
    </span>
  );
}
