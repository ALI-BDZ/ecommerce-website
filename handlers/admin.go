package handlers

import (
	"context"
	"strconv"

	"github.com/gofiber/fiber/v3"
	"github.com/yourorg/ecommerce/database"
)

func AdminDashboard(c fiber.Ctx) error {
	if database.DB == nil {
		return c.Status(503).JSON(fiber.Map{"error": "database not connected"})
	}
	ctx := context.Background()
	CheckLowStock()

	var revenueToday, revenueWeek, revenueMonth, totalRevenue int64
	var totalOrders, nonCancelled int64
	var pendingOrders, shippedOrders, deliveredOrders, cancelledOrders, returnedOrders int64
	var totalCustomers, activeProducts, lowStockCount int64

	database.DB.QueryRow(ctx, `SELECT COALESCE(SUM(total),0) FROM orders WHERE status!='cancelled' AND created_at::date=now()::date`).Scan(&revenueToday)
	database.DB.QueryRow(ctx, `SELECT COALESCE(SUM(total),0) FROM orders WHERE status!='cancelled' AND created_at>=now()-'7 days'::interval`).Scan(&revenueWeek)
	database.DB.QueryRow(ctx, `SELECT COALESCE(SUM(total),0) FROM orders WHERE status!='cancelled' AND created_at>=now()-'30 days'::interval`).Scan(&revenueMonth)
	database.DB.QueryRow(ctx, `SELECT COALESCE(SUM(total),0) FROM orders WHERE status!='cancelled'`).Scan(&totalRevenue)
	database.DB.QueryRow(ctx, `SELECT COUNT(*) FROM orders`).Scan(&totalOrders)
	database.DB.QueryRow(ctx, `SELECT COUNT(*) FROM orders WHERE status!='cancelled'`).Scan(&nonCancelled)
	database.DB.QueryRow(ctx, `SELECT COUNT(*) FROM orders WHERE status='pending'`).Scan(&pendingOrders)
	database.DB.QueryRow(ctx, `SELECT COUNT(*) FROM orders WHERE status='shipped'`).Scan(&shippedOrders)
	database.DB.QueryRow(ctx, `SELECT COUNT(*) FROM orders WHERE status='delivered'`).Scan(&deliveredOrders)
	database.DB.QueryRow(ctx, `SELECT COUNT(*) FROM orders WHERE status='cancelled'`).Scan(&cancelledOrders)
	database.DB.QueryRow(ctx, `SELECT COUNT(*) FROM orders WHERE status='returned'`).Scan(&returnedOrders)
	database.DB.QueryRow(ctx, `SELECT COUNT(*) FROM customers`).Scan(&totalCustomers)
	database.DB.QueryRow(ctx, `SELECT COUNT(*) FROM products WHERE is_active=true`).Scan(&activeProducts)
	database.DB.QueryRow(ctx, `SELECT COUNT(*) FROM products WHERE stock<=low_stock_threshold AND is_active=true`).Scan(&lowStockCount)

	aov := int64(0)
	dailyAvg := revenueMonth / 30
	if nonCancelled > 0 {
		aov = totalRevenue / nonCancelled
	}

	var prevRevenue int64
	database.DB.QueryRow(ctx, `SELECT COALESCE(SUM(total),0) FROM orders WHERE status!='cancelled' AND created_at>=now()-'60 days'::interval AND created_at<now()-'30 days'::interval`).Scan(&prevRevenue)
	revChange := 0.0
	ordChange := 0.0
	if prevRevenue > 0 && revenueMonth > 0 {
		revChange = float64(revenueMonth-prevRevenue) / float64(prevRevenue) * 100
	}
	var prevOrders int64
	database.DB.QueryRow(ctx, `SELECT COUNT(*) FROM orders WHERE created_at>=now()-'60 days'::interval AND created_at<now()-'30 days'::interval`).Scan(&prevOrders)
	if prevOrders > 0 {
		ordChange = float64(totalOrders-prevOrders) / float64(prevOrders) * 100
	}

	type D struct{ Date string `json:"date"`; Val int64 `json:"val"` }
	rRev := []D{}
	rrows, _ := database.DB.Query(ctx, `SELECT to_char(created_at::date,'YYYY-MM-DD'),COALESCE(SUM(total),0) FROM orders WHERE created_at>=now()-'30 days'::interval AND status!='cancelled' GROUP BY created_at::date ORDER BY created_at::date`)
	if rrows != nil {
		defer rrows.Close()
		for rrows.Next() {
			var d string; var v int64
			rrows.Scan(&d, &v)
			rRev = append(rRev, D{d, v})
		}
	}
	rOrd := []D{}
	orows, _ := database.DB.Query(ctx, `SELECT to_char(created_at::date,'YYYY-MM-DD'),COUNT(*) FROM orders WHERE created_at>=now()-'30 days'::interval GROUP BY created_at::date ORDER BY created_at::date`)
	if orows != nil {
		defer orows.Close()
		for orows.Next() {
			var d string; var v int64
			orows.Scan(&d, &v)
			rOrd = append(rOrd, D{d, v})
		}
	}

	lrows, _ := database.DB.Query(ctx, `SELECT id,name,stock,low_stock_threshold FROM products WHERE stock<=low_stock_threshold AND is_active=true ORDER BY stock ASC LIMIT 5`)
	alerts := []fiber.Map{}
	if lrows != nil {
		defer lrows.Close()
		for lrows.Next() {
			var id, n string; var s, th int
			lrows.Scan(&id, &n, &s, &th)
			alerts = append(alerts, fiber.Map{"type": "low_stock", "message": n + " - " + strconv.Itoa(s) + " left (thresh: " + strconv.Itoa(th) + ")", "link": "/admin/products?id=" + id})
		}
	}

	type BS struct{ ID, Name string; Qty, Revenue int64 }
	bestQty := []BS{}
	bqrows, _ := database.DB.Query(ctx, `SELECT oi.product_id,oi.product_name,SUM(oi.quantity)::bigint,SUM(oi.price*oi.quantity)::bigint FROM order_items oi JOIN orders o ON o.id=oi.order_id WHERE o.status!='cancelled' GROUP BY oi.product_id,oi.product_name ORDER BY SUM(oi.quantity) DESC LIMIT 5`)
	if bqrows != nil {
		defer bqrows.Close()
		for bqrows.Next() {
			var id, n string; var q, rv int64
			bqrows.Scan(&id, &n, &q, &rv)
			bestQty = append(bestQty, BS{id, n, q, rv})
		}
	}
	bestRev := []BS{}
	brrows, _ := database.DB.Query(ctx, `SELECT oi.product_id,oi.product_name,SUM(oi.quantity)::bigint,SUM(oi.price*oi.quantity)::bigint FROM order_items oi JOIN orders o ON o.id=oi.order_id WHERE o.status!='cancelled' GROUP BY oi.product_id,oi.product_name ORDER BY SUM(oi.price*oi.quantity) DESC LIMIT 5`)
	if brrows != nil {
		defer brrows.Close()
		for brrows.Next() {
			var id, n string; var q, rv int64
			brrows.Scan(&id, &n, &q, &rv)
			bestRev = append(bestRev, BS{id, n, q, rv})
		}
	}

	type UP struct{ ID, Name, SKU string; Price, Stock int64; CreatedAt string }
	neverSold := []UP{}
	nsrows, _ := database.DB.Query(ctx, `SELECT p.id,p.name,COALESCE(p.sku,''),p.price,p.stock,p.created_at FROM products p WHERE p.is_active=true AND NOT EXISTS (SELECT 1 FROM order_items oi WHERE oi.product_id=p.id::text) ORDER BY p.created_at DESC`)
	if nsrows != nil {
		defer nsrows.Close()
		for nsrows.Next() {
			var id, n, sku, ca string; var pr, st int64
			nsrows.Scan(&id, &n, &sku, &pr, &st, &ca)
			neverSold = append(neverSold, UP{id, n, sku, pr, st, ca})
		}
	}
	stale := []UP{}
	strows, _ := database.DB.Query(ctx, `SELECT p.id,p.name,COALESCE(p.sku,''),p.price,p.stock FROM products p WHERE p.is_active=true AND EXISTS (SELECT 1 FROM order_items oi WHERE oi.product_id=p.id::text) AND NOT EXISTS (SELECT 1 FROM order_items oi JOIN orders o ON o.id=oi.order_id WHERE oi.product_id=p.id::text AND o.status NOT IN ('cancelled') AND o.created_at>=now()-'30 days'::interval) ORDER BY p.created_at DESC LIMIT 10`)
	if strows != nil {
		defer strows.Close()
		for strows.Next() {
			var id, n, sku string; var pr, st int64
			strows.Scan(&id, &n, &sku, &pr, &st)
			stale = append(stale, UP{id, n, sku, pr, st, ""})
		}
	}

	type TC struct{ ID, FirstName, LastName, Phone string; Orders int; Lifetime int64 }
	topCust := []TC{}
	tcrows, _ := database.DB.Query(ctx, `SELECT id,first_name,last_name,phone,total_orders,lifetime_value FROM customers ORDER BY lifetime_value DESC LIMIT 10`)
	if tcrows != nil {
		defer tcrows.Close()
		for tcrows.Next() {
			var id, fn, ln, ph string; var o int; var lv int64
			tcrows.Scan(&id, &fn, &ln, &ph, &o, &lv)
			topCust = append(topCust, TC{id, fn, ln, ph, o, lv})
		}
	}

	type CR struct{ ID, FirstName, LastName, Phone string; Failed, Total int; Rate float64 }
	codRisk := []CR{}
	crrows, _ := database.DB.Query(ctx, `SELECT c.id,c.first_name,c.last_name,c.phone,COUNT(*) FILTER (WHERE cdh.event IN ('refused','no_answer','wrong_address','courier_returned'))::int,COUNT(*)::int FROM customer_delivery_history cdh JOIN customers c ON c.id=cdh.customer_id GROUP BY c.id,c.first_name,c.last_name,c.phone HAVING COUNT(*) FILTER (WHERE cdh.event IN ('refused','no_answer','wrong_address','courier_returned'))>0 ORDER BY COUNT(*) FILTER (WHERE cdh.event IN ('refused','no_answer','wrong_address','courier_returned')) DESC LIMIT 10`)
	if crrows != nil {
		defer crrows.Close()
		for crrows.Next() {
			var id, fn, ln, ph string; var f, t int
			crrows.Scan(&id, &fn, &ln, &ph, &f, &t)
			r := 0.0
			if t > 0 {
				r = float64(f) / float64(t) * 100
			}
			codRisk = append(codRisk, CR{id, fn, ln, ph, f, t, r})
		}
	}

	type CB struct{ Name string; Orders int64; Percentage float64 }
	catBreak := []CB{}
	cbrows, _ := database.DB.Query(ctx, `SELECT COALESCE(c.name,'Uncategorized'),COUNT(DISTINCT o.id) FROM orders o JOIN order_items oi ON oi.order_id=o.id LEFT JOIN products p ON p.id::text=oi.product_id LEFT JOIN categories c ON c.id=p.category_id WHERE o.status!='cancelled' GROUP BY c.name ORDER BY COUNT(DISTINCT o.id) DESC`)
	if cbrows != nil {
		defer cbrows.Close()
		var catTotal int64
		raw := []struct{ name string; cnt int64 }{}
		for cbrows.Next() {
			var n string; var cnt int64
			cbrows.Scan(&n, &cnt)
			raw = append(raw, struct{ name string; cnt int64 }{n, cnt})
			catTotal += cnt
		}
		for _, r := range raw {
			pct := 0.0
			if catTotal > 0 {
				pct = float64(r.cnt) / float64(catTotal) * 100
			}
			catBreak = append(catBreak, CB{r.name, r.cnt, pct})
		}
	}

	type RO struct{ ID, Status, FirstName, LastName, CreatedAt string; OrderNumber int; Total int64 }
	recentOrd := []RO{}
	rorows, _ := database.DB.Query(ctx, `SELECT id,order_number,status,first_name,last_name,total,created_at FROM orders ORDER BY created_at DESC LIMIT 20`)
	if rorows != nil {
		defer rorows.Close()
		for rorows.Next() {
			var id, s, fn, ln, ca string; var on int; var t int64
			rorows.Scan(&id, &on, &s, &fn, &ln, &t, &ca)
			recentOrd = append(recentOrd, RO{id, s, fn, ln, ca, on, t})
		}
	}

	return c.JSON(fiber.Map{
		"kpi": fiber.Map{
			"revenueToday":    revenueToday,
			"revenueWeek":     revenueWeek,
			"revenueMonth":    revenueMonth,
			"totalRevenue":    totalRevenue,
			"totalOrders":     totalOrders,
			"nonCancelled":    nonCancelled,
			"pendingOrders":   pendingOrders,
			"shippedOrders":   shippedOrders,
			"deliveredOrders": deliveredOrders,
			"cancelledOrders": cancelledOrders,
			"returnedOrders":  returnedOrders,
			"totalCustomers":  totalCustomers,
			"activeProducts":  activeProducts,
			"lowStockCount":   lowStockCount,
			"aov":             aov,
			"dailyAvg":        dailyAvg,
			"revChange":       revChange,
			"ordChange":       ordChange,
		},
		"revenueChart": rRev,
		"orderChart":   rOrd,
		"alerts":       alerts,
		"bestSellers": fiber.Map{
			"byQuantity": bestQty,
			"byRevenue":  bestRev,
		},
		"unsold": fiber.Map{
			"neverSold": neverSold,
			"stale":     stale,
		},
		"topCustomers":      topCust,
		"codReliability":    codRisk,
		"categoryBreakdown": catBreak,
		"recentOrders":      recentOrd,
	})
}

