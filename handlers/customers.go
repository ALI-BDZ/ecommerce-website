package handlers

import (
	"context"
	"strconv"

	"github.com/gofiber/fiber/v3"
	"github.com/yourorg/ecommerce/database"
)

func AdminGetCustomerDetail(c fiber.Ctx) error {
	if database.DB == nil {
		return c.Status(503).JSON(fiber.Map{"error": "database not connected"})
	}
	id := c.Params("id")
	ctx := context.Background()
	type C struct {
		ID             string  `json:"id"`
		FirstName      string  `json:"first_name"`
		LastName       string  `json:"last_name"`
		Phone          string  `json:"phone"`
		Email          string  `json:"email"`
		TotalOrders    int     `json:"total_orders"`
		DeliveredOrders int    `json:"delivered_orders"`
		LifetimeValue  int64   `json:"lifetime_value"`
		AverageBasket  float64 `json:"average_basket"`
		RiskScore      int     `json:"risk_score"`
		IsBlacklisted  bool    `json:"is_blacklisted"`
		CreatedAt      string  `json:"created_at"`
	}
	var c2 C
	err := database.DB.QueryRow(ctx,
		`SELECT id,first_name,last_name,phone,COALESCE(email,''),total_orders,delivered_orders,lifetime_value,average_basket,risk_score,is_blacklisted,created_at FROM customers WHERE id=$1`, id).
		Scan(&c2.ID, &c2.FirstName, &c2.LastName, &c2.Phone, &c2.Email, &c2.TotalOrders, &c2.DeliveredOrders, &c2.LifetimeValue, &c2.AverageBasket, &c2.RiskScore, &c2.IsBlacklisted, &c2.CreatedAt)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "not found"})
	}

	type O struct {
		ID          string `json:"id"`
		OrderNumber int    `json:"order_number"`
		Status      string `json:"status"`
		Total       int64  `json:"total"`
		CreatedAt   string `json:"created_at"`
	}
	orders, _ := database.DB.Query(ctx, `SELECT id,order_number,status,total,created_at FROM orders WHERE phone=$1 ORDER BY created_at DESC`, c2.Phone)
	defer orders.Close()
	orderList := []O{}
	for orders.Next() {
		var o O
		orders.Scan(&o.ID, &o.OrderNumber, &o.Status, &o.Total, &o.CreatedAt)
		orderList = append(orderList, o)
	}

	type DH struct {
		ID        string `json:"id"`
		OrderID   string `json:"order_id"`
		Event     string `json:"event"`
		Notes     string `json:"notes"`
		CreatedAt string `json:"created_at"`
	}
	dhRows, _ := database.DB.Query(ctx, `SELECT id,order_id,event,COALESCE(notes,''),created_at FROM customer_delivery_history WHERE customer_id=$1 ORDER BY created_at DESC LIMIT 20`, id)
	defer dhRows.Close()
	delivery := []DH{}
	for dhRows.Next() {
		var d DH
		dhRows.Scan(&d.ID, &d.OrderID, &d.Event, &d.Notes, &d.CreatedAt)
		delivery = append(delivery, d)
	}

	return c.JSON(fiber.Map{"customer": c2, "orders": orderList, "deliveryHistory": delivery})
}

func AdminSearchCustomers(c fiber.Ctx) error {
	if database.DB == nil {
		return c.Status(503).JSON(fiber.Map{"error": "database not connected"})
	}
	search := c.Query("q")
	if search == "" {
		return c.JSON(fiber.Map{"customers": []fiber.Map{}})
	}
	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "10"))
	if page < 1 { page = 1 }
	if limit < 1 || limit > 50 { limit = 10 }
	offset := (page - 1) * limit

	ctx := context.Background()
	rows, err := database.DB.Query(ctx,
		`SELECT id,first_name,last_name,phone,total_orders,lifetime_value FROM customers WHERE first_name ILIKE $1 OR last_name ILIKE $1 OR phone ILIKE $1 OR email ILIKE $1 ORDER BY total_orders DESC LIMIT $2 OFFSET $3`,
		"%"+search+"%", limit, offset)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "database query failed"})
	}
	defer rows.Close()
	type C struct {
		ID           string `json:"id"`
		FirstName    string `json:"first_name"`
		LastName     string `json:"last_name"`
		Phone        string `json:"phone"`
		TotalOrders  int    `json:"total_orders"`
		Lifetime     int64  `json:"lifetime_value"`
	}
	custs := []C{}
	for rows.Next() {
		var c C
		rows.Scan(&c.ID, &c.FirstName, &c.LastName, &c.Phone, &c.TotalOrders, &c.Lifetime)
		custs = append(custs, c)
	}
	return c.JSON(fiber.Map{"customers": custs})
}
