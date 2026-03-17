package main

import (
	"flag"
	"fmt"
	"os"

	"localize-agent/workflow/internal/dbmigration"
)

func main() {
	var sourceSQLite string
	var destDSN string
	var truncateDst bool

	flag.StringVar(&sourceSQLite, "source-sqlite", "", "source SQLite checkpoint DB path")
	flag.StringVar(&destDSN, "dest-dsn", "", "destination PostgreSQL DSN")
	flag.BoolVar(&truncateDst, "truncate-dst", false, "truncate destination PostgreSQL tables before import")
	flag.Parse()

	if sourceSQLite == "" || destDSN == "" {
		fmt.Fprintln(os.Stderr, "usage: go-migrate-checkpoint --source-sqlite <path> --dest-dsn <dsn> [--truncate-dst]")
		os.Exit(2)
	}

	summary, err := dbmigration.MigrateSQLiteCheckpointToPostgres(sourceSQLite, destDSN, truncateDst)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println(summary.String())
}
