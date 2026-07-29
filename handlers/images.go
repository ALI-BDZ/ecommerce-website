package handlers

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "image/gif"
	_ "image/png"

	"github.com/gofiber/fiber/v3"
	"github.com/yourorg/ecommerce/database"
	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

const (
	maxUploadSize   = 10 << 20 // 10 MB
	thumbSize       = 320
	cardSize        = 640
	detailSize      = 1200
	jpegQuality     = 85
)

var allowedMimeTypes = map[string]bool{
	"image/jpeg": true,
	"image/png":  true,
	"image/webp": true,
}

// ImageSizes holds the 4 processed variants
type ImageSizes struct {
	Thumb    []byte // 320px
	Card     []byte // 640px
	Detail   []byte // 1200px
	Original []byte // Original dimensions, WebP encoded
}

// ProcessImage decodes, resizes, and encodes to JPEG in 4 sizes
func ProcessImage(data []byte, filename string) (*ImageSizes, error) {
	img, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode failed: %w (format: %s)", err, format)
	}

	bounds := img.Bounds()
	origW := bounds.Dx()
	origH := bounds.Dy()

	result := &ImageSizes{}

	sizes := []struct {
		name string
		max  int
		dst  *[]byte
	}{
		{"thumb", thumbSize, &result.Thumb},
		{"card", cardSize, &result.Card},
		{"detail", detailSize, &result.Detail},
	}

	for _, s := range sizes {
		resized := resizeImage(img, origW, origH, s.max)
		encoded, err := encodeJPEG(resized, jpegQuality)
		if err != nil {
			return nil, fmt.Errorf("encode %s failed: %w", s.name, err)
		}
		*s.dst = encoded
	}

	origEncoded, err := encodeJPEG(img, jpegQuality)
	if err != nil {
		return nil, fmt.Errorf("encode original failed: %w", err)
	}
	result.Original = origEncoded

	return result, nil
}

// resizeImage resizes to fit within maxPx while maintaining aspect ratio
func resizeImage(src image.Image, srcW, srcH, maxPx int) image.Image {
	if srcW <= maxPx && srcH <= maxPx {
		return src
	}

	var newW, newH int
	if srcW > srcH {
		newW = maxPx
		newH = srcH * maxPx / srcW
	} else {
		newH = maxPx
		newW = srcW * maxPx / srcH
	}

	dst := image.NewRGBA(image.Rect(0, 0, newW, newH))
	draw.ApproxBiLinear.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Over, nil)
	return dst
}

// encodeJPEG encodes image to JPEG — much faster than WebP (native stdlib)
func encodeJPEG(img image.Image, quality int) ([]byte, error) {
	var buf bytes.Buffer
	err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality})
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ValidateUpload checks size and MIME type
func ValidateUpload(data []byte, filename string) error {
	if int64(len(data)) > maxUploadSize {
		return fmt.Errorf("file too large: %d bytes (max %d)", len(data), maxUploadSize)
	}

	// Detect MIME type from content, not extension
	contentType := http.DetectContentType(data)
	if !allowedMimeTypes[contentType] {
		return fmt.Errorf("unsupported file type: %s (allowed: jpeg, png, webp)", contentType)
	}

	return nil
}

