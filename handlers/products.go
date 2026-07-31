package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/jackc/pgx/v5"
	"github.com/yourorg/ecommerce/database"
)

func uniqueSlug(base string, excludeID string) string {
	if database.DB == nil {
		return base
	}
	ctx := context.Background()
	slug := base
	for i := 0; ; i++ {
		var exists int
		q := `SELECT COUNT(*) FROM products WHERE slug=$1`
		args := []interface{}{slug}
		if excludeID != "" {
			q += ` AND id!=$2::uuid`
			args = append(args, excludeID)
		}
		database.DB.QueryRow(ctx, q, args...).Scan(&exists)
		if exists == 0 {
			return slug
		}
		slug = fmt.Sprintf("%s-%d", base, i+2)
	}
}

func AdminBrands(c fiber.Ctx) error {
	if database.DB == nil {
		return c.JSON(fiber.Map{"brands": []fiber.Map{}})
	}
	rows, err := database.DB.Query(context.Background(), "SELECT id,name FROM brands WHERE is_active=true ORDER BY name")
	if err != nil {
		return c.JSON(fiber.Map{"brands": []fiber.Map{}})
	}
	defer rows.Close()
	brands := []fiber.Map{}
	for rows.Next() {
		var id, name string
		rows.Scan(&id, &name)
		brands = append(brands, fiber.Map{"id": id, "name": name})
	}
	return c.JSON(fiber.Map{"brands": brands})
}

func AdminCategories(c fiber.Ctx) error {
	if database.DB == nil {
		return c.JSON(fiber.Map{"categories": []fiber.Map{}})
	}
	rows, err := database.DB.Query(context.Background(), "SELECT id,name FROM categories WHERE is_active=true ORDER BY name")
	if err != nil {
		return c.JSON(fiber.Map{"categories": []fiber.Map{}})
	}
	defer rows.Close()
	cats := []fiber.Map{}
	for rows.Next() {
		var id, name string
		rows.Scan(&id, &name)
		cats = append(cats, fiber.Map{"id": id, "name": name})
	}
	return c.JSON(fiber.Map{"categories": cats})
}

func CreateBrand(c fiber.Ctx) error {
	if database.DB == nil {
		return c.Status(503).JSON(fiber.Map{"error": "db not connected"})
	}
	body := struct{ Name string }{}
	if err := c.Bind().JSON(&body); err != nil || body.Name == "" {
		return c.Status(400).JSON(fiber.Map{"error": "name required"})
	}
	slug := strings.ToLower(strings.ReplaceAll(body.Name, " ", "-"))
	slug = strings.NewReplacer("--", "-", "'", "", "\"", "").Replace(slug)
	var id string
	err := database.DB.QueryRow(context.Background(), "INSERT INTO brands (name,slug) VALUES ($1,$2) RETURNING id", body.Name, slug).Scan(&id)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "brand already exists or database error"})
	}
	return c.JSON(fiber.Map{"success": true, "id": id})
}

func CreateCategory(c fiber.Ctx) error {
	if database.DB == nil {
		return c.Status(503).JSON(fiber.Map{"error": "db not connected"})
	}
	body := struct{ Name string }{}
	if err := c.Bind().JSON(&body); err != nil || body.Name == "" {
		return c.Status(400).JSON(fiber.Map{"error": "name required"})
	}
	slug := strings.ToLower(strings.ReplaceAll(body.Name, " ", "-"))
	slug = strings.NewReplacer("--", "-", "'", "", "\"", "").Replace(slug)
	var id string
	err := database.DB.QueryRow(context.Background(), "INSERT INTO categories (name,slug) VALUES ($1,$2) RETURNING id", body.Name, slug).Scan(&id)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "category already exists or database error"})
	}
	return c.JSON(fiber.Map{"success": true, "id": id})
}

