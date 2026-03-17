package platform

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

func OpenTranslationCheckpointDB(backend string, path string, dsn string) (*sql.DB, error) {
	normalizedBackend, err := NormalizeDBBackend(backend)
	if err != nil {
		return nil, err
	}
	switch normalizedBackend {
	case DBBackendSQLite:
		return openSQLite(path, []string{"PRAGMA journal_mode=WAL", "PRAGMA synchronous=FULL", "PRAGMA foreign_keys=ON"})
	case DBBackendPostgres:
		if strings.TrimSpace(dsn) == "" {
			return nil, fmt.Errorf("postgres dsn required")
		}
		return openPostgres(dsn)
	default:
		return nil, fmt.Errorf("unsupported checkpoint backend: %s", normalizedBackend)
	}
}

func RebindSQL(backend string, query string) string {
	if backend != DBBackendPostgres {
		return query
	}
	var out strings.Builder
	out.Grow(len(query) + 8)
	argIndex := 1
	for i := 0; i < len(query); i++ {
		if query[i] == '?' {
			out.WriteString(fmt.Sprintf("$%d", argIndex))
			argIndex++
			continue
		}
		out.WriteByte(query[i])
	}
	return out.String()
}

func NormalizeSQLValue(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case []byte:
		return string(x)
	case time.Time:
		return x.UTC().Format(time.RFC3339)
	default:
		return fmt.Sprint(x)
	}
}
