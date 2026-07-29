# IronFuel Nutrition — Complete Technical Audit

**Date:** 2026-07-28
**Auditor:** AI Review (backend architect handoff)
**Scope:** Full-stack audit of Go/Fiber + PostgreSQL + Alpine.js e-commerce

---

## 1. Image Storage & Pipeline

### Current State
- **Storage:** Supabase Storage bucket `products` (public bucket)
- **Upload:** `POST /api/admin/upload` — reads entire file into `[]byte` in memory, then POSTs to Supabase REST API via `http.Client`
- **Naming:** `products/{unix_nano}{ext}` — no content-hash dedup, no conflict resolution
- **Format:** No server-side format enforcement. Upload accepts whatever the client sends.
- **Delete:** Product delete removes DB rows (`product_images`) but **never deletes files from Supabase Storage** — orphaned files accumulate forever.

### Issues
| Severity | Issue |
|----------|-------|
| HIGH | No file size limit on upload — a 2GB file will OOM the server (reads entire file into `[]byte`) |
| HIGH | No content-type validation — can upload `exe`, `html`, `svg` (XSS vector) |
| HIGH | No image deletion from storage on product/image delete — orphan leak |
| MEDIUM | No dedup — uploading the same image twice creates two storage objects |
| MEDIUM | No image processing — raw uploads served directly to customers |
| LOW | Naming uses `time.UnixNano()` — no collision in single-threaded uploads, but race possible under concurrent admin sessions |

### Recommendations
- Add `file.Open()` + `io.LimitReader` with a 10MB cap before reading
- Whitelist: `image/jpeg`, `image/png`, `image/webp` only
- Delete storage objects when images are removed from `product_images`
- Use content-hash naming (`sha256[:16].webp`) for dedup
- Add `imagick` or `libvips` post-upload pipeline to generate thumbnails

---

## 2. Image Processing

### Current State
**None.** Zero server-side image processing exists anywhere in the codebase.

- No resizing
- No format conversion (WebP/AVIF)
- No thumbnail generation
- No quality compression
- No EXIF stripping

### Impact
- Admin uploads a 4000×4000 8MB JPEG → served as-is to mobile users on 3G
- Product cards render 8MB images in a 300×300 thumbnail slot
- No `srcset` or `<picture>` elements anywhere in frontend HTML
- LCP element (product hero image) downloads full-resolution on every page load

### Recommendations
- Post-upload pipeline: generate 3 variants per image — 600×600 (card), 1200×1200 (detail), original
- Convert to WebP with quality 80
- Store variants in Supabase Storage under `products/{id}/card.webp`, `products/{id}/detail.webp`
- Strip EXIF data (privacy + size reduction)

---

## 3. Image Delivery

### Current State
Images are served via Supabase public URLs:
```
https://xqscjifdvfnlygwwtmgl.supabase.co/storage/v1/object/public/products/{filename}
```

- **CDN:** Supabase uses AWS CloudFront behind the scenes (auto)
- **Cache-Control:** Supabase default (likely `no-cache` or short TTL)
- **Compression:** Supabase serves as-is — no WebP negotiation

### Issues
| Severity | Issue |
|----------|-------|
| MEDIUM | No explicit `Cache-Control` headers — browser may re-download on every visit |
| MEDIUM | No image CDN with on-the-fly transforms (like imgix/Cloudinary) |
| LOW | Supabase free tier has 1GB storage limit — at ~2MB per product image, ~500 products exhausts it |

### Recommendations
- Set `Cache-Control: public, max-age=31536000, immutable` on storage bucket (filename changes on re-upload)
- Consider Cloudinary or imgix as CDN layer with `?w=600&q=80&f=auto` transform params
- Monitor Supabase storage usage; upgrade plan before hitting 1GB

---

## 4. Image Sizes Per Frontend Location

### File Sizes by Page

