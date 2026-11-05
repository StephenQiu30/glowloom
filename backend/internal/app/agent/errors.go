package agent

import (
	"errors"

	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain"
	bible "github.com/StephenQiu30/glowloom-server/backend/internal/bible/application"
	bibledomain "github.com/StephenQiu30/glowloom-server/backend/internal/bible/domain"
	canvas "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	canvasdomain "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
	media "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	operation "github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	operationdomain "github.com/StephenQiu30/glowloom-server/backend/internal/operation/domain"
	script "github.com/StephenQiu30/glowloom-server/backend/internal/script/application"
	scriptdomain "github.com/StephenQiu30/glowloom-server/backend/internal/script/domain"
	subject "github.com/StephenQiu30/glowloom-server/backend/internal/subject/application"
)

func businessError(err error) error {
	if err == nil {
		return nil
	}
	var revision *canvas.RevisionConflict
	if errors.As(err, &revision) || errors.Is(err, canvas.ErrYjsContention) || errors.Is(err, canvas.ErrIdempotencyConflict) || errors.Is(err, bible.ErrConflict) || errors.Is(err, bible.ErrIdempotencyConflict) || errors.Is(err, script.ErrConflict) || errors.Is(err, script.ErrIdempotencyConflict) || errors.Is(err, operation.ErrQuoteStale) || errors.Is(err, operation.ErrQuoteSourceStale) || errors.Is(err, operation.ErrQuoteKeyReused) || errors.Is(err, operation.ErrConfirmationKeyReused) || errors.Is(err, operationdomain.ErrQuoteNotConfirmable) {
		return errors.Join(domain.ErrConflict, err)
	}
	if errors.Is(err, canvas.ErrNotFound) || errors.Is(err, bible.ErrNotFound) || errors.Is(err, script.ErrNotFound) || errors.Is(err, subject.ErrNotFound) || errors.Is(err, media.ErrNotFound) || errors.Is(err, operation.ErrPublicNotFound) {
		return errors.Join(domain.ErrNotFound, err)
	}
	if errors.Is(err, canvasdomain.ErrInvalidCommand) || errors.Is(err, bibledomain.ErrInvalidContent) || errors.Is(err, scriptdomain.ErrInvalidSource) || errors.Is(err, scriptdomain.ErrInvalidStructure) || errors.Is(err, scriptdomain.ErrInvalidSpan) || errors.Is(err, operation.ErrInvalidFreeQuote) || errors.Is(err, operation.ErrInvalidConfirmSingleQuote) {
		return errors.Join(domain.ErrInvalid, err)
	}
	return err
}
