package main

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelrelay/modelrelay/sdk/go/llm"
)

func TestBuildAttachmentParts_File(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.pdf")
	data := []byte("hello")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	parts, err := buildAttachmentParts([]string{path}, "", strings.NewReader(""))
	if err != nil {
		t.Fatalf("buildAttachmentParts: %v", err)
	}
	if len(parts) != 1 {
		t.Fatalf("expected 1 part, got %d", len(parts))
	}
	part := parts[0]
	if part.Type != llm.ContentPartTypeFile || part.File == nil {
		t.Fatalf("expected file content part, got %+v", part)
	}
	if part.File.MimeType != llm.MimeTypePDF {
		t.Fatalf("expected mime_type %q, got %q", llm.MimeTypePDF, part.File.MimeType)
	}
	if part.File.Filename != "doc.pdf" {
		t.Fatalf("expected filename doc.pdf, got %q", part.File.Filename)
	}
	if part.File.SizeBytes != int64(len(data)) {
		t.Fatalf("expected size %d, got %d", len(data), part.File.SizeBytes)
	}
	if part.File.DataBase64 != base64.StdEncoding.EncodeToString(data) {
		t.Fatalf("unexpected base64 data")
	}
}

func TestBuildAttachmentParts_StdinOverrideMime(t *testing.T) {
	reader := strings.NewReader("data")
	parts, err := buildAttachmentParts([]string{"-"}, "image/png", reader)
	if err != nil {
		t.Fatalf("buildAttachmentParts: %v", err)
	}
	if len(parts) != 1 {
		t.Fatalf("expected 1 part, got %d", len(parts))
	}
	part := parts[0]
	if part.Type != llm.ContentPartTypeFile || part.File == nil {
		t.Fatalf("expected file content part, got %+v", part)
	}
	if part.File.MimeType != llm.MimeTypePNG {
		t.Fatalf("expected mime_type %q, got %q", llm.MimeTypePNG, part.File.MimeType)
	}
	if part.File.Filename != "" {
		t.Fatalf("expected empty filename for stdin, got %q", part.File.Filename)
	}
}

func TestResolveAttachmentInputs_AttachStdinAddsDash(t *testing.T) {
	paths, err := resolveAttachmentInputs(nil, "", true, false)
	if err != nil {
		t.Fatalf("resolveAttachmentInputs: %v", err)
	}
	if len(paths) != 1 || paths[0] != "-" {
		t.Fatalf("expected stdin attachment, got %v", paths)
	}
}

func TestResolveAttachmentInputs_AttachStdinRequiresPipe(t *testing.T) {
	if _, err := resolveAttachmentInputs(nil, "", true, true); err == nil {
		t.Fatalf("expected error when stdin is a TTY")
	}
}

func TestResolveAttachmentInputs_AutoAttachWithTypeAndPipe(t *testing.T) {
	paths, err := resolveAttachmentInputs(nil, "application/pdf", false, false)
	if err != nil {
		t.Fatalf("resolveAttachmentInputs: %v", err)
	}
	if len(paths) != 1 || paths[0] != "-" {
		t.Fatalf("expected stdin attachment, got %v", paths)
	}
}

func TestResolveAttachmentInputs_NoAutoWhenAttachmentProvided(t *testing.T) {
	paths, err := resolveAttachmentInputs([]string{"doc.pdf"}, "application/pdf", false, false)
	if err != nil {
		t.Fatalf("resolveAttachmentInputs: %v", err)
	}
	if len(paths) != 1 || paths[0] != "doc.pdf" {
		t.Fatalf("unexpected paths: %v", paths)
	}
}