func UpdateBrand(c fiber.Ctx) error {
	if database.DB == nil {
		return c.Status(503).JSON(fiber.Map{"error": "db not connected"})
	}
	id := c.Params("id")
	body := struct{ Name string }{}
	if err := c.Bind().JSON(&body); err != nil || body.Name == "" {
		return c.Status(400).JSON(fiber.Map{"error": "name required"})
	}
	tag, err := database.DB.Exec(context.Background(), `UPDATE brands SET name=$1, updated_at=now() WHERE id=$2`, body.Name, id)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to update brand"})
	}
	if tag.RowsAffected() == 0 {
		return c.Status(404).JSON(fiber.Map{"error": "not found"})
	}
	return c.JSON(fiber.Map{"success": true})
}

func DeleteBrand(c fiber.Ctx) error {
	if database.DB == nil {
		return c.Status(503).JSON(fiber.Map{"error": "db not connected"})
	}
	id := c.Params("id")
	var productCount int
	database.DB.QueryRow(context.Background(), `SELECT COUNT(*) FROM products WHERE brand_id=$1`, id).Scan(&productCount)
	if productCount > 0 {
		database.DB.Exec(context.Background(), `UPDATE brands SET is_active=false WHERE id=$1`, id)
		return c.JSON(fiber.Map{"success": true, "archived": true, "message": "Brand has products — archived instead of deleted"})
	}
	tag, err := database.DB.Exec(context.Background(), `DELETE FROM brands WHERE id=$1`, id)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to delete brand"})
	}
	if tag.RowsAffected() == 0 {
		return c.Status(404).JSON(fiber.Map{"error": "not found"})
	}
	return c.JSON(fiber.Map{"success": true})
}

func UpdateCategory(c fiber.Ctx) error {
	if database.DB == nil {
		return c.Status(503).JSON(fiber.Map{"error": "db not connected"})
	}
	id := c.Params("id")
	body := struct{ Name string }{}
	if err := c.Bind().JSON(&body); err != nil || body.Name == "" {
		return c.Status(400).JSON(fiber.Map{"error": "name required"})
	}
	tag, err := database.DB.Exec(context.Background(), `UPDATE categories SET name=$1, updated_at=now() WHERE id=$2`, body.Name, id)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to update category"})
	}
	if tag.RowsAffected() == 0 {
		return c.Status(404).JSON(fiber.Map{"error": "not found"})
	}
	return c.JSON(fiber.Map{"success": true})
}

func DeleteCategory(c fiber.Ctx) error {
	if database.DB == nil {
		return c.Status(503).JSON(fiber.Map{"error": "db not connected"})
	}
	id := c.Params("id")
	var productCount int
	database.DB.QueryRow(context.Background(), `SELECT COUNT(*) FROM products WHERE category_id=$1`, id).Scan(&productCount)
	if productCount > 0 {
		database.DB.Exec(context.Background(), `UPDATE categories SET is_active=false WHERE id=$1`, id)
		return c.JSON(fiber.Map{"success": true, "archived": true, "message": "Category has products — archived instead of deleted"})
	}
	tag, err := database.DB.Exec(context.Background(), `DELETE FROM categories WHERE id=$1`, id)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to delete category"})
	}
	if tag.RowsAffected() == 0 {
		return c.Status(404).JSON(fiber.Map{"error": "not found"})
	}
	return c.JSON(fiber.Map{"success": true})
}

