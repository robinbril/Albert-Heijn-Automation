package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/robinbril/openclaw-skill-albert-heijn/internal/ah"
)

func debugCommands() []command {
	var stdin bool

	return []command{
		{
			name:    "graphql",
			args:    "<query>",
			summary: "Run a raw GraphQL query against the AH API",
			flags: func(fs *flag.FlagSet) {
				fs.BoolVar(&stdin, "stdin", false, "read the query from stdin")
			},
			run: func(ctx context.Context, c *ah.Client, fs *flag.FlagSet) (any, error) {
				query, err := readQuery(fs, stdin)
				if err != nil {
					return nil, err
				}
				var result json.RawMessage
				if err := c.API().DoGraphQL(ctx, query, nil, &result); err != nil {
					return nil, err
				}
				return result, nil
			},
		},
	}
}

// readQuery takes the GraphQL document from stdin or the positional argument.
func readQuery(fs *flag.FlagSet, stdin bool) (string, error) {
	if stdin {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", fmt.Errorf("read query from stdin: %w", err)
		}
		query := strings.TrimSpace(string(data))
		if query == "" {
			return "", fmt.Errorf("empty query on stdin")
		}
		return query, nil
	}
	return requireArg(fs, 0, "query")
}
