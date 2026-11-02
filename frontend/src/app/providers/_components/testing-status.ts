export function testErrorMessage(code: string) {
  const messages: Record<string, string> = {
    auth_failed: "凭据被拒，请核对当前 Key。",
    permission_denied: "接口权限不足，请检查服务开通与模型授权。",
    region_restricted: "当前地域不受支持。",
    ip_restricted: "请求 IP 不在服务商允许范围。",
    insufficient_balance: "服务商余额不足，请补充余额后再验证。",
    insufficient_quota: "服务商额度不足，请检查计费设置。",
    project_limit_exceeded: "服务商项目支出已达上限。",
    organization_limit_exceeded: "服务商组织用量已达上限。",
    rate_limited: "请求过于频繁，请稍后再试。",
    not_found: "模型不可用，请检查区域与版本。",
    invalid_parameters: "验证配置不匹配，请检查模型参数。",
    invalid_response: "服务商响应格式无法识别，未判定为通过。",
    unreachable: "无法连接服务商。",
    upstream_unavailable: "服务商暂不可用，尚未确认模型可用。",
    unknown: "结果尚未确认，请先核查原请求。",
    dns_failed: "域名解析失败。",
    tls_failed: "TLS 连接失败。",
    connection_failed: "无法建立连接。",
    timeout: "连接超时。",
    configuration_changed: "配置已变化，本次检查已结束，请验证当前版本。",
    check_interrupted: "检查中断，尚未确认结果。",
  };
  return messages[code] ?? "结果尚未确认，请先核查原请求。";
}
export const checkRunning = (check?: API.ProviderCheck | null) =>
  ["pending", "running"].includes(check?.status ?? "");
