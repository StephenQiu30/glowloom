package application

import (
	"context"
	"fmt"
	"strconv"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	prompt "github.com/StephenQiu30/glowloom-server/backend/internal/prompt/application"
	promptdomain "github.com/StephenQiu30/glowloom-server/backend/internal/prompt/domain"
)

// ValidateSubjectNodes reauthorizes every fixed version and all its media through
// the subject owner. It deliberately does not reuse previously accepted proofs.
func ValidateSubjectNodes(ctx context.Context, actor identity.Principal, project uuid.UUID, nodes []domain.Node, sources prompt.SubjectSources) error {
	for _, node := range nodes {
		pin := node.Config.Subject
		if pin == nil || pin.SubjectID == nil && pin.VersionID == nil {
			continue
		}
		if sources == nil || pin.SubjectID == nil || pin.VersionID == nil {
			return promptdomain.ErrReferenceUnavailable
		}
		source, found, err := sources.ResolveSubject(ctx, actor, project, pin.SubjectID.String(), pin.VersionID.String())
		if err != nil {
			return fmt.Errorf("%w: %w", promptdomain.ErrReferenceUnavailable, err)
		}
		if !found || source.Kind != "subject" || source.SourceID != pin.SubjectID.String() || source.SourceVersion != pin.VersionID.String() || len(source.Media) == 0 {
			return promptdomain.ErrReferenceUnavailable
		}
		mediaInputs := source.Media
		if pin.Local != nil {
			mediaInputs = nil
			for _, selected := range pin.Local.Media() {
				if selected.BindingID == nil {
					continue
				}
				candidate := promptdomain.SourceMedia{SourceID: selected.AssetID.String(), SourceVersion: strconv.FormatInt(selected.AssetRevision, 10), MediaKind: selected.Kind, BindingID: selected.BindingID.String()}
				found := false
				for _, original := range source.Media {
					if original == candidate {
						found = true
						break
					}
				}
				if !found {
					return promptdomain.ErrReferenceUnavailable
				}
				mediaInputs = append(mediaInputs, candidate)
			}
		}
		for _, media := range mediaInputs {
			if media.BindingID == "" {
				return promptdomain.ErrReferenceUnavailable
			}
			if err := sources.ValidateBinding(ctx, actor, project, media, false); err != nil {
				return fmt.Errorf("%w: %w", promptdomain.ErrReferenceUnavailable, err)
			}
		}
	}
	return nil
}
