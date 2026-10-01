# Database models and migrations

The database schema is defined by the GORM models in `internal/database/models`.
Atlas reads those models through the pinned `atlas-provider-gorm` Go tool and
creates versioned PostgreSQL SQL migrations in `migrations/`.

## Create a migration

1. Edit the GORM model(s) to describe the desired schema.
2. From the `server` directory, create a migration:

   ```powershell
   atlas migrate diff add_a_short_change_name --env gorm
   ```

   Atlas uses a temporary PostgreSQL 17 development database for schema diffing.
   Docker must be available. This command writes a new SQL file and updates
   `migrations/atlas.sum`; it does not apply the migration to the application DB.
3. Review the generated SQL. Keep model changes, SQL migration, and `atlas.sum`
   together in the same source-control change.

If a new migration is written or edited by hand, refresh its checksum with:

```powershell
atlas migrate hash --dir file://migrations
```

## Apply migrations

Install the Atlas CLI and make `atlas` available on `PATH`. On Windows, use the
official Windows binary installation instructions:
<https://atlasgo.io/faq/atlas-on-windows>.

When a PostgreSQL database is available, set its URL in the current PowerShell
session and apply pending migration files:

```powershell
$env:DATABASE_URL = "postgres://USER:PASSWORD@HOST:5432/DATABASE?sslmode=disable"
atlas migrate apply --env local
```

Atlas applies the versioned SQL files and records the applied version in the
database. Run this deployment step against the target database before starting
the server version that depends on the change. The API process does not run
schema migrations automatically.

## Initial migration

The initial schema is in
`migrations/20261001120000_create_users_and_presence_sessions.sql`. It has been
prepared as a migration file only; it has not been applied to a database.