func AdminProductDetail(c fiber.Ctx) error {
	if database.DB == nil {
		return c.Status(503).JSON(fiber.Map{"error": "db not connected"})
	}
	ctx := context.Background()
	type PD struct {
		ID, Name, Slug, Description, ShortDescription, Ingredients, HowToUse, Barcode, BrandID, CategoryID, CreatedAt, UpdatedAt string
		Price, CompareAtPrice int64
		Stock, Reserved, Incoming, LowStockThreshold int
		WeightGrams float64
		IsActive, IsFeatured bool
		Sections []string
		Images   []fiber.Map
	}
	var p PD
	var sid, cid *string
	var w *float64
	err := database.DB.QueryRow(ctx, `SELECT id,name,slug,COALESCE(description,''),COALESCE(short_description,''),COALESCE(ingredients,''),COALESCE(how_to_use,''),COALESCE(barcode,''),brand_id,category_id,price,COALESCE(compare_at_price,0),stock,reserved,incoming,low_stock_threshold,weight_grams,is_active,is_featured,created_at,updated_at FROM products WHERE id=$1`, c.Params("id")).Scan(
		&p.ID, &p.Name, &p.Slug, &p.Description, &p.ShortDescription, &p.Ingredients, &p.HowToUse, &p.Barcode, &sid, &cid,
		&p.Price, &p.CompareAtPrice, &p.Stock, &p.Reserved, &p.Incoming, &p.LowStockThreshold, &w, &p.IsActive, &p.IsFeatured, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "not found"})
	}
	if sid != nil {
		p.BrandID = *sid
	}
	if cid != nil {
		p.CategoryID = *cid
	}
	if w != nil {
		p.WeightGrams = *w
	}

	irows, _ := database.DB.Query(ctx, "SELECT id,url,COALESCE(alt,''),sort_order FROM product_images WHERE product_id=$1 ORDER BY sort_order", c.Params("id"))
	if irows != nil {
		defer irows.Close()
		for irows.Next() {
			var id, url, alt string
			var so int
			irows.Scan(&id, &url, &alt, &so)
			p.Images = append(p.Images, fiber.Map{"id": id, "url": url, "alt": alt, "sort": so})
		}
	}

	sec := []string{}
	if p.Ingredients != "" {
		sec = append(sec, "ingredients")
	}
	if p.HowToUse != "" {
		sec = append(sec, "how_to_use")
	}
	p.Sections = sec
	return c.JSON(p)
}

func CreateProduct(c fiber.Ctx) error {
	if database.DB == nil {
		return c.Status(503).JSON(fiber.Map{"error": "db not connected"})
	}
	body := struct {
		Name             string   `json:"name"`
		Description      string   `json:"description"`
		ShortDescription string   `json:"short_description"`
		Ingredients      string   `json:"ingredients"`
		HowToUse         string   `json:"how_to_use"`
		Barcode          string   `json:"barcode"`
		BrandID          string   `json:"brand_id"`
		CategoryID       string   `json:"category_id"`
		Sections         string   `json:"sections"`
		Price            int64    `json:"price"`
		CompareAtPrice   int64    `json:"compare_at_price"`
		Stock            int      `json:"stock"`
		LowStockThreshold int     `json:"low_stock_threshold"`
		WeightGrams      float64  `json:"weight_grams"`
		IsActive         bool     `json:"is_active"`
		IsFeatured       bool     `json:"is_featured"`
		ImageURLs        []string `json:"image_urls"`
	}{}
	if err := json.Unmarshal(c.Body(), &body); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid body"})
	}
	if body.Name == "" {
		return c.Status(400).JSON(fiber.Map{"error": "name required"})
	}

	ctx := context.Background()
	slug := strings.ToLower(strings.ReplaceAll(body.Name, " ", "-"))
	slug = strings.NewReplacer("--", "-", "'", "", "\"", "", "(", "", ")", "").Replace(slug)
	slug = strings.Trim(slug, "-")
	if slug == "" {
		slug = "product"
	}
	slug = uniqueSlug(slug, "")

	secMap := fiber.Map{}
	secParts := strings.Split(body.Sections, ",")
	for _, s := range secParts {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		switch s {
		case "ingredients":
			if body.Ingredients != "" {
				secMap[s] = true
			}
		case "how_to_use":
			if body.HowToUse != "" {
				secMap[s] = true
			}
		}
	}

	var bid, cid *string
	if body.BrandID != "" {
		bid = &body.BrandID
	}
	if body.CategoryID != "" {
		cid = &body.CategoryID
	}

	var prodID string
	err := database.DB.QueryRow(ctx, `INSERT INTO products (name,slug,description,short_description,ingredients,how_to_use,barcode,brand_id,category_id,price,compare_at_price,stock,low_stock_threshold,weight_grams,is_active,is_featured) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16) RETURNING id`,
		body.Name, slug, body.Description, body.ShortDescription, body.Ingredients, body.HowToUse, body.Barcode, bid, cid,
		body.Price, body.CompareAtPrice, body.Stock, body.LowStockThreshold, body.WeightGrams, body.IsActive, body.IsFeatured).Scan(&prodID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to create product"})
	}

	for i, url := range body.ImageURLs {
		database.DB.Exec(ctx, "INSERT INTO product_images (product_id,url,alt,sort_order) VALUES ($1,$2,$3,$4)", prodID, url, body.Name, i)
	}

	LogActivity("product_created", "product", prodID, body.Name)

	return c.JSON(fiber.Map{"success": true, "id": prodID, "slug": slug})
}

