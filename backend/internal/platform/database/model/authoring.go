package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

type NodeDefinitionVersion struct {
	ID           uuid.UUID      `gorm:"type:uuid;primaryKey"`
	Key          string         `gorm:"type:varchar(100);not null;uniqueIndex:uq_aut_node_definition_key_version,priority:1"`
	Version      string         `gorm:"type:varchar(40);not null;uniqueIndex:uq_aut_node_definition_key_version,priority:2"`
	Name         string         `gorm:"type:varchar(120);not null"`
	Category     string         `gorm:"type:varchar(40);not null"`
	Executor     string         `gorm:"type:varchar(120);not null"`
	InputPorts   datatypes.JSON `gorm:"type:jsonb;not null"`
	OutputPorts  datatypes.JSON `gorm:"type:jsonb;not null"`
	ConfigSchema datatypes.JSON `gorm:"type:jsonb;not null"`
	CachePolicy  string         `gorm:"type:varchar(20);not null;check:ck_aut_node_cache_policy,cache_policy IN ('never','by_inputs')"`
	RiskLevel    string         `gorm:"type:varchar(20);not null;check:ck_aut_node_risk_level,risk_level IN ('low','external_ai','human_gate')"`
	Executable   bool           `gorm:"not null"`
	ContentHash  string         `gorm:"type:char(64);not null;check:ck_aut_node_content_hash,char_length(content_hash) = 64"`
	CreatedAt    time.Time      `gorm:"type:timestamptz;not null"`
}

func (NodeDefinitionVersion) TableName() string { return "aut_node_definition_versions" }

type NodeCatalogVersion struct {
	ID            uuid.UUID      `gorm:"type:uuid;primaryKey"`
	Key           string         `gorm:"type:varchar(100);not null;uniqueIndex:uq_aut_node_catalog_key_version,priority:1"`
	Version       string         `gorm:"type:varchar(40);not null;uniqueIndex:uq_aut_node_catalog_key_version,priority:2"`
	Definitions   datatypes.JSON `gorm:"type:jsonb;not null"`
	ContentHash   string         `gorm:"type:char(64);not null;check:ck_aut_catalog_content_hash,char_length(content_hash) = 64"`
	ExecutionHash string         `gorm:"type:char(64);not null;check:ck_aut_catalog_execution_hash,char_length(execution_hash) = 64"`
	Status        string         `gorm:"type:varchar(20);not null;check:ck_aut_catalog_status,status = 'published'"`
	CreatedAt     time.Time      `gorm:"type:timestamptz;not null"`
}

func (NodeCatalogVersion) TableName() string { return "aut_node_catalog_versions" }