func AdminOrders(c fiber.Ctx) error {
	if database.DB == nil {
		return c.Status(503).JSON(fiber.Map{"error": "database not connected"})
	}
	ctx := context.Background()
	search, statusF := c.Query("search"), c.Query("status")
	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	where, args, aidx := "WHERE 1=1", []interface{}{}, 0
	if search != "" {
		aidx++
		where += " AND (first_name ILIKE $" + strconv.Itoa(aidx) + " OR last_name ILIKE $" + strconv.Itoa(aidx) + " OR phone ILIKE $" + strconv.Itoa(aidx) + " OR order_number::text ILIKE $" + strconv.Itoa(aidx) + ")"
		args = append(args, "%"+search+"%")
	}
	if statusF != "" {
		aidx++
		where += " AND status=$" + strconv.Itoa(aidx)
		args = append(args, statusF)
	}
	var total int64
	database.DB.QueryRow(ctx, "SELECT COUNT(*) FROM orders o "+where, args...).Scan(&total)
	aidx++
	args = append(args, limit, (page-1)*limit)
	q := "SELECT id,order_number,status,first_name,last_name,phone,total,payment_method,created_at,(SELECT COUNT(*) FROM order_items WHERE order_id=orders.id) FROM orders o " + where + " ORDER BY created_at DESC LIMIT $" + strconv.Itoa(aidx) + " OFFSET $" + strconv.Itoa(aidx+1)
	rows, err := database.DB.Query(ctx, q, args...)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	defer rows.Close()
	type O struct {
		ID, Status, FirstName, LastName, Phone, PaymentMethod, CreatedAt string
		OrderNumber int; Total int64; ItemCount int
	}
	orders := []O{}
	for rows.Next() {
		var o O
		rows.Scan(&o.ID, &o.OrderNumber, &o.Status, &o.FirstName, &o.LastName, &o.Phone, &o.Total, &o.PaymentMethod, &o.CreatedAt, &o.ItemCount)
		orders = append(orders, o)
	}
	return c.JSON(fiber.Map{"orders": orders, "total": total, "page": page, "limit": limit})
}

