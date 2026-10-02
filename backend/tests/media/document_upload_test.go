package media_test

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	mediaflow "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/workflow"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
)

func documentDOCX(t *testing.T, extra map[string]string) []byte {
	t.Helper()
	parts := map[string]string{
		"[Content_Types].xml": `<?xml version="1.0"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/></Types>`,
		"_rels/.rels":         `<?xml version="1.0"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/></Relationships>`,
		"word/document.xml":   `<?xml version="1.0"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>第一场：抵达</w:t></w:r></w:p><w:tbl><w:tr><w:tc><w:p><w:r><w:t>对白</w:t></w:r></w:p></w:tc></w:tr></w:tbl></w:body></w:document>`,
	}
	for name, content := range extra {
		parts[name] = content
	}
	var out bytes.Buffer
	w := zip.NewWriter(&out)
	for name, content := range parts {
		entry, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(entry, content); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func TestDocumentUploadPreservesOriginalBytesWithoutAVRenditions(t *testing.T) {
	for _, tc := range []struct {
		name string
		data []byte
		mime string
	}{
		{"剧本.txt", []byte("第一场\n你好。\n"), "text/plain"},
		{"UTF16LE.txt", []byte{0xff, 0xfe, 0x2c, 0x7b, 0x00, 0x0a}, "text/plain"},
		{"UTF16BE.txt", []byte{0xfe, 0xff, 0x7b, 0x2c, 0x0a, 0x00}, "text/plain"},
		{"GB18030.txt", []byte{0xb5, 0xda, 0xd2, 0xbb, 0xb3, 0xa1}, "text/plain"},
		{"literal.txt", []byte("MZ 是角色在信里的暗号。"), "text/plain"},
		{"剧本.docx", documentDOCX(t, nil), "application/vnd.openxmlformats-officedocument.wordprocessingml.document"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			file, err := mediaapp.ReadUpload(t.Context(), bytes.NewReader(tc.data), tc.name)
			if err != nil {
				t.Fatalf("document read: %v", err)
			}
			defer func() { _ = file.Close() }()
			if file.MIMEType != tc.mime {
				t.Fatalf("actual MIME = %q", file.MIMEType)
			}
			repo, objects := &uploadRepoFake{}, &uploadObjectsFake{}
			in := mediaapp.UploadInput{Actor: identityapp.Principal{ID: uuid.New(), OrgID: uuid.New()}, Request: mediaapp.UploadRequest{ProjectID: uuid.New(), Key: uuid.New(), FileName: tc.name, RequestID: uuid.New()}, File: file, LocalReviewConfirmed: true}
			service := mediaapp.NewUploadService(repo, mediaflow.FFUploadProber{}, mediaflow.FFUploadNormalizer{}, mediaflow.FFUploadRenderer{}, objects, time.Now)
			got, err := service.Upload(t.Context(), in)
			if err != nil || got.Asset.Kind != "document" || repo.asset.SHA256 == nil || *repo.asset.SHA256 != file.SHA256 || repo.asset.ByteSize != int64(len(tc.data)) || len(repo.rends) != 0 || len(objects.items) != 1 {
				t.Fatalf("reviewed original document: got=%+v err=%v renditions=%d objects=%d", got, err, len(repo.rends), len(objects.items))
			}
			if _, err := file.File.Seek(0, io.SeekStart); err != nil {
				t.Fatal(err)
			}
			actual, err := io.ReadAll(file.File)
			if err != nil || !bytes.Equal(actual, tc.data) {
				t.Fatal("document was normalized or changed", err)
			}
		})
	}
}

func TestDocumentUploadRejectsSpoofedMalformedAndActiveContainers(t *testing.T) {
	for _, tc := range []struct {
		name string
		data []byte
	}{
		{"binary.txt", []byte{0, 1, 2, 3}},
		{"image.txt", uploadPNG(t)},
		{"not-word.docx", []byte("not an OOXML document")},
		{"unsafe.docx", documentDOCX(t, map[string]string{"../escaped.xml": "<x/>"})},
		{"macro.docx", documentDOCX(t, map[string]string{"word/vbaProject.bin": "macro"})},
		{"invalid.docx", documentDOCX(t, map[string]string{"word/document.xml": "<document>"})},
		{"wrong-main.docx", documentDOCX(t, map[string]string{"[Content_Types].xml": `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"/>`})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			file, err := mediaapp.ReadUpload(t.Context(), bytes.NewReader(tc.data), tc.name)
			if err == nil {
				defer func() { _ = file.Close() }()
				_, err = (mediaflow.FFUploadProber{}).Probe(t.Context(), file)
			}
			if !errors.Is(err, mediaapp.ErrUnsupportedUpload) {
				t.Fatalf("invalid document accepted: %v", err)
			}
		})
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := mediaapp.ReadUpload(ctx, bytes.NewReader([]byte("script")), "cancelled.txt"); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled document read", err)
	}
}

func TestDocumentListIsExplicitAndCannotPreviewOrReferenceCanvas(t *testing.T) {
	project, id := uuid.New(), uuid.New()
	now, sha, codec := time.Now(), "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "txt"
	asset := domain.MediaAsset{ID: id, ProjectID: project, Kind: domain.KindDocument, Origin: domain.OriginUpload, Status: domain.StatusReady, ModerationStatus: domain.ModerationPassed, FileName: "剧本.txt", MimeType: "text/plain", ByteSize: 3, Revision: 1, SHA256: &sha, Codec: &codec, CreateTime: now, UpdateTime: now, ObjectKey: "projects/" + project.String() + "/document/2026/10/" + id.String() + ".txt"}
	signer := &previewSigner{}
	q := mediaapp.NewAssetQuery(readStore{asset: asset}, signer)
	if got, err := q.List(t.Context(), identityapp.Principal{}, project, "document", "", 50); err != nil || len(got.Items) != 1 || got.Items[0].Kind != "document" {
		t.Fatal("explicit document list", got, err)
	}
	if _, err := q.List(t.Context(), identityapp.Principal{}, project, "", "", 50); !errors.Is(err, mediaapp.ErrUnavailable) {
		t.Fatal("default list leaked document", err)
	}
	if _, err := q.Reference(t.Context(), identityapp.Principal{}, project, id); !errors.Is(err, mediaapp.ErrNotFound) {
		t.Fatal("document entered canvas media reference", err)
	}
	if _, err := q.Preview(t.Context(), identityapp.Principal{}, project, id); !errors.Is(err, mediaapp.ErrNotFound) || signer.calls != 0 {
		t.Fatal("document got inline private preview", err)
	}
}
