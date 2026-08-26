package application

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"sort"
	"strings"

	"github.com/google/uuid"

	storygraph "github.com/StephenQiu30/lanverse/backend/internal/storygraph/domain"
)

const (
	VersionRefCurrent   = "current"
	ScopeProject        = "project"
	ScopeStoryNode      = "story_node"
	DirectionUpstream   = "upstream"
	DirectionDownstream = "downstream"
	maxQueryDepth       = 8
	maxQueryLimit       = 200
)

var lensNodeTypes = map[string]map[storygraph.NodeType]struct{}{
	"outline": nodeTypeSet(storygraph.NodeTypeSourceRevision, storygraph.NodeTypeEpisode, storygraph.NodeTypeScene),
	"narrative": nodeTypeSet(
		storygraph.NodeTypeScene, storygraph.NodeTypeDialogue, storygraph.NodeTypeNarrativeBeat,
		storygraph.NodeTypeOccurrence, storygraph.NodeTypeShot,
	),
	"entity": nodeTypeSet(
		storygraph.NodeTypeAssetIdentity, storygraph.NodeTypeCharacterSpecification,
		storygraph.NodeTypeLocationSpecification, storygraph.NodeTypePropSpecification,
		storygraph.NodeTypeAssetState, storygraph.NodeTypeOccurrence,
		storygraph.NodeTypeRelationshipClaim, storygraph.NodeTypeContinuityClaim,
		storygraph.NodeTypeCausalClaim, storygraph.NodeTypeForeshadowingClaim,
		storygraph.NodeTypePayoffClaim,
	),
	"production": nodeTypeSet(
		storygraph.NodeTypeShot, storygraph.NodeTypeProductionBinding,
		storygraph.NodeTypeShotProductionBindingVersion, storygraph.NodeTypeGenerationTarget,
		storygraph.NodeTypeArtifact, storygraph.NodeTypeAssetVersion,
		storygraph.NodeTypeShotImageBindingVersion,
	),
	"impact": nil,
}

type VersionReader interface {
	GetCurrentVersion(context.Context, Actor, string) (storygraph.Version, error)
	GetExactVersion(context.Context, Actor, string, string) (storygraph.Version, error)
	GetCurrentOwnerSetHash(context.Context, Actor, string) (string, error)
}

type QueryService struct{ reader VersionReader }

func NewQueryService(reader VersionReader) *QueryService { return &QueryService{reader: reader} }

type VersionQuery struct {
	ProjectID, VersionRef string
}

type VersionResult struct {
	Version      storygraph.Version
	CompiledFrom []storygraph.OwnerHeadRef
	Stale        bool
}

type LensQuery struct {
	ProjectID, VersionRef, Lens, ScopeKind, ScopeID, Cursor string
	Depth, Limit                                            int
}

type TraceQuery struct {
	ProjectID, VersionRef, StoryNodeKey, Direction, Cursor string
	Depth, Limit                                           int
}

type DiffQuery struct {
	ProjectID, BaseVersionID, TargetVersionID, Cursor string
	Limit                                             int
}

type SubgraphResult struct {
	VersionID, ContentHash              string
	VersionNo                           int64
	Lens, ScopeKind, ScopeID, Direction string
	Depth                               int
	Nodes                               []storygraph.Node
	Edges                               []storygraph.Edge
	Truncated                           bool
	NextCursor, ResultHash              string
}

type NodeChange struct {
	StoryNodeKey, ChangeType, BeforeContentHash, AfterContentHash string
}

type EdgeChange struct {
	EdgeKey, ChangeType, BeforeContentHash, AfterContentHash string
}

type DiffResult struct {
	BaseVersionID, BaseContentHash, TargetVersionID, TargetContentHash string
	NodeChanges                                                        []NodeChange
	EdgeChanges                                                        []EdgeChange
	Truncated                                                          bool
	NextCursor, ResultHash                                             string
}

type queryCursor struct {
	SnapshotKey string `json:"snapshot_key"`
	QueryHash   string `json:"query_hash"`
	Offset      int    `json:"offset"`
}

