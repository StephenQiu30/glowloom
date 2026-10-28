package application

import (
	"encoding/hex"
	"encoding/json"
	"slices"
	"strings"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/domain"
)

type freeQuoteRoleLimit struct {
	MinCount      int64    `json:"min_count"`
	ForModes      []string `json:"for_modes"`
	MaxCount      int64    `json:"max_count"`
	Types         []string `json:"types"`
	MaxMS         int64    `json:"max_ms"`
	MaxBytes      int64    `json:"max_bytes"`
	MinResolution string   `json:"min_resolution"`
}

func prepareFreeQuoteMedia(requested []FreeQuoteMediaInput, media []FreeQuoteMediaFact,
	inputRoles []string, rawLimits json.RawMessage, operationID uuid.UUID, mode string,
) ([]FingerprintPart, []domain.OperationInput, error) {
	if len(requested) != len(media) {
		return nil, nil, ErrFreeQuoteInputNotReady
	}
	var limits struct {
		Roles map[string]json.RawMessage `json:"roles"`
	}
	if err := json.Unmarshal(rawLimits, &limits); err != nil || (len(requested) > 0 && len(limits.Roles) == 0) {
		return nil, nil, ErrFreeQuoteInputNotReady
	}
	counts := make(map[string]int64)
	totals := make(map[string]int64)
	parts := make([]FingerprintPart, 0, len(requested))
	inputs := make([]domain.OperationInput, 0, len(requested))
	for index, request := range requested {
		fact := media[index]
		if fact.ID != request.MediaAssetID || !slices.Contains(inputRoles, request.Role) ||
			(fact.Kind != "image" && fact.Kind != "video" && fact.Kind != "audio") ||
			fact.ByteSize < 0 || len(fact.SHA256) != 64 {
			return nil, nil, ErrFreeQuoteInputNotReady
		}
		if _, err := hex.DecodeString(fact.SHA256); err != nil {
			return nil, nil, ErrFreeQuoteInputNotReady
		}
		raw, ok := limits.Roles[request.Role]
		if !ok {
			return nil, nil, ErrFreeQuoteInputNotReady
		}
		var limit freeQuoteRoleLimit
		if err := json.Unmarshal(raw, &limit); err != nil || limit.MaxCount < 0 ||
			!slices.Contains(limit.Types, fact.Kind) ||
			(len(limit.ForModes) > 0 && !slices.Contains(limit.ForModes, mode)) ||
			(limit.MaxBytes > 0 && fact.ByteSize > limit.MaxBytes) ||
			(fact.DurationMS != nil && *fact.DurationMS <= 0) ||
			(limit.MaxMS > 0 && (fact.DurationMS == nil || *fact.DurationMS > limit.MaxMS)) ||
			limit.MinResolution != "" {
			// The provider-specific min_resolution format has no shared adapter yet.
			return nil, nil, ErrFreeQuoteInputNotReady
		}
		counts[request.Role]++
		if counts[request.Role] > limit.MaxCount {
			return nil, nil, ErrFreeQuoteInputNotReady
		}
		totals[fact.Kind]++
		seqNo := int32(index + 1)
		assetID := fact.ID
		contentHash := strings.ToLower(fact.SHA256)
		refType, refID, refVersion := "media_asset", assetID, contentHash
		if request.SubjectBindingID != nil {
			if fact.SubjectBindingID == nil || *fact.SubjectBindingID != *request.SubjectBindingID || len(fact.SubjectBindingSHA256) != 64 {
				return nil, nil, ErrFreeQuoteInputNotReady
			}
			if _, err := hex.DecodeString(fact.SubjectBindingSHA256); err != nil {
				return nil, nil, ErrFreeQuoteInputNotReady
			}
			refType, refID, refVersion = "subject_media", *request.SubjectBindingID, strings.ToLower(fact.SubjectBindingSHA256)
		} else if fact.SubjectBindingID != nil || fact.SubjectBindingSHA256 != "" {
			return nil, nil, ErrFreeQuoteInputNotReady
		}
		parts = append(parts, FingerprintPart{
			SeqNo: seqNo, Role: request.Role, RefType: refType,
			RefID: &refID, RefVersion: refVersion,
			MediaAssetID: &assetID, MediaSHA256: contentHash,
		})
		inputs = append(inputs, domain.OperationInput{
			ID: uuid.New(), OperationID: operationID, SeqNo: seqNo,
			Role: request.Role, RefType: refType, RefID: &refID,
			RefVersion: &refVersion, MediaAssetID: &assetID,
		})
	}
	for name, raw := range limits.Roles {
		if strings.HasPrefix(name, "_") {
			continue
		}
		var role freeQuoteRoleLimit
		if json.Unmarshal(raw, &role) != nil || role.MinCount < 0 || role.MinCount > role.MaxCount {
			return nil, nil, ErrFreeQuoteInputNotReady
		}
		if (len(role.ForModes) == 0 || slices.Contains(role.ForModes, mode)) && counts[name] < role.MinCount {
			return nil, nil, ErrFreeQuoteInputNotReady
		}
	}
	for kind, count := range totals {
		raw, ok := limits.Roles["_total_"+kind+"s"]
		if !ok {
			continue
		}
		var maximum int64
		if err := json.Unmarshal(raw, &maximum); err != nil || count > maximum {
			return nil, nil, ErrFreeQuoteInputNotReady
		}
	}
	return parts, inputs, nil
}
