package handlers

import (
	"context"
	"encoding/json"

	"github.com/gofiber/fiber/v3"
	"github.com/yourorg/ecommerce/database"
)

func AdminGetStaff(c fiber.Ctx) error {
	if database.DB == nil {
		return c.Status(503).JSON(fiber.Map{"error": "database not connected"})
	}
	rows, err := database.DB.Query(context.Background(), `SELECT id,name,role,phone,is_active,created_at FROM staff ORDER BY created_at DESC`)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "database query failed"})
	}
	defer rows.Close()
	type S struct {
		ID        string `json:"id"`
		Name      string `json:"name"`
		Role      string `json:"role"`
		Phone     string `json:"phone"`
		IsActive  bool   `json:"is_active"`
		CreatedAt string `json:"created_at"`
	}
	staff := []S{}
	for rows.Next() {
		var s S
		rows.Scan(&s.ID, &s.Name, &s.Role, &s.Phone, &s.IsActive, &s.CreatedAt)
		staff = append(staff, s)
	}
	return c.JSON(fiber.Map{"staff": staff})
}

func AdminCreateStaff(c fiber.Ctx) error {
	if database.DB == nil {
		return c.Status(503).JSON(fiber.Map{"error": "database not connected"})
	}
	body := struct {
		Name  string `json:"name"`
		Role  string `json:"role"`
		Phone string `json:"phone"`
	}{}
	if err := json.Unmarshal(c.Body(), &body); err != nil || body.Name == "" {
		return c.Status(400).JSON(fiber.Map{"error": "name required"})
	}
	if body.Role == "" { body.Role = "staff" }
	var id string
	err := database.DB.QueryRow(context.Background(),
		`INSERT INTO staff (name,role,phone) VALUES ($1,$2,$3) RETURNING id`,
		body.Name, body.Role, body.Phone).Scan(&id)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to create staff"})
	}
	return c.JSON(fiber.Map{"success": true, "id": id})
}

func AdminUpdateStaff(c fiber.Ctx) error {
	if database.DB == nil {
		return c.Status(503).JSON(fiber.Map{"error": "database not connected"})
	}
	id := c.Params("id")
	body := struct {
		Name     string `json:"name"`
		Role     string `json:"role"`
		Phone    string `json:"phone"`
		IsActive *bool  `json:"is_active"`
	}{}
	if err := json.Unmarshal(c.Body(), &body); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid body"})
	}
	ctx := context.Background()
	if body.Name != "" {
		database.DB.Exec(ctx, `UPDATE staff SET name=$1 WHERE id=$2`, body.Name, id)
	}
	if body.Role != "" {
		database.DB.Exec(ctx, `UPDATE staff SET role=$1 WHERE id=$2`, body.Role, id)
	}
	if body.Phone != "" {
		database.DB.Exec(ctx, `UPDATE staff SET phone=$1 WHERE id=$2`, body.Phone, id)
	}
	if body.IsActive != nil {
		database.DB.Exec(ctx, `UPDATE staff SET is_active=$1 WHERE id=$2`, *body.IsActive, id)
	}
	return c.JSON(fiber.Map{"success": true})
}

func AdminDeleteStaff(c fiber.Ctx) error {
	if database.DB == nil {
		return c.Status(503).JSON(fiber.Map{"error": "database not connected"})
	}
	tag, err := database.DB.Exec(context.Background(), `DELETE FROM staff WHERE id=$1`, c.Params("id"))
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to delete staff"})
	}
	if tag.RowsAffected() == 0 {
		return c.Status(404).JSON(fiber.Map{"error": "not found"})
	}
	return c.JSON(fiber.Map{"success": true})
}
