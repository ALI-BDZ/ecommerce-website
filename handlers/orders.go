package handlers

import (
	"context"
	"encoding/json"
	"log"
	"strconv"

	"github.com/gofiber/fiber/v3"
	"github.com/yourorg/ecommerce/database"
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

func CreateOrder(c fiber.Ctx) error {
	var req OrderRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"success": false, "message": "Invalid request body"})
	}

	if req.FirstName == "" || req.LastName == "" || req.Phone == "" || req.Address == "" || req.Wilaya == "" || req.City == "" {
		return c.Status(400).JSON(fiber.Map{"success": false, "message": "Missing required fields"})
	}

	if len(req.Items) == 0 {
		return c.Status(400).JSON(fiber.Map{"success": false, "message": "No items in order"})
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

	var serverSubtotal int64
	type validatedItem struct {
		ProductID   string
		ProductName string
		Brand       string
		Variant     string
		ImageURL    string
		Price       int64
		Quantity    int
	}
	validated := make([]validatedItem, 0, len(req.Items))

	for _, item := range req.Items {
		var dbPrice int64
		var dbStock int
		var dbIsActive bool
		var dbName string
		err := tx.QueryRow(ctx,
			`SELECT price, stock, is_active, name FROM products WHERE id=$1::uuid FOR UPDATE`,
			item.ProductID,
		).Scan(&dbPrice, &dbStock, &dbName, &dbIsActive)
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"success": false, "message": "Product not found: " + item.ProductID})
		}
		if !dbIsActive {
			return c.Status(400).JSON(fiber.Map{"success": false, "message": "Product not available: " + dbName})
		}
		if item.Quantity < 1 {
			return c.Status(400).JSON(fiber.Map{"success": false, "message": "Invalid quantity for: " + dbName})
		}
		if dbStock < item.Quantity {
			return c.Status(400).JSON(fiber.Map{"success": false, "message": "Insufficient stock for: " + dbName + " (available: " + strconv.Itoa(dbStock) + ")"})
		}
		serverSubtotal += dbPrice * int64(item.Quantity)
		validated = append(validated, validatedItem{
			ProductID:   item.ProductID,
			ProductName: dbName,
			Brand:       item.Brand,
			Variant:     item.Variant,
			ImageURL:    item.ImageURL,
			Price:       dbPrice,
			Quantity:    item.Quantity,
		})
	}

	serverShipping := req.ShippingCost
	serverDiscount := req.Discount
	if serverShipping < 0 {
		serverShipping = 0
	}
	if serverDiscount < 0 {
		serverDiscount = 0
	}
	if serverDiscount > serverSubtotal {
		serverDiscount = serverSubtotal
	}
	serverTotal := serverSubtotal + serverShipping - serverDiscount

	var orderID string
	err = tx.QueryRow(ctx,
		`INSERT INTO orders (first_name, last_name, phone, email, address, wilaya, city, notes, payment_method, subtotal, shipping_cost, discount, total)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13) RETURNING id`,
		req.FirstName, req.LastName, req.Phone, req.Email,
		req.Address, req.Wilaya, req.City, req.Notes,
		req.PaymentMethod, serverSubtotal, serverShipping, serverDiscount, serverTotal,
	).Scan(&orderID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"success": false, "message": "Failed to create order"})
	}

	paymentStatus := "unpaid"
	if req.PaymentMethod != "cod" {
		paymentStatus = "unpaid"
	}
	_, err = tx.Exec(ctx, `UPDATE orders SET payment_status=$1 WHERE id=$2::uuid`, paymentStatus, orderID)
	if err != nil {
		log.Printf("warning: failed to set payment status: %v", err)
	}

	for _, item := range validated {
		_, err := tx.Exec(ctx,
			`INSERT INTO order_items (order_id, product_id, product_name, product_brand, variant, image_url, price, quantity)
			 VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
			orderID, item.ProductID, item.ProductName, item.Brand, item.Variant, item.ImageURL, item.Price, item.Quantity,
		)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"success": false, "message": "Failed to save order items"})
		}

		_, err = tx.Exec(ctx,
			`UPDATE products SET stock=stock-$1, updated_at=now() WHERE id=$2::uuid`,
			item.Quantity, item.ProductID,
		)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"success": false, "message": "Failed to update stock"})
		}

		itemRevenue := item.Price * int64(item.Quantity)
		_, err = tx.Exec(ctx,
			`UPDATE products SET orders_count=orders_count+1, revenue=revenue+$1, updated_at=now() WHERE id=$2::uuid`,
			itemRevenue, item.ProductID,
		)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"success": false, "message": "Failed to update product stats"})
		}
	}

	var custExists int
	tx.QueryRow(ctx, `SELECT COUNT(*) FROM customers WHERE phone=$1`, req.Phone).Scan(&custExists)
	if custExists == 0 {
		_, err = tx.Exec(ctx,
			`INSERT INTO customers (first_name,last_name,phone,email,total_orders,delivered_orders,lifetime_value,average_basket,last_order_at)
			 VALUES ($1,$2,$3,$4,1,0,$5,$5,now())`,
			req.FirstName, req.LastName, req.Phone, req.Email, serverTotal,
		)
	} else {
		_, err = tx.Exec(ctx,
			`UPDATE customers SET total_orders=total_orders+1, lifetime_value=lifetime_value+$1,
			 average_basket=(lifetime_value+$1)/(total_orders+1), last_order_at=now(), updated_at=now()
			 WHERE phone=$2`,
			serverTotal, req.Phone,
		)
	}
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"success": false, "message": "Failed to update customer stats"})
	}

	if err := tx.Commit(ctx); err != nil {
		return c.Status(500).JSON(fiber.Map{"success": false, "message": "Failed to commit order"})
	}

	CreateNotification("order_created", "New Order", req.FirstName+" "+req.LastName+" placed an order", map[string]interface{}{
		"order_id": orderID, "phone": req.Phone, "total": serverTotal,
	})

	return c.JSON(fiber.Map{"success": true, "orderId": orderID, "message": "Order placed successfully"})
}

func GetOrder(c fiber.Ctx) error {
	orderID := c.Params("id")
	phone := c.Query("phone")
	if phone == "" {
		return c.Status(400).JSON(fiber.Map{"success": false, "message": "phone query parameter required"})
	}
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
		 FROM orders WHERE id = $1 AND phone = $2`, orderID, phone,
	).Scan(&order.ID, &order.OrderNumber, &order.Status, &order.PaymentMethod,
		&order.FirstName, &order.LastName, &order.Phone,
		&order.Address, &order.Wilaya, &order.City, &order.Total, &order.CreatedAt)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"success": false, "message": "Order not found"})
	}

	return c.JSON(fiber.Map{"success": true, "order": order})
}

