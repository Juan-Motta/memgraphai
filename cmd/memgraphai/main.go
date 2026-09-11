package main

import (
	"context"
	"fmt"
	"os"

	"memgraphai/internal/store/sqlite"
)

func main() {
	if len(os.Args) != 2 || os.Args[1] != "probe" {
		fmt.Fprintln(os.Stderr, "usage: memgraphai probe")
		os.Exit(2)
	}

	result, err := sqlite.Probe(context.Background(), "", "project")
	if err != nil {
		fmt.Fprintf(os.Stderr, "memgraphai: SQLite FTS5 probe failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("memgraphai: SQLite FTS5 probe succeeded (matches=%d, transaction=%s)\n", result.FTS5Matches, result.TransactionValue)
}
