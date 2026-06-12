// Package naming holds every identifier-case rule shared by the backends:
// snake_case ↔ camelCase ↔ PascalCase conversion, Go acronym handling, Go
// package sanitizing, and Prisma datasource naming.
package naming

import "strings"

// goAcronyms maps lowercase tokens to their canonical Go uppercase forms per
// https://github.com/golang/go/wiki/CodeReviewComments#initialisms.
var goAcronyms = map[string]string{
	"id": "ID", "url": "URL", "http": "HTTP", "https": "HTTPS",
	"uuid": "UUID", "api": "API", "uri": "URI", "ip": "IP",
	"isbn": "ISBN", "sql": "SQL", "db": "DB", "json": "JSON",
	"xml": "XML", "html": "HTML", "ok": "OK", "uid": "UID",
}

// PascalGo converts snake_case to PascalCase following Go acronym conventions.
// "author_id" → "AuthorID", "isbn" → "ISBN", "display_name" → "DisplayName".
func PascalGo(s string) string {
	var b strings.Builder
	for _, part := range strings.Split(s, "_") {
		if upper, ok := goAcronyms[strings.ToLower(part)]; ok {
			b.WriteString(upper)
		} else if len(part) > 0 {
			b.WriteString(strings.ToUpper(part[:1]) + part[1:])
		}
	}
	return b.String()
}

// Camel converts snake_case to camelCase with Prisma acronym conventions.
// Only "id"/"ids" segments after the first are uppercased.
// "author_id" → "authorID", "display_name" → "displayName", "name" → "name".
func Camel(s string) string {
	parts := strings.Split(s, "_")
	if len(parts) == 0 {
		return s
	}
	var b strings.Builder
	b.WriteString(parts[0]) // first segment stays lowercase as-is
	for _, p := range parts[1:] {
		switch strings.ToLower(p) {
		case "id":
			b.WriteString("ID")
		case "ids":
			b.WriteString("IDs")
		default:
			if len(p) > 0 {
				b.WriteString(strings.ToUpper(p[:1]) + p[1:])
			}
		}
	}
	return b.String()
}

// CamelFirst lowercases the first rune of a PascalCase model name.
// "Author" → "author", "BookAuthor" → "bookAuthor".
func CamelFirst(s string) string {
	if len(s) == 0 {
		return s
	}
	return strings.ToLower(s[:1]) + s[1:]
}

// SnakeCase converts PascalCase to snake_case. "BookAuthor" → "book_author".
func SnakeCase(s string) string {
	var b strings.Builder
	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			b.WriteByte('_')
		}
		b.WriteRune(r)
	}
	return strings.ToLower(b.String())
}

// SnakePlural converts PascalCase to snake_case + "s". "BookAuthor" → "book_authors".
func SnakePlural(s string) string { return SnakeCase(s) + "s" }

// GoPackage sanitizes a schema name into a valid idiomatic Go package name:
// lowercase, no underscores. "bookstore_v1" → "bookstorev1".
func GoPackage(s string) string {
	return strings.ToLower(strings.ReplaceAll(s, "_", ""))
}

// DatasourceName builds a Prisma datasource identifier from the database name
// and provider suffix, matching the hand-written convention:
// ("bookstore_db", "pgsql") → "bookstoredbpgsql".
func DatasourceName(dbName, suffix string) string {
	return strings.ReplaceAll(dbName, "_", "") + suffix
}

// EnumValueName strips the enum-name prefix from a proto enum value, per the
// proto style guide ("GENRE_FICTION" with enum "Genre" → "FICTION").
func EnumValueName(enumName, valueName string) string {
	prefix := strings.ToUpper(SnakeCase(enumName)) + "_"
	if trimmed := strings.TrimPrefix(valueName, prefix); trimmed != "" {
		return trimmed
	}
	return valueName
}