| Page | HTML Size | Inline CSS | Inline JS | External Assets |
|------|-----------|------------|-----------|-----------------|
| `admin.html` | 133.9 KB | ~60 KB | ~50 KB | Alpine.js CDN |
| `product.html` | 77.8 KB | ~35 KB | ~25 KB | Alpine.js CDN |
| `shop.html` | 72.4 KB | ~30 KB | ~25 KB | Alpine.js CDN |
| `index.html` | 65.6 KB | ~28 KB | ~22 KB | Alpine.js CDN |
| `cart.html` | 30.7 KB | ~12 KB | ~10 KB | Alpine.js CDN |
| `pack.html` | 29.2 KB | ~12 KB | ~10 KB | Alpine.js CDN |
| `about.html` | 19.6 KB | ~8 KB | ~6 KB | Alpine.js CDN |
| `deals.html` | 13.9 KB | ~6 KB | ~5 KB | Alpine.js CDN |
| `admin-login.html` | 4.9 KB | ~2 KB | ~1 KB | — |

### Image Slot Dimensions (where images render)

| Location | Display Size | Current Source |
|----------|-------------|----------------|
| Homepage hero banner | 1920×600 | Store images from Supabase |
| Product card (shop/grid) | 300×300 | Full-res Supabase URL |
| Product detail gallery | 600×600 | Full-res Supabase URL |
| Product detail lightbox | 1200×1200 | Full-res Supabase URL |
| Pack card | 400×300 | Full-res Supabase URL |
| Cart item thumbnail | 100×100 | Full-res Supabase URL |
| Admin product list | 80×80 | Full-res Supabase URL |

**Critical:** Every location downloads the full-resolution image. No `srcset`, no `<picture>`, no lazy loading.

---

## 5. HTML Optimization

### Critical Missing Features

| Feature | Status | Impact |
|---------|--------|--------|
| `loading="lazy"` on images | MISSING | All images load above + below fold on page load |
| `srcset` / `<picture>` | MISSING | Full-res served to mobile users |
| `<meta viewport>` | Present | OK |
| Minification | MISSING | 65-134 KB HTML per page |
| Critical CSS inlining | MISSING | Render-blocking inline `<style>` blocks |
| Preload hints | MISSING | No `<link rel="preload">` for hero images |
| Service worker | MISSING | No offline/caching strategy |
| `fetchpriority="high"` on LCP | MISSING | Hero image loads with same priority as icons |

### Per-Page Analysis

**index.html (65.6 KB)**
- Hero banner: `<img src="...">` no lazy, no srcset, no fetchpriority
- Product grid: 8+ images, all eager-loaded
- Bundle section: background image in CSS, no preload

**shop.html (72.4 KB)**
- Filter panel uses inline CSS (~30 KB) — could be external + cached
- Product grid: pagination loads new page but all images still eager
- Sort dropdown: no debounced search

**product.html (77.8 KB)**
- Gallery: images loaded via Alpine `x-for`, no lazy loading
- Related products: 6 images, all eager
- No `<link rel="preconnect" href="supabase.co">`

**admin.html (133.9 KB)**
- Single-page app with all sections in one file
- Dashboard: 15+ DB queries per load (see Section 14)
- All admin sections (orders, products, customers, store, packs, notifications) loaded eagerly

### Recommendations
- Add `loading="lazy"` to all images below the fold
- Add `fetchpriority="high"` to LCP hero image
- Add `<link rel="preconnect" href="https://xqscjifdvfnlygwwtmgl.supabase.co">` to all pages
- Minify HTML in production build
- Extract inline CSS to external files for caching
- Use `<picture>` with WebP sources for product images

---

## 6. Search Implementation

### Current State
All search uses PostgreSQL `ILIKE` with `%term%` pattern:

```sql
-- Order search
WHERE (first_name ILIKE '%term%' OR last_name ILIKE '%term%' 
       OR phone ILIKE '%term%' OR order_number::text ILIKE '%term%')

-- Product search
WHERE (p.name ILIKE '%term%' OR p.sku ILIKE '%term%')

-- Customer search
WHERE (first_name ILIKE '%term%' OR last_name ILIKE '%term%' 
       OR phone ILIKE '%term%' OR email ILIKE '%term%')
```

### Issues
| Severity | Issue |
|----------|-------|
| MEDIUM | `ILIKE '%term%'` cannot use B-tree indexes — full table scan on every search |
| MEDIUM | No trigram (pg_trgm) index for fuzzy matching |
| LOW | No minimum search length — single-char searches scan entire table |
| LOW | No search result ranking — results ordered by `created_at`, not relevance |
| LOW | Product search only checks `name` and `sku` — doesn't search `description`, `brand.name`, or `category.name` |

