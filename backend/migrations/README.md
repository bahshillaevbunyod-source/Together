# Migrations

Plain PostgreSQL `.sql` files, applied manually for now (no migration CLI yet).

Naming: `<version>_<name>.{up,down}.sql` — the format used by common tools
(e.g. golang-migrate), so a CLI can be adopted later without renaming.

Apply / roll back manually:

```bash
psql "$DATABASE_URL" -f migrations/000001_create_users.up.sql
psql "$DATABASE_URL" -f migrations/000001_create_users.down.sql
```