func UpdateOrderStatus(c fiber.Ctx) error {
	if database.DB == nil {
		return c.Status(503).JSON(fiber.Map{"success": false, "message": "Database not connected"})
	}
	orderID := c.Params("id")
	body := struct {
		Status string `json:"status"`
	}{}
	if err := json.Unmarshal(c.Body(), &body); err != nil || body.Status == "" {
		return c.Status(400).JSON(fiber.Map{"success": false, "message": "status required"})
	}

	validStatuses := map[string]bool{
		"pending": true, "confirmed": true, "shipped": true,
		"delivered": true, "cancelled": true, "returned": true, "refunded": true,
	}
	if !validStatuses[body.Status] {
		return c.Status(400).JSON(fiber.Map{"success": false, "message": "invalid status"})
	}

	ctx := context.Background()
	tx, err := database.DB.Begin(ctx)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"success": false, "message": "Failed to start transaction"})
	}
	defer tx.Rollback(ctx)

	var currentStatus, phone string
	var orderTotal int64
	err = tx.QueryRow(ctx, `SELECT status, phone, total FROM orders WHERE id=$1::uuid FOR UPDATE`, orderID).Scan(&currentStatus, &phone, &orderTotal)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"success": false, "message": "Order not found"})
	}

	allowed := map[string][]string{
		"pending":   {"confirmed", "cancelled"},
		"confirmed": {"shipped", "cancelled"},
		"shipped":   {"delivered", "returned"},
		"delivered": {"returned", "refunded"},
		"returned":  {"refunded"},
	}
	valid := false
	for _, s := range allowed[currentStatus] {
		if s == body.Status {
			valid = true
			break
		}
	}
	if !valid {
		return c.Status(400).JSON(fiber.Map{"success": false, "message": "cannot transition from " + currentStatus + " to " + body.Status})
	}

	statusUpdate := ""
	switch body.Status {
	case "shipped":
		statusUpdate = ", shipped_at=now()"
	case "delivered":
		statusUpdate = ", delivered_at=now()"
	}

	if body.Status == "cancelled" || body.Status == "returned" {
		irows, err := tx.Query(ctx, `SELECT product_id, price, quantity FROM order_items WHERE order_id=$1::uuid`, orderID)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"success": false, "message": "Failed to read order items"})
		}
		defer irows.Close()

		type itemData struct {
			ProductID string
			Price     int64
			Quantity  int
		}
		items := []itemData{}
		for irows.Next() {
			var it itemData
			irows.Scan(&it.ProductID, &it.Price, &it.Quantity)
			items = append(items, it)
		}

		for _, it := range items {
			_, err := tx.Exec(ctx,
				`UPDATE products SET stock=stock+$1, updated_at=now() WHERE id=$2::uuid FOR UPDATE`,
				it.Quantity, it.ProductID,
			)
			if err != nil {
				return c.Status(500).JSON(fiber.Map{"success": false, "message": "Failed to restore stock"})
			}

			itemRevenue := it.Price * int64(it.Quantity)
			_, err = tx.Exec(ctx,
				`UPDATE products SET
					orders_count=GREATEST(orders_count-1,0),
					revenue=GREATEST(revenue-$1,0),
					return_count=return_count+$2,
					updated_at=now()
				 WHERE id=$3::uuid`,
				itemRevenue, it.Quantity, it.ProductID,
			)
			if err != nil {
				return c.Status(500).JSON(fiber.Map{"success": false, "message": "Failed to update product stats"})
			}
		}

		if body.Status == "cancelled" {
			_, err = tx.Exec(ctx,
				`UPDATE customers SET
					total_orders=GREATEST(total_orders-1,0),
					lifetime_value=GREATEST(lifetime_value-$1,0),
					cancelled_orders=cancelled_orders+1,
					updated_at=now()
				 WHERE phone=$2`,
				orderTotal, phone,
			)
		} else {
			_, err = tx.Exec(ctx,
				`UPDATE customers SET
					returned_orders=returned_orders+1,
					updated_at=now()
				 WHERE phone=$1`,
				phone,
			)
		}
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"success": false, "message": "Failed to update customer stats"})
		}
	}

	if body.Status == "delivered" {
		_, err = tx.Exec(ctx,
			`UPDATE customers SET delivered_orders=delivered_orders+1, updated_at=now() WHERE phone=$1`, phone)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"success": false, "message": "Failed to update customer stats"})
		}
		_, err = tx.Exec(ctx, `UPDATE orders SET payment_status='paid' WHERE id=$1::uuid`, orderID)
		if err != nil {
			log.Printf("warning: failed to update payment status: %v", err)
		}
	}

	_, err = tx.Exec(ctx,
		`UPDATE orders SET status=$1, updated_at=now()`+statusUpdate+` WHERE id=$2::uuid`,
		body.Status, orderID,
	)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"success": false, "message": "Failed to update order"})
	}

	if err := tx.Commit(ctx); err != nil {
		return c.Status(500).JSON(fiber.Map{"success": false, "message": "Failed to commit"})
	}

	log.Printf("Order %s status updated: %s -> %s", orderID, currentStatus, body.Status)

	var customerID string
	database.DB.QueryRow(context.Background(), `SELECT id FROM customers WHERE phone=$1`, phone).Scan(&customerID)

	eventMap := map[string]string{
		"confirmed": "order_confirmed",
		"shipped":   "order_shipped",
		"delivered": "order_delivered",
		"cancelled": "order_cancelled",
		"returned":  "order_returned",
		"refunded":  "order_refunded",
	}
	if ev, ok := eventMap[body.Status]; ok {
		RecordDeliveryHistory(customerID, orderID, ev, "Status: "+body.Status)
	}

	CreateNotification("order_"+body.Status, "Order "+body.Status, "Order status updated to "+body.Status, map[string]interface{}{
		"order_id": orderID, "status": body.Status, "phone": phone,
	})

	return c.JSON(fiber.Map{"success": true, "message": "Order status updated to " + body.Status})
}
