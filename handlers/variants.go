package handlers

import (
	"context"
	"encoding/json"

	"github.com/gofiber/fiber/v3"
	"github.com/yourorg/ecommerce/database"
)

func GetProductVariants(c fiber.Ctx) error {
	if database.DB == nil {
		return c.JSON(fiber.Map{"variants": []fiber.Map{}})
	}
	productID := c.Params("id")
	rows, err := database.DB.Query(context.Background(),
		`SELECT id,product_id,name,sku,price,stock,is_active FROM product_variants WHERE product_id=$1 ORDER BY name`, productID)
	if err != nil {
		return c.JSON(fiber.Map{"variants": []fiber.Map{}})
	}
	defer rows.Close()
	type V struct {
		ID        string `json:"id"`
		ProductID string `json:"productId"`
		Name      string `json:"name"`
		SKU       string `json:"sku"`
		Price     int64  `json:"price"`
		Stock     int    `json:"stock"`
		IsActive  bool   `json:"is_active"`
	}
	variants := []V{}
	for rows.Next() {
		var v V
		rows.Scan(&v.ID, &v.ProductID, &v.Name, &v.SKU, &v.Price, &v.Stock, &v.IsActive)
		variants = append(variants, v)
	}
	return c.JSON(fiber.Map{"variants": variants})
}

func AdminCreateVariant(c fiber.Ctx) error {
	if database.DB == nil {
		return c.Status(503).JSON(fiber.Map{"error": "database not connected"})
	}
	body := struct {
		ProductID string `json:"product_id"`
		Name      string `json:"name"`
		SKU       string `json:"sku"`
		Price     int64  `json:"price"`
		Stock     int    `json:"stock"`
	}{}
	if err := json.Unmarshal(c.Body(), &body); err != nil || body.ProductID == "" || body.Name == "" {
		return c.Status(400).JSON(fiber.Map{"error": "product_id and name required"})
	}
	var id string
	err := database.DB.QueryRow(context.Background(),
		`INSERT INTO product_variants (product_id,name,sku,price,stock) VALUES ($1,$2,$3,$4,$5) RETURNING id`,
		body.ProductID, body.Name, body.SKU, body.Price, body.Stock).Scan(&id)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to create variant"})
	}
	return c.JSON(fiber.Map{"success": true, "id": id})
}

func AdminUpdateVariant(c fiber.Ctx) error {
	if database.DB == nil {
		return c.Status(503).JSON(fiber.Map{"error": "database not connected"})
	}
	id := c.Params("id")
	body := struct {
		Name     string `json:"name"`
		SKU      string `json:"sku"`
		Price    *int64 `json:"price"`
		Stock    *int   `json:"stock"`
		IsActive *bool  `json:"is_active"`
	}{}
	if err := json.Unmarshal(c.Body(), &body); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid body"})
	}
	ctx := context.Background()
	if body.Name != "" {
		database.DB.Exec(ctx, `UPDATE product_variants SET name=$1 WHERE id=$2`, body.Name, id)
	}
	if body.SKU != "" {
		database.DB.Exec(ctx, `UPDATE product_variants SET sku=$1 WHERE id=$2`, body.SKU, id)
	}
	if body.Price != nil {
		database.DB.Exec(ctx, `UPDATE product_variants SET price=$1 WHERE id=$2`, *body.Price, id)
	}
	if body.Stock != nil {
		database.DB.Exec(ctx, `UPDATE product_variants SET stock=$1 WHERE id=$2`, *body.Stock, id)
	}
	if body.IsActive != nil {
		database.DB.Exec(ctx, `UPDATE product_variants SET is_active=$1 WHERE id=$2`, *body.IsActive, id)
	}
	return c.JSON(fiber.Map{"success": true})
}

func AdminDeleteVariant(c fiber.Ctx) error {
	if database.DB == nil {
		return c.Status(503).JSON(fiber.Map{"error": "database not connected"})
	}
	tag, err := database.DB.Exec(context.Background(), `DELETE FROM product_variants WHERE id=$1`, c.Params("id"))
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to delete variant"})
	}
	if tag.RowsAffected() == 0 {
		return c.Status(404).JSON(fiber.Map{"error": "not found"})
	}
	return c.JSON(fiber.Map{"success": true})
}
