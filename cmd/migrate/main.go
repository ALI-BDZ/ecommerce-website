package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/jackc/pgx/v5"
	"github.com/joho/godotenv"
)

func main() {
	godotenv.Load()
	dsn := os.Getenv("DATABASE_DIRECT_URL")
	if dsn == "" {
		dsn = os.Getenv("DATABASE_URL")
	}
	if dsn == "" {
		log.Fatal("DATABASE_URL not set")
	}

	conn, err := pgx.Connect(context.Background(), dsn)
	if err != nil {
		log.Fatalf("connect: %v", err)
	}
	defer conn.Close(context.Background())

	if err := conn.Ping(context.Background()); err != nil {
		log.Fatalf("ping: %v", err)
	}
	fmt.Println("connected to supabase")

	migrations := []struct{ name, sql string }{
		{"001_orders", `
CREATE TABLE IF NOT EXISTS orders (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  order_number SERIAL UNIQUE,
  status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','confirmed','shipped','delivered','cancelled','returned','refunded')),
  payment_method TEXT NOT NULL CHECK (payment_method IN ('cod','ccp','card')),
  payment_status TEXT NOT NULL DEFAULT 'unpaid' CHECK (payment_status IN ('unpaid','paid','refunded')),
  first_name TEXT NOT NULL, last_name TEXT NOT NULL, phone TEXT NOT NULL, email TEXT,
  address TEXT NOT NULL, wilaya TEXT NOT NULL, city TEXT NOT NULL, notes TEXT,
  subtotal BIGINT NOT NULL, shipping_cost BIGINT NOT NULL DEFAULT 0, discount BIGINT NOT NULL DEFAULT 0, total BIGINT NOT NULL,
  timeline JSONB, courier_notes TEXT, staff_id UUID, delivery_date TIMESTAMPTZ,
  shipped_at TIMESTAMPTZ, delivered_at TIMESTAMPTZ, profit BIGINT NOT NULL DEFAULT 0,
  coupon_id UUID REFERENCES coupons(id) ON DELETE SET NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS order_items (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  order_id UUID NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
  product_id TEXT NOT NULL, product_name TEXT NOT NULL, product_brand TEXT,
  variant TEXT, image_url TEXT, price BIGINT NOT NULL, quantity INTEGER NOT NULL DEFAULT 1,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_orders_status ON orders(status);
CREATE INDEX IF NOT EXISTS idx_orders_created ON orders(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_order_items_order ON order_items(order_id);
`},
		{"002_admin_tables", `
CREATE TABLE IF NOT EXISTS brands (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name TEXT NOT NULL UNIQUE, slug TEXT NOT NULL UNIQUE, description TEXT, logo_url TEXT,
  website TEXT, sort_order INTEGER NOT NULL DEFAULT 0, is_active BOOLEAN NOT NULL DEFAULT true,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS categories (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  parent_id UUID, name TEXT NOT NULL, slug TEXT NOT NULL UNIQUE, description TEXT,
  image_url TEXT, sort_order INTEGER NOT NULL DEFAULT 0, is_active BOOLEAN NOT NULL DEFAULT true,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS products (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name TEXT NOT NULL, slug TEXT NOT NULL UNIQUE,
  brand_id UUID REFERENCES brands(id) ON DELETE SET NULL,
  category_id UUID REFERENCES categories(id) ON DELETE SET NULL,
  description TEXT, short_description TEXT, ingredients TEXT, how_to_use TEXT,
  price BIGINT NOT NULL, compare_at_price BIGINT, cost_price BIGINT,
  sku TEXT UNIQUE, barcode TEXT, weight_grams NUMERIC(10,2),
  stock INTEGER NOT NULL DEFAULT 0, reserved INTEGER NOT NULL DEFAULT 0, incoming INTEGER NOT NULL DEFAULT 0,
  low_stock_threshold INTEGER NOT NULL DEFAULT 5,
  is_active BOOLEAN NOT NULL DEFAULT true, is_featured BOOLEAN NOT NULL DEFAULT false,
  views INTEGER NOT NULL DEFAULT 0, clicks INTEGER NOT NULL DEFAULT 0,
  orders_count INTEGER NOT NULL DEFAULT 0, revenue BIGINT NOT NULL DEFAULT 0, profit BIGINT NOT NULL DEFAULT 0,
  return_count INTEGER NOT NULL DEFAULT 0, wishlist_count INTEGER NOT NULL DEFAULT 0,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS product_variants (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  product_id UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
  name TEXT NOT NULL, sku TEXT, price BIGINT, stock INTEGER NOT NULL DEFAULT 0,
  is_active BOOLEAN NOT NULL DEFAULT true, sort_order INTEGER NOT NULL DEFAULT 0,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS product_images (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  product_id UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
  url TEXT NOT NULL, alt TEXT, sort_order INTEGER NOT NULL DEFAULT 0,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS customers (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  first_name TEXT NOT NULL, last_name TEXT NOT NULL, phone TEXT NOT NULL UNIQUE, email TEXT,
  password_hash TEXT, total_orders INTEGER NOT NULL DEFAULT 0, delivered_orders INTEGER NOT NULL DEFAULT 0,
  cancelled_orders INTEGER NOT NULL DEFAULT 0, returned_orders INTEGER NOT NULL DEFAULT 0,
  lifetime_value BIGINT NOT NULL DEFAULT 0, average_basket BIGINT NOT NULL DEFAULT 0,
  last_order_at TIMESTAMPTZ, favorite_category_id UUID, favorite_brand_id UUID,
  last_seen_at TIMESTAMPTZ, referral_source TEXT, notes TEXT,
  tags TEXT[] NOT NULL DEFAULT '{}', risk_score INTEGER NOT NULL DEFAULT 100,
  is_blacklisted BOOLEAN NOT NULL DEFAULT false,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS coupons (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  code TEXT NOT NULL UNIQUE, description TEXT,
  discount_type TEXT NOT NULL CHECK (discount_type IN ('percentage','fixed')),
  discount_value BIGINT NOT NULL, min_order BIGINT NOT NULL DEFAULT 0, max_discount BIGINT,
  max_uses INTEGER, used_count INTEGER NOT NULL DEFAULT 0,
  revenue_generated BIGINT NOT NULL DEFAULT 0, profit_lost BIGINT NOT NULL DEFAULT 0,
  customers_acquired INTEGER NOT NULL DEFAULT 0, is_active BOOLEAN NOT NULL DEFAULT true,
  starts_at TIMESTAMPTZ, expires_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS staff (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name TEXT NOT NULL, email TEXT NOT NULL UNIQUE, password_hash TEXT NOT NULL,
  role TEXT NOT NULL DEFAULT 'staff' CHECK (role IN ('owner','manager','warehouse','support','marketing','courier','editor','staff')),
  is_active BOOLEAN NOT NULL DEFAULT true, last_login_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS customer_flags (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  customer_id UUID NOT NULL REFERENCES customers(id) ON DELETE CASCADE,
  flag TEXT NOT NULL CHECK (flag IN ('no_answer','wrong_address','refused_package','repeated_cancellation','vip','frequent_buyer','cod_risk','fraud_risk','spam','wholesale')),
  note TEXT, created_by UUID, created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS customer_delivery_history (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  customer_id UUID NOT NULL REFERENCES customers(id) ON DELETE CASCADE,
  order_id UUID, event TEXT NOT NULL CHECK (event IN ('delivered','cancelled','courier_returned','no_answer','refused','wrong_address','phone_invalid')),
  note TEXT, created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS reviews (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  product_id UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
  customer_id UUID REFERENCES customers(id) ON DELETE SET NULL,
  rating INTEGER NOT NULL CHECK (rating >= 1 AND rating <= 5),
  title TEXT, body TEXT, status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','approved','rejected','reported')),
  helpful_votes INTEGER NOT NULL DEFAULT 0,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS notifications (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  type TEXT NOT NULL CHECK (type IN ('order_created','cancelled','payment_failed','refund','review','low_stock','stock_out','customer_message','delivery_delay','large_order','fraud_suspicion','coupon_abuse','high_return_rate')),
  title TEXT NOT NULL, body TEXT, data JSONB, is_read BOOLEAN NOT NULL DEFAULT false,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS activity_logs (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  staff_id UUID, action TEXT NOT NULL, entity_type TEXT NOT NULL, entity_id TEXT,
  details JSONB, ip_address TEXT, created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS content (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  title TEXT NOT NULL, slug TEXT NOT NULL UNIQUE, body TEXT, excerpt TEXT,
  image_url TEXT, author_id UUID, status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','published','archived')),
  published_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
`},
		{"004_store_info", `
CREATE TABLE IF NOT EXISTS store_info (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  store_name TEXT NOT NULL DEFAULT '',
  owner_name TEXT NOT NULL DEFAULT '',
  phone TEXT NOT NULL DEFAULT '',
  location TEXT NOT NULL DEFAULT '',
  instagram TEXT NOT NULL DEFAULT '',
  tiktok TEXT NOT NULL DEFAULT '',
  images JSONB NOT NULL DEFAULT '[]'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
INSERT INTO store_info (store_name,owner_name,phone,location,instagram,tiktok) SELECT 'IronFuel Nutrition','Yacine','+213 555 12 34 56','Algiers, Algeria','https://instagram.com/ironfuel_dz','https://tiktok.com/@ironfuel_dz' WHERE NOT EXISTS (SELECT 1 FROM store_info);
`},
		{"005_packs", `
CREATE TABLE IF NOT EXISTS packs (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  price BIGINT NOT NULL,
  original_price BIGINT NOT NULL DEFAULT 0,
  image TEXT NOT NULL DEFAULT '',
  product_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
  days_valid INT NOT NULL DEFAULT 30,
  expires_at TIMESTAMPTZ NOT NULL DEFAULT now() + interval '30 days',
  is_active BOOLEAN NOT NULL DEFAULT true,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
`},
		{"003_seed", `
INSERT INTO brands (name,slug,description,sort_order) VALUES
  ('Optimum Nutrition','optimum-nutrition','Premium sports nutrition brand',1),
  ('BSN','bsn','High-performance supplements',2),
  ('MuscleTech','muscle-tech','Science-based sports nutrition',3),
  ('Dymatize','dymatize','Premium protein & supplements',4),
  ('MyProtein','my-protein','Quality supplements at great value',5)
ON CONFLICT (slug) DO NOTHING;

INSERT INTO categories (name,slug,description,sort_order) VALUES
  ('Whey Protein','whey-protein','Whey protein powders & isolates',1),
  ('Creatine','creatine','Creatine monohydrate & blends',2),
  ('Pre-Workout','pre-workout','Pre-workout supplements & boosters',3),
  ('BCAA','bcaa','Branch chain amino acids',4),
  ('Vitamins','vitamins','Daily vitamins & minerals',5),
  ('Weight Gainer','weight-gainer','High-calorie mass gainers',6),
  ('Fat Burner','fat-burner','Weight loss & thermogenics',7),
  ('Accessories','accessories','Shakers, gear & accessories',8)
ON CONFLICT (slug) DO NOTHING;
`},
		{"006_promo_banner", `
ALTER TABLE store_info ADD COLUMN IF NOT EXISTS promo_banner JSONB NOT NULL DEFAULT '{}'::jsonb;
`},
		{"007_location_link", `
ALTER TABLE store_info ADD COLUMN IF NOT EXISTS location_link TEXT NOT NULL DEFAULT '';
`},
		{"008_store_description", `
ALTER TABLE store_info ADD COLUMN IF NOT EXISTS store_description TEXT NOT NULL DEFAULT '';
`},
		{"009_cleanup_phantom_items", `
DELETE FROM order_items WHERE product_name IS NULL OR product_name = '';
DELETE FROM orders WHERE NOT EXISTS (SELECT 1 FROM order_items WHERE order_id = orders.id);
DELETE FROM notifications WHERE type='order_created' AND data->>'order_id' IS NOT NULL AND NOT EXISTS (SELECT 1 FROM orders WHERE orders.id::text = notifications.data->>'order_id');
DELETE FROM notifications WHERE type='cancelled' AND data->>'order_id' IS NOT NULL AND NOT EXISTS (SELECT 1 FROM orders WHERE orders.id::text = notifications.data->>'order_id');
	`},
		{"010_remove_test_orders", `
DELETE FROM notifications WHERE type='order_created' AND data->>'order_id' IS NOT NULL AND EXISTS (SELECT 1 FROM orders WHERE orders.id::text = notifications.data->>'order_id' AND orders.status='pending');
DELETE FROM order_items WHERE order_id IN (SELECT id FROM orders WHERE status='pending');
DELETE FROM orders WHERE status='pending';
	`},
		{"011_add_missing_fks", `
-- customer_delivery_history.order_id → orders.id (clean orphans first)
DELETE FROM customer_delivery_history WHERE order_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM orders WHERE id=customer_delivery_history.order_id);
DO $$ BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='fk_cdh_order') THEN
    ALTER TABLE customer_delivery_history ADD CONSTRAINT fk_cdh_order FOREIGN KEY (order_id) REFERENCES orders(id) ON DELETE SET NULL;
  END IF;
END $$;

-- categories.parent_id → categories.id (clean orphans first)
UPDATE categories SET parent_id=NULL WHERE parent_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM categories c WHERE c.id=categories.parent_id);
DO $$ BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='fk_cat_parent') THEN
    ALTER TABLE categories ADD CONSTRAINT fk_cat_parent FOREIGN KEY (parent_id) REFERENCES categories(id) ON DELETE SET NULL;
  END IF;
END $$;

-- content.author_id → staff.id (clean orphans first)
UPDATE content SET author_id=NULL WHERE author_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM staff WHERE id=content.author_id);
DO $$ BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='fk_content_author') THEN
    ALTER TABLE content ADD CONSTRAINT fk_content_author FOREIGN KEY (author_id) REFERENCES staff(id) ON DELETE SET NULL;
  END IF;
END $$;

-- activity_logs.staff_id → staff.id (clean orphans first)
UPDATE activity_logs SET staff_id=NULL WHERE staff_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM staff WHERE id=activity_logs.staff_id);
DO $$ BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='fk_al_staff') THEN
    ALTER TABLE activity_logs ADD CONSTRAINT fk_al_staff FOREIGN KEY (staff_id) REFERENCES staff(id) ON DELETE SET NULL;
  END IF;
END $$;
	`},
		{"012_drop_sku", `
ALTER TABLE products DROP COLUMN IF EXISTS sku;
	`},
		{"013_drop_cost_price", `
ALTER TABLE products DROP COLUMN IF EXISTS cost_price;
	`},
		{"014_add_whatsapp", `
ALTER TABLE store_info ADD COLUMN IF NOT EXISTS whatsapp TEXT NOT NULL DEFAULT '';
	`},
	}

	for _, m := range migrations {
		fmt.Printf("running %s...\n", m.name)
		if _, err := conn.Exec(context.Background(), m.sql); err != nil {
			log.Fatalf("%s failed: %v", m.name, err)
		}
		fmt.Printf("%s complete\n", m.name)
	}

	fmt.Println("all migrations complete")
}