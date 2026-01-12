package main

import (
	"errors"
	"fmt"

	sdk "github.com/modelrelay/modelrelay/sdk/go"
	"github.com/modelrelay/modelrelay/sdk/go/llm"
)

type fsReadFileArgs struct {
	Path     string `json:"path" description:"workspace-relative path"`
	MaxBytes int64  `json:"max_bytes,omitempty" description:"maximum bytes to return"`
}

type fsListFilesArgs struct {
	Path       string `json:"path,omitempty" description:"workspace-relative directory"`
	MaxEntries int64  `json:"max_entries,omitempty" description:"maximum entries to return"`
}

type fsSearchArgs struct {
	Query      string `json:"query" description:"search query"`
	Path       string `json:"path,omitempty" description:"workspace-relative directory"`
	MaxMatches int64  `json:"max_matches,omitempty" description:"maximum matches to return"`
}

type fsEditArgs struct {
	Path       string `json:"path" description:"workspace-relative path"`
	OldString  string `json:"old_string" description:"exact string to find"`
	NewString  string `json:"new_string" description:"replacement string"`
	ReplaceAll bool   `json:"replace_all,omitempty" description:"replace all occurrences"`
}

func fsToolDefinitions() []llm.Tool {
	return []llm.Tool{
		sdk.MustFunctionToolFromType[fsReadFileArgs](sdk.ToolNameFSReadFile, "Read a file from the local workspace"),
		sdk.MustFunctionToolFromType[fsListFilesArgs](sdk.ToolNameFSListFiles, "List files in the local workspace"),
		sdk.MustFunctionToolFromType[fsSearchArgs](sdk.ToolNameFSSearch, "Search within the local workspace"),
		sdk.MustFunctionToolFromType[fsEditArgs](sdk.ToolNameFSEdit, "Edit a file in the local workspace"),
	}
}

func appendToolDefs(defs []llm.Tool, seen map[sdk.ToolName]struct{}, add ...llm.Tool) ([]llm.Tool, error) {
	if seen == nil {
		return nil, errors.New("tool name registry required")
	}
	for _, tool := range add {
		name := toolNameForDefinition(tool)
		if name == "" {
			return nil, errors.New("tool definition missing name")
		}
		if _, exists := seen[name]; exists {
			return nil, fmt.Errorf("duplicate tool name %q", name)
		}
		seen[name] = struct{}{}
		defs = append(defs, tool)
	}
	return defs, nil
}

func toolNameForDefinition(tool llm.Tool) sdk.ToolName {
	if tool.Function == nil {
		return ""
	}
	return tool.Function.Name
}
