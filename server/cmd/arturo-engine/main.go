// Command arturo-engine is the CLI entry-point for the Arturo script engine.
//
// Usage:
//
//	arturo-engine validate <file.art>       Validate a script (JSON to stdout)
//	arturo-engine devices  --profiles <dir> List device profiles as JSON
//	arturo-engine run      <file.art>       Execute a script (requires Redis)
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/holla2040/arturo/internal/protocol"
	"github.com/holla2040/arturo/internal/script/executor"
	"github.com/holla2040/arturo/internal/script/lexer"
	"github.com/holla2040/arturo/internal/script/parser"
	"github.com/holla2040/arturo/internal/script/profile"
	"github.com/holla2040/arturo/internal/script/redisrouter"
	"github.com/holla2040/arturo/internal/script/result"
	"github.com/holla2040/arturo/internal/script/validate"
	"github.com/redis/go-redis/v9"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "validate":
		cmdValidate(os.Args[2:])
	case "devices":
		cmdDevices(os.Args[2:])
	case "run":
		cmdRun(os.Args[2:])
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		usage()
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "Usage:")
	fmt.Fprintln(os.Stderr, "  arturo-engine validate <file.art>                           Validate a script")
	fmt.Fprintln(os.Stderr, "  arturo-engine devices --profiles <dir>                      List device profiles")
	fmt.Fprintln(os.Stderr, "  arturo-engine run [--redis addr] [--station id] <file.art>  Execute a script")
}

// ---------------------------------------------------------------------------
// validate
// ---------------------------------------------------------------------------

func cmdValidate(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "validate requires a file path")
		os.Exit(1)
	}

	res, err := validate.ValidateFile(args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(res); err != nil {
		fmt.Fprintf(os.Stderr, "json encode: %v\n", err)
		os.Exit(1)
	}

	if !res.Valid {
		os.Exit(1)
	}
}

// ---------------------------------------------------------------------------
// devices
// ---------------------------------------------------------------------------

func cmdDevices(args []string) {
	dir := "profiles"
	for i := 0; i < len(args)-1; i++ {
		if args[i] == "--profiles" {
			dir = args[i+1]
		}
	}

	profiles, err := profile.LoadAllProfiles(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error loading profiles: %v\n", err)
		os.Exit(1)
	}

	introspection := profile.BuildIntrospection(profiles)
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(introspection); err != nil {
		fmt.Fprintf(os.Stderr, "json encode: %v\n", err)
		os.Exit(1)
	}
}

// ---------------------------------------------------------------------------
// run
// ---------------------------------------------------------------------------

func cmdRun(args []string) {
	// Parse flags: --redis <addr> --station <id> <file.art>
	redisAddr := "localhost:6379"
	station := "station-01"
	var scriptPath string

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--redis":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "--redis requires an address")
				os.Exit(1)
			}
			i++
			redisAddr = args[i]
		case "--station":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "--station requires an id")
				os.Exit(1)
			}
			i++
			station = args[i]
		default:
			scriptPath = args[i]
		}
	}

	if scriptPath == "" {
		fmt.Fprintln(os.Stderr, "run requires a file path")
		os.Exit(1)
	}

	source, err := os.ReadFile(scriptPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading file: %v\n", err)
		os.Exit(1)
	}

	// Lex.
	tokens, lexErrs := lexer.New(string(source)).Tokenize()
	if len(lexErrs) > 0 {
		var msgs []string
		for _, le := range lexErrs {
			msgs = append(msgs, fmt.Sprintf("line %d:%d: %s", le.Line, le.Column, le.Message))
		}
		fmt.Fprintf(os.Stderr, "lex errors:\n  %s\n", strings.Join(msgs, "\n  "))
		os.Exit(1)
	}

	// Parse.
	program, parseErrs := parser.New(tokens).Parse()
	if len(parseErrs) > 0 {
		var msgs []string
		for _, pe := range parseErrs {
			msgs = append(msgs, fmt.Sprintf("line %d:%d: %s", pe.Line, pe.Column, pe.Message))
		}
		fmt.Fprintf(os.Stderr, "parse errors:\n  %s\n", strings.Join(msgs, "\n  "))
		os.Exit(1)
	}

	// Connect to Redis and create router.
	rdb := redis.NewClient(&redis.Options{Addr: redisAddr})
	defer rdb.Close()

	engineSource := protocol.Source{
		Service:  "arturo-engine",
		Instance: "engine-01",
		Version:  "1.0.0",
	}
	router := redisrouter.New(rdb, engineSource, station)

	// Execute.
	collector := result.NewCollector(scriptPath)
	exec := executor.New(
		context.Background(),
		executor.WithCollector(collector),
		executor.WithRouter(router),
		executor.WithLogger(os.Stderr),
	)

	if err := exec.Execute(program); err != nil {
		fmt.Fprintf(os.Stderr, "execution error: %v\n", err)
	}

	// Output report.
	report := collector.Finalize()
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(report); err != nil {
		fmt.Fprintf(os.Stderr, "json encode: %v\n", err)
		os.Exit(1)
	}

	if report.Summary.Failed > 0 || report.Summary.Errors > 0 {
		os.Exit(1)
	}
}