func UpdateProduct(c fiber.Ctx) error {
	if database.DB == nil {
		return c.Status(503).JSON(fiber.Map{"error": "db not connected"})
	}
	body := struct {
		Name             string   `json:"name"`
		Description      string   `json:"description"`
		ShortDescription string   `json:"short_description"`
		Ingredients      string   `json:"ingredients"`
		HowToUse         string   `json:"how_to_use"`
		Barcode          string   `json:"barcode"`
		BrandID          string   `json:"brand_id"`
		CategoryID       string   `json:"category_id"`
		Sections         string   `json:"sections"`
		Price            int64    `json:"price"`
		CompareAtPrice   int64    `json:"compare_at_price"`
		Stock            int      `json:"stock"`
		LowStockThreshold int     `json:"low_stock_threshold"`
		WeightGrams      float64  `json:"weight_grams"`
		IsActive         bool     `json:"is_active"`
		IsFeatured       bool     `json:"is_featured"`
		ImageURLs        []string `json:"image_urls"`
	}{}
	if err := json.Unmarshal(c.Body(), &body); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid body"})
	}
	ctx := context.Background()
	slug := strings.ToLower(strings.ReplaceAll(body.Name, " ", "-"))
	slug = strings.NewReplacer("--", "-", "'", "", "\"", "", "(", "", ")", "").Replace(slug)
	slug = strings.Trim(slug, "-")
	if slug == "" {
		slug = "product"
	}
	slug = uniqueSlug(slug, c.Params("id"))

	var bid, cid *string
	if body.BrandID != "" {
		bid = &body.BrandID
	}
	if body.CategoryID != "" {
		cid = &body.CategoryID
	}

	tag, err := database.DB.Exec(ctx, `UPDATE products SET name=$1,slug=$2,description=$3,short_description=$4,ingredients=$5,how_to_use=$6,barcode=$7,brand_id=$8,category_id=$9,price=$10,compare_at_price=$11,stock=$12,low_stock_threshold=$13,weight_grams=$14,is_active=$15,is_featured=$16,updated_at=now() WHERE id=$17::uuid`,
		body.Name, slug, body.Description, body.ShortDescription, body.Ingredients, body.HowToUse, body.Barcode, bid, cid,
		body.Price, body.CompareAtPrice, body.Stock, body.LowStockThreshold, body.WeightGrams, body.IsActive, body.IsFeatured, c.Params("id"))
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to update product"})
	}
	log.Printf("UPDATE product %s: rows_affected=%d", c.Params("id"), tag.RowsAffected())

	CheckLowStock()

	oldImages := []string{}
	orows, _ := database.DB.Query(ctx, "SELECT url FROM product_images WHERE product_id=$1", c.Params("id"))
	if orows != nil {
		defer orows.Close()
		for orows.Next() {
			var u string
			orows.Scan(&u)
			oldImages = append(oldImages, u)
		}
	}

	database.DB.Exec(ctx, "DELETE FROM product_images WHERE product_id=$1", c.Params("id"))
	for i, url := range body.ImageURLs {
		database.DB.Exec(ctx, "INSERT INTO product_images (product_id,url,alt,sort_order) VALUES ($1,$2,$3,$4)", c.Params("id"), url, body.Name, i)
	}

	newSet := make(map[string]bool)
	for _, u := range body.ImageURLs {
		newSet[u] = true
	}
	for _, old := range oldImages {
		if !newSet[old] {
			DeleteSupabaseObject(old)
		}
	}

	LogActivity("product_updated", "product", c.Params("id"), body.Name)

	return c.JSON(fiber.Map{"success": true, "slug": slug})
}

