import { notFound } from "next/navigation";
import { CanvasPreview } from "./screen";

export const metadata = { title: "画布 · Mock 预览" };
export default function Page() {
  if (process.env.NODE_ENV !== "development") notFound();
  return <CanvasPreview />;
}
