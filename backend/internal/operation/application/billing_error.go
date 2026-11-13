package application

// SafeProviderFailureCode keeps structured, reviewed provider classifications
// across the workflow boundary. A classification is never proof of zero charge.
func SafeProviderFailureCode(code, state string) string {
	switch code {
	case "invalid_provider_request":
		return "provider_invalid_parameters"
	case "insufficient_balance", "project_limit_exceeded", "organization_limit_exceeded", "insufficient_quota", "rate_limited", "request_limited", "payment_required", "auth_failed", "permission_denied", "ip_restricted", "region_restricted", "invalid_parameters", "input_content_rejected", "output_content_rejected", "request_rejected", "not_found", "upstream_unavailable", "transport_unknown", "input_unavailable", "provider_unavailable":
		return "provider_" + code
	default:
		if state == "not_submitted" {
			return "provider_not_submitted"
		}
		return "provider_rejected"
	}
}
