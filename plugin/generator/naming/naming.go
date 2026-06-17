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

// SnakeCase converts PascalCase to snake_case, keeping acronym runs intact:
// "BookAuthor" → "book_author", "AuthorID" → "author_id", "HTTPServer" →
// "http_server", "ISBN" → "isbn". A word boundary precedes an uppercase rune
// only when the previous rune is not uppercase (lower→Upper, e.g. "kA"), or
// when an uppercase run ends because the next rune is lowercase (the trailing
// capital starts a new word, e.g. the "S" in "HTTPServer").
func SnakeCase(s string) string {
	var b strings.Builder
	runes := []rune(s)
	for i, r := range runes {
		if i > 0 && r >= 'A' && r <= 'Z' {
			prevUpper := runes[i-1] >= 'A' && runes[i-1] <= 'Z'
			nextLower := i+1 < len(runes) && runes[i+1] >= 'a' && runes[i+1] <= 'z'
			if !prevUpper || nextLower {
				b.WriteByte('_')
			}
		}
		b.WriteRune(r)
	}
	return strings.ToLower(b.String())
}

// SnakePlural converts PascalCase to a snake_case table name and pluralizes the
// final word with English inflection rules: "BookAuthor" → "book_authors",
// "CancellationPolicy" → "cancellation_policies", "BufferSettings" →
// "buffer_settings" (already plural, no double-s), "Box" → "boxes". Used only
// when no authoritative plural is available (a nested child table, or a resource
// without google.api.resource.plural); annotated plurals win upstream.
func SnakePlural(s string) string { return Pluralize(SnakeCase(s)) }

// Pluralize returns the English plural of a lower_snake_case noun, inflecting
// only the final underscore-separated word. The rules cover the cases a schema
// actually hits without pulling in a full inflection library:
//   - ends in a sibilant ("s", "x", "z", "ch", "sh") → keep "s"/"x"… resolved
//     below; a word already ending in "s" is left unchanged so plural-form
//     message names ("settings", "constraints") don't become "settingss".
//   - consonant + "y" → "ies" ("policy" → "policies", "summary" → "summaries").
//   - otherwise → "s".
func Pluralize(snake string) string {
	if snake == "" {
		return snake
	}
	head, word := "", snake
	if i := strings.LastIndexByte(snake, '_'); i >= 0 {
		head, word = snake[:i+1], snake[i+1:]
	}
	return head + pluralizeWord(word)
}

// pluralizeWord pluralizes a single lowercase word.
func pluralizeWord(w string) string {
	switch {
	case w == "":
		return w
	case strings.HasSuffix(w, "s"):
		return w // already plural ("settings") or sibilant; avoid a double "s"
	case strings.HasSuffix(w, "x"), strings.HasSuffix(w, "z"),
		strings.HasSuffix(w, "ch"), strings.HasSuffix(w, "sh"):
		return w + "es"
	case strings.HasSuffix(w, "y") && len(w) >= 2 && !isVowel(w[len(w)-2]):
		return w[:len(w)-1] + "ies"
	default:
		return w + "s"
	}
}

// isVowel reports whether b is an ASCII vowel.
func isVowel(b byte) bool {
	switch b {
	case 'a', 'e', 'i', 'o', 'u':
		return true
	}
	return false
}

// GoPackage sanitizes a schema name into a valid idiomatic Go package name:
// lowercase, no underscores. "bookstore_v1" → "bookstorev1".
func GoPackage(s string) string {
	return strings.ToLower(strings.ReplaceAll(s, "_", ""))
}

