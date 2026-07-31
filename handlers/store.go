package handlers

import (
	"context"
	"encoding/json"
	"log"

	"github.com/gofiber/fiber/v3"
	"github.com/yourorg/ecommerce/database"
)

type storeInfo struct {
	StoreName        string
	OwnerName        string
	Phone            string
	Location         string
	LocationLink     string
	StoreDescription string
	Instagram        string
	TikTok           string
	WhatsApp         string
	Images           []string
	PromoBanner      map[string]interface{}
}

func loadStoreFromDB() *storeInfo {
	s := &storeInfo{}
	if database.DB == nil {
		return s
	}
	var imagesJSON, promoJSON string
	err := database.DB.QueryRow(context.Background(),
		`SELECT COALESCE(store_name,''),COALESCE(owner_name,''),COALESCE(phone,''),COALESCE(location,''),COALESCE(location_link,''),COALESCE(store_description,''),COALESCE(instagram,''),COALESCE(tiktok,''),COALESCE(whatsapp,''),COALESCE(images::text,'[]'),COALESCE(promo_banner::text,'{}') FROM store_info ORDER BY updated_at DESC LIMIT 1`,
	).Scan(&s.StoreName, &s.OwnerName, &s.Phone, &s.Location, &s.LocationLink, &s.StoreDescription, &s.Instagram, &s.TikTok, &s.WhatsApp, &imagesJSON, &promoJSON)
	if err != nil {
		log.Printf("store db load: %v", err)
		return s
	}
	json.Unmarshal([]byte(imagesJSON), &s.Images)
	if s.PromoBanner == nil {
		s.PromoBanner = map[string]interface{}{}
	}
	json.Unmarshal([]byte(promoJSON), &s.PromoBanner)
	return s
}

func CleanupStoreInfo() {
	if database.DB == nil {
		return
	}
	var count int
	database.DB.QueryRow(context.Background(), "SELECT COUNT(*) FROM store_info").Scan(&count)
	if count <= 1 {
		return
	}
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

func FixProductData() {
	if database.DB == nil {
		return
	}
	ctx := context.Background()

	database.DB.Exec(ctx, `DELETE FROM products WHERE name ILIKE 'TEST%' AND stock=10 AND price=100`)

	tag, _ := database.DB.Exec(ctx, `UPDATE products SET is_active=true WHERE name ILIKE '%Gold Standard%' AND is_active=false`)
	if ta := tag.RowsAffected(); ta > 0 {
		log.Printf("Fixed %d Gold Standard product(s): set is_active=true", ta)
	}

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

func GetStoreInfo(c fiber.Ctx) error {
	s := loadStoreFromDB()
	imgs := s.Images
	if imgs == nil {
		imgs = []string{}
	}
	promo := s.PromoBanner
	if promo == nil {
		promo = map[string]interface{}{}
	}
	return c.JSON(fiber.Map{
		"store_name": s.StoreName, "owner_name": s.OwnerName, "phone": s.Phone,
		"location": s.Location, "location_link": s.LocationLink,
		"store_description": s.StoreDescription,
		"instagram": s.Instagram, "tiktok": s.TikTok, "whatsapp": s.WhatsApp, "images": imgs,
		"promo_banner": promo,
	})
}

func UpdateStoreInfo(c fiber.Ctx) error {
	body := struct {
		StoreName        string                 `json:"store_name"`
		OwnerName        string                 `json:"owner_name"`
		Phone            string                 `json:"phone"`
		Location         string                 `json:"location"`
		LocationLink     string                 `json:"location_link"`
		StoreDescription string                 `json:"store_description"`
		Instagram        string                 `json:"instagram"`
		TikTok           string                 `json:"tiktok"`
		WhatsApp         string                 `json:"whatsapp"`
		Images           []string               `json:"images"`
		PromoBanner      map[string]interface{} `json:"promo_banner"`
	}{}
	if err := c.Bind().JSON(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid body"})
	}
	if body.Images == nil {
		body.Images = []string{}
	}
	if body.PromoBanner == nil {
		body.PromoBanner = map[string]interface{}{}
	}
	imgJSON, _ := json.Marshal(body.Images)
	imgStr := string(imgJSON)
	promoJSON, _ := json.Marshal(body.PromoBanner)
	promoStr := string(promoJSON)

	if database.DB == nil {
		return c.Status(503).JSON(fiber.Map{"error": "database not connected"})
	}

	tag, err := database.DB.Exec(context.Background(),
		`UPDATE store_info SET store_name=$1,owner_name=$2,phone=$3,location=$4,location_link=$5,store_description=$6,instagram=$7,tiktok=$8,whatsapp=$9,images=$10,promo_banner=$11,updated_at=now()
		 WHERE id=(SELECT id FROM store_info ORDER BY updated_at DESC LIMIT 1)`,
		body.StoreName, body.OwnerName, body.Phone, body.Location, body.LocationLink, body.StoreDescription, body.Instagram, body.TikTok, body.WhatsApp, imgStr, promoStr)
	if err != nil {
		log.Printf("store-info update error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "database write failed"})
	}
	if tag.RowsAffected() == 0 {
		_, err = database.DB.Exec(context.Background(),
			`INSERT INTO store_info (store_name,owner_name,phone,location,location_link,store_description,instagram,tiktok,whatsapp,images,promo_banner,updated_at)
			 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,now())`,
			body.StoreName, body.OwnerName, body.Phone, body.Location, body.LocationLink, body.StoreDescription, body.Instagram, body.TikTok, body.WhatsApp, imgStr, promoStr)
		if err != nil {
			log.Printf("store-info insert error: %v", err)
			return c.Status(500).JSON(fiber.Map{"error": "database write failed"})
		}
	}
	return c.JSON(fiber.Map{"success": true})
}