func AdminProducts(c fiber.Ctx) error {
	if database.DB == nil {
		return c.Status(503).JSON(fiber.Map{"error": "database not connected"})
	}
	ctx := context.Background()
	search := c.Query("search")
	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	where, args, aidx := "WHERE 1=1", []interface{}{}, 0
	if search != "" {
		aidx++
		where += " AND (p.name ILIKE $" + strconv.Itoa(aidx) + " OR p.sku ILIKE $" + strconv.Itoa(aidx) + ")"
		args = append(args, "%"+search+"%")
	}
	var total int64
	database.DB.QueryRow(ctx, "SELECT COUNT(*) FROM products p "+where, args...).Scan(&total)
	aidx++
	args = append(args, limit, (page-1)*limit)
	q := "SELECT p.id,p.name,p.sku,p.price,p.stock,p.is_active,p.orders_count,p.revenue,p.created_at,COALESCE(b.name,''),COALESCE(c.name,''),COALESCE(ROUND(AVG(r.rating),1),0),COALESCE((SELECT url FROM product_images WHERE product_id=p.id ORDER BY sort_order LIMIT 1),'') FROM products p LEFT JOIN brands b ON b.id=p.brand_id LEFT JOIN categories c ON c.id=p.category_id LEFT JOIN reviews r ON r.product_id=p.id " + where + " GROUP BY p.id,b.name,c.name ORDER BY p.created_at DESC LIMIT $" + strconv.Itoa(aidx) + " OFFSET $" + strconv.Itoa(aidx+1)
	rows, err := database.DB.Query(ctx, q, args...)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	defer rows.Close()
	type P struct {
		ID, Name, SKU, Brand, Category, CreatedAt, Image string
		Price, Revenue int64; Stock, Orders int; IsActive bool; Rating float64
	}
	prods := []P{}
	for rows.Next() {
		var p P
		rows.Scan(&p.ID, &p.Name, &p.SKU, &p.Price, &p.Stock, &p.IsActive, &p.Orders, &p.Revenue, &p.CreatedAt, &p.Brand, &p.Category, &p.Rating, &p.Image)
		prods = append(prods, p)
	}
	return c.JSON(fiber.Map{"products": prods, "total": total, "page": page, "limit": limit})
}

