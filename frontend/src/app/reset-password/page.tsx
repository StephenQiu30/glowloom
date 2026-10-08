import { AuthView } from "@/components/auth/auth-view";

export const metadata = { title: "设置新密码" };
export default function Page() {
  return <AuthView mode="reset-password" />;
}
