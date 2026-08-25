package application

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"strconv"
	"time"

	"github.com/StephenQiu30/lanverse/backend/internal/production/storyboard/domain"
)

type packageResult struct {
	Bytes       []byte
	ContentHash string
	Manifest    map[string]any
	Files       []domain.ExportFile
}
type packageFile struct {
	name, mediaType string
	contents        []byte
}

func buildPackage(episodeID, inputHash string, shots []domain.Shot) (packageResult, error) {
	storyboardJSON, err := json.MarshalIndent(map[string]any{"episode_id": episodeID, "shots": shots}, "", "  ")
	if err != nil {
		return packageResult{}, err
	}
	storyboardJSON = append(storyboardJSON, '\n')
	var csvBuffer bytes.Buffer
	csvWriter := csv.NewWriter(&csvBuffer)
	_ = csvWriter.Write([]string{"position", "title", "duration_ms", "content_hash"})
	for _, shot := range shots {
		duration := 0
		if value, ok := shot.Spec["duration_ms"].(float64); ok {
			duration = int(value)
		}
		_ = csvWriter.Write([]string{strconv.Itoa(shot.Position), shot.Title, strconv.Itoa(duration), shot.ContentHash})
	}
	csvWriter.Flush()
	if err = csvWriter.Error(); err != nil {
		return packageResult{}, err
	}
	var htmlBuffer bytes.Buffer
	page := template.Must(template.New("storyboard").Parse(`<!doctype html><html lang="zh-CN"><head><meta charset="utf-8"><title>Lanverse 分镜包</title></head><body><main><h1>分镜表</h1>{{range .}}<article><h2>{{.Position}}. {{.Title}}</h2><p>内容哈希：<code>{{.ContentHash}}</code></p></article>{{end}}</main></body></html>`))
	if err = page.Execute(&htmlBuffer, shots); err != nil {
		return packageResult{}, err
	}
	payloadFiles := []packageFile{{"storyboard.json", "application/json", storyboardJSON}, {"storyboard.csv", "text/csv; charset=utf-8", csvBuffer.Bytes()}, {"storyboard.html", "text/html; charset=utf-8", htmlBuffer.Bytes()}}
	manifestFiles := make([]map[string]any, len(payloadFiles))
	for index, file := range payloadFiles {
		manifestFiles[index] = map[string]any{"name": file.name, "media_type": file.mediaType, "size": len(file.contents), "sha256": hashBytes(file.contents)}
	}
	manifest := map[string]any{"schema_version": "storyboard-export-v1", "episode_id": episodeID, "input_hash": inputHash, "shot_count": len(shots), "files": manifestFiles}
	manifestJSON, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return packageResult{}, err
	}
	manifestJSON = append(manifestJSON, '\n')
	files := append([]packageFile{{"manifest.json", "application/json", manifestJSON}}, payloadFiles...)
	publicFiles := make([]domain.ExportFile, len(files))
	var archive bytes.Buffer
	writer := zip.NewWriter(&archive)
	fixed := time.Date(1980, time.January, 1, 0, 0, 0, 0, time.UTC)
	for index, file := range files {
		header := &zip.FileHeader{Name: file.name, Method: zip.Deflate}
		header.SetModTime(fixed)
		header.SetMode(0o644)
		entry, createErr := writer.CreateHeader(header)
		if createErr != nil {
			return packageResult{}, createErr
		}
		if _, writeErr := entry.Write(file.contents); writeErr != nil {
			return packageResult{}, writeErr
		}
		publicFiles[index] = domain.ExportFile{Name: file.name, MediaType: file.mediaType, Size: len(file.contents), SHA256: hashBytes(file.contents)}
	}
	if err = writer.Close(); err != nil {
		return packageResult{}, fmt.Errorf("close storyboard package: %w", err)
	}
	return packageResult{Bytes: archive.Bytes(), ContentHash: hashBytes(archive.Bytes()), Manifest: manifest, Files: publicFiles}, nil
}

func hashBytes(value []byte) string { hash := sha256.Sum256(value); return hex.EncodeToString(hash[:]) }
