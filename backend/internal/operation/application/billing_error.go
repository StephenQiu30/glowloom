package application

// SafeProviderFailureCode keeps structured, reviewed provider classifications
// across the workflow boundary. A classification is never proof of zero charge.
func SafeProviderFailureCode(code, state string) string {
	switch code {
	case "insufficient_balance", "project_limit_exceeded", "organization_limit_exceeded", "insufficient_quota", "rate_limited", "request_limited", "payment_required", "auth_failed", "permission_denied", "ip_restricted", "region_restricted", "invalid_parameters", "not_found", "upstream_unavailable", "transport_unknown", "input_unavailable", "provider_unavailable":
		return "provider_" + code
	default:
		if state == "not_submitted" {
			return "provider_not_submitted"
		}
		return "provider_rejected"
	}
}