func (service *QueryService) Version(ctx context.Context, actor Actor, query VersionQuery) (VersionResult, error) {
	if err := validateProjectAndVersion(query.ProjectID, query.VersionRef); err != nil {
		return VersionResult{}, err
	}
	value, err := service.loadVersion(ctx, actor, query.ProjectID, query.VersionRef, nil)
	if err != nil {
		return VersionResult{}, normalizeError(err)
	}
	result := VersionResult{Version: value, CompiledFrom: value.OwnerHeads}
	if query.VersionRef == VersionRefCurrent {
		currentOwnerSetHash, hashErr := service.reader.GetCurrentOwnerSetHash(ctx, actor, query.ProjectID)
		if hashErr != nil {
			return VersionResult{}, normalizeError(hashErr)
		}
		result.Stale = currentOwnerSetHash != value.OwnerSetHash
	}
	return result, nil
}

func (service *QueryService) Lens(ctx context.Context, actor Actor, query LensQuery) (SubgraphResult, error) {
	if err := validateLensQuery(query); err != nil {
		return SubgraphResult{}, err
	}
	fingerprint, err := storygraph.HashCanonicalValue(struct {
		Kind, ProjectID, Lens, ScopeKind, ScopeID string
		Depth, Limit                              int
	}{"lens", query.ProjectID, query.Lens, query.ScopeKind, query.ScopeID, query.Depth, query.Limit})
	if err != nil {
		return SubgraphResult{}, err
	}
	cursor, err := decodeQueryCursor(query.Cursor, fingerprint)
	if err != nil {
		return SubgraphResult{}, err
	}
	version, err := service.loadVersion(ctx, actor, query.ProjectID, query.VersionRef, cursor)
	if err != nil {
		return SubgraphResult{}, normalizeError(err)
	}
	keys, err := lensKeys(version, query)
	if err != nil {
		return SubgraphResult{}, err
	}
	return pageSubgraph(version, keys, query.Limit, cursor, fingerprint, SubgraphResult{
		Lens: query.Lens, ScopeKind: query.ScopeKind, ScopeID: query.ScopeID, Depth: query.Depth,
	})
}

func (service *QueryService) Trace(ctx context.Context, actor Actor, query TraceQuery) (SubgraphResult, error) {
	if err := validateTraceQuery(query); err != nil {
		return SubgraphResult{}, err
	}
	fingerprint, err := storygraph.HashCanonicalValue(struct {
		Kind, ProjectID, StoryNodeKey, Direction string
		Depth, Limit                             int
	}{"trace", query.ProjectID, query.StoryNodeKey, query.Direction, query.Depth, query.Limit})
	if err != nil {
		return SubgraphResult{}, err
	}
	cursor, err := decodeQueryCursor(query.Cursor, fingerprint)
	if err != nil {
		return SubgraphResult{}, err
	}
	version, err := service.loadVersion(ctx, actor, query.ProjectID, query.VersionRef, cursor)
	if err != nil {
		return SubgraphResult{}, normalizeError(err)
	}
	keys, err := traceKeys(version, query.StoryNodeKey, query.Direction, query.Depth)
	if err != nil {
		return SubgraphResult{}, err
	}
	return pageSubgraph(version, keys, query.Limit, cursor, fingerprint, SubgraphResult{
		ScopeKind: ScopeStoryNode, ScopeID: query.StoryNodeKey,
		Direction: query.Direction, Depth: query.Depth,
	})
}

