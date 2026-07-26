// Command appie-cli is a command line client for the Albert Heijn mobile API.
//
// Every command prints JSON on stdout and diagnostics on stderr, so the output
// can be piped straight into an agent or into jq. Exit status is 0 on success
// and 1 on failure.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"sort"
	"strings"

	"github.com/robinbril/Albert-Heijn-Automation/internal/ah"
)

// authMode declares what a command needs from the AH API.
type authMode int

const (
	// authNone works without any token; the client fetches an anonymous one.
	authNone authMode = iota
	// authRequired needs a logged-in member.
	authRequired
	// authNoClient handles its own client, for login and logout.
	authNoClient
)

// command is one CLI verb.
type command struct {
	name    string
	args    string
	summary string
	auth    authMode
	run     func(ctx context.Context, c *ah.Client, fs *flag.FlagSet) (any, error)
	// flags registers command specific flags before parsing.
	flags func(fs *flag.FlagSet)
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	name := os.Args[1]
	if name == "-h" || name == "--help" || name == "help" {
		usage()
		return
	}

	cmd, ok := lookup(name)
	if !ok {
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", name)
		usage()
		os.Exit(1)
	}

	fs := flag.NewFlagSet(cmd.name, flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: appie-cli %s %s\n\n%s\n", cmd.name, cmd.args, cmd.summary)
		fs.PrintDefaults()
	}
	if cmd.flags != nil {
		cmd.flags(fs)
	}
	if err := fs.Parse(permute(fs, os.Args[2:])); err != nil {
		os.Exit(1)
	}

	client, err := clientFor(ctx, cmd.auth)
	if err != nil {
		fail(err)
	}

	result, err := cmd.run(ctx, client, fs)
	if err != nil {
		fail(err)
	}
	if result != nil {
		emit(result)
	}
}

// clientFor builds the client a command needs.
func clientFor(ctx context.Context, mode authMode) (*ah.Client, error) {
	switch mode {
	case authNoClient:
		return nil, nil
	case authRequired:
		return ah.Authenticated()
	default:
		return ah.Anonymous(ctx)
	}
}

func lookup(name string) (command, bool) {
	for _, cmd := range commands() {
		if cmd.name == name {
			return cmd, true
		}
	}
	return command{}, false
}

// commands returns every CLI verb, grouped by area in the order they are shown.
func commands() []command {
	var all []command
	all = append(all, authCommands()...)
	all = append(all, catalogCommands()...)
	all = append(all, recipeCommands()...)
	all = append(all, listCommands()...)
	all = append(all, orderCommands()...)
	all = append(all, historyCommands()...)
	all = append(all, debugCommands()...)
	return all
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: appie-cli <command> [flags]")
	fmt.Fprintln(os.Stderr, "\nEvery command prints JSON on stdout.")
	fmt.Fprintln(os.Stderr, "\nCommands:")

	cmds := commands()
	width := 0
	for _, cmd := range cmds {
		if n := len(cmd.name + " " + cmd.args); n > width {
			width = n
		}
	}
	for _, cmd := range cmds {
		invocation := strings.TrimSpace(cmd.name + " " + cmd.args)
		fmt.Fprintf(os.Stderr, "  %-*s  %s\n", width, invocation, cmd.summary)
	}

	fmt.Fprintln(os.Stderr, "\nEnvironment:")
	fmt.Fprintf(os.Stderr, "  %-*s  %s\n", width, ah.EnvConfig, "path to the token file")
	fmt.Fprintf(os.Stderr, "  %-*s  %s\n", width, ah.EnvReadOnly, "set to 1 to block every write")
}

// emit writes a result as indented JSON.
func emit(v any) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		fail(fmt.Errorf("encode result: %w", err))
	}
}

// fail prints a JSON error object on stderr and exits non-zero. Encoding the
// message with the JSON encoder keeps the output parseable no matter which
// quotes or backslashes the error contains.
func fail(err error) {
	payload := map[string]string{"error": err.Error()}
	if errors.Is(err, ah.ErrNotAuthenticated) {
		payload["hint"] = "run: appie-cli login"
	}
	if errors.Is(err, ah.ErrReadOnly) {
		payload["hint"] = "unset " + ah.EnvReadOnly + " to allow writes"
	}

	enc := json.NewEncoder(os.Stderr)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(payload)
	os.Exit(1)
}

// boolFlag matches flags that take no value, as flag.boolValue does.
type boolFlag interface{ IsBoolFlag() bool }

// permute moves flags ahead of positional arguments. Go's flag package stops
// parsing at the first non-flag argument, which silently ignored everything in
// `appie-cli recipes pasta -limit 3`. Reordering makes the two forms behave the
// same, which is what anyone typing the command expects.
func permute(fs *flag.FlagSet, args []string) []string {
	flags := make([]string, 0, len(args))
	positional := make([]string, 0, len(args))

	for i := 0; i < len(args); i++ {
		current := args[i]

		if current == "--" {
			positional = append(positional, args[i+1:]...)
			break
		}
		if len(current) < 2 || current[0] != '-' {
			positional = append(positional, current)
			continue
		}

		flags = append(flags, current)
		name := strings.TrimLeft(current, "-")
		if strings.Contains(name, "=") {
			continue
		}
		// A non-boolean flag consumes the next argument as its value.
		if f := fs.Lookup(name); f != nil && !isBool(f) && i+1 < len(args) {
			i++
			flags = append(flags, args[i])
		}
	}

	return append(flags, positional...)
}

func isBool(f *flag.Flag) bool {
	b, ok := f.Value.(boolFlag)
	return ok && b.IsBoolFlag()
}

// arg returns the nth positional argument, or "" when absent.
func arg(fs *flag.FlagSet, n int) string {
	if fs.NArg() <= n {
		return ""
	}
	return fs.Arg(n)
}

// requireArg returns the nth positional argument or an error naming it.
func requireArg(fs *flag.FlagSet, n int, name string) (string, error) {
	value := strings.TrimSpace(arg(fs, n))
	if value == "" {
		return "", fmt.Errorf("missing required argument: %s", name)
	}
	return value, nil
}

// sortedKeys returns map keys in a stable order for deterministic output.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
