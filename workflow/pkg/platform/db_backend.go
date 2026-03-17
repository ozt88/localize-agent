package platform

import (
	"fmt"
	"strings"
)

const (
	DBBackendSQLite   = "sqlite"
	DBBackendPostgres = "postgres"
)

func NormalizeDBBackend(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", DBBackendSQLite:
		return DBBackendSQLite, nil
	case DBBackendPostgres, "postgresql":
		return DBBackendPostgres, nil
	default:
		return "", fmt.Errorf("invalid db backend: %s (expected sqlite or postgres)", raw)
	}
}