func (service *QueryService) Diff(ctx context.Context, actor Actor, query DiffQuery) (DiffResult, error) {
	query.ProjectID = strings.TrimSpace(query.ProjectID)
	query.BaseVersionID = strings.TrimSpace(query.BaseVersionID)
	query.TargetVersionID = strings.TrimSpace(query.TargetVersionID)
	if _, err := uuid.Parse(query.ProjectID); err != nil || query.BaseVersionID == query.TargetVersionID || query.Limit < 1 || query.Limit > maxQueryLimit {
		return DiffResult{}, invalid("Invalid StoryGraph diff query")
	}
	if _, err := uuid.Parse(query.BaseVersionID); err != nil {
		return DiffResult{}, invalid("Invalid StoryGraph diff query")
	}
	if _, err := uuid.Parse(query.TargetVersionID); err != nil {
		return DiffResult{}, invalid("Invalid StoryGraph diff query")
	}
	fingerprint, err := storygraph.HashCanonicalValue(struct {
		Kind, ProjectID string
		Limit           int
	}{"diff", query.ProjectID, query.Limit})
	if err != nil {
		return DiffResult{}, err
	}
	cursor, err := decodeQueryCursor(query.Cursor, fingerprint)
	if err != nil {
		return DiffResult{}, err
	}
	snapshotKey := query.BaseVersionID + ":" + query.TargetVersionID
	if cursor != nil && cursor.SnapshotKey != snapshotKey {
		return DiffResult{}, staleCursor()
	}
	base, err := service.reader.GetExactVersion(ctx, actor, query.ProjectID, query.BaseVersionID)
	if err != nil {
		return DiffResult{}, normalizeError(err)
	}
	target, err := service.reader.GetExactVersion(ctx, actor, query.ProjectID, query.TargetVersionID)
	if err != nil {
		return DiffResult{}, normalizeError(err)
	}
	return diffVersions(base, target, query.Limit, cursor, fingerprint)
}

func (service *QueryService) loadVersion(ctx context.Context, actor Actor, projectID, versionRef string, cursor *queryCursor) (storygraph.Version, error) {
	if cursor != nil {
		if versionRef != VersionRefCurrent && cursor.SnapshotKey != versionRef {
			return storygraph.Version{}, staleCursor()
		}
		return service.reader.GetExactVersion(ctx, actor, projectID, cursor.SnapshotKey)
	}
	if versionRef == VersionRefCurrent {
		return service.reader.GetCurrentVersion(ctx, actor, projectID)
	}
	return service.reader.GetExactVersion(ctx, actor, projectID, versionRef)
}

func validateProjectAndVersion(projectID, versionRef string) error {
	if _, err := uuid.Parse(strings.TrimSpace(projectID)); err != nil {
		return invalid("Invalid StoryGraph version query")
	}
	versionRef = strings.TrimSpace(versionRef)
	if versionRef == VersionRefCurrent {
		return nil
	}
	if _, err := uuid.Parse(versionRef); err != nil {
		return invalid("Invalid StoryGraph version query")
	}
	return nil
}

func validateLensQuery(query LensQuery) error {
	if err := validateProjectAndVersion(query.ProjectID, query.VersionRef); err != nil {
		return err
	}
	if _, ok := lensNodeTypes[query.Lens]; !ok || query.Depth < 0 || query.Depth > maxQueryDepth || query.Limit < 1 || query.Limit > maxQueryLimit {
		return invalid("Invalid StoryGraph Lens query")
	}
	if query.ScopeKind != ScopeProject && query.ScopeKind != ScopeStoryNode || strings.TrimSpace(query.ScopeID) == "" {
		return invalid("Invalid StoryGraph Lens scope")
	}
	if query.ScopeKind == ScopeProject && query.ScopeID != query.ProjectID || query.Lens == "impact" && query.ScopeKind != ScopeStoryNode {
		return invalid("Invalid StoryGraph Lens scope")
	}
	return nil
}

func validateTraceQuery(query TraceQuery) error {
	if err := validateProjectAndVersion(query.ProjectID, query.VersionRef); err != nil {
		return err
	}
	if query.Direction != DirectionUpstream && query.Direction != DirectionDownstream || strings.TrimSpace(query.StoryNodeKey) == "" ||
		query.Depth < 0 || query.Depth > maxQueryDepth || query.Limit < 1 || query.Limit > maxQueryLimit {
		return invalid("Invalid StoryGraph trace query")
	}
	return nil
}

