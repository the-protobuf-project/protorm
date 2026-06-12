// Package types is the single home for every type table protorm uses:
// proto → PostgreSQL inference, and the canonical-PostgreSQL → Go / Prisma /
// MongoDB-Prisma projections each backend renders.
//
// The IR always stores the canonical PostgreSQL type. Backends project it:
//
//	proto field ──infer──▶ canonical PG type ──project──▶ Go | Prisma | Mongo
package types

import "fmt"

// Provider identifies the database backend a datasource targets.
// Parsed from (protorm.v1.datasource).provider.
type Provider string

const (
	Postgres Provider = "postgres"
	MongoDB  Provider = "mongodb"
)

// ParseProvider normalizes a datasource provider string. Empty means Postgres.
func ParseProvider(s string) (Provider, error) {
	switch s {
	case "", "postgres", "postgresql":
		return Postgres, nil
	case "mongodb", "mongo":
		return MongoDB, nil
	default:
		return "", fmt.Errorf("unknown datasource provider %q (want postgres or mongodb)", s)
	}
}

// PrismaProvider is the provider string written into a Prisma datasource block.
func (p Provider) PrismaProvider() string {
	if p == MongoDB {
		return "mongodb"
	}
	return "postgresql"
}

// FragmentExt is the sub-extension used in generated fragment file names:
// <domain>.postgres.prisma or <domain>.mongodb.prisma.
func (p Provider) FragmentExt() string { return string(p) }