func UpdateProductStatus(c fiber.Ctx) error {
	if database.DB == nil {
		return c.Status(503).JSON(fiber.Map{"error": "db not connected"})
	}
	body := struct {
		IsActive   *bool `json:"is_active"`
		IsFeatured *bool `json:"is_featured"`
	}{}
	if err := json.Unmarshal(c.Body(), &body); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid body"})
	}
	ctx := context.Background()
	if body.IsActive != nil {
		database.DB.Exec(ctx, `UPDATE products SET is_active=$1, updated_at=now() WHERE id=$2::uuid`, *body.IsActive, c.Params("id"))
	}
	if body.IsFeatured != nil {
		database.DB.Exec(ctx, `UPDATE products SET is_featured=$1, updated_at=now() WHERE id=$2::uuid`, *body.IsFeatured, c.Params("id"))
	}
	return c.JSON(fiber.Map{"success": true})
}

func DeleteProduct(c fiber.Ctx) error {
	if database.DB == nil {
		return c.Status(503).JSON(fiber.Map{"error": "db not connected"})
	}
	ctx := context.Background()
	pid := c.Params("id")

	var exists int
	database.DB.QueryRow(ctx, `SELECT COUNT(*) FROM order_items WHERE product_id=$1`, pid).Scan(&exists)
	if exists > 0 {
		tag, err := database.DB.Exec(ctx, `UPDATE products SET is_active=false, updated_at=now() WHERE id=$1::uuid`, pid)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		if tag.RowsAffected() == 0 {
			return c.Status(404).JSON(fiber.Map{"error": "not found"})
		}
		return c.JSON(fiber.Map{"success": true, "archived": true, "message": "Product has order history — archived instead of deleted"})
	}

	DeleteProductImages(pid)
	database.DB.Exec(ctx, `DELETE FROM product_images WHERE product_id=$1`, pid)
	database.DB.Exec(ctx, `DELETE FROM product_variants WHERE product_id=$1`, pid)
	tag, err := database.DB.Exec(ctx, `DELETE FROM products WHERE id=$1::uuid`, pid)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to delete product"})
	}
	if tag.RowsAffected() == 0 {
		return c.Status(404).JSON(fiber.Map{"error": "not found"})
	}

	LogActivity("product_deleted", "product", pid, "")

	return c.JSON(fiber.Map{"success": true, "message": "Product deleted"})
}

