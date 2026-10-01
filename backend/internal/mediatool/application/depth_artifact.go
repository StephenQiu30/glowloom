package application

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"path"
	"strconv"

	"github.com/google/uuid"

	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	mediadomain "github.com/StephenQiu30/lanverse/backend/internal/media/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/mediatool/domain"
)

// DepthOutputIdentity derives a private immutable key from trusted job facts.
func DepthOutputIdentity(job domain.DepthJob) (uuid.UUID, string) {
	id := uuid.NewSHA1(job.ID, []byte("depth-output/"+strconv.Itoa(job.Attempt)))
	return id, path.Join("projects", job.ProjectID.String(), "video", job.CreatedAt.UTC().Format("2006/01"), id.String()+".mp4")
}

// DepthArtifactDigest validates the complete permanent result binding and hash.
// SQL timestamp representations do not participate; JSON times are canonical UTC.
func DepthArtifactDigest(job domain.DepthJob, f domain.FrozenDepth, a DepthArtifact) (string, []byte, error) {
	id, key := DepthOutputIdentity(job)
	a.Renditions = append([]mediadomain.Rendition(nil), a.Renditions...)
	asset := a.Asset
	if a.JobID != job.ID || a.Attempt != job.Attempt || a.InputSHA256 != f.Input.SHA256 || asset.ID != id || asset.ID == f.Input.AssetID || asset.ProjectID != job.ProjectID || asset.ObjectKey != key || asset.ObjectKey == f.Input.ObjectKey || asset.Validate() != nil || asset.Kind != mediadomain.KindVideo || asset.Origin != mediadomain.OriginSystem || asset.Status != mediadomain.StatusProcessing || asset.ModerationStatus != mediadomain.ModerationPending || asset.ContainsRealPerson || asset.ConsentRecordID != nil || asset.SourceOperationID != nil || asset.ProviderKey != nil || asset.ModelKey != nil || asset.Region != nil || asset.IsDelete || asset.Revision != 1 || len(a.Objects) != 3 || len(a.Renditions) != 2 {
		return "", nil, ErrDepthOutputInvalid
	}
	if asset.SHA256 == nil {
		return "", nil, ErrDepthOutputInvalid
	}
	p := mediaapp.ProbeResult{Kind: asset.Kind, Extension: "mp4", Width: asset.Width, Height: asset.Height, DurationMS: asset.DurationMS, FPS: asset.FPS, AudioChannels: asset.AudioChannels, Codec: asset.Codec}
	if ValidateDepthReceipt(f, &DepthOutput{File: &mediaapp.Downloaded{Size: asset.ByteSize, MIMEType: asset.MimeType, SHA256: *asset.SHA256}, Probe: p, Receipt: a.Receipt}) != nil {
		return "", nil, ErrDepthOutputInvalid
	}
	for i, kind := range []string{"original", "poster", "proxy_720p"} {
		o := a.Objects[i]
		if o.Kind != kind || !domain.ValidDepthSHA(o.SHA256) || o.ByteSize < 1 || o.ByteSize > 500<<20 {
			return "", nil, ErrDepthOutputInvalid
		}
		if i == 0 {
			if o.ObjectKey != key || o.SHA256 != *asset.SHA256 || o.ByteSize != asset.ByteSize || o.MIMEType != "video/mp4" {
				return "", nil, ErrDepthOutputInvalid
			}
			continue
		}
		r := a.Renditions[i-1]
		ext, mime := "png", "image/png"
		if kind == "proxy_720p" {
			ext, mime = "mp4", "video/mp4"
		}
		want := path.Join(key[:len(key)-len(path.Ext(key))], kind+"."+ext)
		if r.Validate() != nil || r.ID != uuid.NewSHA1(id, []byte(kind)) || r.MediaAssetID != id || string(r.Kind) != kind || r.ObjectKey != want || o.ObjectKey != want || o.MIMEType != mime || r.ByteSize == nil || *r.ByteSize != o.ByteSize || r.Width == nil || r.Height == nil || *r.Width < 1 || *r.Height < 1 || r.IsDelete {
			return "", nil, ErrDepthOutputInvalid
		}
		a.Renditions[i-1].CreateTime = r.CreateTime.UTC()
		a.Renditions[i-1].UpdateTime = r.UpdateTime.UTC()
	}
	a.Asset.CreateTime = a.Asset.CreateTime.UTC()
	a.Asset.UpdateTime = a.Asset.UpdateTime.UTC()
	body, err := json.Marshal(a)
	if err != nil || len(body) > 1<<20 {
		return "", nil, ErrDepthOutputInvalid
	}
	h := sha256.Sum256(body)
	return hex.EncodeToString(h[:]), body, nil
}
