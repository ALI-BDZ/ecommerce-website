# Agent Rules

## Server & DevTools
- Do NOT start the Go server or Chrome DevTools unless I explicitly ask.
- If the server needs to be running for a task, wait for me to start it or ask me first.

## Project Reality (differs from README)
The README describes a feature-sliced `internal/` architecture that doesn't exist. The actual structure:
```
cmd/web/          - Fiber v3 server entrypoint + routes/
cmd/migrate/      - Inline migration runner (not SQL files)
handlers/         - Flat package: all HTTP handlers, middleware, image processing
database/         - pgx connection pool + reference SQL migrations
public/           - Static HTML files served by Fiber (HTMX frontend)
```

## Commands
```bash
# Dev server (with hot reload)
air

# Dev server (manual)
go run ./cmd/web/

# Database migrations (run once, inline SQL in cmd/migrate/main.go)
go run ./cmd/migrate/

# Build
go build -o web.exe ./cmd/web/
```

## Architecture
- **Frontend**: Static HTML in `public/` using HTMX + Alpine.js + TailwindCSS. No Go templates.
- **Backend**: Fiber v3 handlers return HTML fragments for HTMX, JSON for API calls.
- **Database**: PostgreSQL via Supabase (pgx/v5 pool). Migrations are inline Go strings in `cmd/migrate/main.go`, not separate files.
- **Auth**: JWT middleware for admin routes. Login at `/api/admin/login`.
- **Rate limiting**: In-memory sliding window (middleware.go). Login: 5/min, Orders: 10/min.

## Key Gotchas
- `DATABASE_URL` uses Supabase transaction pooler (port 6543). `DATABASE_DIRECT_URL` is for migrations (port 5432).
- `.env` has real credentials. Never commit it (already in .gitignore).
- Body limit is 15MB (for image uploads).
- Migrations in `database/migrations/` are reference-only. The actual runner uses hardcoded SQL in Go.
- No test files exist yet.
- Go 1.26, not 1.25 as README claims.

## Adding Features
Follow the existing pattern in `handlers/`. Each feature is a file: `handlers/products.go`, `handlers/orders.go`, etc. Route registration goes in `cmd/web/routes/admin.go` or `public.go`.
