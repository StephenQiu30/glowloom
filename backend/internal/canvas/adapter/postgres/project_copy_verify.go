package postgres

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/lanverse/backend/internal/canvas/domain"
)

func copyDocumentValue(document domain.Document) (any, error) {
	document.Nodes = slices.Clone(document.Nodes)
	document.Edges = slices.Clone(document.Edges)
	slices.SortFunc(document.Nodes, func(a, b domain.Node) int { return strings.Compare(a.ID.String(), b.ID.String()) })
	slices.SortFunc(document.Edges, func(a, b domain.Edge) int { return strings.Compare(a.ID.String(), b.ID.String()) })
	body, err := json.Marshal(document)
	if err != nil {
		return nil, err
	}
	var value any
	err = json.Unmarshal(body, &value)
	return value, err
}
func verifyCopiedDocuments(tx *gorm.DB, copied []domain.ProjectDocumentCopy, targetID uuid.UUID) error {
	var count int
	if err := tx.Raw(`SELECT count(*) FROM canvas.document WHERE project_id=? AND NOT is_delete`, targetID).Scan(&count).Error; err != nil {
		return err
	}
	if count != len(copied) {
		return domain.ErrInvalidProjectCopy
	}
	for _, item := range copied {
		var row documentRow
		read := tx.Raw(`SELECT id,project_id,name,scope,revision,viewport FROM canvas.document WHERE id=? AND project_id=? AND NOT is_delete FOR SHARE`, item.Document.ID, targetID).Scan(&row)
		if read.Error != nil {
			return read.Error
		}
		if read.RowsAffected != 1 {
			return domain.ErrInvalidProjectCopy
		}
		actual, err := loadCopyDocument(tx, row)
		if err != nil {
			return err
		}
		want, err := copyDocumentValue(item.Document)
		if err != nil {
			return err
		}
		got, err := copyDocumentValue(actual)
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(want, got) {
			return domain.ErrInvalidProjectCopy
		}
	}
	return nil
}
