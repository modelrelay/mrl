package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildRLMFileAttachments_File(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	files, cleanup, err := buildRLMFileAttachments([]string{path}, "", bytes.NewBuffer(nil), 1024)
	if cleanup != nil {
		defer cleanup()
	}
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}
	if !filepath.IsAbs(files[0].Path) {
		t.Fatalf("expected absolute path, got %q", files[0].Path)
	}
	if files[0].Name != "sample.txt" {
		t.Fatalf("name = %q", files[0].Name)
	}
	if files[0].Size != 5 {
		t.Fatalf("size = %d", files[0].Size)
	}
	if strings.TrimSpace(files[0].Mime) == "" {
		t.Fatalf("expected mime type to be set")
	}
	if files[0].Text != "hello" {
		t.Fatalf("text = %q", files[0].Text)
	}
}

func TestBuildRLMFileAttachments_TextLimit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	files, cleanup, err := buildRLMFileAttachments([]string{path}, "", bytes.NewBuffer(nil), 2)
	if cleanup != nil {
		defer cleanup()
	}
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}
	if files[0].Text != "" {
		t.Fatalf("expected no inline text, got %q", files[0].Text)
	}
}

func TestBuildRLMFileAttachments_Stdin(t *testing.T) {
	files, cleanup, err := buildRLMFileAttachments([]string{"-"}, "", bytes.NewBufferString("abc"), 1024)
	if cleanup != nil {
		defer cleanup()
	}
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}
	if files[0].Name != "stdin" {
		t.Fatalf("name = %q", files[0].Name)
	}
	if files[0].Size != 3 {
		t.Fatalf("size = %d", files[0].Size)
	}
	if !filepath.IsAbs(files[0].Path) {
		t.Fatalf("expected absolute path, got %q", files[0].Path)
	}
	if _, err := os.Stat(files[0].Path); err != nil {
		t.Fatalf("stdin file not found: %v", err)
	}
	if files[0].Text != "abc" {
		t.Fatalf("text = %q", files[0].Text)
	}
}

func TestMergeRLMContextFiles_Null(t *testing.T) {
	out, err := mergeRLMContextFiles([]byte("null"), []rlmFileAttachment{
		{Name: "a.txt", Path: "/tmp/a.txt"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var obj map[string]any
	if err := json.Unmarshal(out, &obj); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	files, ok := obj["files"].([]any)
	if !ok || len(files) != 1 {
		t.Fatalf("expected files array")
	}
}

func TestMergeRLMContextFiles_Object(t *testing.T) {
	base := `{"foo":"bar","files":[{"path":"/tmp/old.txt"}]}`
	out, err := mergeRLMContextFiles([]byte(base), []rlmFileAttachment{
		{Name: "b.txt", Path: "/tmp/b.txt", Text: "inline"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var obj map[string]any
	if err := json.Unmarshal(out, &obj); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	files, ok := obj["files"].([]any)
	if !ok || len(files) != 2 {
		t.Fatalf("expected 2 files, got %v", files)
	}
	last, ok := files[1].(map[string]any)
	if !ok {
		t.Fatalf("expected file entry to be an object")
	}
	if last["text"] != "inline" {
		t.Fatalf("text = %v", last["text"])
	}
}

func TestMergeRLMContextFiles_InvalidRoot(t *testing.T) {
	_, err := mergeRLMContextFiles([]byte(`["bad"]`), []rlmFileAttachment{{Path: "/tmp/a"}})
	if err == nil {
		t.Fatal("expected error")
	}
}
