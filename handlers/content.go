package handlers

import (
	"context"
	"encoding/json"

	"github.com/gofiber/fiber/v3"
	"github.com/yourorg/ecommerce/database"
)

func GetContentPages(c fiber.Ctx) error {
	if database.DB == nil {
		return c.JSON(fiber.Map{"pages": []fiber.Map{}})
	}
	rows, err := database.DB.Query(context.Background(), `SELECT id,slug,title,COALESCE(body,''),is_active,updated_at FROM content WHERE is_active=true ORDER BY title`)
	if err != nil {
		return c.JSON(fiber.Map{"pages": []fiber.Map{}})
	}
	defer rows.Close()
	type P struct {
		ID        string `json:"id"`
		Slug      string `json:"slug"`
		Title     string `json:"title"`
		Body      string `json:"body"`
		IsActive  bool   `json:"is_active"`
		UpdatedAt string `json:"updated_at"`
	}
	pages := []P{}
	for rows.Next() {
		var p P
		rows.Scan(&p.ID, &p.Slug, &p.Title, &p.Body, &p.IsActive, &p.UpdatedAt)
		pages = append(pages, p)
	}
	return c.JSON(fiber.Map{"pages": pages})
}

func GetContentBySlug(c fiber.Ctx) error {
	if database.DB == nil {
		return c.Status(503).JSON(fiber.Map{"error": "database not connected"})
	}
	slug := c.Params("slug")
	var id, title, body string
	var updatedAt string
	err := database.DB.QueryRow(context.Background(),
		`SELECT id,title,body,updated_at FROM content WHERE slug=$1 AND is_active=true`, slug).
		Scan(&id, &title, &body, &updatedAt)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "not found"})
	}
	return c.JSON(fiber.Map{"id": id, "slug": slug, "title": title, "body": body, "updated_at": updatedAt})
}

func AdminGetContent(c fiber.Ctx) error {
	if database.DB == nil {
		return c.Status(503).JSON(fiber.Map{"error": "database not connected"})
	}
	rows, err := database.DB.Query(context.Background(), `SELECT id,slug,title,COALESCE(body,''),is_active,created_at,updated_at FROM content ORDER BY updated_at DESC`)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "database query failed"})
	}
	defer rows.Close()
	type P struct {
		ID        string `json:"id"`
		Slug      string `json:"slug"`
		Title     string `json:"title"`
		Body      string `json:"body"`
		IsActive  bool   `json:"is_active"`
		CreatedAt string `json:"created_at"`
		UpdatedAt string `json:"updated_at"`
	}
	pages := []P{}
	for rows.Next() {
		var p P
		rows.Scan(&p.ID, &p.Slug, &p.Title, &p.Body, &p.IsActive, &p.CreatedAt, &p.UpdatedAt)
		pages = append(pages, p)
	}
	return c.JSON(fiber.Map{"pages": pages})
}

func AdminUpsertContent(c fiber.Ctx) error {
	if database.DB == nil {
		return c.Status(503).JSON(fiber.Map{"error": "database not connected"})
	}
	body := struct {
		ID       string `json:"id"`
		Slug     string `json:"slug"`
		Title    string `json:"title"`
		Body     string `json:"body"`
		IsActive bool   `json:"is_active"`
	}{}
	if err := json.Unmarshal(c.Body(), &body); err != nil || body.Slug == "" || body.Title == "" {
		return c.Status(400).JSON(fiber.Map{"error": "slug and title required"})
	}
	ctx := context.Background()
	if body.ID != "" {
		_, err := database.DB.Exec(ctx, `UPDATE content SET slug=$1,title=$2,body=$3,is_active=$4,updated_at=now() WHERE id=$5`,
			body.Slug, body.Title, body.Body, body.IsActive, body.ID)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "failed to update"})
		}
		return c.JSON(fiber.Map{"success": true})
	}
	var id string
	err := database.DB.QueryRow(ctx, `INSERT INTO content (slug,title,body,is_active) VALUES ($1,$2,$3,$4) RETURNING id`,
		body.Slug, body.Title, body.Body, body.IsActive).Scan(&id)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to create"})
	}
	return c.JSON(fiber.Map{"success": true, "id": id})
}

func AdminDeleteContent(c fiber.Ctx) error {
	if database.DB == nil {
		return c.Status(503).JSON(fiber.Map{"error": "database not connected"})
	}
	tag, err := database.DB.Exec(context.Background(), `DELETE FROM content WHERE id=$1`, c.Params("id"))
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to delete"})
	}
	if tag.RowsAffected() == 0 {
		return c.Status(404).JSON(fiber.Map{"error": "not found"})
	}
	return c.JSON(fiber.Map{"success": true})
}
