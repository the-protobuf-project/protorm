# protorm

> [!CAUTION]
> Early development — the API and generated output may change between versions.

**protorm** is a [protoc](https://protobuf.dev/) plugin that turns your Protobuf
service definitions into production-grade database schemas. Annotate your
messages with the [Google AIP](https://google.aip.dev/) standards you already
use (`google.api.resource`, `field_behavior`, `resource_reference`) and protorm
infers tables, columns, primary keys, foreign keys, and relations — then emits
them for four backends from one source of truth.

| Target | Output | Notes |
| --- | --- | --- |
| **prisma** | A complete, runnable Prisma 7 project | multi-file schema, `package.json`, `tsconfig.json`, config, `.env.example` |
| **gorm** | Go structs with GORM tags | one package per schema, pointer types for nullables, relation fields |
| **sql** | PostgreSQL DDL | `CREATE SCHEMA/TYPE/TABLE`, FK constraints, indexes |
| **csv** | Flat schema manifest | one row per column — feed to doc tooling or document stores |

Postgres and MongoDB providers are both supported.

---

## How it works

protorm reads ~80% of the schema straight from AIP annotations, and the
remaining 20% (anything AIP can't express) from `protorm.v1.*` options.

| Source annotation | Inferred output |
| --- | --- |
| `google.api.resource` on a message | a table; schema + name from `type` / `plural` |
| `field_behavior = IDENTIFIER` | `PRIMARY KEY NOT NULL` |
| `field_behavior = REQUIRED` | `NOT NULL` (nullable otherwise → pointer/`?` types) |
| `resource_reference` on a field | a `FOREIGN KEY`, resolved to the referenced PK |
| proto scalar / well-known type | the column's SQL type (see [Type mapping](#type-mapping)) |

Everything is built into one intermediate representation, then each target
renders it independently — so all four outputs always agree.

---

## Install

```bash
# Homebrew
brew install oh-tarnished/tap/protoc-gen-protorm

# or go install
go install github.com/oh-tarnished/protorm/plugin/cmd/protoc-gen-protorm@latest
```

The plugin must be on your `PATH` so `protoc`/`buf` can find it.

You'll also need the option definitions on your import path. With
[buf](https://buf.build), add the module to your `buf.yaml` `deps`:

```yaml
deps:
  - buf.build/oh-tarnished/protorm
```

then `import "protorm/v1/annotations.proto";` in your protos.

---

## Quick start

**1. Annotate a proto.**

```proto
syntax = "proto3";
package bookstore.v1;

import "google/api/field_behavior.proto";
import "google/api/resource.proto";
import "protorm/v1/annotations.proto";

option (protorm.v1.datasource) = {
  database: "bookstore_db"
  provider: "postgres"
};

message Author {
  option (google.api.resource) = {
    type: "bookstore.v1/Author"
    pattern: "authors/{author}"
    singular: "author"
    plural: "authors"
  };
  // Use a generated ULID primary key + created_at/updated_at columns.
  option (protorm.v1.table) = { id: ID_STRATEGY_ULID, timestamps: true };

  // IDENTIFIER → the AIP resource name; becomes a UNIQUE lookup column.
  string name = 1 [(google.api.field_behavior) = IDENTIFIER];

  // REQUIRED → NOT NULL; string defaults to VARCHAR(255).
  string display_name = 2 [(google.api.field_behavior) = REQUIRED];

  // Override the default for a free-form column.
  string bio = 3 [(protorm.v1.col) = { type: "TEXT" }];
}
```

**2. Add the plugin to `buf.gen.yaml`.**

```yaml
version: v2
plugins:
  - local: protoc-gen-protorm
    out: generated/prisma
    opt: [target=prisma]   # prisma | gorm | sql | csv
```

**3. Generate.**

```bash
buf generate
```

> [!NOTE]
> protorm doesn't generate Go message stubs, so protoc/buf needs a Go import
> path for each file. If your protos don't set `option go_package`, supply it
> per file in `opt:` with an `M` mapping, e.g.
> `Mbookstore/v1/bookstore.proto=example.com/gen/bookstore/v1`.

### What comes out

The `Author` message above produces, across targets — **Prisma:**

```prisma
model Author {
  id          String   @id @default(ulid()) @map("id")
  name        String   @unique @map("name")
  displayName String   @map("display_name")
  bio         String?  @map("bio")
  createdAt   DateTime @default(now()) @map("created_at")
  updatedAt   DateTime @updatedAt @map("updated_at")
  books       Book[]

  @@map("authors")
  @@schema("bookstore_v1")
}
```

**GORM:**

```go
type Author struct {
  ID          string    `gorm:"column:id;primaryKey;not null"`
  Name        string    `gorm:"column:name;not null;uniqueIndex"`
  DisplayName string    `gorm:"column:display_name;not null"`
  Bio         *string   `gorm:"column:bio"`
  CreatedAt   time.Time `gorm:"column:created_at;autoCreateTime"`
  UpdatedAt   time.Time `gorm:"column:updated_at;autoUpdateTime"`
  Books       []Book    `gorm:"foreignKey:AuthorID"`
}

func (*Author) TableName() string { return "bookstore_v1.authors" }
```

(`///` doc comments and `json`/`validate` tags are emitted too — trimmed here for space.)

---

## Output layout

Files that declare the **same datasource name merge into one database**, so a
multi-file proto package becomes a single schema tree. Each target lays its
output out to match:

```text
generated/prisma/bookstore_db/
├── schema.prisma                          # datasource + generator blocks
├── bookstore_db.config.ts                 # Prisma 7 config (URL via env)
├── package.json, tsconfig.json            # runnable project scaffold
├── .env.example, .gitignore, README.md
├── bookstore_v1/bookstore.postgres.prisma # models & enums, one file per source proto
└── inventory/inventory.postgres.prisma    # (a second file, merged datasource)

generated/gorm/bookstore_db/bookstorev1/models.go   # package = folder name
generated/sql/bookstore_db/bookstore_v1.postgres.sql
generated/csv/bookstore_db/schema.csv
```

The Prisma output is a project you can run immediately:

```bash
cd generated/prisma/bookstore_db
npm install
cp .env.example .env        # then set BOOKSTORE_DB_DATABASE_URL
npm run prisma:generate
```

---

## Options reference

All options live in `protorm/v1/annotations.proto`.

### `(protorm.v1.datasource)` — file level

| Field | Description |
| --- | --- |
| `database` | Database name. Files sharing a name merge into one tree. Defaults to the last proto package segment. |
| `schema` | Override the schema namespace for every table in the file. |
| `url` | Connection URL (documented in config/DDL; Prisma reads it from `.env`). |
| `provider` | `postgres` (default) or `mongodb`. |

### `(protorm.v1.table)` — message level

| Field | Description |
| --- | --- |
| `table` | Explicit table name. Defaults to the snake_case plural of the resource. |
| `skip` | Exclude the message from all output. |
| `indexes` | Composite indexes: `{ columns: [...], unique: bool, index_name: "..." }`. |
| `id` | `ID_STRATEGY_ULID` / `ID_STRATEGY_UUID` — synthesize a generated `id` PK and demote the `IDENTIFIER` field to `UNIQUE`. |
| `timestamps` | Add `created_at` / `updated_at` (`@updatedAt` / GORM `autoUpdateTime`). |

### `(protorm.v1.col)` — field level

| Field | Description |
| --- | --- |
| `column` | Explicit column name (defaults to the proto field name). |
| `type` | Explicit SQL type (escape hatch; prefer the sizing options below). |
| `max_length` | `VARCHAR(n)` instead of the `VARCHAR(255)` default — provider-neutral. |
| `precision` / `scale` | `NUMERIC(p, s)`. |
| `default_value` | SQL default expression, written verbatim. |
| `unique`, `index` | Single-column constraint / index. |
| `skip` | Field exists in the proto contract but not the database. |
| `on_delete` / `on_update` | FK referential action (`CASCADE`, `SET_NULL`, …) for a `resource_reference` field. |

---

## Type mapping

The IR stores a canonical PostgreSQL type, which each backend projects to its
own type system. Highlights:

| Proto | PostgreSQL | Prisma | Go |
| --- | --- | --- | --- |
| `string` | `VARCHAR(255)` | `String` | `string` |
| `int32` | `INTEGER` | `Int` | `int32` |
| `int64` | `BIGINT` | `BigInt` | `int64` |
| `uint64` | `NUMERIC(20,0)` | `Decimal` | `string` |
| `bool` | `BOOLEAN` | `Boolean` | `bool` |
| `bytes` | `BYTEA` | `Bytes` | `[]byte` |
| `enum` | a `CREATE TYPE` enum | `enum` | typed string consts |
| `Timestamp` | `TIMESTAMPTZ` | `DateTime` | `time.Time` |
| `Duration` | `INTERVAL` | `String` | `string` |
| message / `map` | `JSONB` | `Json` | `json.RawMessage` |
| `repeated` scalar | `T[]` | `T[]` | `[]T` |

Unsigned 32/64-bit kinds widen one step (`uint32`→`BIGINT`) so the full range
fits. `google.type.*` (Date, Money, LatLng, …) and the wrapper types are mapped
too. Nullable columns become pointer (`*T`) / optional (`T?`) types.

---

## Building from source

```bash
git clone https://github.com/oh-tarnished/protorm
cd protorm
go build ./plugin/cmd/protoc-gen-protorm   # the plugin binary
go test ./...                              # golden + unit tests
buf lint                                   # proto linting
```

The `examples/` directory is a complete, generated demo — run
`buf generate --template buf.gen.example.yaml` to regenerate it.

---

## License

Licensed under the [Apache License, Version 2.0](LICENSE).
