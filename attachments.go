package main

import (
	"errors"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/modelrelay/modelrelay/sdk/go/llm"
)

func resolveAttachmentInputs(paths []string, attachmentType string, attachStdin bool, stdinIsTTY bool) ([]string, error) {
	resolved := append([]string{}, paths...)
	hasDash := false
	for _, raw := range resolved {
		if strings.TrimSpace(raw) == "-" {
			hasDash = true
			break
		}
	}

	if attachStdin {
		if stdinIsTTY {
			return nil, errors.New("stdin is not piped; use -a PATH or pipe data")
		}
		if !hasDash {
			resolved = append(resolved, "-")
		}
		return resolved, nil
	}

	if len(resolved) == 0 && strings.TrimSpace(attachmentType) != "" && !stdinIsTTY && !hasDash {
		resolved = append(resolved, "-")
	}

	return resolved, nil
}

func buildAttachmentParts(paths []string, overrideMime string, stdin io.Reader) ([]llm.ContentPart, error) {
	if len(paths) == 0 {
		return nil, nil
	}
	var override llm.MimeType
	if strings.TrimSpace(overrideMime) != "" {
		override = llm.NormalizeMimeType(overrideMime)
		if override == "" {
			return nil, errors.New("attachment-type must be a valid MIME type")
		}
	}
	dashCount := 0
	for _, path := range paths {
		if strings.TrimSpace(path) == "-" {
			dashCount++
		}
	}
	if dashCount > 1 {
		return nil, errors.New("stdin attachment can only be specified once")
	}

	parts := make([]llm.ContentPart, 0, len(paths))
	for _, raw := range paths {
		path := strings.TrimSpace(raw)
		if path == "" {
			return nil, errors.New("attachment path is empty")
		}
		if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
			return nil, errors.New("URL attachments are not supported; download the file and pass the path instead")
		}

		var (
			data     []byte
			filename string
			err      error
		)
		if path == "-" {
			data, err = io.ReadAll(stdin)
		} else {
			data, err = os.ReadFile(path)
			filename = filepath.Base(path)
		}
		if err != nil {
			return nil, err
		}
		if len(data) == 0 {
			return nil, errors.New("attachment is empty")
		}

		mimeType := override
		if mimeType == "" {
			mimeType = llm.NormalizeMimeType(detectMimeType(path, data))
		}
		if mimeType == "" {
			return nil, errors.New("could not detect attachment MIME type; use --attachment-type to override")
		}

		parts = append(parts, llm.FilePartFromBytes(data, mimeType, filename))
	}

	return parts, nil
}

func detectMimeType(path string, data []byte) string {
	ext := strings.ToLower(filepath.Ext(path))
	if ext != "" {
		if mt := mime.TypeByExtension(ext); mt != "" {
			return mt
		}
	}
	return http.DetectContentType(data)
}
