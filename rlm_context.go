package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/modelrelay/modelrelay/sdk/go/llm"
)

type rlmFileAttachment struct {
	Name string `json:"name,omitempty"`
	Path string `json:"path,omitempty"`
	Mime string `json:"mime,omitempty"`
	Size int64  `json:"size,omitempty"`
	Text string `json:"text,omitempty"`
}

func hasStdinAttachment(paths []string) bool {
	for _, raw := range paths {
		if strings.TrimSpace(raw) == "-" {
			return true
		}
	}
	return false
}

func buildRLMFileAttachments(paths []string, overrideMime string, stdin io.Reader, textInlineLimit int64) ([]rlmFileAttachment, func(), error) {
	if len(paths) == 0 {
		return nil, nil, nil
	}
	var override llm.MimeType
	if strings.TrimSpace(overrideMime) != "" {
		override = llm.NormalizeMimeType(overrideMime)
		if override == "" {
			return nil, nil, errors.New("attachment-type must be a valid MIME type")
		}
	}

	usesStdin := hasStdinAttachment(paths)
	var (
		tempDir string
		cleanup func()
	)
	if usesStdin {
		dir, err := os.MkdirTemp("", "modelrelay-rlm-input-")
		if err != nil {
			return nil, nil, fmt.Errorf("create temp dir: %w", err)
		}
		tempDir = dir
		cleanup = func() {
			if err := os.RemoveAll(dir); err != nil {
				log.Printf("warning: failed to remove temp dir %s: %v", dir, err)
			}
		}
	}

	seenStdin := false
	files := make([]rlmFileAttachment, 0, len(paths))
	for _, raw := range paths {
		path := strings.TrimSpace(raw)
		if path == "" {
			if cleanup != nil {
				cleanup()
			}
			return nil, nil, errors.New("attachment path is empty")
		}

		if path == "-" {
			if seenStdin {
				if cleanup != nil {
					cleanup()
				}
				return nil, nil, errors.New("stdin attachment can only be specified once")
			}
			seenStdin = true
			if tempDir == "" {
				if cleanup != nil {
					cleanup()
				}
				return nil, nil, errors.New("stdin temp dir not configured")
			}
			stdinPath, size, sample, text, err := writeStdinToTempFile(stdin, tempDir, textInlineLimit)
			if err != nil {
				if cleanup != nil {
					cleanup()
				}
				return nil, nil, err
			}
			mimeType := override
			if mimeType == "" {
				mimeType = llm.NormalizeMimeType(detectMimeType(stdinPath, sample))
			}
			if !isTextMime(mimeType) {
				text = ""
			}
			files = append(files, rlmFileAttachment{
				Name: "stdin",
				Path: stdinPath,
				Mime: string(mimeType),
				Size: size,
				Text: text,
			})
			continue
		}

		info, err := os.Stat(path)
		if err != nil {
			if cleanup != nil {
				cleanup()
			}
			return nil, nil, err
		}
		if info.IsDir() {
			if cleanup != nil {
				cleanup()
			}
			return nil, nil, fmt.Errorf("attachment is a directory: %s", path)
		}
		absPath, err := filepath.Abs(path)
		if err != nil {
			if cleanup != nil {
				cleanup()
			}
			return nil, nil, err
		}
		sample, err := readFileSample(absPath)
		if err != nil {
			if cleanup != nil {
				cleanup()
			}
			return nil, nil, err
		}
		mimeType := override
		if mimeType == "" {
			mimeType = llm.NormalizeMimeType(detectMimeType(absPath, sample))
		}
		text, err := maybeReadInlineText(absPath, mimeType, info.Size(), textInlineLimit)
		if err != nil {
			if cleanup != nil {
				cleanup()
			}
			return nil, nil, err
		}

		files = append(files, rlmFileAttachment{
			Name: filepath.Base(absPath),
			Path: absPath,
			Mime: string(mimeType),
			Size: info.Size(),
			Text: text,
		})
	}

	return files, cleanup, nil
}

