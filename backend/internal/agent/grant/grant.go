package grant

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
)

const (
	TTL                                      = 5 * time.Minute
	sceneAnalysisDispatchAuthorizationDomain = "lanverse.scene-analysis.dispatch-authorization.production"
)

type Signer struct {
	secret []byte
	now    func() time.Time
}

func NewSigner(secret string, now func() time.Time) (*Signer, error) {
	if len([]byte(secret)) < 32 {
		return nil, errors.New("agent execution secret must contain at least 32 bytes")
	}
	return &Signer{secret: []byte(secret), now: now}, nil
}

func (signer *Signer) Issue(invocation contract.StageInvocation, attempt int, fencingToken int64) (string, error) {
	if err := invocation.Validate(); err != nil {
		return "", err
	}
	policyHash, err := invocation.ExecutionPolicy.Hash()
	if err != nil {
		return "", err
	}
	claims := contract.StageExecutionGrantClaims{
		InvocationID: invocation.InvocationID, InputHash: invocation.InputHash,
		ExecutionPolicyHash: policyHash, ExpiresAt: signer.now().UTC().Add(TTL).Unix(),
		Attempt: attempt, FencingToken: fencingToken,
	}
	if err = claims.ValidateFor(invocation, signer.now().UTC().Unix()); err != nil {
		return "", err
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	return encoded + "." + signer.signature(encoded), nil
}

func (signer *Signer) Verify(value string, invocation contract.StageInvocation, attempt int, fencingToken int64) error {
	if err := invocation.Validate(); err != nil {
		return err
	}
	parts := strings.Split(value, ".")
	if len(parts) != 2 || !hmac.Equal([]byte(parts[1]), []byte(signer.signature(parts[0]))) {
		return errors.New("invalid agent execution grant signature")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return errors.New("invalid agent execution grant payload")
	}
	var claims contract.StageExecutionGrantClaims
	if err = json.Unmarshal(payload, &claims); err != nil {
		return errors.New("invalid agent execution grant payload")
	}
	if err = claims.ValidateFor(invocation, signer.now().UTC().Unix()); err != nil || claims.Attempt != attempt || claims.FencingToken != fencingToken {
		return errors.New("agent execution grant does not authorize invocation")
	}
	return nil
}

func (signer *Signer) IssueSceneAnalysisDispatchAuthorization(
	invocation contract.SceneAnalysisInvocation,
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
		ClaimVersion:     int64(claimVersion),
		AgentImageDigest: invocation.StageRelease.AgentImageDigest,
		ExpiresAt:        signer.now().UTC().Add(TTL).Unix(),
	}
	if err := claims.ValidateFor(invocation, claimVersion, signer.now().UTC().Unix()); err != nil {
		return contract.SceneAnalysisDispatchAuthorization{}, err
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return contract.SceneAnalysisDispatchAuthorization{}, err
	}
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	value := encoded + "." + signer.sceneAnalysisDispatchSignature(payload)
	digest := sha256.Sum256([]byte(value))
	authorization := contract.SceneAnalysisDispatchAuthorization{
		Value: value, Hash: hex.EncodeToString(digest[:]), ClaimVersion: claimVersion,
		ExpiresAt: time.Unix(claims.ExpiresAt, 0).UTC(),
	}
	return authorization, authorization.Validate()
}

func (signer *Signer) VerifySceneAnalysisDispatchAuthorization(
	value string,
	invocation contract.SceneAnalysisInvocation,
	claimVersion int64,
) error {
	if err := invocation.Validate(); err != nil {
		return err
	}
	parts := strings.Split(value, ".")
	if len(parts) != 2 {
		return errors.New("invalid Scene Analysis dispatch authorization signature")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return errors.New("invalid Scene Analysis dispatch authorization payload")
	}
	if !hmac.Equal([]byte(parts[1]), []byte(signer.sceneAnalysisDispatchSignature(payload))) {
		return errors.New("invalid Scene Analysis dispatch authorization signature")
	}
	var claims contract.SceneAnalysisDispatchAuthorizationClaims
	if err = json.Unmarshal(payload, &claims); err != nil {
		return errors.New("invalid Scene Analysis dispatch authorization payload")
	}
	if err = claims.ValidateFor(invocation, claimVersion, signer.now().UTC().Unix()); err != nil {
		return errors.New("dispatch authorization does not authorize Scene Analysis invocation")
	}
	return nil
}

func (signer *Signer) sceneAnalysisDispatchSignature(payload []byte) string {
	root := sha256.New()
	_, _ = root.Write([]byte(sceneAnalysisDispatchAuthorizationDomain))
	_, _ = root.Write([]byte{0})
	_, _ = root.Write(payload)
	mac := hmac.New(sha256.New, signer.secret)
	_, _ = mac.Write(root.Sum(nil))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (signer *Signer) signature(payload string) string {
	mac := hmac.New(sha256.New, signer.secret)
	_, _ = mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
