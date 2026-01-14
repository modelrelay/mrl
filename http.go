package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type authMode int

const (
	authModeNone authMode = iota
	authModeAPIKey
)

func doJSON(ctx context.Context, cfg runtimeConfig, mode authMode, method, path string, payload any, out any) error {
	body, err := doJSONRaw(ctx, cfg, mode, method, path, payload)
	if err != nil {
		return err
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(body, out); err != nil {
		return err
	}
	return nil
}

func doJSONRaw(ctx context.Context, cfg runtimeConfig, mode authMode, method, path string, payload any) ([]byte, error) {
	fullURL, err := joinBaseURL(cfg.BaseURL, path)
	if err != nil {
		return nil, err
	}

	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, fullURL, body)
	if err != nil {
		return nil, err
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if err := applyAuth(req, cfg, mode); err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		msg := strings.TrimSpace(string(data))
		if msg == "" {
			msg = resp.Status
		}
		return nil, fmt.Errorf("request failed: status=%d body=%s", resp.StatusCode, msg)
	}

	return data, nil
}

func applyAuth(req *http.Request, cfg runtimeConfig, mode authMode) error {
	switch mode {
	case authModeNone:
		return nil
	case authModeAPIKey:
		if strings.TrimSpace(cfg.APIKey) == "" {
			return errors.New("api key required")
		}
		req.Header.Set("X-ModelRelay-Api-Key", strings.TrimSpace(cfg.APIKey))
	default:
		return errors.New("invalid auth mode")
	}
	return nil
}

func joinBaseURL(baseURL, path string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return "", fmt.Errorf("invalid base url: %w", err)
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	if u.Path == "" {
		u.Path = "/"
	}
	// Separate path and query string to avoid encoding the query string
	pathPart := path
	queryPart := ""
	if idx := strings.Index(path, "?"); idx >= 0 {
		pathPart = path[:idx]
		queryPart = path[idx+1:]
	}
	u.Path = strings.TrimRight(u.Path, "/") + pathPart
	if queryPart != "" {
		u.RawQuery = queryPart
	}
	return u.String(), nil
}
