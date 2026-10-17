import { BasicLayout } from "@/components/layout/basic-layout";
import { InspirationGallery } from "@/components/inspiration/gallery";
import type { AuthMode } from "./auth-dialog";

export function AuthEntry({ mode }: { mode: AuthMode }) {
  return (
    <BasicLayout screen="home" allowGuest initialAuthMode={mode}>
      <InspirationGallery />
    </BasicLayout>
  );
}
