package handlers

import (
	"context"
	"encoding/json"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/yourorg/ecommerce/database"
)

type packJSON struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Description   string   `json:"description"`
	Price         int64    `json:"price"`
	OriginalPrice int64    `json:"original_price"`
	Image         string   `json:"image"`
	ProductIDs    []string `json:"product_ids"`
	DaysValid     int      `json:"days_valid"`
	ExpiresAt     string   `json:"expires_at"`
	IsActive      bool     `json:"is_active,omitempty"`
}

func AdminGetPacks(c fiber.Ctx) error {
	if database.DB == nil {
		return c.Status(503).JSON(fiber.Map{"error": "db not connected"})
	}
	ctx := context.Background()
	rows, err := database.DB.Query(ctx, `SELECT id,name,description,price,original_price,COALESCE(image,''),product_ids::text,days_valid,expires_at,is_active FROM packs ORDER BY created_at DESC`)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "database query failed"})
	}
	defer rows.Close()
	out := []packJSON{}
	for rows.Next() {
		var p packJSON
		var pidJSON, expiresStr string
		rows.Scan(&p.ID, &p.Name, &p.Description, &p.Price, &p.OriginalPrice, &p.Image, &pidJSON, &p.DaysValid, &expiresStr, &p.IsActive)
		json.Unmarshal([]byte(pidJSON), &p.ProductIDs)
		if p.ProductIDs == nil {
			p.ProductIDs = []string{}
		}
		if t, err := time.Parse(time.RFC3339, expiresStr); err == nil {
			p.ExpiresAt = t.Format("2006-01-02T15:04:05Z07:00")
		}
		out = append(out, p)
	}
	return c.JSON(out)
}

func CreatePack(c fiber.Ctx) error {
	if database.DB == nil {
		return c.Status(503).JSON(fiber.Map{"error": "db not connected"})
	}
	body := struct {
		Name        string   `json:"name"`
		Description string   `json:"description"`
		Price       int64    `json:"price"`
		Image       string   `json:"image"`
		ProductIDs  []string `json:"product_ids"`
		DaysValid   int      `json:"days_valid"`
	}{}
	if err := c.Bind().JSON(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid body"})
	}
	if body.DaysValid < 1 {
		body.DaysValid = 30
	}
	pidJSON, _ := json.Marshal(body.ProductIDs)
	var id string
	err := database.DB.QueryRow(context.Background(),
		`INSERT INTO packs (name,description,price,original_price,image,product_ids,days_valid,expires_at)
		 VALUES ($1,$2,$3,0,$4,$5,$6,now()+($6||' days')::interval) RETURNING id`,
		body.Name, body.Description, body.Price, body.Image, string(pidJSON), body.DaysValid,
	).Scan(&id)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to create pack"})
	}

	LogActivity("pack_created", "pack", id, body.Name)

	return c.JSON(fiber.Map{"success": true, "id": id})
}

func UpdatePack(c fiber.Ctx) error {
	if database.DB == nil {
		return c.Status(503).JSON(fiber.Map{"error": "db not connected"})
	}
	id := c.Params("id")
	body := struct {
		Name        string   `json:"name"`
		Description string   `json:"description"`
		Price       int64    `json:"price"`
		Image       string   `json:"image"`
		ProductIDs  []string `json:"product_ids"`
		DaysValid   int      `json:"days_valid"`
	}{}
	if err := c.Bind().JSON(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid body"})
	}
	if body.DaysValid < 1 {
		body.DaysValid = 30
	}
	pidJSON, _ := json.Marshal(body.ProductIDs)
	_, err := database.DB.Exec(context.Background(),
		`UPDATE packs SET name=$1,description=$2,price=$3,image=$4,product_ids=$5,days_valid=$6,expires_at=now()+($6||' days')::interval,updated_at=now() WHERE id=$7`,
		body.Name, body.Description, body.Price, body.Image, string(pidJSON), body.DaysValid, id,
	)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to update pack"})
	}
	return c.JSON(fiber.Map{"success": true})
}

func DeletePack(c fiber.Ctx) error {
	if database.DB == nil {
		return c.Status(503).JSON(fiber.Map{"error": "db not connected"})
	}
	_, err := database.DB.Exec(context.Background(), `DELETE FROM packs WHERE id=$1`, c.Params("id"))
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to delete pack"})
	}

	LogActivity("pack_deleted", "pack", c.Params("id"), "")

	return c.JSON(fiber.Map{"success": true})
}

func GetPacks(c fiber.Ctx) error {
	if database.DB == nil {
		return c.Status(503).JSON(fiber.Map{"error": "db not connected"})
	}
	ctx := context.Background()
	database.DB.Exec(ctx, `DELETE FROM packs WHERE expires_at < now()`)
	rows, err := database.DB.Query(ctx, `SELECT id,name,description,price,original_price,COALESCE(image,''),product_ids::text,days_valid,expires_at FROM packs WHERE is_active=true ORDER BY created_at DESC`)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "database query failed"})
	}
	defer rows.Close()
	out := []packJSON{}
	for rows.Next() {
		var p packJSON
		var pidJSON, expiresStr string
		rows.Scan(&p.ID, &p.Name, &p.Description, &p.Price, &p.OriginalPrice, &p.Image, &pidJSON, &p.DaysValid, &expiresStr)
		json.Unmarshal([]byte(pidJSON), &p.ProductIDs)
		if p.ProductIDs == nil {
			p.ProductIDs = []string{}
		}
		if t, err := time.Parse(time.RFC3339, expiresStr); err == nil {
			p.ExpiresAt = t.Format("2006-01-02T15:04:05Z07:00")
		}
		out = append(out, p)
	}
	return c.JSON(out)
}