### Recommendations
- Add `pg_trgm` extension + GIN index on searchable columns
- Use `ILIKE 'term%'` (prefix match) which can use B-tree indexes
- Add minimum 2-character search validation
- Consider `tsvector` full-text search for product descriptions
- Add search analytics to understand what customers search for

---

## 7. Product Listing (Column Selection + Pagination)

### Public Product Listing (`GET /api/products`)
```sql
SELECT p.id, p.name, p.slug, p.price, 
       COALESCE(p.compare_at_price,0), 
       COALESCE(ROUND(AVG(r.rating),1),0), COUNT(r.id),
       COALESCE(c.name,''), COALESCE(c.slug,''), 
       COALESCE(b.name,''),
       COALESCE((SELECT url FROM product_images WHERE product_id=p.id 
                 ORDER BY sort_order LIMIT 1),'')
FROM products p 
LEFT JOIN categories c ON c.id=p.category_id 
LEFT JOIN brands b ON b.id=p.brand_id 
LEFT JOIN reviews r ON r.product_id=p.id 
WHERE p.is_active=true 
GROUP BY p.id, c.name, c.slug, b.name 
ORDER BY p.created_at DESC
```

### Issues
| Severity | Issue |
|----------|-------|
| HIGH | **No pagination** — returns ALL active products in single response |
| HIGH | **No limit** — 10,000 products = 10,000-row response with subqueries |
| MEDIUM | Correlated subquery `(SELECT url FROM product_images WHERE product_id=p.id ORDER BY sort_order LIMIT 1)` runs per-row |
| MEDIUM | `GROUP BY p.id` with `AVG(r.rating)` + `COUNT(r.id)` + correlated image subquery = 3 operations per product |
| LOW | Returns all product fields even when cards only need name, price, image |

### Admin Product Listing (`GET /api/admin/products`)
```sql
SELECT p.id,p.name,p.sku,p.price,p.stock,p.is_active,
       p.orders_count,p.revenue,p.created_at,
       COALESCE(b.name,''),COALESCE(c.name,''),
       COALESCE(ROUND(AVG(r.rating),1),0),
       COALESCE((SELECT url FROM product_images WHERE product_id=p.id 
                 ORDER BY sort_order LIMIT 1),'')
FROM products p 
LEFT JOIN brands b ON b.id=p.brand_id 
LEFT JOIN categories c ON c.id=p.category_id 
LEFT JOIN reviews r ON r.product_id=p.id 
WHERE ... 
GROUP BY p.id,b.name,c.name 
ORDER BY p.created_at DESC 
LIMIT $N OFFSET $M
```

### Status
- Admin listing has proper LIMIT/OFFSET pagination ✅
- Public listing has NO pagination ❌

### Recommendations
- Add LIMIT/OFFSET or cursor-based pagination to public listing
- Replace correlated subquery with lateral join or pre-computed `cover_image_url` column
- Add `created_at` index for ORDER BY performance
- Consider materialized view for product catalog with pre-joined images/reviews

---

## 8. Pagination Implementation

### Admin Panels (all use same pattern)
```go
page, _ := strconv.Atoi(c.Query("page", "1"))
limit, _ := strconv.Atoi(c.Query("limit", "20"))
// ...
args = append(args, limit, (page-1)*limit)
q := "... LIMIT $N OFFSET $M"
```

### Issues
| Severity | Issue |
|----------|-------|
| MEDIUM | OFFSET-based pagination degrades at high page numbers (OFFSET 10000 = scan 10000 rows then discard) |
| LOW | No max limit enforcement — `?limit=999999` returns entire table |
| LOW | No validation that page >= 1 |
| LOW | Total count runs separate `SELECT COUNT(*)` query — 2 queries per page load |

### Public API
**No pagination at all.** `GET /api/products` returns every active product.

### Recommendations
- Enforce max limit (50) on all paginated endpoints
- Switch to cursor-based pagination for large datasets (use `created_at` + `id` as cursor)
- Add `page >= 1` validation
- For public products: add `?page=1&limit=20` with total count in response

