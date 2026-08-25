package domain

import "time"

type Document struct {
	ID, WorkspaceID, ProjectID, Title, SourceType, Language, RightsDeclaration, Status, CreatedBy string
	SourceMediaVersionID                                                                          *string
	Revision                                                                                      int
	CreatedAt                                                                                     time.Time
}

type Revision struct {
	ID, WorkspaceID, DocumentID, SourceType, RawText, RawHash, NormalizedText, NormalizedHash string
	NormalizerVersion, AnalysisStatus, AnalyzerVersion, CreatedBy                             string
	SourceMediaVersionID                                                                      *string
	VersionNo, CodepointCount                                                                 int
	NormalizationMap                                                                          map[string]any
	CreatedAt                                                                                 time.Time
	Blocks                                                                                    []Block
	Issues                                                                                    []Issue
}

type Block struct {
	ID, DocumentRevisionID, Kind, TextHash string
	Position, SourceStart, SourceEnd       int
	Metadata                               map[string]any
}

type Issue struct {
	ID, DocumentRevisionID, Code, Severity, NextAction         string
	Position, SourceStart, SourceEnd, LineNumber, ColumnNumber int
	Details                                                    map[string]any
}

type Analysis struct {
	Document Document
	Revision Revision
}
