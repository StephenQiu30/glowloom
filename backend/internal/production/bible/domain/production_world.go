package domain

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
)

const BibleProductionWorldFamily = "bible_production_world_set"

type ProductionWorldStateRef struct {
	ID, AssetID, StateKey, ContentHash string
	Revision                           int
}

type ProductionWorldFragmentRef struct {
	ID, Kind, BusinessKey, ContentHash string
	Revision                           int
}

type ProductionWorldEvidence struct {
	ID, WorkspaceID, ProjectID, SubjectKey string
	Revision                               int
	Basis                                  json.RawMessage
	ContentHash, CreatedBy                 string
	CreatedAt                              time.Time
}

type ProductionWorldSpecification struct {
	ID, WorkspaceID, ProjectID, SpecificationKey, IdentityKey, Kind string
	Revision                                                        int
	Asset, Evidence                                                 ProductionWorldFragmentRef
	Slots                                                           json.RawMessage
	ContentHash, CreatedBy                                          string
	CreatedAt                                                       time.Time
}

type ProductionWorldClaimSubject struct {
	IdentityKey, AssetID, AssetContentHash string
}

type ProductionWorldClaim struct {
	ID, WorkspaceID, ProjectID, ClaimKey, ClaimType, Statement string
	Revision                                                   int
	Subjects                                                   []ProductionWorldClaimSubject
	Evidence                                                   ProductionWorldFragmentRef
	ContentHash, CreatedBy                                     string
	CreatedAt                                                  time.Time
}

type ProductionWorldBinding struct {
	ID, WorkspaceID, ProjectID, IdentityKey string
	Revision                                int
	Asset, Specification                    ProductionWorldFragmentRef
	States                                  []ProductionWorldStateRef
	ContentHash, CreatedBy                  string
	CreatedAt                               time.Time
}

type ProductionWorldOwnerRef struct {
	OwnerKind, LogicalID, VersionID, ContentHash string
	Revision                                     int64
}

type ProductionWorldBibleVersion struct {
	ID, WorkspaceID, ProjectID                       string
	Revision                                         int64
	StructureIdentitySet, Candidate                  ProductionWorldOwnerRef
	ReviewDecisionID, PartitionHash, BusinessKeyRoot string
	Evidence, Specifications, Claims, Bindings       []ProductionWorldFragmentRef
	ContentHash, CreatedBy                           string
	CreatedAt                                        time.Time
}

type ProductionWorldBibleHead struct {
	WorkspaceID, ProjectID, CurrentVersionID, ScopeKey string
	ScopeRevision, HeadRevision                        int64
	MemberCount                                        int
	VersionContentHash, ScopeContentHash               string
	MembersHash, CollectionRootHash, HeadContentHash   string
	UpdatedAt                                          time.Time
}

func NewProductionWorldEvidence(id, workspaceID, projectID, subjectKey string, revision int, basis json.RawMessage, createdBy string, createdAt time.Time) (ProductionWorldEvidence, error) {
	if !validProductionWorldIDs(id, workspaceID, projectID, createdBy) || !keyPattern.MatchString(subjectKey) || revision < 1 || createdAt.IsZero() {
		return ProductionWorldEvidence{}, errors.New("invalid Production World Evidence input")
	}
	basis, err := canonicalProductionWorldJSON(basis, '{')
	if err != nil {
		return ProductionWorldEvidence{}, errors.New("invalid Production World Evidence basis")
	}
	value := ProductionWorldEvidence{ID: id, WorkspaceID: workspaceID, ProjectID: projectID, SubjectKey: subjectKey, Revision: revision, Basis: basis, CreatedBy: createdBy, CreatedAt: createdAt.UTC()}
	value.ContentHash, err = CanonicalStoryHash(struct {
		Schema, SubjectKey string
		Basis              json.RawMessage
	}{"production-world-evidence", subjectKey, basis})
	return value, err
}

