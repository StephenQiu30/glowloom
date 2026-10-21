import { notFound } from "next/navigation";
import { CanvasPreview } from "./screen";

export const metadata = { title: "无限画布" };
export default function Page() {
  if (process.env.NODE_ENV !== "development") notFound();
  return <CanvasPreview />;
}