---

## 9. Public Endpoint SQL Query Analysis

### `GET /api/products` — Product Catalog
```
1x main query: 4-way JOIN (products, categories, brands, reviews) + GROUP BY
1x correlated subquery per row: product_images (sort_order LIMIT 1)
N+1 pattern: YES (correlated subquery runs per product)
Estimated cost at 1000 products: ~50ms (sequential scan + subqueries)
```

### `GET /api/products/:slug` — Product Detail
```
1x main query: 4-way LEFT JOIN + GROUP BY + WHERE slug=$1
1x correlated subquery: product_images (LIMIT 1)
1x related products query: same category, 4-way JOIN, LIMIT 6
Total: 3 queries per request ✅ (reasonable)
```

### `GET /api/store-info` — Store Info
```
1x query: single row from store_info
No JOINs, no subqueries ✅
```

### `GET /api/packs` — Active Packs
```
1x DELETE: auto-delete expired packs (runs on every request)
1x query: SELECT from packs WHERE is_active=true
Issue: DELETE on read path — write operation on every GET
```

### `POST /api/orders` — Create Order
```
BEGIN TRANSACTION
1x SELECT FOR UPDATE per product (N queries for N items)
1x INSERT order
1x INSERT order_items per item (N queries)
1x UPDATE products SET stock=stock-N per item (N queries)
1x UPDATE products SET orders_count=orders_count+1 per item (N queries)
1x SELECT COUNT FROM customers (phone check)
1x INSERT or UPDATE customers
COMMIT
Total: 4N + 3 queries per order
Example: 3-item order = 15 queries in single transaction
```

### `GET /api/admin/dashboard` — Admin Dashboard
```
14x individual COUNT/SUM queries (sequential, no parallelism)
1x revenue chart query (30-day aggregation)
1x order chart query (30-day aggregation)
1x low stock alert query
1x best sellers by quantity (with EXISTS subquery)
1x best sellers by revenue (with EXISTS subquery)
1x never-sold products (with NOT EXISTS subquery)
1x stale products query
1x top customers query
1x COD reliability query (with FILTER clause)
1x category breakdown query (with subquery joins)
1x recent orders query
Total: 21+ queries per dashboard load
```

---

## 10. Supabase Storage Configuration

### Current Config
- **Bucket:** `products` (public)
- **Access:** Public read, authenticated write (service role)
- **Region:** `eu-west-1` (AWS Ireland)
- **Project:** `xqscjifdvfnlygwwtmgl`

### Issues
| Severity | Issue |
|----------|-------|
| HIGH | Public bucket means anyone can list/Access all files if they know the path |
| MEDIUM | No file size limits configured at bucket level |
| MEDIUM | No allowed MIME types configured at bucket level |
| LOW | No bucket-level cache control headers |
| LOW | No CORS configuration documented |

### Recommendations
- Set bucket-level file size limit (10MB)
- Set allowed MIME types: `image/jpeg`, `image/png`, `image/webp`
- Add `Cache-Control: public, max-age=31536000` to bucket policy
- Consider making bucket private and serving via signed URLs or edge function

---

## 11. File Size Estimates

### Database (Supabase PostgreSQL Free Tier)
| Table | Est. Rows | Est. Size | Notes |
|-------|-----------|-----------|-------|
| products | 100-500 | 1-5 MB | With all text fields |
| product_images | 200-1000 | 0.5-2 MB | URL text only |
| orders | 100-5000 | 2-20 MB | Grows with order history |
| order_items | 200-15000 | 1-10 MB | 3 items/order average |
| customers | 50-1000 | 0.5-2 MB | |
| reviews | 0-500 | 0.1-0.5 MB | |
| notifications | 0-1000 | 0.5-2 MB | JSONB data field |
| **Total DB** | | **6-44 MB** | Free tier: 500MB ✅ |

### Storage (Supabase Storage Free Tier)
| Asset Type | Per File | 100 Products | 500 Products |
|------------|----------|--------------|--------------|
| Product image (original) | 1-5 MB | 100-500 MB | 500-2500 MB |
| Store banner | 0.5-2 MB | 2-8 MB | 2-8 MB |
| **Total Storage** | | **102-508 MB** | **502-2508 MB** |

