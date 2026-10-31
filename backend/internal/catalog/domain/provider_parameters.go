package domain

import "slices"

// ParameterFields returns fresh parameter slices permitted for the given capability
// and provider configuration, looked up dynamically via the adapter specification registry.
func (c ProviderConfig) ParameterFields(capability string) []string {
	spec, ok := GetAdapterSpec(c.AdapterKey)
	if !ok {
		return nil
	}
	if fields, exists := spec.ParameterFields[capability]; exists {
		return slices.Clone(fields)
	}
	return nil
}

// IsManagedCloud identifies protocols that require a frozen catalog release.
func IsManagedCloud(adapter string) bool {
	if spec, ok := GetAdapterSpec(adapter); ok {
		return spec.IsManagedCloud
	}
	return false
}
