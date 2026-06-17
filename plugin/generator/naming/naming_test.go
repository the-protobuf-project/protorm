package naming

import "testing"

func TestPascalGo(t *testing.T) {
	cases := map[string]string{
		"author_id":    "AuthorID",
		"isbn":         "ISBN",
		"display_name": "DisplayName",
		"api_url":      "APIURL",
		"uuid":         "UUID",
		"pololu_12":    "Pololu12",
		"name":         "Name",
	}
	for in, want := range cases {
		if got := PascalGo(in); got != want {
			t.Errorf("PascalGo(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCamel(t *testing.T) {
	cases := map[string]string{
		"author_id":      "authorID",
		"display_name":   "displayName",
		"published_year": "publishedYear",
		"tag_ids":        "tagIDs",
		"name":           "name",
	}
	for in, want := range cases {
		if got := Camel(in); got != want {
			t.Errorf("Camel(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCamelFirst(t *testing.T) {
	cases := map[string]string{"Author": "author", "BookAuthor": "bookAuthor", "": ""}
	for in, want := range cases {
		if got := CamelFirst(in); got != want {
			t.Errorf("CamelFirst(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSnakeCase(t *testing.T) {
	cases := map[string]string{
		"Book":        "book",
		"BookAuthor":  "book_author",
		"AuthorID":    "author_id",   // trailing acronym
		"HTTPServer":  "http_server", // leading acronym run
		"ISBN":        "isbn",        // all-caps acronym
		"DisplayName": "display_name",
		"shelves":     "shelves", // already lower (google.api plural)
	}
	for in, want := range cases {
		if got := SnakeCase(in); got != want {
			t.Errorf("SnakeCase(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSnakePlural(t *testing.T) {
	cases := map[string]string{
		"Book":       "books",
		"BookAuthor": "book_authors",
		// Consonant + "y" → "ies".
		"CancellationPolicy": "cancellation_policies",
		"MembershipSummary":  "membership_summaries",
		// Already-plural words ending in "s" are left alone (no double "s").
		"BufferSettings":  "buffer_settings",
		"StayConstraints": "stay_constraints",
		// Sibilant endings take "es".
		"Box":  "boxes",
		"Dish": "dishes",
		// Vowel + "y" keeps the "y".
		"Gateway": "gateways",
		// Known limitation: "f" → "ves" is not handled; irregular plurals still
		// need a table-name override.
		"Shelf": "shelfs",
	}
	for in, want := range cases {
		if got := SnakePlural(in); got != want {
			t.Errorf("SnakePlural(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestGoPackage(t *testing.T) {
	cases := map[string]string{"bookstore_v1": "bookstorev1", "inventory": "inventory"}
	for in, want := range cases {
		if got := GoPackage(in); got != want {
			t.Errorf("GoPackage(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDatasourceName(t *testing.T) {
	cases := map[string]string{
		"bookstore_db": "bookstore_db", // valid identifier passes through
		"2fa":          "db_2fa",       // leading digit is illegal in Prisma → prefixed
		"":             "db",           // empty falls back to a generic label
	}
	for in, want := range cases {
		if got := DatasourceName(in); got != want {
			t.Errorf("DatasourceName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestEnumValueName(t *testing.T) {
	cases := []struct{ enum, value, want string }{
		{"Genre", "GENRE_FICTION", "FICTION"},
		{"Genre", "GENRE_NON_FICTION", "NON_FICTION"},
		{"Genre", "GENRE_UNSPECIFIED", "UNSPECIFIED"},
		{"Genre", "FICTION", "FICTION"},          // no prefix present
		{"ServoBoard", "SERVO_BOARD_CAN", "CAN"}, // multi-word enum name
	}
	for _, c := range cases {
		if got := EnumValueName(c.enum, c.value); got != c.want {
			t.Errorf("EnumValueName(%q, %q) = %q, want %q", c.enum, c.value, got, c.want)
		}
	}
}

func TestScreamingSnake(t *testing.T) {
	cases := []struct{ in, want string }{
		{"FICTION", "FICTION"},
		{"FOCUS_TIME", "FOCUS_TIME"},
		{"focusTime", "FOCUS_TIME"},
		{"FocusTime", "FOCUS_TIME"},
		{"9S", "9S"},
		{"out_of_office", "OUT_OF_OFFICE"},
	}
	for _, c := range cases {
		if got := ScreamingSnake(c.in); got != c.want {
			t.Errorf("ScreamingSnake(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestStripPackageVersion(t *testing.T) {
	cases := map[string]string{
		"bookstore_v1":             "bookstore",
		"fleet_tracking_device_v1": "fleet_tracking_device",
		"shop_v2alpha1":            "shop",
		"calendar_app":             "calendar_app", // no trailing version
		"public":                   "public",       // single segment
		"v1":                       "v1",           // version-only: keep, don't empty
	}
	for in, want := range cases {
		if got := StripPackageVersion(in); got != want {
			t.Errorf("StripPackageVersion(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSanitizeIdent(t *testing.T) {
	cases := []struct{ in, want string }{
		{"FICTION", "FICTION"},
		{"9S", "_9S"},
		{"_9S", "_9S"},
		{"", "_"},
	}
	for _, c := range cases {
		if got := SanitizeIdent(c.in); got != c.want {
			t.Errorf("SanitizeIdent(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