func lensKeys(version storygraph.Version, query LensQuery) ([]string, error) {
	allowed := lensNodeTypes[query.Lens]
	if query.ScopeKind == ScopeProject {
		result := make([]string, 0, len(version.Nodes))
		for _, node := range version.Nodes {
			if allowed == nil {
				return nil, invalid("Impact Lens requires a story node scope")
			}
			if _, ok := allowed[node.NodeType]; ok {
				result = append(result, node.StoryNodeKey)
			}
		}
		sort.Strings(result)
		return result, nil
	}
	allKeys, err := neighborhoodKeys(version, query.ScopeID, query.Depth)
	if err != nil {
		return nil, err
	}
	if allowed == nil {
		return allKeys, nil
	}
	result := make([]string, 0, len(allKeys))
	for _, key := range allKeys {
		node, ok := findVersionNode(version, key)
		if ok {
			if _, accepted := allowed[node.NodeType]; accepted {
				result = append(result, key)
			}
		}
	}
	if !containsKey(result, query.ScopeID) {
		return nil, invalid("StoryGraph Lens scope is outside the selected Lens")
	}
	return result, nil
}

func neighborhoodKeys(version storygraph.Version, focus string, depth int) ([]string, error) {
	forward, reverse, exists := adjacency(version, focus)
	if !exists {
		return nil, ErrNotFound
	}
	return breadthFirst(focus, depth, func(key string) []string {
		return sortedUnique(append(append([]string(nil), forward[key]...), reverse[key]...))
	}), nil
}

func traceKeys(version storygraph.Version, focus, direction string, depth int) ([]string, error) {
	forward, reverse, exists := adjacency(version, focus)
	if !exists {
		return nil, ErrNotFound
	}
	selected := forward
	if direction == DirectionUpstream {
		selected = reverse
	}
	return breadthFirst(focus, depth, func(key string) []string { return selected[key] }), nil
}

func adjacency(version storygraph.Version, focus string) (map[string][]string, map[string][]string, bool) {
	forward := make(map[string][]string, len(version.Nodes))
	reverse := make(map[string][]string, len(version.Nodes))
	exists := false
	for _, node := range version.Nodes {
		forward[node.StoryNodeKey] = []string{}
		reverse[node.StoryNodeKey] = []string{}
		if node.StoryNodeKey == focus {
			exists = true
		}
	}
	for _, edge := range version.Edges {
		forward[edge.FromNodeKey] = append(forward[edge.FromNodeKey], edge.ToNodeKey)
		reverse[edge.ToNodeKey] = append(reverse[edge.ToNodeKey], edge.FromNodeKey)
	}
	for key := range forward {
		forward[key] = sortedUnique(forward[key])
		reverse[key] = sortedUnique(reverse[key])
	}
	return forward, reverse, exists
}

func breadthFirst(focus string, depth int, adjacent func(string) []string) []string {
	distance := map[string]int{focus: 0}
	queue := []string{focus}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if distance[current] >= depth {
			continue
		}
		for _, target := range adjacent(current) {
			if _, seen := distance[target]; seen {
				continue
			}
			distance[target] = distance[current] + 1
			queue = append(queue, target)
		}
	}
	result := make([]string, 0, len(distance))
	for key := range distance {
		result = append(result, key)
	}
	sort.Strings(result)
	return result
}

