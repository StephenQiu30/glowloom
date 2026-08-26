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

type Claims struct {
	InvocationID        string `json:"invocation_id"`
	Kind                string `json:"kind"`
	InputHash           string `json:"input_hash"`
	ExecutionPolicyHash string `json:"execution_policy_hash"`
	ExpiresAt           int64  `json:"expires_at"`
}

type Signer struct {
	secret []byte
	now    func() time.Time
}

func NewSigner(secret string, now func() time.Time) (*Signer, error) {
	if len(secret) < 32 {
		return nil, errors.New("agent execution secret must contain at least 32 bytes")
	}
	return &Signer{secret: []byte(secret), now: now}, nil
}

func (signer *Signer) Issue(invocation contract.Invocation) (string, error) {
	if err := invocation.Validate(); err != nil {
		return "", err
	}
	policyHash, err := invocation.ExecutionPolicy.Hash()
	if err != nil {
		return "", err
	}
	claims := Claims{InvocationID: invocation.InvocationID, Kind: invocation.Kind, InputHash: invocation.InputHash, ExecutionPolicyHash: policyHash, ExpiresAt: signer.now().UTC().Add(TTL).Unix()}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	return encoded + "." + signer.signature(encoded), nil
}

func (signer *Signer) Verify(value string, invocation contract.Invocation) error {
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
	var claims Claims
	policyHash, hashErr := invocation.ExecutionPolicy.Hash()
	if err = json.Unmarshal(payload, &claims); err != nil || hashErr != nil || claims.InvocationID != invocation.InvocationID || claims.Kind != invocation.Kind || claims.InputHash != invocation.InputHash || claims.ExecutionPolicyHash != policyHash || claims.ExpiresAt <= signer.now().UTC().Unix() {
		return errors.New("agent execution grant does not authorize invocation")
	}
	return nil
}

func (signer *Signer) signature(payload string) string {
	mac := hmac.New(sha256.New, signer.secret)
	_, _ = mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