func AdminCustomers(c fiber.Ctx) error {
	if database.DB == nil {
		return c.Status(503).JSON(fiber.Map{"error": "database not connected"})
	}
	ctx := context.Background()
	search := c.Query("search")
	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	where, args, aidx := "WHERE 1=1", []interface{}{}, 0
	if search != "" {
		aidx++
		where += " AND (first_name ILIKE $" + strconv.Itoa(aidx) + " OR last_name ILIKE $" + strconv.Itoa(aidx) + " OR phone ILIKE $" + strconv.Itoa(aidx) + " OR email ILIKE $" + strconv.Itoa(aidx) + ")"
		args = append(args, "%"+search+"%")
	}
	var total int64
	database.DB.QueryRow(ctx, "SELECT COUNT(*) FROM customers c "+where, args...).Scan(&total)
	aidx++
	args = append(args, limit, (page-1)*limit)
	q := "SELECT id,first_name,last_name,phone,email,total_orders,lifetime_value,last_order_at,risk_score,is_blacklisted,created_at FROM customers c " + where + " ORDER BY created_at DESC LIMIT $" + strconv.Itoa(aidx) + " OFFSET $" + strconv.Itoa(aidx+1)
	rows, err := database.DB.Query(ctx, q, args...)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	defer rows.Close()
	type C struct {
		ID, FirstName, LastName, Phone, Email, LastOrder, CreatedAt string
		TotalOrders int; Lifetime int64; RiskScore int; Blacklisted bool
	}
	custs := []C{}
	for rows.Next() {
		var c C
		rows.Scan(&c.ID, &c.FirstName, &c.LastName, &c.Phone, &c.Email, &c.TotalOrders, &c.Lifetime, &c.LastOrder, &c.RiskScore, &c.Blacklisted, &c.CreatedAt)
		custs = append(custs, c)
	}
	return c.JSON(fiber.Map{"customers": custs, "total": total, "page": page, "limit": limit})
}

