package controller

import "strings"

// quoteSqlIdentifier quotes a MySQL identifier, such as a database name, so it can be safely included in a statement
func quoteSqlIdentifier(identifier string) string {
	return "`" + strings.ReplaceAll(identifier, "`", "``") + "`"
}

// shellQuote quotes a value so it is passed to a shell command as a single literal argument
func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}
