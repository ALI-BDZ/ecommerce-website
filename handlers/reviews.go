package handlers

import (
	"context"
	"strconv"

	"github.com/gofiber/fiber/v3"
	"github.com/yourorg/ecommerce/database"
)

func GetProductReviews(c fiber.Ctx) error {
	if database.DB == nil {
		return c.Status(503).JSON(fiber.Map{"error": "database not connected"})
	}
	productID := c.Params("id")
	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	if page < 1 { page = 1 }
	if limit < 1 || limit > 50 { limit = 20 }
	offset := (page - 1) * limit

	ctx := context.Background()
	var total int64
	database.DB.QueryRow(ctx, `SELECT COUNT(*) FROM reviews WHERE product_id=$1`, productID).Scan(&total)

	rows, err := database.DB.Query(ctx,
		`SELECT id, product_id, customer_name, rating, comment, is_approved, created_at FROM reviews WHERE product_id=$1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
		productID, limit, offset)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "database query failed"})
	}
	defer rows.Close()

	type R struct {
		ID           string `json:"id"`
		ProductID    string `json:"productId"`
		CustomerName string `json:"customerName"`
		Rating       int    `json:"rating"`
		Comment      string `json:"comment"`
		IsApproved   bool   `json:"isApproved"`
		CreatedAt    string `json:"createdAt"`
	}
	reviews := []R{}
	for rows.Next() {
		var r R
		rows.Scan(&r.ID, &r.ProductID, &r.CustomerName, &r.Rating, &r.Comment, &r.IsApproved, &r.CreatedAt)
		if r.IsApproved {
			reviews = append(reviews, r)
		}
	}
	return c.JSON(fiber.Map{"reviews": reviews, "total": total, "page": page, "limit": limit})
}

func SubmitReview(c fiber.Ctx) error {
	if database.DB == nil {
		return c.Status(503).JSON(fiber.Map{"error": "database not connected"})
	}
	body := struct {
		ProductID    string `json:"productId"`
		CustomerName string `json:"customerName"`
		Rating       int    `json:"rating"`
		Comment      string `json:"comment"`
	}{}
	if err := c.Bind().JSON(&body); err != nil || body.ProductID == "" || body.CustomerName == "" {
		return c.Status(400).JSON(fiber.Map{"error": "productId and customerName required"})
	}
	if body.Rating < 1 || body.Rating > 5 {
		return c.Status(400).JSON(fiber.Map{"error": "rating must be 1-5"})
	}

	ctx := context.Background()
	var exists int
	database.DB.QueryRow(ctx, `SELECT COUNT(*) FROM products WHERE id=$1::uuid AND is_active=true`, body.ProductID).Scan(&exists)
	if exists == 0 {
		return c.Status(404).JSON(fiber.Map{"error": "product not found"})
	}

	var id string
	err := database.DB.QueryRow(ctx,
		`INSERT INTO reviews (product_id, customer_name, rating, comment, is_approved) VALUES ($1,$2,$3,$4,false) RETURNING id`,
		body.ProductID, body.CustomerName, body.Rating, body.Comment).Scan(&id)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to submit review"})
	}
	return c.JSON(fiber.Map{"success": true, "id": id, "message": "Review submitted, pending approval"})
}

func AdminApproveReview(c fiber.Ctx) error {
	if database.DB == nil {
		return c.Status(503).JSON(fiber.Map{"error": "database not connected"})
	}
	id := c.Params("id")
	body := struct {
		IsApproved bool `json:"isApproved"`
	}{}
	if err := c.Bind().JSON(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid body"})
	}
	tag, err := database.DB.Exec(context.Background(), `UPDATE reviews SET is_approved=$1 WHERE id=$2`, body.IsApproved, id)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to update review"})
	}
	if tag.RowsAffected() == 0 {
		return c.Status(404).JSON(fiber.Map{"error": "not found"})
	}
	return c.JSON(fiber.Map{"success": true})
}

func AdminGetReviews(c fiber.Ctx) error {
	if database.DB == nil {
		return c.Status(503).JSON(fiber.Map{"error": "database not connected"})
	}
	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	if page < 1 { page = 1 }
	if limit < 1 || limit > 50 { limit = 20 }
	offset := (page - 1) * limit

	ctx := context.Background()
	var total int64
	database.DB.QueryRow(ctx, `SELECT COUNT(*) FROM reviews`).Scan(&total)

	rows, err := database.DB.Query(ctx,
		`SELECT id, product_id, customer_name, rating, comment, is_approved, created_at FROM reviews ORDER BY created_at DESC LIMIT $1 OFFSET $2`,
		limit, offset)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "database query failed"})
	}
	defer rows.Close()
	type R struct {
		ID           string `json:"id"`
		ProductID    string `json:"productId"`
		CustomerName string `json:"customerName"`
		Rating       int    `json:"rating"`
		Comment      string `json:"comment"`
		IsApproved   bool   `json:"isApproved"`
		CreatedAt    string `json:"createdAt"`
	}
	reviews := []R{}
	for rows.Next() {
		var r R
		rows.Scan(&r.ID, &r.ProductID, &r.CustomerName, &r.Rating, &r.Comment, &r.IsApproved, &r.CreatedAt)
		reviews = append(reviews, r)
	}
	return c.JSON(fiber.Map{"reviews": reviews, "total": total, "page": page, "limit": limit})
}

func AdminDeleteReview(c fiber.Ctx) error {
	if database.DB == nil {
		return c.Status(503).JSON(fiber.Map{"error": "database not connected"})
	}
	tag, err := database.DB.Exec(context.Background(), `DELETE FROM reviews WHERE id=$1`, c.Params("id"))
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to delete review"})
	}
	if tag.RowsAffected() == 0 {
		return c.Status(404).JSON(fiber.Map{"error": "not found"})
	}
	return c.JSON(fiber.Map{"success": true})
}