func GetProducts(c fiber.Ctx) error {
	if database.DB == nil {
		return c.Status(503).JSON(fiber.Map{"error": "db not connected"})
	}
	ctx := context.Background()
	cat := c.Query("category")
	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "24"))
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 24
	}
	if limit > 50 {
		limit = 50
	}
	offset := (page - 1) * limit

	where := "WHERE p.is_active=true"
	args := []interface{}{}
	aidx := 0
	if cat != "" {
		aidx++
		where += " AND c.slug=$" + strconv.Itoa(aidx)
		args = append(args, cat)
	}

	var total int64
	countQ := "SELECT COUNT(*) FROM products p LEFT JOIN categories c ON c.id=p.category_id " + where
	database.DB.QueryRow(ctx, countQ, args...).Scan(&total)

	aidx++
	limitArg := aidx
	aidx++
	offsetArg := aidx
	args = append(args, limit, offset)
	q := `SELECT p.id,p.name,p.slug,p.price,COALESCE(p.compare_at_price,0),COALESCE(ROUND(AVG(r.rating),1),0),COUNT(r.id),COALESCE(c.name,''),COALESCE(c.slug,''),COALESCE(b.name,''),COALESCE((SELECT url FROM product_images WHERE product_id=p.id ORDER BY sort_order LIMIT 1),''),p.created_at FROM products p LEFT JOIN categories c ON c.id=p.category_id LEFT JOIN brands b ON b.id=p.brand_id LEFT JOIN reviews r ON r.product_id=p.id ` + where + ` GROUP BY p.id,c.name,c.slug,b.name ORDER BY p.created_at DESC LIMIT $` + strconv.Itoa(limitArg) + ` OFFSET $` + strconv.Itoa(offsetArg)
	rows, err := database.DB.Query(ctx, q, args...)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "database query failed"})
	}
	defer rows.Close()
	type LP struct {
		ID             string  `json:"id"`
		Name           string  `json:"name"`
		Slug           string  `json:"slug"`
		Category       string  `json:"category"`
		CategorySlug   string  `json:"categorySlug"`
		Brand          string  `json:"brand"`
		Image          string  `json:"image"`
		Price          int64   `json:"price"`
		CompareAtPrice int64   `json:"compareAtPrice"`
		Rating         float64 `json:"rating"`
		Reviews        int     `json:"reviews"`
		CreatedAt      string  `json:"createdAt"`
	}
	prods := []LP{}
	for rows.Next() {
		var p LP
		rows.Scan(&p.ID, &p.Name, &p.Slug, &p.Price, &p.CompareAtPrice, &p.Rating, &p.Reviews, &p.Category, &p.CategorySlug, &p.Brand, &p.Image, &p.CreatedAt)
		prods = append(prods, p)
	}
	return c.JSON(fiber.Map{
		"products": prods,
		"total":    total,
		"page":     page,
		"limit":    limit,
		"pages":    (total + int64(limit) - 1) / int64(limit),
	})
}

