package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"memgraphai/internal/adapter/cli"
	mcpadapter "memgraphai/internal/adapter/mcp"
	"memgraphai/internal/app"
	"memgraphai/internal/store/sqlite"
)

func main() {
	if len(os.Args) == 2 && os.Args[1] == "probe" {
		runProbe()
		return
	}
	if len(os.Args) >= 2 && os.Args[1] == "mcp" {
		explicit := ""
		if len(os.Args) == 4 && os.Args[2] == "--library" {
			explicit = os.Args[3]
		} else if len(os.Args) != 2 {
			fmt.Fprintln(os.Stderr, "usage: memgraphai mcp [--library PATH]")
			os.Exit(2)
		}
		runMCP(explicit)
		return
	}
	if len(os.Args) < 2 || (os.Args[1] != "project" && os.Args[1] != "--library" && os.Args[1] != "--json" && os.Args[1] != "--operation-id") {
		fmt.Fprintln(os.Stderr, "usage: memgraphai probe")
		os.Exit(2)
	}
	os.Exit(cli.Run(context.Background(), os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func runProbe() {
	result, err := sqlite.Probe(context.Background(), "", "project")
	if err != nil {
		fmt.Fprintf(os.Stderr, "memgraphai: SQLite FTS5 probe failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("memgraphai: SQLite FTS5 probe succeeded (matches=%d, transaction=%s)\n", result.FTS5Matches, result.TransactionValue)
}

func runMCP(explicit string) {
	root, err := cli.ResolveLibrary(explicit)
	if err != nil || os.MkdirAll(root, 0o755) != nil {
		fmt.Fprintln(os.Stderr, "memgraphai: library open failed")
		os.Exit(1)
	}
	store, err := sqlite.Open(context.Background(), filepath.Join(root, "database.sqlite"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "memgraphai: library open failed")
		os.Exit(1)
	}
	defer store.Close()
	if err := mcpadapter.RunStdio(context.Background(), app.ProjectService{Store: store}, app.Service{Recorder: store}); err != nil {
		fmt.Fprintln(os.Stderr, "memgraphai: mcp stopped")
		os.Exit(1)
	}
}
