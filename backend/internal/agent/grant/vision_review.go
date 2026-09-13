package grant

import (
	"errors"

	"github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
)

func (signer *Signer) IssueVisionReviewDispatchAuthorization(
	invocation contract.VisionReviewInvocation,
	claimVersion int64,
) (contract.SceneAnalysisDispatchAuthorization, error) {
	if err := invocation.Validate(); err != nil {
		return contract.SceneAnalysisDispatchAuthorization{}, err
	}
	claims := contract.SceneAnalysisDispatchAuthorizationClaims{
		InvocationID: invocation.InvocationID, AttemptID: invocation.AttemptID,
		InputHash: invocation.InputHash, SkillReleaseID: invocation.StageRelease.SkillReleaseID,
		SkillReleaseHash:  invocation.StageRelease.SkillReleaseHash,
		StageReleaseHash:  invocation.StageRelease.StageReleaseHash,
		BundleContentHash: invocation.StageRelease.BundleContentHash,
		ControlHash:       invocation.Control.ControlHash, ReleaseFence: invocation.Control.ReleaseFence,
		ClaimVersion:     claimVersion,
		AgentImageDigest: invocation.StageRelease.AgentImageDigest,
		ExpiresAt:        signer.now().UTC().Add(TTL).Unix(),
	}
	if err := claims.ValidateForVisionReview(invocation, claimVersion, signer.now().UTC().Unix()); err != nil {
		return contract.SceneAnalysisDispatchAuthorization{}, err
	}
	return signer.encodeSceneAnalysisDispatchAuthorization(claims)
}

func (signer *Signer) VerifyVisionReviewDispatchAuthorization(
	value string,
	invocation contract.VisionReviewInvocation,
	claimVersion int64,
) error {
	if err := invocation.Validate(); err != nil {
		return err
	}
	claims, err := signer.decodeSceneAnalysisDispatchAuthorization(value)
	if err != nil {
		return err
	}
	if claims.ExpiresAt > signer.now().UTC().Add(TTL).Unix() {
		return errors.New("Vision Review dispatch authorization expiry exceeds maximum TTL")
	}
	if err = claims.ValidateForVisionReview(invocation, claimVersion, signer.now().UTC().Unix()); err != nil {
		return errors.New("dispatch authorization does not authorize Vision Review invocation")
	}
	return nil
}