func AdminNotifications(c fiber.Ctx) error {
	if database.DB == nil {
		return c.Status(503).JSON(fiber.Map{"error": "database not connected"})
	}
	ctx := context.Background()

	// Clean stale order notifications
	database.DB.Exec(ctx, `DELETE FROM notifications WHERE type='order_created' AND data->>'order_id' IS NOT NULL AND NOT EXISTS (SELECT 1 FROM orders WHERE orders.id::text = notifications.data->>'order_id' AND status='pending')`)
	database.DB.Exec(ctx, `DELETE FROM notifications WHERE type='cancelled' AND data->>'order_id' IS NOT NULL AND NOT EXISTS (SELECT 1 FROM orders WHERE orders.id::text = notifications.data->>'order_id' AND status='cancelled')`)

	var unread int64
	database.DB.QueryRow(ctx, "SELECT COUNT(*) FROM notifications WHERE is_read=false").Scan(&unread)
	rows, err := database.DB.Query(ctx, "SELECT id,type,title,body,is_read,created_at FROM notifications ORDER BY created_at DESC LIMIT 50")
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	defer rows.Close()
	type N struct {
		ID, Type, Title, Body, CreatedAt string; IsRead bool
	}
	notifs := []N{}
	for rows.Next() {
		var n N
		rows.Scan(&n.ID, &n.Type, &n.Title, &n.Body, &n.IsRead, &n.CreatedAt)
		notifs = append(notifs, n)
	}
	return c.JSON(fiber.Map{"notifications": notifs, "unread": unread})
}