**Critical:** Free tier is 1GB. At ~2MB average per product image, ~500 products exhaust storage.

### HTML Files
| File | Size | gzip Est. |
|------|------|-----------|
| admin.html | 133.9 KB | ~25 KB |
| product.html | 77.8 KB | ~15 KB |
| shop.html | 72.4 KB | ~14 KB |
| index.html | 65.6 KB | ~13 KB |
| cart.html | 30.7 KB | ~6 KB |
| pack.html | 29.2 KB | ~6 KB |
| about.html | 19.6 KB | ~4 KB |
| deals.html | 13.9 KB | ~3 KB |
| admin-login.html | 4.9 KB | ~1.5 KB |

---

## 12. Frontend Performance

### Architecture Issues

**No build pipeline:**
- No bundler (Vite, webpack, esbuild)
- No minification
- No tree-shaking
- No code splitting
- No CSS preprocessor

**Inline everything:**
- Each HTML file contains its own `<style>` block (12-60 KB of CSS)
- Each HTML file contains its own `<script>` block (5-50 KB of JS)
- No external CSS/JS files = no browser caching between pages

**Alpine.js CDN:**
- Loaded from `cdn.jsdelivr.net` on every page
- No `integrity` hash (subresource integrity)
- No `defer` attribute on some script tags

### Specific Performance Anti-patterns

1. **admin.html (133.9 KB):** Entire admin SPA in one file. All sections (dashboard, orders, products, customers, store, packs, notifications) loaded eagerly even when not visible.

2. **CheckLowStock() on every dashboard load:** Runs 3 DELETE queries + 1 SELECT query + potential INSERT queries on every `GET /api/admin/dashboard`. This is a write operation on a read path.

3. **No `rel="preconnect"`:** Supabase Storage images loaded without preconnecting to `xqscjifdvfnlygwwtmgl.supabase.co`.

4. **No image dimensions specified:** All `<img>` tags lack `width` and `height` attributes, causing CLS (Cumulative Layout Shift).

### Recommendations
- Extract CSS/JS to external files for caching
- Add `width`/`height` to all `<img>` tags
- Add `<link rel="preconnect">` for Supabase
- Lazy-load below-fold images
- Consider extracting admin.html into route-based chunks
- Move `CheckLowStock()` to background goroutine on timer, not on dashboard read

---

## 13. Network Waterfall Estimates

### Homepage First Load (Cold)
```
1. DNS + TCP + TLS: ~200ms (supabase.co)
2. HTML (index.html): ~50ms (65.6 KB)
3. Alpine.js CDN: ~100ms (15 KB, cached after first)
4. /api/store-info: ~150ms (1 query)
5. /api/products: ~200ms (all products, no pagination)
6. Images (hero): ~500-2000ms (1-5 MB full-res)
7. Images (product cards × 8): ~2000-8000ms (8 × 1-5 MB)
Total estimated: 3-12 seconds on 3G
```

### Shop Page First Load
```
1. HTML: ~50ms (72.4 KB)
2. /api/products: ~200ms (all products)
3. Images (all products): ~5000-20000ms (20+ × 1-5 MB)
Total estimated: 5-20 seconds on 3G
```

### Admin Dashboard Load
```
1. HTML: ~50ms (133.9 KB)
2. /api/admin/dashboard: ~500-2000ms (21+ queries)
3. Images (low stock alerts): ~200ms
Total estimated: 1-3 seconds (admin on WiFi, acceptable)
```

### Create Order
```
1. POST /api/orders: ~200-500ms (15 queries in transaction)
2. Response + redirect: ~50ms
Total: ~250-550ms ✅
```

---

## 14. Database Scale Limits

### Connection Pool
```go
cfg.MaxConns = 10    // Max open connections
cfg.MinConns = 2     // Keep-alive connections
cfg.MaxConnLifetime = 30 * time.Minute
cfg.MaxConnIdleTime = 5 * time.Minute
```

**Analysis:** 10 max connections is appropriate for Supabase free tier (limit: 60 direct, 200 pooler). However, the 21-query dashboard could exhaust pool under concurrent admin users.

### Query Complexity Hotspots