func pageSubgraph(version storygraph.Version, keys []string, limit int, cursor *queryCursor, fingerprint string, result SubgraphResult) (SubgraphResult, error) {
	offset := 0
	if cursor != nil {
		offset = cursor.Offset
	}
	selected := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		selected[key] = struct{}{}
	}
	nodes := make([]storygraph.Node, 0, len(keys))
	for _, node := range version.Nodes {
		if _, ok := selected[node.StoryNodeKey]; ok {
			nodes = append(nodes, node)
		}
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].StoryNodeKey < nodes[j].StoryNodeKey })
	edges := make([]storygraph.Edge, 0)
	for _, edge := range version.Edges {
		_, from := selected[edge.FromNodeKey]
		_, to := selected[edge.ToNodeKey]
		if from && to {
			edges = append(edges, edge)
		}
	}
	sort.Slice(edges, func(i, j int) bool { return edges[i].EdgeKey < edges[j].EdgeKey })
	total := len(nodes) + len(edges)
	if offset < 0 || offset > total {
		return SubgraphResult{}, staleCursor()
	}
	end := offset + limit
	if end > total {
		end = total
	}
	nodeStart, nodeEnd := min(offset, len(nodes)), min(end, len(nodes))
	edgeStart, edgeEnd := max(offset-len(nodes), 0), max(end-len(nodes), 0)
	result.VersionID, result.VersionNo, result.ContentHash = version.ID, version.VersionNo, version.ContentHash
	result.Nodes = append([]storygraph.Node{}, nodes[nodeStart:nodeEnd]...)
	result.Edges = append([]storygraph.Edge{}, edges[edgeStart:edgeEnd]...)
	result.Truncated = end < total
	if result.Truncated {
		result.NextCursor = encodeQueryCursor(queryCursor{SnapshotKey: version.ID, QueryHash: fingerprint, Offset: end})
	}
	hash, err := storygraph.HashCanonicalValue(struct {
		VersionID, ContentHash, Lens, ScopeKind, ScopeID, Direction string
		Depth, Offset                                               int
		Nodes                                                       []storygraph.Node
		Edges                                                       []storygraph.Edge
		Truncated                                                   bool
	}{version.ID, version.ContentHash, result.Lens, result.ScopeKind, result.ScopeID, result.Direction, result.Depth, offset, result.Nodes, result.Edges, result.Truncated})
	if err != nil {
		return SubgraphResult{}, err
	}
	result.ResultHash = hash
	return result, nil
}

func diffVersions(base, target storygraph.Version, limit int, cursor *queryCursor, fingerprint string) (DiffResult, error) {
	items := make([]diffItem, 0)
	baseNodes, targetNodes := nodeMap(base.Nodes), nodeMap(target.Nodes)
	for _, key := range unionKeys(baseNodes, targetNodes) {
		before, beforeFound := baseNodes[key]
		after, afterFound := targetNodes[key]
		changeType := changeType(beforeFound, afterFound, before.ContentHash, after.ContentHash)
		if changeType != "" {
			items = append(items, diffItem{sortKey: "node\x00" + key, node: &NodeChange{StoryNodeKey: key, ChangeType: changeType, BeforeContentHash: before.ContentHash, AfterContentHash: after.ContentHash}})
		}
	}
	baseEdges, targetEdges := edgeMap(base.Edges), edgeMap(target.Edges)
	for _, key := range unionKeys(baseEdges, targetEdges) {
		before, beforeFound := baseEdges[key]
		after, afterFound := targetEdges[key]
		changeType := changeType(beforeFound, afterFound, before.ContentHash, after.ContentHash)
		if changeType != "" {
			items = append(items, diffItem{sortKey: "edge\x00" + key, edge: &EdgeChange{EdgeKey: key, ChangeType: changeType, BeforeContentHash: before.ContentHash, AfterContentHash: after.ContentHash}})
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].sortKey < items[j].sortKey })
	offset := 0
	if cursor != nil {
		offset = cursor.Offset
	}
	if offset < 0 || offset > len(items) {
		return DiffResult{}, staleCursor()
	}
	end := offset + limit
	if end > len(items) {
		end = len(items)
	}
	result := DiffResult{
		BaseVersionID: base.ID, BaseContentHash: base.ContentHash,
		TargetVersionID: target.ID, TargetContentHash: target.ContentHash,
		NodeChanges: []NodeChange{}, EdgeChanges: []EdgeChange{}, Truncated: end < len(items),
	}
	for _, item := range items[offset:end] {
		if item.node != nil {
			result.NodeChanges = append(result.NodeChanges, *item.node)
		} else {
			result.EdgeChanges = append(result.EdgeChanges, *item.edge)
		}
	}
	if result.Truncated {
		result.NextCursor = encodeQueryCursor(queryCursor{SnapshotKey: base.ID + ":" + target.ID, QueryHash: fingerprint, Offset: end})
	}
	hash, err := storygraph.HashCanonicalValue(struct {
		BaseVersionID, BaseContentHash, TargetVersionID, TargetContentHash string
		Offset                                                             int
		NodeChanges                                                        []NodeChange
		EdgeChanges                                                        []EdgeChange
		Truncated                                                          bool
	}{base.ID, base.ContentHash, target.ID, target.ContentHash, offset, result.NodeChanges, result.EdgeChanges, result.Truncated})
	if err != nil {
		return DiffResult{}, err
	}
	result.ResultHash = hash
	return result, nil
}