// UploadImages uploads all 4 sizes to Supabase Storage and returns URLs
func UploadImages(sizes *ImageSizes, baseName string) (map[string]string, error) {
	supabaseURL := os.Getenv("SUPABASE_URL")
	serviceRole := os.Getenv("SUPABASE_SERVICE_ROLE")
	bucket := "products"

	if supabaseURL == "" || serviceRole == "" {
		return nil, fmt.Errorf("supabase credentials not configured")
	}

	ext := filepath.Ext(baseName)
	if ext == "" {
		ext = ".jpg"
	}
	base := strings.TrimSuffix(baseName, ext)
	base = strings.ReplaceAll(base, " ", "-")
	base = strings.ToLower(base)
	timestamp := time.Now().UnixNano()

	variants := []struct {
		name string
		data []byte
		size int
	}{
		{"thumb", sizes.Thumb, thumbSize},
		{"card", sizes.Card, cardSize},
		{"detail", sizes.Detail, detailSize},
		{"original", sizes.Original, 0},
	}

	urls := make(map[string]string)
	for _, v := range variants {
		var objectName string
		if v.size > 0 {
			objectName = fmt.Sprintf("products/%s_%d_%dpx.jpg", base, timestamp, v.size)
		} else {
			objectName = fmt.Sprintf("products/%s_%d.jpg", base, timestamp)
		}

		uploadURL := fmt.Sprintf("%s/storage/v1/object/%s/%s", supabaseURL, bucket, objectName)
		req, _ := http.NewRequest("POST", uploadURL, bytes.NewReader(v.data))
		req.Header.Set("Authorization", "Bearer "+serviceRole)
		req.Header.Set("Content-Type", "image/jpeg")

		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("upload %s failed: %w", v.name, err)
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 300 {
			body, _ := io.ReadAll(resp.Body)
			return nil, fmt.Errorf("supabase error %d for %s: %s", resp.StatusCode, v.name, string(body))
		}

		publicURL := fmt.Sprintf("%s/storage/v1/object/public/%s/%s", supabaseURL, bucket, objectName)
		urls[v.name] = publicURL
	}

	return urls, nil
}

// DeleteSupabaseObject deletes a file from Supabase Storage by URL
func DeleteSupabaseObject(fileURL string) error {
	supabaseURL := os.Getenv("SUPABASE_URL")
	serviceRole := os.Getenv("SUPABASE_SERVICE_ROLE")
	if supabaseURL == "" || serviceRole == "" {
		return nil // silently skip if not configured
	}

	// Extract object path from public URL
	// Format: https://xxx.supabase.co/storage/v1/object/public/products/filename
	prefix := supabaseURL + "/storage/v1/object/public/"
	if !strings.HasPrefix(fileURL, prefix) {
		return nil // not a Supabase URL
	}
	objectPath := strings.TrimPrefix(fileURL, prefix)
	bucket := "products"
	if !strings.HasPrefix(objectPath, bucket+"/") {
		return nil
	}
	objectName := strings.TrimPrefix(objectPath, bucket+"/")

	deleteURL := fmt.Sprintf("%s/storage/v1/object/%s/%s", supabaseURL, bucket, objectName)
	req, _ := http.NewRequest("DELETE", deleteURL, nil)
	req.Header.Set("Authorization", "Bearer "+serviceRole)

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("delete failed: %w", err)
	}
	defer resp.Body.Close()

	// 404 is fine — already deleted
	if resp.StatusCode >= 300 && resp.StatusCode != 404 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("supabase delete error %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// DeleteProductImages deletes all image variants for a product from storage
func DeleteProductImages(productID string) {
	if database.DB == nil {
		return
	}
	ctx := context.Background()
	rows, err := database.DB.Query(ctx,
		"SELECT url FROM product_images WHERE product_id=$1", productID)
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		var url string
		rows.Scan(&url)
		if url != "" {
			DeleteSupabaseObject(url)
		}
	}
}

// GenerateSrcset returns a srcset string for responsive images
// primaryURL is the main image URL from the database
func GenerateSrcset(primaryURL string) string {
	if primaryURL == "" || strings.Contains(primaryURL, "placehold.co") {
		return ""
	}

	// For Supabase-stored processed images, construct srcset from known variants
	// The primary URL is typically the original or card size
	// We'll return the primary URL with a descriptor since we store separate files
	return ""
}

// IsWebpURL checks if a URL points to a WebP image
func IsWebpURL(url string) bool {
	return strings.HasSuffix(strings.ToLower(url), ".webp")
}

func AdminUpload(c fiber.Ctx) error {
	file, err := c.FormFile("file")
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "no file"})
	}
	src, err := file.Open()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "cannot open"})
	}
	defer src.Close()

	data, err := io.ReadAll(src)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "cannot read"})
	}

	if err := ValidateUpload(data, file.Filename); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	sizes, err := ProcessImage(data, file.Filename)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "image processing failed: " + err.Error()})
	}

	urls, err := UploadImages(sizes, file.Filename)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "upload failed: " + err.Error()})
	}

	return c.JSON(fiber.Map{
		"success":  true,
		"url":      urls["card"],
		"thumb":    urls["thumb"],
		"card":     urls["card"],
		"detail":   urls["detail"],
		"original": urls["original"],
	})
}