func readFileSample(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	buf := make([]byte, 512)
	n, err := f.Read(buf)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	return buf[:n], nil
}

func writeStdinToTempFile(stdin io.Reader, dir string, textInlineLimit int64) (string, int64, []byte, string, error) {
	f, err := os.CreateTemp(dir, "rlm-stdin-*")
	if err != nil {
		return "", 0, nil, "", err
	}
	defer f.Close()

	var (
		size   int64
		sample []byte
		text   []byte
		tooBig bool
		buf    = make([]byte, 32*1024)
	)
	for {
		n, readErr := stdin.Read(buf)
		if n > 0 {
			if len(sample) < 512 {
				remain := 512 - len(sample)
				if remain > n {
					remain = n
				}
				sample = append(sample, buf[:remain]...)
			}
			if textInlineLimit > 0 && !tooBig {
				if int64(len(text)+n) <= textInlineLimit {
					text = append(text, buf[:n]...)
				} else {
					tooBig = true
					text = nil
				}
			}
			written, writeErr := f.Write(buf[:n])
			size += int64(written)
			if writeErr != nil {
				return "", 0, nil, "", writeErr
			}
			if written != n {
				return "", 0, nil, "", errors.New("short write to temp file")
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			return "", 0, nil, "", readErr
		}
	}

	absPath, err := filepath.Abs(f.Name())
	if err != nil {
		return "", 0, nil, "", err
	}
	textOut := ""
	if len(text) > 0 && utf8.Valid(text) {
		textOut = string(text)
	}
	return absPath, size, sample, textOut, nil
}

func maybeReadInlineText(path string, mime llm.MimeType, size int64, textInlineLimit int64) (string, error) {
	if textInlineLimit <= 0 || size <= 0 || size > textInlineLimit {
		return "", nil
	}
	if !isTextMime(mime) {
		return "", nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	if !utf8.Valid(data) {
		return "", nil
	}
	return string(data), nil
}

func isTextMime(mime llm.MimeType) bool {
	normalized := strings.ToLower(string(mime))
	if strings.HasPrefix(normalized, "text/") {
		return true
	}
	switch normalized {
	case "application/json",
		"application/xml",
		"application/xhtml+xml",
		"application/csv",
		"application/x-ndjson":
		return true
	default:
		return false
	}
}

func mergeRLMContextFiles(payload []byte, files []rlmFileAttachment) ([]byte, error) {
	if len(files) == 0 {
		return payload, nil
	}
	if len(payload) == 0 {
		payload = []byte("null")
	}

	var root any
	if err := json.Unmarshal(payload, &root); err != nil {
		return nil, fmt.Errorf("context must be valid JSON: %w", err)
	}
	if root == nil {
		root = map[string]any{}
	}
	obj, ok := root.(map[string]any)
	if !ok {
		return nil, errors.New("context must be an object or null when using attachments")
	}

	var list []any
	if existing, ok := obj["files"]; ok {
		arr, ok := existing.([]any)
		if !ok {
			return nil, errors.New("context.files must be an array when using attachments")
		}
		list = arr
	}

	for _, file := range files {
		entry := map[string]any{}
		if strings.TrimSpace(file.Name) != "" {
			entry["name"] = file.Name
		}
		if strings.TrimSpace(file.Path) != "" {
			entry["path"] = file.Path
		}
		if strings.TrimSpace(file.Mime) != "" {
			entry["mime"] = file.Mime
		}
		if file.Size > 0 {
			entry["size"] = file.Size
		}
		if strings.TrimSpace(file.Text) != "" {
			entry["text"] = file.Text
		}
		list = append(list, entry)
	}
	obj["files"] = list

	out, err := json.Marshal(obj)
	if err != nil {
		return nil, err
	}
	return out, nil
}