| Endpoint | Queries | Complexity | Risk |
|----------|---------|------------|------|
| Dashboard | 21+ | High (sequential) | MEDIUM — blocks pool |
| Order creation | 4N+3 | High (transaction) | LOW — fast queries |
| Product listing | 1+N | High (N+1 subquery) | HIGH at scale |
| Order status update | 5+ | Medium (transaction) | LOW |
| CheckLowStock | 3+DELETE + N | Medium (write on read) | MEDIUM |

### Supabase Free Tier Limits
| Resource | Limit | Current Usage | Risk |
|----------|-------|---------------|------|
| Database size | 500 MB | ~6-44 MB | LOW |
| Storage | 1 GB | ~100-500 MB | HIGH at 500+ products |
| Connections (pooler) | 200 | 10 max | LOW |
| Connections (direct) | 60 | 10 max | LOW |
| Edge function invocations | 500K/month | Unknown | LOW |
| Bandwidth | 5 GB/month | Unknown | MEDIUM |

### Recommendations
- Add connection pool monitoring (log pool exhaustion)
- Add query timing middleware (log slow queries >100ms)
- Consider read replicas for dashboard queries (Supabase Pro tier)
- Add database indexes for ILIKE searches (pg_trgm)
- Monitor storage usage before product expansion

---

## 15. Current Bottlenecks

### Critical (Fix Before Production)

1. **No image processing:** Full-resolution images served to mobile. A single product page can download 20-40MB of images.

2. **No pagination on public products:** `GET /api/products` returns ALL products. At 1000 products, response is ~2MB JSON + correlated subqueries.

3. **CheckLowStock() on dashboard read:** 3 DELETE + 1 SELECT + N INSERT queries run on every dashboard page load. This is a write operation that should be async.

4. **No file size limit on upload:** Admin can upload a 2GB file, OOM-killing the server.

### High (Fix Before Scale)

5. **ILIKE '%term%' search:** Full table scan on every search. At 10K orders, search takes >500ms.

6. **OFFSET pagination:** Admin panels use OFFSET, which degrades at page 50+ (scans 1000 rows to skip them).

7. **21 queries per dashboard load:** No query batching or materialized views. Dashboard is O(21) regardless of data size.

8. **Orphaned storage files:** Product delete never cleans up Supabase Storage. Storage leaks forever.

### Medium (Fix Before Growth)

9. **No caching layer:** Every request hits PostgreSQL. No Redis, no in-memory cache, no CDN for API responses.

10. **Admin sessions in Go memory:** Server restart loses all admin logins. No persistence.

11. **No rate limiting:** API endpoints have no rate limiting. DDoS vector.

12. **No request validation middleware:** Each handler validates independently. No centralized validation.

---

## 16. Security Audit

### Critical Issues

| Severity | Issue | Location |
|----------|-------|----------|
| CRITICAL | **Hardcoded DB credentials in .env committed to git** | `.env` line 5-8 |
| CRITICAL | **Supabase service role key in .env** | `.env` line 16 |
| CRITICAL | **Admin credentials: `admin@gmail.com` / `admin123`** | `.env` line 32-33 |
| HIGH | **No CSRF protection** on POST/PUT/DELETE endpoints | All API routes |
| HIGH | **No rate limiting** on login or order creation | `POST /api/admin/login`, `POST /api/orders` |
| HIGH | **No input sanitization** — XSS possible via product names/descriptions | Admin product create/update |
| HIGH | **SQL injection mitigated** by parameterized queries ✅ | All queries use `$1, $2` |

### High Issues

| Severity | Issue | Location |
|----------|-------|----------|
| HIGH | **JWT secret is `dev-secret-change-in-production`** | `.env` line 29 |
| HIGH | **No authentication on public order creation** — anyone can create orders | `POST /api/orders` |
| HIGH | **No phone number validation** — can inject任意 strings | `POST /api/orders` |
| HIGH | **Admin token not tied to IP/User-Agent** — stolen token works anywhere | `admin.go` session store |
| HIGH | **No HTTPS enforcement** — Fiber serves HTTP by default | `main.go` line 1104 |

### Medium Issues