func NewProductionWorldSpecification(id, workspaceID, projectID, specificationKey, identityKey, kind string, revision int, asset, evidence ProductionWorldFragmentRef, slots json.RawMessage, createdBy string, createdAt time.Time) (ProductionWorldSpecification, error) {
	if !validProductionWorldIDs(id, workspaceID, projectID, createdBy) || !keyPattern.MatchString(specificationKey) || !keyPattern.MatchString(identityKey) ||
		!strings.HasPrefix(specificationKey, "specification_") || !oneOf(kind, "character", "location", "prop") || revision < 1 ||
		asset.Kind != "asset_identity" || asset.BusinessKey != identityKey || evidence.Kind != "source_evidence" || evidence.BusinessKey != specificationKey ||
		!validProductionWorldFragmentRef(asset) || !validProductionWorldFragmentRef(evidence) || createdAt.IsZero() {
		return ProductionWorldSpecification{}, errors.New("invalid Production World Specification input")
	}
	slots, err := canonicalProductionWorldJSON(slots, '[')
	if err != nil {
		return ProductionWorldSpecification{}, errors.New("invalid Production World Specification slots")
	}
	value := ProductionWorldSpecification{ID: id, WorkspaceID: workspaceID, ProjectID: projectID, SpecificationKey: specificationKey, IdentityKey: identityKey, Kind: kind, Revision: revision, Asset: asset, Evidence: evidence, Slots: slots, CreatedBy: createdBy, CreatedAt: createdAt.UTC()}
	value.ContentHash, err = CanonicalStoryHash(struct {
		Schema, SpecificationKey, IdentityKey, Kind string
		Asset, Evidence                             ProductionWorldFragmentRef
		Slots                                       json.RawMessage
	}{"production-world-specification", specificationKey, identityKey, kind, asset, evidence, slots})
	return value, err
}

func NewProductionWorldClaim(id, workspaceID, projectID, claimKey, claimType, statement string, revision int, subjects []ProductionWorldClaimSubject, evidence ProductionWorldFragmentRef, createdBy string, createdAt time.Time) (ProductionWorldClaim, error) {
	if !validProductionWorldIDs(id, workspaceID, projectID, createdBy) || !keyPattern.MatchString(claimKey) || !strings.HasPrefix(claimKey, "claim_") ||
		!oneOf(claimType, "world_rule", "relationship", "story_arc", "plot_thread") || strings.TrimSpace(statement) == "" || revision < 1 ||
		evidence.Kind != "source_evidence" || evidence.BusinessKey != claimKey || !validProductionWorldFragmentRef(evidence) || len(subjects) == 0 || createdAt.IsZero() {
		return ProductionWorldClaim{}, errors.New("invalid Production World Claim input")
	}
	subjects = append([]ProductionWorldClaimSubject(nil), subjects...)
	slices.SortFunc(subjects, func(left, right ProductionWorldClaimSubject) int {
		return strings.Compare(left.IdentityKey, right.IdentityKey)
	})
	for index, subject := range subjects {
		if !keyPattern.MatchString(subject.IdentityKey) || !validProductionWorldUUID(subject.AssetID) || !hashPattern.MatchString(subject.AssetContentHash) ||
			(index > 0 && subjects[index-1].IdentityKey == subject.IdentityKey) {
			return ProductionWorldClaim{}, errors.New("invalid Production World Claim subject")
		}
	}
	value := ProductionWorldClaim{ID: id, WorkspaceID: workspaceID, ProjectID: projectID, ClaimKey: claimKey, ClaimType: claimType, Statement: strings.TrimSpace(statement), Revision: revision, Subjects: subjects, Evidence: evidence, CreatedBy: createdBy, CreatedAt: createdAt.UTC()}
	var err error
	value.ContentHash, err = CanonicalStoryHash(struct {
		Schema, ClaimKey, ClaimType, Statement string
		Subjects                               []ProductionWorldClaimSubject
		Evidence                               ProductionWorldFragmentRef
	}{"production-world-claim", claimKey, claimType, value.Statement, subjects, evidence})
	return value, err
}

