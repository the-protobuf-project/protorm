package generator

import "testing"

func TestDeStutteredName(t *testing.T) {
	cases := []struct {
		name, schema string
		taken        []string
		want         string
	}{
		// Eponymous tables: stripping leaves nothing → generic word. "resource"
		// is preferred; the resource schema falls back to "entity" (it stutters).
		{"bookings", "booking", []string{"bookings"}, "resource"},
		{"schedules", "schedule", []string{"schedules"}, "resource"},
		{"organisations", "organisation", []string{"organisations"}, "resource"},
		{"promo_codes", "promocode", []string{"promo_codes"}, "resource"},
		{"resources", "resource", []string{"resources"}, "entity"},

		// Prefixed tables: drop the leading schema word, keep the remainder.
		{"schedule_exceptions", "schedule", []string{"schedule_exceptions"}, "exceptions"},
		{"promo_code_applicable_resources", "promocode", []string{"promo_code_applicable_resources"}, "applicable_resources"},

		// Prefixed table whose remainder collides with an existing table → _link.
		{"resource_offerings", "resource", []string{"resource_offerings", "offerings"}, "offerings_link"},

		// No stutter: left unchanged.
		{"contacts", "booking", []string{"contacts"}, "contacts"},
		{"users", "identity", []string{"users"}, "users"},
		{"books", "bookstore_v1", []string{"books"}, "books"},
	}
	for _, c := range cases {
		taken := map[string]bool{}
		for _, n := range c.taken {
			taken[n] = true
		}
		if got := deStutteredName(c.name, c.schema, taken); got != c.want {
			t.Errorf("deStutteredName(%q, %q) = %q, want %q", c.name, c.schema, got, c.want)
		}
	}
}
