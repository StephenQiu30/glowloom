// A safe receipt must describe this exact connection before UI claims success.
export function isCredentialReceipt(result: unknown, target: string) {
  return (
    !!result &&
    typeof result === "object" &&
    "id" in result &&
    typeof result.id === "string" &&
    !!result.id &&
    "provider_id" in result &&
    result.provider_id === target &&
    "status" in result &&
    result.status === "active" &&
    "last4" in result &&
    typeof result.last4 === "string" &&
    result.last4.length === 4
  );
}
