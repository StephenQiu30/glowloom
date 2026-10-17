import { BasicLayout } from "@/components/layout/basic-layout";
import { ModelsPage } from "./_components/models-page";
export const metadata = { title: "模型管理 · 织光" };
export default function Page() {
  return (
    <BasicLayout screen="models">
      <ModelsPage />
    </BasicLayout>
  );
}
