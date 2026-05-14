package sql

import (
	"fmt"
	"strings"
)

// BuildInClausePg builds an `IN ($1, $2, ...)` clause for Postgres-style
// numbered placeholders. The placeholders always start at $1; if the caller
// composes this clause with other parameters, those other parameters must
// come after the IN-clause args in the final argument slice.
func BuildInClausePg(column string, num int) string {
	if num == 0 {
		return fmt.Sprintf("%s IN ()", column)
	}

	placeholders := make([]string, num)
	for i := 0; i < num; i++ {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
	}
	return fmt.Sprintf("%s IN (%s)", column, strings.Join(placeholders, ","))
}