func NewProductionWorldBinding(id, workspaceID, projectID, identityKey string, revision int, asset, specification ProductionWorldFragmentRef, states []ProductionWorldStateRef, createdBy string, createdAt time.Time) (ProductionWorldBinding, error) {
	if !validProductionWorldIDs(id, workspaceID, projectID, createdBy) || !keyPattern.MatchString(identityKey) || revision < 1 ||
		asset.Kind != "asset_identity" || asset.BusinessKey != identityKey || specification.Kind != "specification" ||
		!validProductionWorldFragmentRef(asset) || !validProductionWorldFragmentRef(specification) || len(states) == 0 || createdAt.IsZero() {
		return ProductionWorldBinding{}, errors.New("invalid Production World Binding input")
	}
	states = append([]ProductionWorldStateRef(nil), states...)
	slices.SortFunc(states, func(left, right ProductionWorldStateRef) int { return strings.Compare(left.StateKey, right.StateKey) })
	for index, state := range states {
		if !validProductionWorldUUID(state.ID) || state.AssetID != asset.ID || !statePattern.MatchString(state.StateKey) || state.Revision < 1 ||
			!hashPattern.MatchString(state.ContentHash) || (index > 0 && states[index-1].StateKey == state.StateKey) {
			return ProductionWorldBinding{}, errors.New("invalid Production World Binding state")
		}
	}
	value := ProductionWorldBinding{ID: id, WorkspaceID: workspaceID, ProjectID: projectID, IdentityKey: identityKey, Revision: revision, Asset: asset, Specification: specification, States: states, CreatedBy: createdBy, CreatedAt: createdAt.UTC()}
	var err error
	value.ContentHash, err = CanonicalStoryHash(struct {
		Schema, IdentityKey  string
		Asset, Specification ProductionWorldFragmentRef
		States               []ProductionWorldStateRef
	}{"production-world-binding", identityKey, asset, specification, states})
	return value, err
}

func NewProductionWorldBibleVersion(value ProductionWorldBibleVersion) (ProductionWorldBibleVersion, error) {
	if !validProductionWorldIDs(value.ID, value.WorkspaceID, value.ProjectID, value.ReviewDecisionID, value.CreatedBy) || value.Revision < 1 ||
		!validProductionWorldOwnerRef(value.StructureIdentitySet, "production/bible") || !validProductionWorldOwnerRef(value.Candidate, "agent") ||
		!hashPattern.MatchString(value.PartitionHash) || !hashPattern.MatchString(value.BusinessKeyRoot) || len(value.Specifications) == 0 || len(value.Bindings) == 0 || value.CreatedAt.IsZero() {
		return ProductionWorldBibleVersion{}, errors.New("invalid Production World Bible input")
	}
	for _, refs := range [][]ProductionWorldFragmentRef{value.Evidence, value.Specifications, value.Claims, value.Bindings} {
		if !canonicalProductionWorldRefs(refs) {
			return ProductionWorldBibleVersion{}, errors.New("invalid Production World Bible refs")
		}
	}
	value.CreatedAt = value.CreatedAt.UTC()
	value.ContentHash = ""
	hash, err := CanonicalStoryHash(struct {
		Schema                                           string
		StructureIdentitySet, Candidate                  ProductionWorldOwnerRef
		ReviewDecisionID, PartitionHash, BusinessKeyRoot string
		Evidence, Specifications, Claims, Bindings       []ProductionWorldFragmentRef
	}{"production-world-bible", value.StructureIdentitySet, value.Candidate, value.ReviewDecisionID, value.PartitionHash, value.BusinessKeyRoot, value.Evidence, value.Specifications, value.Claims, value.Bindings})
	value.ContentHash = hash
	return value, err
}

