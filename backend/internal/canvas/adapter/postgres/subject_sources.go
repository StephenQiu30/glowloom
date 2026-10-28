package postgres

import (
	"gorm.io/gorm"

	prompt "github.com/StephenQiu30/glowloom-server/backend/internal/prompt/application"
)

// SubjectSourcesFactory keeps formal subject bindings in the canvas transaction.
type SubjectSourcesFactory func(*gorm.DB) prompt.SubjectSources

// WithSubjectSources binds fixed subject versions through their owning context.
func (s *Store) WithSubjectSources(factory SubjectSourcesFactory) *Store {
	s.subjects = factory
	return s
}
func (s *Store) subjectSources(tx *gorm.DB) prompt.SubjectSources {
	if s.subjects == nil {
		return nil
	}
	return s.subjects(tx)
}