func GetProduct(c fiber.Ctx) error {
	if database.DB == nil {
		return c.Status(503).JSON(fiber.Map{"error": "db not connected"})
	}
	ctx := context.Background()
	type PubP struct {
		ID             string      `json:"id"`
		Name           string      `json:"name"`
		Slug           string      `json:"slug"`
		Description    string      `json:"description"`
		Brand          string      `json:"brand"`
		Category       string      `json:"category"`
		CreatedAt      string      `json:"createdAt"`
		Image          string      `json:"image"`
		Ingredients    string      `json:"ingredients"`
		HowToUse       string      `json:"howToUse"`
		Price          int64       `json:"price"`
		CompareAtPrice int64       `json:"compareAtPrice"`
		Stock          int         `json:"stock"`
		IsActive       bool        `json:"isActive"`
		Rating         float64     `json:"rating"`
		ReviewCount    int         `json:"reviewCount"`
		Sections       []fiber.Map `json:"sections"`
		Related        []fiber.Map `json:"related"`
	}
	var p PubP
	var ing, htu *string
	err := database.DB.QueryRow(ctx, `SELECT p.id,p.name,p.slug,COALESCE(p.description,''),p.ingredients,p.how_to_use,p.price,COALESCE(p.compare_at_price,0),p.stock,p.is_active,COALESCE(ROUND(AVG(r.rating),1),0),COUNT(r.id),COALESCE(b.name,''),COALESCE(c.name,''),COALESCE((SELECT url FROM product_images WHERE product_id=p.id ORDER BY sort_order LIMIT 1),'') FROM products p LEFT JOIN brands b ON b.id=p.brand_id LEFT JOIN categories c ON c.id=p.category_id LEFT JOIN reviews r ON r.product_id=p.id WHERE p.slug=$1 GROUP BY p.id,b.name,c.name`, c.Params("slug")).Scan(
		&p.ID, &p.Name, &p.Slug, &p.Description, &ing, &htu, &p.Price, &p.CompareAtPrice, &p.Stock, &p.IsActive, &p.Rating, &p.ReviewCount, &p.Brand, &p.Category, &p.Image)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "not found"})
	}
	if ing != nil {
		p.Ingredients = *ing
	}
	if htu != nil {
		p.HowToUse = *htu
	}

	if p.Image == "" {
		p.Image = "https://placehold.co/600x600/f5f5f6/0d0d0d?text=Product"
	}

	if p.Description != "" {
		p.Sections = append(p.Sections, fiber.Map{"title": "DESCRIPTION", "content": p.Description})
	}
	if p.Ingredients != "" {
		p.Sections = append(p.Sections, fiber.Map{"title": "INGREDIENTS", "content": "<ul><li>" + strings.ReplaceAll(p.Ingredients, "\n", "</li><li>") + "</li></ul>"})
	}
	if p.HowToUse != "" {
		p.Sections = append(p.Sections, fiber.Map{"title": "HOW TO USE", "content": "<ul><li>" + strings.ReplaceAll(p.HowToUse, "\n", "</li><li>") + "</li></ul>"})
	}

	var brandID *string
	database.DB.QueryRow(ctx, "SELECT brand_id FROM products WHERE id=$1", p.ID).Scan(&brandID)

	scanRelated := func(rows pgx.Rows) []fiber.Map {
		var out []fiber.Map
		if rows == nil { return out }
		defer rows.Close()
		for rows.Next() {
			var id, name, img string
			var price int64
			var rating float64
			var revs int
			rows.Scan(&id, &name, &price, &rating, &revs, &img)
			out = append(out, fiber.Map{"id": id, "name": name, "price": price, "rating": rating, "reviews": revs, "image": img})
		}
		return out
	}

	existingMap := func(items []fiber.Map) map[string]bool {
		m := make(map[string]bool)
		for _, r := range items { m[r["id"].(string)] = true }
		return m
	}

	relatedSQL := `SELECT p.id,p.name,p.price,COALESCE(ROUND(AVG(r.rating),1),0),COUNT(r.id),
		COALESCE((SELECT url FROM product_images WHERE product_id=p.id ORDER BY sort_order LIMIT 1),'')
		FROM products p LEFT JOIN reviews r ON r.product_id=p.id
		WHERE p.is_active=true AND p.id!=$1
		GROUP BY p.id ORDER BY COALESCE(SUM(p.orders_count),0) DESC, AVG(r.rating) DESC NULLS LAST LIMIT $2`

	catID := ""
	if p.Category != "" {
		database.DB.QueryRow(ctx, "SELECT category_id FROM products WHERE id=$1", p.ID).Scan(&catID)
	}

	if catID != "" && brandID != nil {
		rows, _ := database.DB.Query(ctx, `SELECT p.id,p.name,p.price,COALESCE(ROUND(AVG(r.rating),1),0),COUNT(r.id),
			COALESCE((SELECT url FROM product_images WHERE product_id=p.id ORDER BY sort_order LIMIT 1),'')
			FROM products p LEFT JOIN reviews r ON r.product_id=p.id
			WHERE p.is_active=true AND p.id!=$1 AND p.category_id=$2 AND p.brand_id=$3
			GROUP BY p.id ORDER BY COALESCE(SUM(p.orders_count),0) DESC, AVG(r.rating) DESC NULLS LAST LIMIT $4`, p.ID, catID, *brandID, 6)
		p.Related = append(p.Related, scanRelated(rows)...)
	}

	if len(p.Related) < 6 && catID != "" {
		existing := existingMap(p.Related)
		existing[p.ID] = true
		rows, _ := database.DB.Query(ctx, `SELECT p.id,p.name,p.price,COALESCE(ROUND(AVG(r.rating),1),0),COUNT(r.id),
			COALESCE((SELECT url FROM product_images WHERE product_id=p.id ORDER BY sort_order LIMIT 1),'')
			FROM products p LEFT JOIN reviews r ON r.product_id=p.id
			WHERE p.is_active=true AND p.id!=$1 AND p.category_id=$2
			GROUP BY p.id ORDER BY COALESCE(SUM(p.orders_count),0) DESC, AVG(r.rating) DESC NULLS LAST LIMIT $3`, p.ID, catID, 12)
		for _, item := range scanRelated(rows) {
			if len(p.Related) >= 6 { break }
			if !existing[item["id"].(string)] { existing[item["id"].(string)] = true; p.Related = append(p.Related, item) }
		}
	}

	if len(p.Related) < 6 {
		existing := existingMap(p.Related)
		existing[p.ID] = true
		need := 6 - len(p.Related)
		rows, _ := database.DB.Query(ctx, relatedSQL, p.ID, need+6)
		for _, item := range scanRelated(rows) {
			if len(p.Related) >= 6 { break }
			if !existing[item["id"].(string)] { existing[item["id"].(string)] = true; p.Related = append(p.Related, item) }
		}
	}

	return c.JSON(p)
}

