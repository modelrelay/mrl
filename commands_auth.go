package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

// loginResponse is the subset of the /auth/login AuthResponse the CLI persists.
type loginResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

func newAuthCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Account authentication for project/tier admin",
		Long: `Account authentication.

Most mrl commands use a data-plane secret API key (mr_sk_*). Project and tier
administration (e.g. 'mrl tier create') instead require an account bearer token,
which 'mrl auth login' obtains and stores in the active profile.`,
	}
	cmd.AddCommand(newAuthLoginCmd(), newAuthLogoutCmd())
	return cmd
}

func newAuthLoginCmd() *cobra.Command {
	var email string
	var password string
	var passwordStdin bool
	var web bool
	var provider string

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Log in and store an account token (password or --web OAuth)",
		Long: `Obtain a ModelRelay account token and store it in the active profile,
for use by project/tier admin commands.

Two modes:

  # Browser OAuth (GitHub/Google accounts) — opens your browser, no password:
  mrl auth login --web                     # provider defaults to github
  mrl auth login --web --provider google

  # Email + password (password accounts) — via --password-stdin, MODELRELAY_PASSWORD,
  # or --password:
  printf '%s' "$PASS" | mrl auth login --email you@example.com --password-stdin`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := runtimeConfigFrom(cmd)
			if err != nil {
				return err
			}
			if web {
				return runWebLogin(cfg, strings.TrimSpace(provider))
			}
			email = strings.TrimSpace(email)
			if email == "" {
				return errors.New("--email is required (or use --web for browser OAuth)")
			}
			password, err = resolveLoginPassword(password, passwordStdin)
			if err != nil {
				return err
			}

			ctx, cancel := contextWithTimeout(cfg.Timeout)
			defer cancel()

			var resp loginResponse
			if err := doJSON(ctx, cfg, authModeNone, http.MethodPost, "/auth/login",
				map[string]any{"email": email, "password": password}, &resp); err != nil {
				return err
			}
			if strings.TrimSpace(resp.AccessToken) == "" {
				return errors.New("login succeeded but no access token was returned")
			}

			if err := persistAccountToken(cfg.Profile, resp.AccessToken, resp.RefreshToken); err != nil {
				return err
			}
			fmt.Printf("logged in as %s; account token saved to profile %s\n", email, cfg.Profile)
			return nil
		},
	}
	cmd.Flags().BoolVar(&web, "web", false, "Log in via the browser (OAuth) instead of a password")
	cmd.Flags().StringVar(&provider, "provider", "github", "OAuth provider for --web (e.g. github, google)")
	cmd.Flags().StringVar(&email, "email", "", "Account email (password login)")
	cmd.Flags().StringVar(&password, "password", "", "Account password (prefer --password-stdin)")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "Read the password from stdin")
	return cmd
}

func newAuthLogoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Clear the stored account token for the active profile",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := runtimeConfigFrom(cmd)
			if err != nil {
				return err
			}
			if err := persistAccountToken(cfg.Profile, "", ""); err != nil {
				return err
			}
			fmt.Printf("cleared account token for profile %s\n", cfg.Profile)
			return nil
		},
	}
}

// resolveLoginPassword reads the password from stdin, --password, or the
// MODELRELAY_PASSWORD environment variable, in that order of preference.
func resolveLoginPassword(passwordFlag string, passwordStdin bool) (string, error) {
	if passwordStdin {
		data, err := io.ReadAll(bufio.NewReader(os.Stdin))
		if err != nil {
			return "", fmt.Errorf("read password from stdin: %w", err)
		}
		pass := strings.TrimRight(string(data), "\r\n")
		if pass == "" {
			return "", errors.New("no password received on stdin")
		}
		return pass, nil
	}
	if strings.TrimSpace(passwordFlag) != "" {
		return passwordFlag, nil
	}
	if env := strings.TrimSpace(os.Getenv("MODELRELAY_PASSWORD")); env != "" {
		return env, nil
	}
	return "", errors.New("password required: use --password-stdin, --password, or MODELRELAY_PASSWORD")
}

// persistAccountToken writes the account token (and refresh token) into the named
// profile, leaving all other profile fields untouched.
func persistAccountToken(profileName, token, refreshToken string) error {
	fileCfg, err := loadCLIConfig()
	if err != nil {
		return err
	}
	if fileCfg.Profiles == nil {
		fileCfg.Profiles = map[string]cliProfile{}
	}
	profileCfg := profileFor(fileCfg, profileName)
	profileCfg.Token = token
	profileCfg.RefreshToken = refreshToken
	fileCfg.Profiles[profileName] = profileCfg
	return writeCLIConfig(fileCfg)
}
