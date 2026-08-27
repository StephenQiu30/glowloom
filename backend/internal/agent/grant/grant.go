package grant

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
)

const TTL = 5 * time.Minute

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

func (signer *Signer) signature(payload string) string {
	mac := hmac.New(sha256.New, signer.secret)
	_, _ = mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