| Severity | Issue | Location |
|----------|-------|----------|
| MEDIUM | **CORS allows all origins** — `cors.New()` with no config | `main.go` line 216 |
| MEDIUM | **No Content-Security-Policy headers** | All responses |
| MEDIUM | **No X-Frame-Options** — admin can be framed (clickjacking) | All responses |
| MEDIUM | **No X-Content-Type-Options** — MIME sniffing possible | All responses |
| MEDIUM | **Admin password compared via `==`** — timing attack possible | `main.go` line 238 |
| MEDIUM | **Error messages leak internal details** — `err.Error()` returned to client | Multiple handlers |

### Low Issues

| Severity | Issue | Location |
|----------|-------|----------|
| LOW | **No request ID tracking** — impossible to trace requests across logs | All handlers |
| LOW | **No audit logging** — admin actions not recorded | Admin CRUD operations |
| LOW | **No account lockout** on failed login attempts | `POST /api/admin/login` |
| LOW | **Admin session expires in 24h** with no refresh mechanism | `main.go` line 148 |
| LOW | **Static file cache: 10s** — too short for production | `main.go` line 219 |

### Security Recommendations (Priority Order)
1. Rotate all credentials immediately (DB, Supabase, JWT secret)
2. Add rate limiting middleware (10 req/min on login, 30 req/min on order creation)
3. Add CSRF middleware (Fiber has `fibercsrf` or manual double-submit cookie)
4. Add security headers middleware (CSP, X-Frame-Options, X-Content-Type-Options)
5. Add request ID middleware for tracing
6. Hash admin passwords with bcrypt (currently plaintext comparison)
7. Add CORS whitelist for production domain only
8. Enforce HTTPS via redirect middleware
9. Add account lockout after 5 failed login attempts
10. Log all admin actions to `activity_logs` table

---

## 17. Final Summary Table

### Overall Scores

| Category | Score | Status |
|----------|-------|--------|
| **Data Integrity** | 7/10 | GOOD — FOR UPDATE, server-side validation, transaction safety ✅ |
| **Image Pipeline** | 2/10 | CRITICAL — No processing, no deletion, no size limits |
| **Search** | 3/10 | POOR — ILIKE full scan, no pagination, no fuzzy |
| **Pagination** | 4/10 | PARTIAL — Admin yes, public no |
| **Frontend Perf** | 3/10 | POOR — No lazy load, no srcset, no caching |
| **Security** | 4/10 | POOR — No rate limit, no CSRF, hardcoded creds |
| **DB Performance** | 5/10 | OK — Good indexes, but N+1 and 21-query dashboard |
| **Scalability** | 4/10 | LOW — Free tier limits, no caching, no connection pooling strategy |

### Architecture Score: 4.4/10

### What's Done Well ✅
- Transaction safety with FOR UPDATE on stock operations
- Server-side price validation (ignores client prices)
- Stock decrement + restore on cancel/return
- Denormalized counters (orders_count, revenue) for fast reads
- Slug uniqueness handling
- Product soft-delete when order history exists
- Customer upsert with lifetime value tracking
- Order status state machine with validated transitions

### What Needs Immediate Attention 🔴
1. **Image pipeline:** Add processing, size limits, deletion
2. **Public pagination:** Add LIMIT/OFFSET to product listing
3. **Security:** Rate limiting, CSRF, credential rotation, security headers
4. **File upload safety:** Size limits, MIME validation, storage cleanup
5. **CheckLowStock async:** Move to background timer, not on dashboard read

### What Needs Pre-Production ⚡
1. Extract inline CSS/JS to cached external files
2. Add `loading="lazy"` + `srcset` to images
3. Add `<link rel="preconnect">` for Supabase
4. Add `width`/`height` to all `<img>` tags
5. Minify HTML in production
6. Add query timing middleware
7. Add request ID tracking
8. Enforce HTTPS
9. Configure CORS for production domain
10. Add database indexes for ILIKE searches

### What Needs Pre-Scale 📈
1. Redis caching layer for product catalog + store info
2. Materialized view for dashboard aggregations
3. Cursor-based pagination for large datasets
4. Read replicas for analytics queries
5. CDN for API responses (or edge caching)
6. Connection pool monitoring + alerting
7. Supabase Pro tier (or migrate to self-hosted)

---

*End of Technical Audit*
