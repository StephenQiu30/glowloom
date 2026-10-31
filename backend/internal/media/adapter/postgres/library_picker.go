package postgres

import (
	"strconv"
	"strings"

	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
)

// libraryPickerFilters runs before count/order/page in the same authorized library transaction.
func libraryPickerFilters(q application.LibraryQuery) ([]string, []any) {
	var conditions []string
	var args []any
	if q.CreatedFrom != nil {
		conditions = append(conditions, "create_time>=?")
		args = append(args, *q.CreatedFrom)
	}
	if q.CreatedBefore != nil {
		conditions = append(conditions, "create_time<?")
		args = append(args, *q.CreatedBefore)
	}
	if q.Origin != "" {
		conditions = append(conditions, "asset_facts->>'origin'=?")
		args = append(args, q.Origin)
	}
	if len(q.Resolutions) > 0 {
		dimensions := make([]int, 0, len(q.Resolutions))
		for _, value := range q.Resolutions {
			dimensions = append(dimensions, map[string]int{"1K": 1024, "2K": 2048, "4K": 4096, "8K": 8192}[value])
		}
		conditions = append(conditions, "greatest((asset_facts->>'width')::integer,(asset_facts->>'height')::integer) IN ?")
		args = append(args, dimensions)
	}
	if len(q.Ratios) > 0 {
		ratios := make([]string, 0, len(q.Ratios))
		for _, value := range q.Ratios {
			parts := strings.Split(value, ":")
			w, _ := strconv.Atoi(parts[0])
			h, _ := strconv.Atoi(parts[1])
			ratios = append(ratios, "((asset_facts->>'width')::bigint * ? = (asset_facts->>'height')::bigint * ?)")
			args = append(args, h, w)
		}
		conditions = append(conditions, "((asset_facts->>'width')::integer>0 AND (asset_facts->>'height')::integer>0 AND ("+strings.Join(ratios, " OR ")+"))")
	}
	return conditions, args
}