func NewProductionWorldBibleHead(workspaceID, projectID string, revision int64, version ProductionWorldBibleVersion, updatedAt time.Time) (ProductionWorldBibleHead, error) {
	if !validProductionWorldIDs(workspaceID, projectID) || version.WorkspaceID != workspaceID || version.ProjectID != projectID || version.Revision != revision || revision < 1 || !hashPattern.MatchString(version.ContentHash) || updatedAt.IsZero() {
		return ProductionWorldBibleHead{}, errors.New("invalid Production World Bible Head input")
	}
	member := ProductionWorldOwnerRef{OwnerKind: "production/bible", LogicalID: projectID, VersionID: version.ID, Revision: version.Revision, ContentHash: version.ContentHash}
	scopeKey := "project:" + projectID
	scopeHash, err := CanonicalStoryHash(struct {
		Schema, OwnerKind, Family, ScopeKey string
		ScopeRevision                       int64
		RootRefs                            []ProductionWorldOwnerRef
	}{"production-world-bible-scope", "production/bible", BibleProductionWorldFamily, scopeKey, revision, []ProductionWorldOwnerRef{member}})
	if err != nil {
		return ProductionWorldBibleHead{}, err
	}
	membersHash, err := CanonicalStoryHash(struct {
		Schema  string
		Members []ProductionWorldOwnerRef
	}{"production-world-bible-members", []ProductionWorldOwnerRef{member}})
	if err != nil {
		return ProductionWorldBibleHead{}, err
	}
	collectionRootHash, err := CanonicalStoryHash(struct {
		Schema, Family, ScopeKey, ScopeContentHash, MembersHash string
		ScopeRevision                                           int64
		MemberCount                                             int
	}{"production-world-bible-collection", BibleProductionWorldFamily, scopeKey, scopeHash, membersHash, revision, 1})
	if err != nil {
		return ProductionWorldBibleHead{}, err
	}
	value := ProductionWorldBibleHead{
		WorkspaceID: workspaceID, ProjectID: projectID, CurrentVersionID: version.ID, ScopeKey: scopeKey,
		ScopeRevision: revision, HeadRevision: revision, MemberCount: 1,
		VersionContentHash: version.ContentHash, ScopeContentHash: scopeHash,
		MembersHash: membersHash, CollectionRootHash: collectionRootHash, UpdatedAt: updatedAt.UTC(),
	}
	value.HeadContentHash, err = CanonicalStoryHash(struct {
		Schema, WorkspaceID, ProjectID, CurrentVersionID, ScopeKey, VersionContentHash string
		ScopeContentHash, MembersHash, CollectionRootHash                              string
		ScopeRevision, HeadRevision                                                    int64
		MemberCount                                                                    int
	}{"production-world-bible-head", workspaceID, projectID, version.ID, scopeKey, version.ContentHash,
		scopeHash, membersHash, collectionRootHash, revision, revision, 1})
	return value, err
}

func FragmentRef(id, kind, key, hash string, revision int) ProductionWorldFragmentRef {
	return ProductionWorldFragmentRef{ID: id, Kind: kind, BusinessKey: key, Revision: revision, ContentHash: hash}
}

func validProductionWorldIDs(values ...string) bool {
	for _, value := range values {
		if !validProductionWorldUUID(value) {
			return false
		}
	}
	return true
}
func validProductionWorldUUID(value string) bool { _, err := uuid.Parse(value); return err == nil }
func validProductionWorldFragmentRef(value ProductionWorldFragmentRef) bool {
	return validProductionWorldUUID(value.ID) && keyPattern.MatchString(value.BusinessKey) && strings.TrimSpace(value.Kind) != "" && value.Revision >= 1 && hashPattern.MatchString(value.ContentHash)
}
func validProductionWorldOwnerRef(value ProductionWorldOwnerRef, owner string) bool {
	return value.OwnerKind == owner && strings.TrimSpace(value.LogicalID) != "" && validProductionWorldUUID(value.VersionID) && value.Revision >= 1 && hashPattern.MatchString(value.ContentHash)
}
func canonicalProductionWorldRefs(values []ProductionWorldFragmentRef) bool {
	for index, value := range values {
		if !validProductionWorldFragmentRef(value) || (index > 0 && values[index-1].BusinessKey >= value.BusinessKey) {
			return false
		}
	}
	return values != nil
}
func canonicalProductionWorldJSON(raw json.RawMessage, leading byte) (json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, errors.New("multiple JSON values")
	}
	if len(bytes.TrimSpace(raw)) == 0 || bytes.TrimSpace(raw)[0] != leading {
		return nil, errors.New("unexpected JSON shape")
	}
	return json.Marshal(value)
}
