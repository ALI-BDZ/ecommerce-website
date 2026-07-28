package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/cors"
	"github.com/gofiber/fiber/v3/middleware/static"
	"github.com/joho/godotenv"
	"github.com/yourorg/ecommerce/database"
	"github.com/yourorg/ecommerce/handlers"
)

type OrderRequest struct {
	FirstName     string      `json:"firstName"`
	LastName      string      `json:"lastName"`
	Phone         string      `json:"phone"`
	Email         string      `json:"email"`
	Address       string      `json:"address"`
	Wilaya        string      `json:"wilaya"`
	City          string      `json:"city"`
	Notes         string      `json:"notes"`
	PaymentMethod string      `json:"paymentMethod"`
	Subtotal      int64       `json:"subtotal"`
	ShippingCost  int64       `json:"shippingCost"`
	Discount      int64       `json:"discount"`
	Total         int64       `json:"total"`
	Items         []OrderItem `json:"items"`
}

type OrderItem struct {
	ProductID   string `json:"productId"`
	ProductName string `json:"productName"`
	Brand       string `json:"brand"`
	Variant     string `json:"variant"`
	ImageURL    string `json:"imageUrl"`
	Price       int64  `json:"price"`
	Quantity    int    `json:"quantity"`
}

type storeInfo struct {
	StoreName        string
	OwnerName        string
	Phone            string
	Location         string
	LocationLink     string
	StoreDescription string
	Instagram        string
	TikTok           string
	Images           []string
	PromoBanner      map[string]interface{}
}

func loadStoreFromDB() *storeInfo {
	s := &storeInfo{}
	if database.DB == nil { return s }
	var imagesJSON, promoJSON string
	err := database.DB.QueryRow(context.Background(),
		`SELECT COALESCE(store_name,''),COALESCE(owner_name,''),COALESCE(phone,''),COALESCE(location,''),COALESCE(location_link,''),COALESCE(store_description,''),COALESCE(instagram,''),COALESCE(tiktok,''),COALESCE(images::text,'[]'),COALESCE(promo_banner::text,'{}') FROM store_info ORDER BY updated_at DESC LIMIT 1`,
	).Scan(&s.StoreName, &s.OwnerName, &s.Phone, &s.Location, &s.LocationLink, &s.StoreDescription, &s.Instagram, &s.TikTok, &imagesJSON, &promoJSON)
	if err != nil {
		log.Printf("store db load: %v", err)
		return s
	}
	json.Unmarshal([]byte(imagesJSON), &s.Images)
	if s.PromoBanner == nil { s.PromoBanner = map[string]interface{}{} }
	json.Unmarshal([]byte(promoJSON), &s.PromoBanner)
	return s
}

func cleanupStoreInfo() {
	if database.DB == nil { return }
	var count int
	database.DB.QueryRow(context.Background(), "SELECT COUNT(*) FROM store_info").Scan(&count)
	if count <= 1 { return }
	log.Printf("Found %d store_info rows, merging to 1...", count)
	_, err := database.DB.Exec(context.Background(),
		`DELETE FROM store_info WHERE id NOT IN (
			SELECT id FROM store_info ORDER BY updated_at DESC LIMIT 1
		)`)
	if err != nil {
		log.Printf("store_info cleanup error: %v", err)
	} else {
		log.Println("store_info cleanup done")
	}
}

func fixProductData() {
	if database.DB == nil { return }
	ctx := context.Background()

	// Delete test/debug products with no images and no orders
	database.DB.Exec(ctx, `DELETE FROM products WHERE name ILIKE 'TEST%' AND stock=10 AND price=100`)

	// Fix Gold Standard: set active if it was accidentally deactivated
	tag, _ := database.DB.Exec(ctx, `UPDATE products SET is_active=true WHERE name ILIKE '%Gold Standard%' AND is_active=false`)
	if ta := tag.RowsAffected(); ta > 0 {
		log.Printf("Fixed %d Gold Standard product(s): set is_active=true", ta)
	}

	// Add image for Gold Standard if missing
	var hasImg int
	database.DB.QueryRow(ctx, `SELECT COUNT(*) FROM product_images pi JOIN products p ON pi.product_id=p.id WHERE p.name ILIKE '%Gold Standard%'`).Scan(&hasImg)
	if hasImg == 0 {
		var pid string
		err := database.DB.QueryRow(ctx, `SELECT id FROM products WHERE name ILIKE '%Gold Standard%' LIMIT 1`).Scan(&pid)
		if err == nil && pid != "" {
			database.DB.Exec(ctx, `INSERT INTO product_images (product_id,url,alt,sort_order) VALUES ($1,$2,$3,0)`, pid, "https://xqscjifdvfnlygwwtmgl.supabase.co/storage/v1/object/public/products/gold-standard-whey.jpg", "Gold Standard 100% Whey")
			log.Println("Added image for Gold Standard 100% Whey")
		}
	}
}

type adminSession struct {
	Email     string
	ExpiresAt time.Time
}

var (
	adminSessions   = make(map[string]*adminSession)
	adminSessionsMu sync.RWMutex
)

func generateAdminToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func createAdminSession(email string) string {
	token := generateAdminToken()
	adminSessionsMu.Lock()
	adminSessions[token] = &adminSession{
		Email:     email,
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	adminSessionsMu.Unlock()
	return token
}

func validateAdminToken(token string) (string, bool) {
	adminSessionsMu.RLock()
	s, ok := adminSessions[token]
	adminSessionsMu.RUnlock()
	if !ok || time.Now().After(s.ExpiresAt) {
		if ok {
			adminSessionsMu.Lock()
			delete(adminSessions, token)
			adminSessionsMu.Unlock()
		}
		return "", false
	}
	return s.Email, true
}

func adminAuthMiddleware(c fiber.Ctx) error {
	auth := c.Get("Authorization")
	token := strings.TrimPrefix(auth, "Bearer ")
	if token == "" || token == auth {
		return c.Status(401).JSON(fiber.Map{"success": false, "message": "Unauthorized"})
	}
	email, ok := validateAdminToken(token)
	if !ok {
		return c.Status(401).JSON(fiber.Map{"success": false, "message": "Invalid or expired token"})
	}
	c.Locals("admin_email", email)
	return c.Next()
}

func main() {
	godotenv.Load()

	if err := database.Connect(); err != nil {
		log.Printf("WARNING: database not connected: %v", err)
		log.Println("Orders will not persist to database")
	} else {
		log.Println("database connected")
		defer database.Close()
		cleanupStoreInfo()
		fixProductData()
	}

	app := fiber.New()

	app.Use(cors.New())

	app.Get("/*", static.New("./public", static.Config{
		CacheDuration: 10 * time.Second,
	}))

	app.Get("/cart", func(c fiber.Ctx) error {
		return c.SendFile("./public/cart.html")
	})

	app.Get("/health", func(c fiber.Ctx) error {
		dbOK := database.DB != nil
		return c.JSON(fiber.Map{"status": "ok", "database": dbOK})
	})

	api := app.Group("/api")

	admin := api.Group("/admin")

	admin.Post("/login", func(c fiber.Ctx) error {
		email := c.FormValue("email")
		pass := c.FormValue("password")
		if email == os.Getenv("ADMIN_USER") && pass == os.Getenv("ADMIN_PASS") {
			token := createAdminSession(email)
			return c.JSON(fiber.Map{"success": true, "message": "Login successful", "token": token})
		}
		return c.Status(401).JSON(fiber.Map{"success": false, "message": "Invalid credentials"})
	})

	admin.Use(adminAuthMiddleware)

	admin.Get("/dashboard", handlers.AdminDashboard)
	admin.Get("/orders", handlers.AdminOrders)
	admin.Get("/products", handlers.AdminProducts)
	admin.Get("/customers", handlers.AdminCustomers)
	admin.Get("/notifications", handlers.AdminNotifications)

	admin.Get("/brands", func(c fiber.Ctx) error {
		if database.DB == nil { return c.JSON(fiber.Map{"brands": []fiber.Map{}}) }
		rows, err := database.DB.Query(context.Background(), "SELECT id,name FROM brands WHERE is_active=true ORDER BY name")
		if err != nil { return c.JSON(fiber.Map{"brands": []fiber.Map{}}) }
		defer rows.Close()
		brands := []fiber.Map{}
		for rows.Next() { var id, name string; rows.Scan(&id, &name); brands = append(brands, fiber.Map{"id": id, "name": name}) }
		return c.JSON(fiber.Map{"brands": brands})
	})

	admin.Get("/categories", func(c fiber.Ctx) error {
		if database.DB == nil { return c.JSON(fiber.Map{"categories": []fiber.Map{}}) }
		rows, err := database.DB.Query(context.Background(), "SELECT id,name FROM categories WHERE is_active=true ORDER BY name")
		if err != nil { return c.JSON(fiber.Map{"categories": []fiber.Map{}}) }
		defer rows.Close()
		cats := []fiber.Map{}
		for rows.Next() { var id, name string; rows.Scan(&id, &name); cats = append(cats, fiber.Map{"id": id, "name": name}) }
		return c.JSON(fiber.Map{"categories": cats})
	})

	admin.Post("/brands", func(c fiber.Ctx) error {
		if database.DB == nil { return c.Status(503).JSON(fiber.Map{"error": "db not connected"}) }
		body := struct{ Name string }{}
		if err := c.Bind().JSON(&body); err != nil || body.Name == "" { return c.Status(400).JSON(fiber.Map{"error": "name required"}) }
		slug := strings.ToLower(strings.ReplaceAll(body.Name, " ", "-"))
		slug = strings.NewReplacer("--", "-", "'", "", "\"", "").Replace(slug)
		var id string
		err := database.DB.QueryRow(context.Background(), "INSERT INTO brands (name,slug) VALUES ($1,$2) RETURNING id", body.Name, slug).Scan(&id)
		if err != nil { return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("brand exists or failed: %v", err)}) }
		return c.JSON(fiber.Map{"success": true, "id": id})
	})

	admin.Post("/categories", func(c fiber.Ctx) error {
		if database.DB == nil { return c.Status(503).JSON(fiber.Map{"error": "db not connected"}) }
		body := struct{ Name string }{}
		if err := c.Bind().JSON(&body); err != nil || body.Name == "" { return c.Status(400).JSON(fiber.Map{"error": "name required"}) }
		slug := strings.ToLower(strings.ReplaceAll(body.Name, " ", "-"))
		slug = strings.NewReplacer("--", "-", "'", "", "\"", "").Replace(slug)
		var id string
		err := database.DB.QueryRow(context.Background(), "INSERT INTO categories (name,slug) VALUES ($1,$2) RETURNING id", body.Name, slug).Scan(&id)
		if err != nil { return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("category exists or failed: %v", err)}) }
		return c.JSON(fiber.Map{"success": true, "id": id})
	})

	admin.Get("/products/detail/:id", func(c fiber.Ctx) error {
		if database.DB == nil { return c.Status(503).JSON(fiber.Map{"error": "db not connected"}) }
		ctx := context.Background()
		type PD struct {
			ID,Name,Slug,Description,ShortDescription,Ingredients,HowToUse,SKU,Barcode,BrandID,CategoryID,CreatedAt,UpdatedAt string
			Price,CompareAtPrice,CostPrice int64; Stock,Reserved,Incoming,LowStockThreshold int; WeightGrams float64
			IsActive,IsFeatured bool
			Sections []string; Images []fiber.Map
		}
		var p PD
		var sid, cid *string; var w *float64
		err := database.DB.QueryRow(ctx, `SELECT id,name,slug,COALESCE(description,''),COALESCE(short_description,''),COALESCE(ingredients,''),COALESCE(how_to_use,''),COALESCE(sku,''),COALESCE(barcode,''),brand_id,category_id,price,COALESCE(compare_at_price,0),COALESCE(cost_price,0),stock,reserved,incoming,low_stock_threshold,weight_grams,is_active,is_featured,created_at,updated_at FROM products WHERE id=$1`, c.Params("id")).Scan(
			&p.ID,&p.Name,&p.Slug,&p.Description,&p.ShortDescription,&p.Ingredients,&p.HowToUse,&p.SKU,&p.Barcode,&sid,&cid,
			&p.Price,&p.CompareAtPrice,&p.CostPrice,&p.Stock,&p.Reserved,&p.Incoming,&p.LowStockThreshold,&w,&p.IsActive,&p.IsFeatured,&p.CreatedAt,&p.UpdatedAt)
		if err != nil { return c.Status(404).JSON(fiber.Map{"error": "not found"}) }
		if sid != nil { p.BrandID = *sid }; if cid != nil { p.CategoryID = *cid }
		if w != nil { p.WeightGrams = *w }

		irows, _ := database.DB.Query(ctx, "SELECT id,url,COALESCE(alt,''),sort_order FROM product_images WHERE product_id=$1 ORDER BY sort_order", c.Params("id"))
		if irows != nil { defer irows.Close(); for irows.Next() { var id,url,alt string; var so int; irows.Scan(&id,&url,&alt,&so); p.Images = append(p.Images, fiber.Map{"id":id,"url":url,"alt":alt,"sort":so}) } }

		sec := []string{}
		if p.Ingredients != "" { sec = append(sec, "ingredients") }
		if p.HowToUse != "" { sec = append(sec, "how_to_use") }
		p.Sections = sec
		return c.JSON(p)
	})

	admin.Post("/products", func(c fiber.Ctx) error {
		if database.DB == nil { return c.Status(503).JSON(fiber.Map{"error": "db not connected"}) }
		body := struct {
			Name,Description,ShortDescription,Ingredients,HowToUse,SKU,Barcode,BrandID,CategoryID,Sections string
			Price,CompareAtPrice,CostPrice int64; Stock,LowStockThreshold int; WeightGrams float64
			IsActive,IsFeatured bool; ImageURLs []string
		}{}
		if err := json.Unmarshal(c.Body(), &body); err != nil { return c.Status(400).JSON(fiber.Map{"error": "invalid body"}) }
		if body.Name == "" { return c.Status(400).JSON(fiber.Map{"error": "name required"}) }

		ctx := context.Background()
		slug := strings.ToLower(strings.ReplaceAll(body.Name, " ", "-"))
		slug = strings.NewReplacer("--", "-", "'", "", "\"", "", "(", "", ")", "").Replace(slug)
		slug = strings.Trim(slug, "-")
		if slug == "" { slug = "product" }

		// Build sections JSON based on which sections are enabled
		secMap := fiber.Map{}
		secParts := strings.Split(body.Sections, ",")
		for _, s := range secParts {
			s = strings.TrimSpace(s)
			if s == "" { continue }
			switch s {
			case "ingredients": if body.Ingredients != "" { secMap[s] = true }
			case "how_to_use": if body.HowToUse != "" { secMap[s] = true }
			}
		}

		var bid, cid *string
		if body.BrandID != "" { bid = &body.BrandID }
		if body.CategoryID != "" { cid = &body.CategoryID }

		var prodID string
		err := database.DB.QueryRow(ctx, `INSERT INTO products (name,slug,description,short_description,ingredients,how_to_use,sku,barcode,brand_id,category_id,price,compare_at_price,cost_price,stock,low_stock_threshold,weight_grams,is_active,is_featured) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18) RETURNING id`,
			body.Name,slug,body.Description,body.ShortDescription,body.Ingredients,body.HowToUse,body.SKU,body.Barcode,bid,cid,
			body.Price,body.CompareAtPrice,body.CostPrice,body.Stock,body.LowStockThreshold,body.WeightGrams,body.IsActive,body.IsFeatured).Scan(&prodID)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("failed to create: %v", err)})
		}

		// Insert images
		for i, url := range body.ImageURLs {
			database.DB.Exec(ctx, "INSERT INTO product_images (product_id,url,alt,sort_order) VALUES ($1,$2,$3,$4)", prodID, url, body.Name, i)
		}

		return c.JSON(fiber.Map{"success": true, "id": prodID, "slug": slug})
	})

	admin.Put("/products/:id", func(c fiber.Ctx) error {
		if database.DB == nil { return c.Status(503).JSON(fiber.Map{"error": "db not connected"}) }
		body := struct {
			Name,Description,ShortDescription,Ingredients,HowToUse,SKU,Barcode,BrandID,CategoryID,Sections string
			Price,CompareAtPrice,CostPrice int64; Stock,LowStockThreshold int; WeightGrams float64
			IsActive,IsFeatured bool; ImageURLs []string
		}{}
		if err := json.Unmarshal(c.Body(), &body); err != nil { return c.Status(400).JSON(fiber.Map{"error": "invalid body"}) }
		ctx := context.Background()
		slug := strings.ToLower(strings.ReplaceAll(body.Name, " ", "-"))
		slug = strings.NewReplacer("--", "-", "'", "", "\"", "", "(", "", ")", "").Replace(slug)
		slug = strings.Trim(slug, "-")

		var bid, cid *string
		if body.BrandID != "" { bid = &body.BrandID }
		if body.CategoryID != "" { cid = &body.CategoryID }

		tag, err := database.DB.Exec(ctx, `UPDATE products SET name=$1,slug=$2,description=$3,short_description=$4,ingredients=$5,how_to_use=$6,sku=$7,barcode=$8,brand_id=$9,category_id=$10,price=$11,compare_at_price=$12,cost_price=$13,stock=$14,low_stock_threshold=$15,weight_grams=$16,is_active=$17,is_featured=$18,updated_at=now() WHERE id=$19::uuid`,
			body.Name,slug,body.Description,body.ShortDescription,body.Ingredients,body.HowToUse,body.SKU,body.Barcode,bid,cid,
			body.Price,body.CompareAtPrice,body.CostPrice,body.Stock,body.LowStockThreshold,body.WeightGrams,body.IsActive,body.IsFeatured,c.Params("id"))
		if err != nil { return c.Status(500).JSON(fiber.Map{"error": err.Error()}) }
		log.Printf("UPDATE product %s: rows_affected=%d", c.Params("id"), tag.RowsAffected())

		handlers.CheckLowStock()

		// Replace images: delete existing, insert new
		database.DB.Exec(ctx, "DELETE FROM product_images WHERE product_id=$1", c.Params("id"))
		for i, url := range body.ImageURLs {
			database.DB.Exec(ctx, "INSERT INTO product_images (product_id,url,alt,sort_order) VALUES ($1,$2,$3,$4)", c.Params("id"), url, body.Name, i)
		}
		return c.JSON(fiber.Map{"success": true, "slug": slug})
	})

	admin.Post("/upload", func(c fiber.Ctx) error {
		file, err := c.FormFile("file")
		if err != nil { return c.Status(400).JSON(fiber.Map{"error": "no file"}) }
		src, err := file.Open()
		if err != nil { return c.Status(500).JSON(fiber.Map{"error": "cannot open"}) }
		defer src.Close()

		data, err := io.ReadAll(src)
		if err != nil { return c.Status(500).JSON(fiber.Map{"error": "cannot read"}) }

		ext := ""
		if idx := strings.LastIndex(file.Filename, "."); idx >= 0 { ext = file.Filename[idx:] }
		objectName := fmt.Sprintf("products/%d%s", time.Now().UnixNano(), ext)

		supabaseURL := os.Getenv("SUPABASE_URL")
		serviceRole := os.Getenv("SUPABASE_SERVICE_ROLE")
		bucket := "products"
		uploadURL := fmt.Sprintf("%s/storage/v1/object/%s/%s", supabaseURL, bucket, objectName)

		req, _ := http.NewRequest("POST", uploadURL, bytes.NewReader(data))
		req.Header.Set("Authorization", "Bearer "+serviceRole)
		req.Header.Set("Content-Type", file.Header.Get("Content-Type"))

		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Do(req)
		if err != nil { return c.Status(500).JSON(fiber.Map{"error": "upload failed: " + err.Error()}) }
		defer resp.Body.Close()

		if resp.StatusCode >= 300 {
			respBody, _ := io.ReadAll(resp.Body)
			return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("supabase error %d: %s", resp.StatusCode, string(respBody))})
		}

		publicURL := fmt.Sprintf("%s/storage/v1/object/public/%s/%s", supabaseURL, bucket, objectName)
		return c.JSON(fiber.Map{"success": true, "url": publicURL})
	})

	// Store info (public) — reads from DB directly (no cache to avoid stale images)
	api.Get("/store-info", func(c fiber.Ctx) error {
		s := loadStoreFromDB()
		imgs := s.Images
		if imgs == nil { imgs = []string{} }
		promo := s.PromoBanner
		if promo == nil { promo = map[string]interface{}{} }
		return c.JSON(fiber.Map{
			"store_name": s.StoreName, "owner_name": s.OwnerName, "phone": s.Phone,
			"location": s.Location, "location_link": s.LocationLink,
			"store_description": s.StoreDescription,
			"instagram": s.Instagram, "tiktok": s.TikTok, "images": imgs,
			"promo_banner": promo,
		})
	})

	// Admin store-info update — uses upsert, returns DB errors
	admin.Put("/store-info", func(c fiber.Ctx) error {
		body := struct {
			StoreName        string                 `json:"store_name"`
			OwnerName        string                 `json:"owner_name"`
			Phone            string                 `json:"phone"`
			Location         string                 `json:"location"`
			LocationLink     string                 `json:"location_link"`
			StoreDescription string                 `json:"store_description"`
			Instagram        string                 `json:"instagram"`
			TikTok           string                 `json:"tiktok"`
			Images           []string               `json:"images"`
			PromoBanner      map[string]interface{} `json:"promo_banner"`
		}{}
		if err := c.Bind().JSON(&body); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid body"})
		}
		imgJSON, _ := json.Marshal(body.Images)
		imgStr := string(imgJSON)
		promoJSON, _ := json.Marshal(body.PromoBanner)
		promoStr := string(promoJSON)

		if database.DB == nil {
			return c.Status(503).JSON(fiber.Map{"error": "database not connected"})
		}

		tag, err := database.DB.Exec(context.Background(),
			`UPDATE store_info SET store_name=$1,owner_name=$2,phone=$3,location=$4,location_link=$5,store_description=$6,instagram=$7,tiktok=$8,images=$9,promo_banner=$10,updated_at=now()
			 WHERE id=(SELECT id FROM store_info ORDER BY updated_at DESC LIMIT 1)`,
			body.StoreName, body.OwnerName, body.Phone, body.Location, body.LocationLink, body.StoreDescription, body.Instagram, body.TikTok, imgStr, promoStr)
		if err != nil {
			log.Printf("store-info update error: %v", err)
			return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("database write failed: %v", err)})
		}
		if tag.RowsAffected() == 0 {
			_, err = database.DB.Exec(context.Background(),
				`INSERT INTO store_info (store_name,owner_name,phone,location,location_link,store_description,instagram,tiktok,images,promo_banner,updated_at)
				 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,now())`,
				body.StoreName, body.OwnerName, body.Phone, body.Location, body.LocationLink, body.StoreDescription, body.Instagram, body.TikTok, imgStr, promoStr)
			if err != nil {
				log.Printf("store-info insert error: %v", err)
				return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("database write failed: %v", err)})
			}
		}
		return c.JSON(fiber.Map{"success": true})
	})

	// ===== PACKS (admin CRUD) =====
	// Auto-delete expired packs, then return remaining
	admin.Get("/packs", func(c fiber.Ctx) error {
		if database.DB == nil { return c.Status(503).JSON(fiber.Map{"error": "db not connected"}) }
		ctx := context.Background()
		database.DB.Exec(ctx, `DELETE FROM packs WHERE expires_at < now()`)
		rows, err := database.DB.Query(ctx, `SELECT id,name,description,price,original_price,COALESCE(image,''),product_ids::text,days_valid,expires_at,is_active FROM packs ORDER BY created_at DESC`)
		if err != nil { return c.Status(500).JSON(fiber.Map{"error": err.Error()}) }
		defer rows.Close()
		type Pack struct {
			ID            string   `json:"id"`
			Name          string   `json:"name"`
			Description   string   `json:"description"`
			Price         int64    `json:"price"`
			OriginalPrice int64    `json:"original_price"`
			Image         string   `json:"image"`
			ProductIDs    []string `json:"product_ids"`
			DaysValid     int      `json:"days_valid"`
			ExpiresAt     string   `json:"expires_at"`
			IsActive      bool     `json:"is_active"`
		}
		out := []Pack{}
		for rows.Next() {
			var p Pack
			var pidJSON, expiresStr string
			rows.Scan(&p.ID, &p.Name, &p.Description, &p.Price, &p.OriginalPrice, &p.Image, &pidJSON, &p.DaysValid, &expiresStr, &p.IsActive)
			json.Unmarshal([]byte(pidJSON), &p.ProductIDs)
			if p.ProductIDs == nil { p.ProductIDs = []string{} }
			if t, err := time.Parse(time.RFC3339, expiresStr); err == nil { p.ExpiresAt = t.Format("2006-01-02T15:04:05Z07:00") }
			out = append(out, p)
		}
		return c.JSON(out)
	})

	admin.Post("/packs", func(c fiber.Ctx) error {
		if database.DB == nil { return c.Status(503).JSON(fiber.Map{"error": "db not connected"}) }
		body := struct {
			Name        string   `json:"name"`
			Description string   `json:"description"`
			Price       int64    `json:"price"`
			Image       string   `json:"image"`
			ProductIDs  []string `json:"product_ids"`
			DaysValid   int      `json:"days_valid"`
		}{}
		if err := c.Bind().JSON(&body); err != nil { return c.Status(400).JSON(fiber.Map{"error": "invalid body"}) }
		if body.DaysValid < 1 { body.DaysValid = 30 }
		pidJSON, _ := json.Marshal(body.ProductIDs)
		var id string
		err := database.DB.QueryRow(context.Background(),
			`INSERT INTO packs (name,description,price,original_price,image,product_ids,days_valid,expires_at)
			 VALUES ($1,$2,$3,0,$4,$5,$6,now()+($6||' days')::interval) RETURNING id`,
			body.Name, body.Description, body.Price, body.Image, string(pidJSON), body.DaysValid,
		).Scan(&id)
		if err != nil { return c.Status(500).JSON(fiber.Map{"error": err.Error()}) }
		return c.JSON(fiber.Map{"success": true, "id": id})
	})

	admin.Put("/packs/:id", func(c fiber.Ctx) error {
		if database.DB == nil { return c.Status(503).JSON(fiber.Map{"error": "db not connected"}) }
		id := c.Params("id")
		body := struct {
			Name        string   `json:"name"`
			Description string   `json:"description"`
			Price       int64    `json:"price"`
			Image       string   `json:"image"`
			ProductIDs  []string `json:"product_ids"`
			DaysValid   int      `json:"days_valid"`
		}{}
		if err := c.Bind().JSON(&body); err != nil { return c.Status(400).JSON(fiber.Map{"error": "invalid body"}) }
		if body.DaysValid < 1 { body.DaysValid = 30 }
		pidJSON, _ := json.Marshal(body.ProductIDs)
		_, err := database.DB.Exec(context.Background(),
			`UPDATE packs SET name=$1,description=$2,price=$3,image=$4,product_ids=$5,days_valid=$6,expires_at=now()+($6||' days')::interval,updated_at=now() WHERE id=$7`,
			body.Name, body.Description, body.Price, body.Image, string(pidJSON), body.DaysValid, id,
		)
		if err != nil { return c.Status(500).JSON(fiber.Map{"error": err.Error()}) }
		return c.JSON(fiber.Map{"success": true})
	})

	admin.Delete("/packs/:id", func(c fiber.Ctx) error {
		if database.DB == nil { return c.Status(503).JSON(fiber.Map{"error": "db not connected"}) }
		_, err := database.DB.Exec(context.Background(), `DELETE FROM packs WHERE id=$1`, c.Params("id"))
		if err != nil { return c.Status(500).JSON(fiber.Map{"error": err.Error()}) }
		return c.JSON(fiber.Map{"success": true})
	})

	// Product listing
	api.Get("/products", func(c fiber.Ctx) error {
		if database.DB == nil { return c.Status(503).JSON(fiber.Map{"error": "db not connected"}) }
		ctx := context.Background()
		cat := c.Query("category")
		where := "WHERE p.is_active=true"
		if cat != "" { where += " AND c.slug=$1" }
		q := `SELECT p.id,p.name,p.slug,p.price,COALESCE(p.compare_at_price,0),COALESCE(ROUND(AVG(r.rating),1),0),COUNT(r.id),COALESCE(c.name,''),COALESCE(c.slug,''),COALESCE(b.name,''),COALESCE((SELECT url FROM product_images WHERE product_id=p.id ORDER BY sort_order LIMIT 1),'') FROM products p LEFT JOIN categories c ON c.id=p.category_id LEFT JOIN brands b ON b.id=p.brand_id LEFT JOIN reviews r ON r.product_id=p.id ` + where + ` GROUP BY p.id,c.name,c.slug,b.name ORDER BY p.created_at DESC`
		var rows interface{ Close(); Next() bool; Scan(...interface{}) error }
		var err error
		if cat != "" {
			r, e := database.DB.Query(ctx, q, cat)
			rows, err = r, e
		} else {
			r, e := database.DB.Query(ctx, q)
			rows, err = r, e
		}
		if err != nil { return c.Status(500).JSON(fiber.Map{"error": err.Error()}) }
		defer rows.Close()
		type LP struct {
			ID             string `json:"id"`
			Name           string `json:"name"`
			Slug           string `json:"slug"`
			Category       string `json:"category"`
			CategorySlug   string `json:"categorySlug"`
			Brand          string `json:"brand"`
			Image          string `json:"image"`
			Price          int64   `json:"price"`
			CompareAtPrice int64   `json:"compareAtPrice"`
			Rating         float64 `json:"rating"`
			Reviews        int     `json:"reviews"`
		}
		prods := []LP{}
		for rows.Next() {
			var p LP
			rows.Scan(&p.ID,&p.Name,&p.Slug,&p.Price,&p.CompareAtPrice,&p.Rating,&p.Reviews,&p.Category,&p.CategorySlug,&p.Brand,&p.Image)
			prods = append(prods, p)
		}
		return c.JSON(prods)
	})

	// Public packs listing
	api.Get("/packs", func(c fiber.Ctx) error {
		if database.DB == nil { return c.Status(503).JSON(fiber.Map{"error": "db not connected"}) }
		ctx := context.Background()
		database.DB.Exec(ctx, `DELETE FROM packs WHERE expires_at < now()`)
		rows, err := database.DB.Query(ctx, `SELECT id,name,description,price,original_price,COALESCE(image,''),product_ids::text,days_valid,expires_at FROM packs WHERE is_active=true ORDER BY created_at DESC`)
		if err != nil { return c.Status(500).JSON(fiber.Map{"error": err.Error()}) }
		defer rows.Close()
		type Pack struct {
			ID            string   `json:"id"`
			Name          string   `json:"name"`
			Description   string   `json:"description"`
			Price         int64    `json:"price"`
			OriginalPrice int64    `json:"original_price"`
			Image         string   `json:"image"`
			ProductIDs    []string `json:"product_ids"`
			DaysValid     int      `json:"days_valid"`
			ExpiresAt     string   `json:"expires_at"`
		}
		out := []Pack{}
		for rows.Next() {
			var p Pack
			var pidJSON, expiresStr string
			rows.Scan(&p.ID, &p.Name, &p.Description, &p.Price, &p.OriginalPrice, &p.Image, &pidJSON, &p.DaysValid, &expiresStr)
			json.Unmarshal([]byte(pidJSON), &p.ProductIDs)
			if p.ProductIDs == nil { p.ProductIDs = []string{} }
			if t, err := time.Parse(time.RFC3339, expiresStr); err == nil { p.ExpiresAt = t.Format("2006-01-02T15:04:05Z07:00") }
			out = append(out, p)
		}
		return c.JSON(out)
	})

	// Public product detail
	api.Get("/products/:slug", func(c fiber.Ctx) error {
		if database.DB == nil { return c.Status(503).JSON(fiber.Map{"error": "db not connected"}) }
		ctx := context.Background()
		type PubP struct {
			ID             string     `json:"id"`
			Name           string     `json:"name"`
			Slug           string     `json:"slug"`
			Description    string     `json:"description"`
			Brand          string     `json:"brand"`
			Category       string     `json:"category"`
			CreatedAt      string     `json:"createdAt"`
			Image          string     `json:"image"`
			Ingredients    string     `json:"ingredients"`
			HowToUse       string     `json:"howToUse"`
			Price          int64      `json:"price"`
			CompareAtPrice int64      `json:"compareAtPrice"`
			Stock          int        `json:"stock"`
			IsActive       bool       `json:"isActive"`
			Rating         float64    `json:"rating"`
			ReviewCount    int        `json:"reviewCount"`
			Sections       []fiber.Map `json:"sections"`
			Related        []fiber.Map `json:"related"`
		}
		var p PubP
		var ing, htu *string
		err := database.DB.QueryRow(ctx, `SELECT p.id,p.name,p.slug,COALESCE(p.description,''),p.ingredients,p.how_to_use,p.price,COALESCE(p.compare_at_price,0),p.stock,p.is_active,COALESCE(ROUND(AVG(r.rating),1),0),COUNT(r.id),COALESCE(b.name,''),COALESCE(c.name,''),COALESCE((SELECT url FROM product_images WHERE product_id=p.id ORDER BY sort_order LIMIT 1),'') FROM products p LEFT JOIN brands b ON b.id=p.brand_id LEFT JOIN categories c ON c.id=p.category_id LEFT JOIN reviews r ON r.product_id=p.id WHERE p.slug=$1 GROUP BY p.id,b.name,c.name`, c.Params("slug")).Scan(
			&p.ID,&p.Name,&p.Slug,&p.Description,&ing,&htu,&p.Price,&p.CompareAtPrice,&p.Stock,&p.IsActive,&p.Rating,&p.ReviewCount,&p.Brand,&p.Category,&p.Image)
		if err != nil { return c.Status(404).JSON(fiber.Map{"error": "not found"}) }
		if ing != nil { p.Ingredients = *ing }; if htu != nil { p.HowToUse = *htu }

		if p.Image == "" { p.Image = "https://placehold.co/600x600/f5f5f6/0d0d0d?text=Product" }

		// Build dynamic sections
		if p.Description != "" { p.Sections = append(p.Sections, fiber.Map{"title":"DESCRIPTION","content":p.Description}) }
		if p.Ingredients != "" { p.Sections = append(p.Sections, fiber.Map{"title":"INGREDIENTS","content":"<ul><li>"+strings.ReplaceAll(p.Ingredients, "\n", "</li><li>")+"</li></ul>"}) }
		if p.HowToUse != "" { p.Sections = append(p.Sections, fiber.Map{"title":"HOW TO USE","content":"<ul><li>"+strings.ReplaceAll(p.HowToUse, "\n", "</li><li>")+"</li></ul>"}) }

		// Related products
		if p.Category != "" {
			rrows, _ := database.DB.Query(ctx, "SELECT p.id,p.name,p.price,COALESCE(ROUND(AVG(r.rating),1),0),COUNT(r.id),COALESCE((SELECT url FROM product_images WHERE product_id=p.id ORDER BY sort_order LIMIT 1),'') FROM products p LEFT JOIN reviews r ON r.product_id=p.id WHERE p.is_active=true AND p.id!=$1 AND (p.category_id=(SELECT category_id FROM products WHERE id=$1)) GROUP BY p.id LIMIT 6", p.ID)
			if rrows != nil { defer rrows.Close(); for rrows.Next() { var id,name,img string; var price int64; var rating float64; var revs int; rrows.Scan(&id,&name,&price,&rating,&revs,&img); p.Related = append(p.Related, fiber.Map{"id":id,"name":name,"price":price,"rating":rating,"reviews":revs,"image":img}) } }
		}

		return c.JSON(p)
	})

	api.Post("/cart/add", func(c fiber.Ctx) error {
		return c.JSON(fiber.Map{"success": true, "message": "Added to cart"})
	})

	// Create order
	api.Post("/orders", func(c fiber.Ctx) error {
		var req OrderRequest
		if err := c.Bind().JSON(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"success": false, "message": "Invalid request body"})
		}

		if req.FirstName == "" || req.LastName == "" || req.Phone == "" || req.Address == "" || req.Wilaya == "" || req.City == "" {
			return c.Status(400).JSON(fiber.Map{"success": false, "message": "Missing required fields"})
		}

		if req.PaymentMethod == "" {
			req.PaymentMethod = "cod"
		}

		if database.DB == nil {
			return c.JSON(fiber.Map{"success": true, "orderId": "DEMO-" + req.Phone, "message": "Order placed (demo mode)"})
		}

		ctx := context.Background()
		tx, err := database.DB.Begin(ctx)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"success": false, "message": "Failed to start transaction"})
		}
		defer tx.Rollback(ctx)

		var orderID string
		err = tx.QueryRow(ctx,
			`INSERT INTO orders (first_name, last_name, phone, email, address, wilaya, city, notes, payment_method, subtotal, shipping_cost, discount, total)
			 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13) RETURNING id`,
			req.FirstName, req.LastName, req.Phone, req.Email,
			req.Address, req.Wilaya, req.City, req.Notes,
			req.PaymentMethod, req.Subtotal, req.ShippingCost, req.Discount, req.Total,
		).Scan(&orderID)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"success": false, "message": "Failed to create order"})
		}

		for _, item := range req.Items {
			_, err := tx.Exec(ctx,
				`INSERT INTO order_items (order_id, product_id, product_name, product_brand, variant, image_url, price, quantity)
				 VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
				orderID, item.ProductID, item.ProductName, item.Brand, item.Variant, item.ImageURL, item.Price, item.Quantity,
			)
			if err != nil {
				return c.Status(500).JSON(fiber.Map{"success": false, "message": "Failed to save order items"})
			}
		}

		if err := tx.Commit(ctx); err != nil {
			return c.Status(500).JSON(fiber.Map{"success": false, "message": "Failed to commit order"})
		}

		return c.JSON(fiber.Map{"success": true, "orderId": orderID, "message": "Order placed successfully"})
	})

	// Get single order
	api.Get("/orders/:id", func(c fiber.Ctx) error {
		orderID := c.Params("id")
		if database.DB == nil {
			return c.Status(503).JSON(fiber.Map{"success": false, "message": "Database not connected"})
		}

		ctx := context.Background()
		var order struct {
			ID            string `json:"id"`
			OrderNumber   int    `json:"orderNumber"`
			Status        string `json:"status"`
			PaymentMethod string `json:"paymentMethod"`
			FirstName     string `json:"firstName"`
			LastName      string `json:"lastName"`
			Phone         string `json:"phone"`
			Address       string `json:"address"`
			Wilaya        string `json:"wilaya"`
			City          string `json:"city"`
			Total         int64  `json:"total"`
			CreatedAt     string `json:"createdAt"`
		}

		err := database.DB.QueryRow(ctx,
			`SELECT id, order_number, status, payment_method, first_name, last_name, phone, address, wilaya, city, total, created_at
			 FROM orders WHERE id = $1`, orderID,
		).Scan(&order.ID, &order.OrderNumber, &order.Status, &order.PaymentMethod,
			&order.FirstName, &order.LastName, &order.Phone,
			&order.Address, &order.Wilaya, &order.City, &order.Total, &order.CreatedAt)
		if err != nil {
			return c.Status(404).JSON(fiber.Map{"success": false, "message": "Order not found"})
		}

		return c.JSON(fiber.Map{"success": true, "order": order})
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("server starting on :%s", port)
	log.Fatal(app.Listen(":" + port))
}
