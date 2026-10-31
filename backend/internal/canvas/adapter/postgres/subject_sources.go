package postgres

import (
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"

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

// CopySubjectSourcesFactory authorizes fixed pins against one unpublished copy target.
type CopySubjectSourcesFactory func(*gorm.DB, application.ProjectCopyBinding) prompt.SubjectSources

// WithSubjectSources supplies the copy owner; missing proofs fail closed for pinned nodes.
func (s *ProjectCopyStore) WithSubjectSources(factory CopySubjectSourcesFactory) *ProjectCopyStore {
	s.subjects = factory
	return s
}
func (s *ProjectCopyStore) copySubjectSources(tx *gorm.DB, binding application.ProjectCopyBinding) prompt.SubjectSources {
	if s.subjects == nil {
		return nil
	}
	return s.subjects(tx, binding)
}
