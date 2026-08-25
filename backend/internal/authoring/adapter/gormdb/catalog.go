package gormdb

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/StephenQiu30/lanverse/backend/internal/authoring/domain"
	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
)

type Store struct{ database *gorm.DB }

func New(database *gorm.DB) *Store { return &Store{database: database} }

type definitionReference struct {
	ID          string `json:"id"`
	Key         string `json:"key"`
	Version     string `json:"version"`
	ContentHash string `json:"content_hash"`
}

func (store *Store) EnsureCatalog(
	ctx context.Context,
	catalog domain.Catalog,
	now time.Time,
	newID func() string,
) (string, error) {
	if newID == nil || len(catalog.ContentHash) != 64 || len(catalog.ExecutionHash) != 64 {
		return "", errors.New("invalid node catalog persistence request")
	}
	now = now.UTC()
	var catalogID uuid.UUID
	err := platformdatabase.WithinTransaction(ctx, store.database, func(transaction *gorm.DB) error {
		references := make([]definitionReference, 0, len(catalog.Definitions))
		for _, definition := range catalog.Definitions {
			record, err := ensureDefinition(transaction, definition, now, newID)
			if err != nil {
				return err
			}
			references = append(references, definitionReference{
				ID: record.ID.String(), Key: record.Key, Version: record.Version, ContentHash: record.ContentHash,
			})
		}
		encodedReferences, err := json.Marshal(references)
		if err != nil {
			return err
		}
		requestedID, err := parseNewID(newID)
		if err != nil {
			return err
		}
		desired := model.NodeCatalogVersion{
			ID: requestedID, Key: catalog.Key, Version: catalog.Version, Definitions: datatypes.JSON(encodedReferences),
			ContentHash: catalog.ContentHash, ExecutionHash: catalog.ExecutionHash, Status: "published", CreatedAt: now,
		}
		if err = transaction.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "key"}, {Name: "version"}},
			DoNothing: true,
		}).Create(&desired).Error; err != nil {
			return err
		}
		var persisted model.NodeCatalogVersion
		if err = transaction.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("key = ? AND version = ?", catalog.Key, catalog.Version).First(&persisted).Error; err != nil {
			return err
		}
		if persisted.ContentHash != catalog.ContentHash || persisted.ExecutionHash != catalog.ExecutionHash ||
			persisted.Status != "published" || !equalJSON(persisted.Definitions, encodedReferences) {
			return fmt.Errorf("node catalog %s@%s already exists with different content", catalog.Key, catalog.Version)
		}
		catalogID = persisted.ID
		return nil
	})
	if err != nil {
		return "", err
	}
	return catalogID.String(), nil
}

func ensureDefinition(
	transaction *gorm.DB,
	definition domain.NodeDefinition,
	now time.Time,
	newID func() string,
) (model.NodeDefinitionVersion, error) {
	inputPorts, err := json.Marshal(definition.InputPorts)
	if err != nil {
		return model.NodeDefinitionVersion{}, err
	}
	outputPorts, err := json.Marshal(definition.OutputPorts)
	if err != nil {
		return model.NodeDefinitionVersion{}, err
	}
	requestedID, err := parseNewID(newID)
	if err != nil {
		return model.NodeDefinitionVersion{}, err
	}
	desired := model.NodeDefinitionVersion{
		ID: requestedID, Key: definition.Key, Version: definition.Version, Name: definition.Name,
		Category: definition.Category, Executor: definition.Executor,
		InputPorts: datatypes.JSON(inputPorts), OutputPorts: datatypes.JSON(outputPorts), ConfigSchema: datatypes.JSON(definition.ConfigSchema),
		CachePolicy: definition.CachePolicy, RiskLevel: definition.RiskLevel, Executable: definition.Executable,
		ContentHash: definition.ContentHash, CreatedAt: now,
	}
	if err = transaction.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}, {Name: "version"}},
		DoNothing: true,
	}).Create(&desired).Error; err != nil {
		return model.NodeDefinitionVersion{}, err
	}
	var persisted model.NodeDefinitionVersion
	if err = transaction.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("key = ? AND version = ?", definition.Key, definition.Version).First(&persisted).Error; err != nil {
		return model.NodeDefinitionVersion{}, err
	}
	if persisted.ContentHash != definition.ContentHash || persisted.Name != definition.Name ||
		persisted.Category != definition.Category || persisted.Executor != definition.Executor ||
		persisted.CachePolicy != definition.CachePolicy || persisted.RiskLevel != definition.RiskLevel || persisted.Executable != definition.Executable ||
		!equalJSON(persisted.InputPorts, inputPorts) || !equalJSON(persisted.OutputPorts, outputPorts) || !equalJSON(persisted.ConfigSchema, definition.ConfigSchema) {
		return model.NodeDefinitionVersion{}, fmt.Errorf("node definition %s@%s already exists with different content", definition.Key, definition.Version)
	}
	return persisted, nil
}

func parseNewID(newID func() string) (uuid.UUID, error) {
	value, err := uuid.Parse(newID())
	if err != nil {
		return uuid.Nil, errors.New("node catalog id generator returned an invalid UUID")
	}
	return value, nil
}

func equalJSON(left, right []byte) bool {
	var leftValue, rightValue any
	if json.Unmarshal(left, &leftValue) != nil || json.Unmarshal(right, &rightValue) != nil {
		return false
	}
	leftCanonical, leftErr := json.Marshal(leftValue)
	rightCanonical, rightErr := json.Marshal(rightValue)
	return leftErr == nil && rightErr == nil && bytes.Equal(leftCanonical, rightCanonical)
}
