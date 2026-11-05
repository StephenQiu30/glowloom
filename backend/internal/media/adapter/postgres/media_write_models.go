package postgres

import (
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

type mediaAssetRecord struct {
	ID                 uuid.UUID
	ProjectID          *uuid.UUID
	PersonalOrgID      *uuid.UUID
	PersonalActorID    *uuid.UUID
	Kind               string
	Origin             string
	Status             string
	ObjectKey          string
	ObjectLocationID   *uuid.UUID
	FileName           string
	MimeType           string
	ByteSize           int64
	SHA256             *string
	Width              *int32
	Height             *int32
	DurationMS         *int32
	FPS                *float64
	AudioChannels      *int32
	Codec              *string
	SourceOperationID  *uuid.UUID
	ProviderKey        *string
	ModelKey           *string
	Region             *string
	ModerationStatus   string
	ModerationDetail   jsonDocument
	AIGCMarked         bool
	ContainsRealPerson bool
	ConsentRecordID    *uuid.UUID
	UploadID           *string
	FailureReason      *string
	DeleteTime         *time.Time
	PurgeAfter         *time.Time
	Revision           int64
	CreateTime         time.Time
	UpdateTime         time.Time
}

func (mediaAssetRecord) TableName() string { return "media.media_asset" }

type mediaRenditionRecord struct {
	ID               uuid.UUID
	MediaAssetID     uuid.UUID
	Kind             string
	ObjectKey        string
	ObjectLocationID *uuid.UUID
	Width            *int32
	Height           *int32
	ByteSize         *int64
	CreateTime       time.Time
	UpdateTime       time.Time
}

func (mediaRenditionRecord) TableName() string { return "media.rendition" }

func mediaAssetRecordFromDomain(asset domain.MediaAsset) mediaAssetRecord {
	record := mediaAssetRecord{
		ID: asset.ID, Kind: string(asset.Kind), Origin: string(asset.Origin), Status: string(asset.Status),
		ObjectKey: asset.ObjectKey, ObjectLocationID: locationID(asset.ObjectLocation), FileName: asset.FileName,
		MimeType: asset.MimeType, ByteSize: asset.ByteSize, SHA256: asset.SHA256, Width: asset.Width,
		Height: asset.Height, DurationMS: asset.DurationMS, FPS: asset.FPS, AudioChannels: asset.AudioChannels,
		Codec: asset.Codec, SourceOperationID: asset.SourceOperationID, ProviderKey: asset.ProviderKey,
		ModelKey: asset.ModelKey, Region: asset.Region, ModerationStatus: string(asset.ModerationStatus),
		ModerationDetail: jsonDocument(asset.ModerationDetail), AIGCMarked: asset.AIGCMarked,
		ContainsRealPerson: asset.ContainsRealPerson, ConsentRecordID: asset.ConsentRecordID,
		UploadID: asset.UploadID, FailureReason: asset.FailureReason, DeleteTime: asset.DeleteTime,
		PurgeAfter: asset.PurgeAfter, Revision: asset.Revision,
		CreateTime: asset.CreateTime, UpdateTime: asset.UpdateTime,
	}
	if asset.Personal == nil {
		record.ProjectID = &asset.ProjectID
	} else {
		record.PersonalOrgID = &asset.Personal.OrgID
		record.PersonalActorID = &asset.Personal.ActorID
	}
	return record
}

func mediaRenditionRecordFromDomain(rendition domain.Rendition) mediaRenditionRecord {
	return mediaRenditionRecord{
		ID: rendition.ID, MediaAssetID: rendition.MediaAssetID, Kind: string(rendition.Kind),
		ObjectKey: rendition.ObjectKey, ObjectLocationID: locationID(rendition.ObjectLocation),
		Width: rendition.Width, Height: rendition.Height, ByteSize: rendition.ByteSize,
		CreateTime: rendition.CreateTime, UpdateTime: rendition.UpdateTime,
	}
}
