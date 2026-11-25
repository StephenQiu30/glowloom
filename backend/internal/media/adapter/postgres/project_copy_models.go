package postgres

import "github.com/google/uuid"

type projectCopySnapshotRecord struct {
	ID, JobID, OrgID, SourceProjectID, TargetProjectID uuid.UUID
	ManifestSHA256                                     string
	AssetCount, RenditionCount                         int
	Content, AssetMapping                              jsonDocument
}

func (projectCopySnapshotRecord) TableName() string { return "media.project_copy_snapshot" }

type projectCopyReceiptRecord struct {
	SnapshotID                 uuid.UUID
	ContentSHA256              string
	AssetCount, RenditionCount int
}

func (projectCopyReceiptRecord) TableName() string { return "media.project_copy_receipt" }
