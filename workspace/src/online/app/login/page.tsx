import { Login } from "@/online/components/login";
export const metadata = { title: "访问工作空间" };
export default function LoginPage() {
  return (
    <main id="main" className="page" style={{ maxWidth: 650, paddingTop: 90 }}>
      <div className="page-heading">
        <div>
          <h1>访问工作空间</h1>
          <p>验证本机身份后，进入知识与研发协作。</p>
        </div>
      </div>
      <Login />
    </main>
  );
}
