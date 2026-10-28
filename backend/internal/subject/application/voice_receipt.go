package application

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"path"
	"strings"
	"unicode"
	"unicode/utf8"
)

// VoiceAudioKey is the stable original in the creator's real personal library.
func VoiceAudioKey(work VoiceWork) string {
	return path.Join("personal", work.Owner.PersonalOrgID.String(), work.Job.CurrentActorID.String(), "audio", work.Job.CreatedAt.UTC().Format("2006"), work.Job.CreatedAt.UTC().Format("01"), work.AssetID.String()+".mp3")
}

// Digest identifies the exact immutable manifest given to the media owner.
func (r VoiceReceipt) Digest() string {
	if len(r.Usage) > 0 {
		var usage any
		decoder := json.NewDecoder(bytes.NewReader(r.Usage))
		decoder.UseNumber()
		if decoder.Decode(&usage) != nil {
			return ""
		}
		raw, err := json.Marshal(usage)
		if err != nil {
			return ""
		}
		r.Usage = raw
	}
	data, _ := json.Marshal(r)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// Validate prevents a recovery manifest from rebinding any frozen ownership,
// submission or physical original. A complete provider terminal is mandatory.
func (r VoiceReceipt) Validate(work VoiceWork) error {
	digest, err := hex.DecodeString(r.AudioSHA256)
	if r.SchemaVersion != 1 || r.JobID != work.Job.ID || r.ActorID != work.Job.CurrentActorID || r.OwnerScopeID != work.Owner.ScopeID || r.PersonalOrgID != work.Owner.PersonalOrgID || r.AssetID != work.AssetID || r.ReleaseID != work.Job.ReleaseID || r.ModelVersionID != work.Job.ModelVersionID || r.ProviderRequestID != work.Job.ID || r.RequestSHA256 != work.RequestSHA256 || r.AudioKey != VoiceAudioKey(work) || r.TerminalCode != 20000000 || r.MIMEType != "audio/mpeg" || r.AudioByteSize < 1 || r.AudioByteSize > 8<<20 || err != nil || len(digest) != sha256.Size || r.AudioSHA256 != strings.ToLower(r.AudioSHA256) || len(r.ProviderLogID) > 256 || strings.ContainsAny(r.ProviderLogID, "\x00\r\n") {
		return ErrVoiceExecutionUnknown
	}
	if !utf8.ValidString(r.ProviderLogID) {
		return ErrVoiceExecutionUnknown
	}
	for _, c := range r.ProviderLogID {
		if unicode.IsControl(c) {
			return ErrVoiceExecutionUnknown
		}
	}
	if len(r.Usage) > 0 && (r.Usage[0] != '{' || !json.Valid(r.Usage)) {
		return ErrVoiceExecutionUnknown
	}
	if len(r.Usage) > 16384 || len(r.Usage) > 0 && !json.Valid(r.Usage) {
		return ErrVoiceExecutionUnknown
	}
	return nil
}
