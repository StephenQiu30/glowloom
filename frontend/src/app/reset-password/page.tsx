import { AuthPage } from "@/components/fuguang/auth-pages";

export const metadata = { title: "设置新密码" };
export default function Page() {
  return <AuthPage mode="reset-password" />;
}
