package main

import (
	"fmt"
	"strings"
	"time"

	sdk "github.com/modelrelay/modelrelay/sdk/go"
)

func buildFSToolOptions(manifest *toolManifest) ([]sdk.LocalFSOption, error) {
	if manifest == nil || manifest.FS == nil {
		return nil, nil
	}
	fsCfg := manifest.FS
	opts := make([]sdk.LocalFSOption, 0, 6)
	if len(fsCfg.IgnoreDirs) > 0 {
		opts = append(opts, sdk.WithLocalFSIgnoreDirs(fsCfg.IgnoreDirs...))
	}
	if fsCfg.MaxReadBytes != nil && *fsCfg.MaxReadBytes > 0 {
		opts = append(opts, sdk.WithLocalFSMaxReadBytes(*fsCfg.MaxReadBytes))
	}
	if fsCfg.MaxListEntries != nil && *fsCfg.MaxListEntries > 0 {
		opts = append(opts, sdk.WithLocalFSMaxListEntries(*fsCfg.MaxListEntries))
	}
	if fsCfg.MaxSearchMatches != nil && *fsCfg.MaxSearchMatches > 0 {
		opts = append(opts, sdk.WithLocalFSMaxSearchMatches(*fsCfg.MaxSearchMatches))
	}
	if fsCfg.MaxSearchBytes != nil && *fsCfg.MaxSearchBytes > 0 {
		opts = append(opts, sdk.WithLocalFSMaxSearchBytes(*fsCfg.MaxSearchBytes))
	}
	if strings.TrimSpace(fsCfg.SearchTimeout) != "" {
		dur, err := time.ParseDuration(strings.TrimSpace(fsCfg.SearchTimeout))
		if err != nil {
			return nil, fmt.Errorf("invalid fs search_timeout %q: %w", fsCfg.SearchTimeout, err)
		}
		opts = append(opts, sdk.WithLocalFSSearchTimeout(dur))
	}
	return opts, nil
}