type diffItem struct {
	sortKey string
	node    *NodeChange
	edge    *EdgeChange
}

func nodeMap(values []storygraph.Node) map[string]storygraph.Node {
	result := make(map[string]storygraph.Node, len(values))
	for _, value := range values {
		result[value.StoryNodeKey] = value
	}
	return result
}

func edgeMap(values []storygraph.Edge) map[string]storygraph.Edge {
	result := make(map[string]storygraph.Edge, len(values))
	for _, value := range values {
		result[value.EdgeKey] = value
	}
	return result
}

func unionKeys[T any](left, right map[string]T) []string {
	set := make(map[string]struct{}, len(left)+len(right))
	for key := range left {
		set[key] = struct{}{}
	}
	for key := range right {
		set[key] = struct{}{}
	}
	result := make([]string, 0, len(set))
	for key := range set {
		result = append(result, key)
	}
	sort.Strings(result)
	return result
}

func changeType(beforeFound, afterFound bool, beforeHash, afterHash string) string {
	switch {
	case !beforeFound && afterFound:
		return "added"
	case beforeFound && !afterFound:
		return "removed"
	case beforeHash != afterHash:
		return "changed"
	default:
		return ""
	}
}

func findVersionNode(version storygraph.Version, key string) (storygraph.Node, bool) {
	index := sort.Search(len(version.Nodes), func(index int) bool { return version.Nodes[index].StoryNodeKey >= key })
	if index < len(version.Nodes) && version.Nodes[index].StoryNodeKey == key {
		return version.Nodes[index], true
	}
	return storygraph.Node{}, false
}

func encodeQueryCursor(cursor queryCursor) string {
	encoded, _ := json.Marshal(cursor)
	return base64.RawURLEncoding.EncodeToString(encoded)
}

func decodeQueryCursor(value, expectedHash string) (*queryCursor, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return nil, invalid("Invalid StoryGraph query cursor")
	}
	var cursor queryCursor
	decoder := json.NewDecoder(bytes.NewReader(decoded))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&cursor); err != nil {
		return nil, invalid("Invalid StoryGraph query cursor")
	}
	if err = decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) || cursor.SnapshotKey == "" || cursor.QueryHash != expectedHash || cursor.Offset < 1 {
		return nil, staleCursor()
	}
	return &cursor, nil
}

func staleCursor() error {
	return &Error{
		Code: "stale_storygraph_cursor", Message: "StoryGraph query cursor does not match the requested snapshot", Status: 409,
		NextAction: "Restart the query or continue from the exact StoryGraph version embedded in the cursor",
	}
}

func nodeTypeSet(values ...storygraph.NodeType) map[storygraph.NodeType]struct{} {
	result := make(map[storygraph.NodeType]struct{}, len(values))
	for _, value := range values {
		result[value] = struct{}{}
	}
	return result
}

func sortedUnique(values []string) []string {
	sort.Strings(values)
	result := values[:0]
	for _, value := range values {
		if len(result) == 0 || result[len(result)-1] != value {
			result = append(result, value)
		}
	}
	return result
}

func containsKey(values []string, expected string) bool {
	index := sort.SearchStrings(values, expected)
	return index < len(values) && values[index] == expected
}
