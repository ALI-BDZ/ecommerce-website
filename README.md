# ecommerce

Production-grade e-commerce platform built with Go, Fiber, Templ, HTMX, and TailwindCSS.

## Stack

- **Backend**: Go 1.25, Fiber v3, Templ
- **Frontend**: HTMX, Alpine.js, TailwindCSS v4, GSAP, Swiper
- **Database**: PostgreSQL, SQLC, Redis
- **Storage**: S3-compatible
- **Payments**: Stripe
- **Infra**: Docker, Docker Compose, Nginx, GitHub Actions

## Structure

```
cmd/         - Application entrypoints (web, worker, migrate)
internal/    - Feature-sliced backend (auth, products, cart, checkout, orders, etc.)
web/         - Global UI (layouts, components, assets, CSS, JS)
database/    - Migrations, seeds, SQL queries, SQLC gen
config/      - Configuration loading
storage/     - File storage directories
scripts/     - Dev/build/deploy scripts
tests/       - Feature-mirrored test packages
```

## Quick Start

```bash
# Start services
docker compose -f deployments/docker-compose.yml up -d

# Run migrations
go run ./cmd/migrate/ up
go run ./cmd/migrate/ seed

# Start dev server
./scripts/dev.sh
```

## Architecture

Every feature is a vertical slice. Each `internal/` feature owns:

- `handlers/` - HTTP handlers
- `service/` - Business logic
- `repository/` - Data access
- `templates/` - Templ components
- `css/` - Feature-specific styles
- `js/` - Feature-specific scripts
- `dto/` - Data transfer objects
- `validators/` - Input validation
- `routes.go` - Route registration
- `types.go` - Domain types

Changes stay local. HTMX returns HTML fragments, not JSON.