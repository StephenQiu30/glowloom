package application

import "slices"

// validLibraryPickerFilters uses measured asset dimensions; quality is not inferred from pixels.
func validLibraryPickerFilters(q LibraryQuery) bool {
	if !slices.Contains([]string{"", "upload", "generated", "system"}, q.Origin) {
		return false
	}
	if q.CreatedFrom != nil && q.CreatedFrom.IsZero() || q.CreatedBefore != nil && q.CreatedBefore.IsZero() || q.CreatedFrom != nil && q.CreatedBefore != nil && !q.CreatedFrom.Before(*q.CreatedBefore) {
		return false
	}
	if (len(q.Resolutions) > 0 || len(q.Ratios) > 0) && q.Kind != "image" {
		return false
	}
	for _, filter := range []struct{ values, allowed []string }{
		{q.Resolutions, []string{"1K", "2K", "4K", "8K"}},
		{q.Ratios, []string{"21:9", "16:9", "3:2", "4:3", "1:1", "3:4", "2:3", "9:16"}},
	} {
		seen := make(map[string]bool)
		for _, value := range filter.values {
			if !slices.Contains(filter.allowed, value) || seen[value] {
				return false
			}
			seen[value] = true
		}
	}
	return true
}
