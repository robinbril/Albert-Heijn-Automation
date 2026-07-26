package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/robinbril/Albert-Heijn-Automation/internal/ah"
)

func authCommands() []command {
	return []command{
		{
			name:    "login",
			summary: "Log in to Albert Heijn in your browser",
			auth:    authNoClient,
			run: func(ctx context.Context, _ *ah.Client, _ *flag.FlagSet) (any, error) {
				fmt.Fprintln(os.Stderr, "Opening your browser to log in to Albert Heijn...")
				client, err := ah.Login(ctx)
				if err != nil {
					return nil, err
				}
				return map[string]any{
					"ok":         true,
					"tokenStore": client.ConfigPath(),
				}, nil
			},
		},
		{
			name:    "logout",
			summary: "Delete the stored tokens",
			auth:    authNoClient,
			run: func(_ context.Context, _ *ah.Client, _ *flag.FlagSet) (any, error) {
				path, err := ah.ConfigPath()
				if err != nil {
					return nil, err
				}
				if err := os.Remove(path); err != nil {
					if os.IsNotExist(err) {
						return map[string]any{"ok": true, "removed": false}, nil
					}
					return nil, fmt.Errorf("remove %s: %w", path, err)
				}
				return map[string]any{"ok": true, "removed": true, "tokenStore": path}, nil
			},
		},
		{
			name:    "status",
			summary: "Show login state, token location and read-only mode",
			auth:    authNoClient,
			run: func(ctx context.Context, _ *ah.Client, _ *flag.FlagSet) (any, error) {
				path, err := ah.ConfigPath()
				if err != nil {
					return nil, err
				}

				status := map[string]any{
					"tokenStore":    path,
					"authenticated": false,
					"readOnly":      os.Getenv(ah.EnvReadOnly) != "",
				}

				client, err := ah.Authenticated()
				if err != nil {
					status["hint"] = "run: appie-cli login"
					return status, nil
				}
				status["authenticated"] = true
				status["readOnly"] = client.ReadOnly()

				// A stored token can still be stale; confirm it against the API.
				profile, err := client.GetMemberProfile(ctx, false)
				if err != nil {
					status["tokenValid"] = false
					status["tokenError"] = err.Error()
					return status, nil
				}
				status["tokenValid"] = true
				status["audiences"] = len(profile.Audiences)
				return status, nil
			},
		},
	}
}
