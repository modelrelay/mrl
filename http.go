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
	authModeToken
	authModeAPIKey
	authModeTokenOrAPIKey
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
	case authModeToken:
		if strings.TrimSpace(cfg.Token) == "" {
			return errors.New("access token required")
		}
		req.Header.Set("Authorization", "Bearer "+normalizeBearer(cfg.Token))
	case authModeAPIKey:
		if strings.TrimSpace(cfg.APIKey) == "" {
			return errors.New("api key required")
		}
		req.Header.Set("X-ModelRelay-Api-Key", strings.TrimSpace(cfg.APIKey))
	case authModeTokenOrAPIKey:
		if strings.TrimSpace(cfg.Token) != "" {
			req.Header.Set("Authorization", "Bearer "+normalizeBearer(cfg.Token))
			return nil
		}
		if strings.TrimSpace(cfg.APIKey) != "" {
			req.Header.Set("X-ModelRelay-Api-Key", strings.TrimSpace(cfg.APIKey))
			return nil
		}
		return errors.New("auth required")
	default:
		return errors.New("invalid auth mode")
	}
	return nil
}

func normalizeBearer(token string) string {
	trimmed := strings.TrimSpace(token)
	lower := strings.ToLower(trimmed)
	if strings.HasPrefix(lower, "bearer ") {
		return strings.TrimSpace(trimmed[len("bearer "):])
	}
	return trimmed
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
	u.Path = strings.TrimRight(u.Path, "/") + path
	return u.String(), nil
}
