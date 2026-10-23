/** Public, server-sanitized diagnostics only; upstream messages never enter UI copy. */
export type GenerationFailure = {
  state?: string;
  failureKind?: string;
  failureDiagnostic?: {
    stage?: string;
    httpStatus?: number;
    providerCode?: string;
    providerRequestId?: string;
  };
};

type FailurePresentation = {
  reason: string;
  action: "edit" | "details" | "query" | "retry";
};

const administratorCodes = new Set([
  "AuthenticationError",
  "InvalidAccountStatus",
  "OperationDenied.ServiceNotOpen",
  "OperationDenied.ServiceOverdue",
  "OperationDenied.PermissionDenied",
  "InvalidEndpointOrModel.NotFound",
  "InvalidEndpointOrModel.ModelIDAccessDisabled",
  "ModelNotOpen",
  "UnsupportedModel",
  "InvalidEndpoint.ClosedEndpoint",
]);
const parameterCodes = new Set([
  "MissingParameter",
  "InvalidParameter",
  "InvalidParameter.TaskTypeConstraint",
  "InvalidParameter.TaskTypeMismatch",
  "InvalidParameter.UnsupportedParameter",
  "InvalidParameter.TosURLInvalid",
]);
const rateLimitCodes = new Set([
  "RateLimitExceeded.EndpointRPMExceeded",
  "RateLimitExceeded.EndpointTPMExceeded",
  "RateLimitExceeded.EndpointFlexTPMExceeded",
  "ModelAccountRpmRateLimitExceeded",
]);
const contentCodes = new Set([
  "SensitiveContentDetected",
  "SensitiveContentDetected.SevereViolation",
  "SensitiveContentDetected.Violence",
  "OutputImageSensitiveContentDetected.DeepFake",
  ...["Input", "Output"].flatMap((direction) =>
    ["Text", "Image", "Video", "Audio"].flatMap((type) => [
      `${direction}${type}SensitiveContentDetected`,
      `${direction}${type}SensitiveContentDetected.PolicyViolation`,
    ]),
  ),
]);

export function generationFailurePresentation(
  task?: GenerationFailure,
): FailurePresentation {
  const diagnostic = task?.failureDiagnostic;
  const code = diagnostic?.providerCode ?? "";
  // An unknown acceptance or failed read/control never authorizes another submit.
  if (
    ["SUBMISSION_UNKNOWN", "UNCONFIRMED"].includes(task?.state ?? "") ||
    ["submission_unknown", "query_unavailable"].includes(
      task?.failureKind ?? "",
    ) ||
    ["provider_query", "provider_control"].includes(diagnostic?.stage ?? "")
  )
    return { reason: "任务状态待确认，请查询原任务", action: "query" };
  if (diagnostic?.stage === "archive" || task?.failureKind === "archive_failed")
    return { reason: "结果保存失败，请恢复原任务", action: "query" };
  if (diagnostic?.stage === "input_validation")
    return { reason: "输入参数不符合模型要求，请修改输入", action: "edit" };
  if (
    code === "InputImageSensitiveContentDetected.PrivacyInformation" ||
    code === "InputVideoSensitiveContentDetected.PrivacyInformation"
  )
    return {
      reason: "人脸参考未通过供应商校验，请核对素材资格或更换参考",
      action: "edit",
    };
  if (contentCodes.has(code))
    return {
      reason: "输入或生成内容未通过供应商要求，请修改输入",
      action: "edit",
    };
  if (parameterCodes.has(code))
    return { reason: "生成参数不符合模型要求，请修改输入", action: "edit" };
  if (code === "QuotaExceeded")
    return { reason: "供应商额度不足，请联系管理员核对", action: "details" };
  if (
    administratorCodes.has(code) ||
    task?.failureKind === "authentication" ||
    diagnostic?.httpStatus === 401
  )
    return {
      reason: "模型鉴权或使用权限异常，请联系管理员核对",
      action: "details",
    };
  if (
    ["FAILED", "DISPATCH_FAILED"].includes(task?.state ?? "") &&
    (task?.failureKind === "rate_limit" || rateLimitCodes.has(code))
  )
    return { reason: "供应商请求过多，可稍后主动重试", action: "retry" };
  if (task?.failureKind === "rejected" && !diagnostic)
    return { reason: "请求被拒绝，未保存具体原因", action: "edit" };
  return { reason: "失败原因暂不明确，请先查询任务状态", action: "query" };
}

export function seedanceReferenceHint(
  modelName: string | undefined,
  hasReferences: boolean,
) {
  return hasReferences && /seedance/i.test(modelName ?? "")
    ? "人脸参考请核对素材资格：普通上传不等于供应商素材资格，请使用同账号原始信任产物或符合要求的授权素材。"
    : undefined;
}