// DatasourceName returns a valid Prisma datasource identifier for the database.
// The block name is just a label (nothing references it), so the database name
// is used directly — "bookstore_db" → "bookstore_db". A leading digit, illegal
// for a Prisma identifier, is prefixed with "db_"; other names pass through.
func DatasourceName(dbName string) string {
	if dbName == "" {
		return "db"
	}
	if c := dbName[0]; c >= '0' && c <= '9' {
		return "db_" + dbName
	}
	return dbName
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

// ScreamingSnake normalizes an identifier to SCREAMING_SNAKE_CASE: uppercase
// with `_` word separators. Already-SCREAMING_SNAKE input is preserved
// ("FOCUS_TIME" → "FOCUS_TIME"); camelCase/PascalCase boundaries become
// underscores ("focusTime" → "FOCUS_TIME"). Runs of underscores collapse to one.
// Used to keep generated enum values identical across the Prisma, GORM, and SQL
// targets (all read EnumValue.MapName).
func ScreamingSnake(s string) string {
	var b strings.Builder
	prevLower := false
	for _, r := range s {
		isUpper := r >= 'A' && r <= 'Z'
		if isUpper && prevLower {
			b.WriteByte('_')
		}
		b.WriteRune(r)
		prevLower = r >= 'a' && r <= 'z'
	}
	out := strings.ToUpper(b.String())
	for strings.Contains(out, "__") {
		out = strings.ReplaceAll(out, "__", "_")
	}
	return out
}

// SchemaDomain extracts the domain segment from a schema name for use as a
// disambiguating prefix on globally-namespaced Prisma model/enum names: the last
// underscore segment with any trailing API version dropped.
// "machanirobotics_app_productivity_calendar_v1" → "calendar";
// "store_apps_health_wellness_v1" → "wellness"; "public" → "public".
func SchemaDomain(schemaName string) string {
	parts := strings.Split(schemaName, "_")
	for len(parts) > 1 {
		last := parts[len(parts)-1]
		if len(last) >= 2 && last[0] == 'v' && last[1] >= '0' && last[1] <= '9' {
			parts = parts[:len(parts)-1]
			continue
		}
		break
	}
	if len(parts) == 0 {
		return schemaName
	}
	return parts[len(parts)-1]
}

// StripPackageVersion drops a trailing API-version segment from an underscore-
// joined schema name, flattening the version out while keeping the rest:
// "bookstore_v1" → "bookstore", "fleet_tracking_device_v1" →
// "fleet_tracking_device", "shop_v2alpha1" → "shop". A name with no trailing
// version segment ("calendar_app", "public") is returned unchanged, and a name
// that is only a version segment ("v1") is kept as-is rather than emptied.
// Opt-in via the protorm.yaml strip_version option.
func StripPackageVersion(schemaName string) string {
	parts := strings.Split(schemaName, "_")
	if len(parts) < 2 {
		return schemaName
	}
	if last := parts[len(parts)-1]; len(last) >= 2 && last[0] == 'v' && last[1] >= '0' && last[1] <= '9' {
		return strings.Join(parts[:len(parts)-1], "_")
	}
	return schemaName
}

// FKFieldBase returns the snake_case base used to derive a foreign-key column's
// language-level field name. When the column is a foreign key whose name doesn't
// already end in "_id", an "_id" suffix is appended ("resource" → "resource_id")
// so the bare name ("resource") stays free for the relation/association field and
// needn't take a numeric suffix ("resource2"). The database column name itself is
// unchanged — only the Prisma/Go field identifier differs (kept aligned to the DB
// column via @map / gorm:"column:..."). Non-FK columns are returned unchanged.
func FKFieldBase(colName string, isFK bool) string {
	if isFK && !strings.HasSuffix(colName, "_id") {
		return colName + "_id"
	}
	return colName
}

// StripIDSuffix drops a trailing "_id" so a FK column yields a relation stem:
// "location_id" → "location", "organizer_user_app_ref_id" →
// "organizer_user_app_ref". A bare "id" (or a name without the suffix) is
// returned unchanged.
func StripIDSuffix(col string) string {
	if s := strings.TrimSuffix(col, "_id"); s != "" && s != col {
		return s
	}
	return col
}

// SanitizeIdent makes s a valid Prisma/SQL identifier by prefixing `_` when it
// would otherwise begin with a non-letter (e.g. a digit-leading enum value
// "9S" → "_9S"). The empty string maps to "_".
func SanitizeIdent(s string) string {
	if s == "" {
		return "_"
	}
	c := s[0]
	if (c < 'A' || c > 'Z') && (c < 'a' || c > 'z') && c != '_' {
		return "_" + s
	}
	return s
}
