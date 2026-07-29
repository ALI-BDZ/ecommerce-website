package handlers

import (
	"sync"
	"time"

	"github.com/gofiber/fiber/v3"
)

// Rate limiter using in-memory sliding window
type rateLimiter struct {
	mu       sync.Mutex
	requests map[string][]time.Time
	limit    int
	window   time.Duration
}

var limiter = &rateLimiter{
	requests: make(map[string][]time.Time),
	limit:    30,
	window:   1 * time.Minute,
}

// RateLimit returns 429 if client exceeds limit requests per window
func RateLimit(limit int, window time.Duration) fiber.Handler {
	rl := &rateLimiter{
		requests: make(map[string][]time.Time),
		limit:    limit,
		window:   window,
	}
	return func(c fiber.Ctx) error {
		ip := c.IP()
		now := time.Now()
		cutoff := now.Add(-rl.window)

		rl.mu.Lock()
		reqs := rl.requests[ip]
		valid := make([]time.Time, 0, len(reqs))
		for _, t := range reqs {
			if t.After(cutoff) {
				valid = append(valid, t)
			}
		}
		if len(valid) >= rl.limit {
			rl.mu.Unlock()
			return c.Status(429).JSON(fiber.Map{
				"success": false,
				"message": "Too many requests. Please try again later.",
			})
		}
		rl.requests[ip] = append(valid, now)
		rl.mu.Unlock()
		return c.Next()
	}
}

// SecurityHeaders adds standard security headers
func SecurityHeaders(c fiber.Ctx) error {
	c.Set("X-Content-Type-Options", "nosniff")
	c.Set("X-Frame-Options", "DENY")
	c.Set("X-XSS-Protection", "1; mode=block")
	c.Set("Referrer-Policy", "strict-origin-when-cross-origin")
	return c.Next()
}

// HTTPSRedirect redirects HTTP to HTTPS (use behind reverse proxy)
func HTTPSRedirect(c fiber.Ctx) error {
	if c.Get("X-Forwarded-Proto") == "http" {
		return c.Redirect().Status(fiber.StatusMovedPermanently).To("https://" + c.Host() + c.OriginalURL())
	}
	return c.Next()
}
