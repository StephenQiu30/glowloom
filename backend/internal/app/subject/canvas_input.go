package subject

import (
	"context"
	"strconv"

	"github.com/google/uuid"

	scopewire "github.com/StephenQiu30/glowloom-server/backend/internal/app/scope"
	canvaspg "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/adapter/postgres"
	canvas "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	canvasdomain "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	promptdomain "github.com/StephenQiu30/glowloom-server/backend/internal/prompt/domain"
)

// ResolveCanvasSubject expands only the exact saved local input in the authorized project.
func (g generationAccess) ResolveCanvasSubject(ctx context.Context, actor identity.Principal, project uuid.UUID, id, version string) (promptdomain.ReferenceSource, error) {
	out := promptdomain.ReferenceSource{Kind: "canvas_subject", SourceID: id, SourceVersion: version, Media: []promptdomain.SourceMedia{}}
	nodeID, e1 := uuid.Parse(id)
	revision, e2 := strconv.ParseInt(version, 10, 64)
	if e1 != nil || e2 != nil || revision < 1 || strconv.FormatInt(revision, 10) != version {
		return out, promptdomain.ErrReferenceUnavailable
	}
	if _, err := scopewire.ProvideProjectContentAccess(g.tx).Authorize(ctx, actor, project, false); err != nil {
		return out, err
	}
	node, err := canvaspg.ReadSubjectInput(ctx, g.tx, project, nodeID, revision)
	if err != nil {
		return out, err
	}
	if err := canvas.ValidateSubjectNodes(ctx, actor, project, []canvasdomain.Node{node}, g); err != nil {
		return out, err
	}
	local := node.Config.Subject.Local
	out.Description = node.Title
	if local.Description != "" {
		out.Description += "：" + local.Description
	}
	for _, media := range local.Media() {
		source := promptdomain.SourceMedia{SourceID: media.AssetID.String(), SourceVersion: strconv.FormatInt(media.AssetRevision, 10), MediaKind: media.Kind}
		if media.BindingID != nil {
			source.BindingID = media.BindingID.String()
		}
		out.Media = append(out.Media, source)
	}
	return out, nil
}