func CheckLowStock() {
	if database.DB == nil {
		return
	}
	ctx := context.Background()

	// Clean up stale order notifications first
	database.DB.Exec(ctx, `DELETE FROM notifications WHERE type='order_created' AND data->>'order_id' IS NOT NULL AND NOT EXISTS (SELECT 1 FROM orders WHERE orders.id::text = notifications.data->>'order_id' AND status='pending')`)
	database.DB.Exec(ctx, `DELETE FROM notifications WHERE type='cancelled' AND data->>'order_id' IS NOT NULL AND NOT EXISTS (SELECT 1 FROM orders WHERE orders.id::text = notifications.data->>'order_id' AND status='cancelled')`)

	// Clean up old low stock notifications for products no longer low
	database.DB.Exec(ctx, `DELETE FROM notifications WHERE type IN ('low_stock','stock_out') AND data->>'product_id' IS NOT NULL AND NOT EXISTS (SELECT 1 FROM products WHERE products.id::text = notifications.data->>'product_id' AND is_active=true AND stock <= low_stock_threshold AND low_stock_threshold > 0)`)

	rows, err := database.DB.Query(ctx, "SELECT id, name, stock, low_stock_threshold FROM products WHERE is_active=true AND low_stock_threshold > 0 AND stock <= low_stock_threshold")
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		var id, name string
		var stock, threshold int
		rows.Scan(&id, &name, &stock, &threshold)

		var exists int
		database.DB.QueryRow(ctx,
			"SELECT COUNT(*) FROM notifications WHERE type IN ('low_stock','stock_out') AND data->>'product_id'=$1 AND created_at > now()-interval '24 hours'", id).Scan(&exists)
		if exists > 0 {
			continue
		}

		notifType := "low_stock"
		title := "Low Stock: " + name
		body := name + " has " + strconv.Itoa(stock) + " units left (threshold: " + strconv.Itoa(threshold) + ")"
		if stock == 0 {
			notifType = "stock_out"
			title = "Out of Stock: " + name
			body = name + " is completely out of stock"
		}

		database.DB.Exec(ctx,
			`INSERT INTO notifications (type, title, body, data) VALUES ($1,$2,$3,$4)`,
			notifType, title, body,
			`{"product_id":"`+id+`","product_name":"`+name+`","stock":`+strconv.Itoa(stock)+`,"threshold":`+strconv.Itoa(threshold)+`}`)
	}
}