func PublicCategories(c fiber.Ctx) error {
	if database.DB == nil {
		return c.Status(503).JSON(fiber.Map{"error": "db not connected"})
	}
	rows, err := database.DB.Query(context.Background(),
		`SELECT c.id, c.name, c.slug, COUNT(p.id)::int
		 FROM categories c
		 LEFT JOIN products p ON p.category_id = c.id AND p.is_active = true
		 WHERE c.is_active = true
		 GROUP BY c.id, c.name, c.slug
		 ORDER BY c.name`)
	if err != nil {
		return c.JSON(fiber.Map{"categories": []fiber.Map{}})
	}
	defer rows.Close()
	cats := []fiber.Map{}
	for rows.Next() {
		var id, name, slug string
		var count int
		rows.Scan(&id, &name, &slug, &count)
		cats = append(cats, fiber.Map{"id": id, "name": name, "slug": slug, "count": count})
	}
	return c.JSON(fiber.Map{"categories": cats})
}

func PublicBrands(c fiber.Ctx) error {
	if database.DB == nil {
		return c.Status(503).JSON(fiber.Map{"error": "db not connected"})
	}
	rows, err := database.DB.Query(context.Background(),
		`SELECT b.id, b.name, b.slug, COUNT(p.id)::int
		 FROM brands b
		 LEFT JOIN products p ON p.brand_id = b.id AND p.is_active = true
		 WHERE b.is_active = true
		 GROUP BY b.id, b.name, b.slug
		 ORDER BY b.name`)
	if err != nil {
		return c.JSON(fiber.Map{"brands": []fiber.Map{}})
	}
	defer rows.Close()
	brands := []fiber.Map{}
	for rows.Next() {
		var id, name, slug string
		var count int
		rows.Scan(&id, &name, &slug, &count)
		brands = append(brands, fiber.Map{"id": id, "name": name, "slug": slug, "count": count})
	}
	return c.JSON(fiber.Map{"brands": brands})
}

func AddToCart(c fiber.Ctx) error {
	body := struct {
		ProductID string `json:"productId"`
		Quantity  int    `json:"quantity"`
	}{}
	if err := c.Bind().JSON(&body); err != nil || body.ProductID == "" {
		return c.Status(400).JSON(fiber.Map{"success": false, "message": "productId required"})
	}
	if body.Quantity < 1 {
		body.Quantity = 1
	}
	if database.DB == nil {
		return c.JSON(fiber.Map{"success": true, "message": "Added to cart (demo)"})
	}
	ctx := context.Background()
	var price int64
	var stock int
	var isActive bool
	var name string
	err := database.DB.QueryRow(ctx, `SELECT price, stock, is_active, name FROM products WHERE id=$1::uuid`, body.ProductID).Scan(&price, &stock, &isActive, &name)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"success": false, "message": "Product not found"})
	}
	if !isActive {
		return c.Status(400).JSON(fiber.Map{"success": false, "message": "Product not available"})
	}
	if stock < body.Quantity {
		return c.Status(400).JSON(fiber.Map{"success": false, "message": "Insufficient stock"})
	}
	return c.JSON(fiber.Map{"success": true, "message": "Added to cart", "product": fiber.Map{"id": body.ProductID, "name": name, "price": price, "qty": body.Quantity}})
}
