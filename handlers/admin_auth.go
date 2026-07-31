package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v3"
	"golang.org/x/crypto/bcrypt"
)

type adminSession struct {
	Email     string
	ExpiresAt time.Time
}

var (
	adminSessions   = make(map[string]*adminSession)
	adminSessionsMu sync.RWMutex
)

func generateAdminToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func createAdminSession(email string) string {
	token := generateAdminToken()
	adminSessionsMu.Lock()
	adminSessions[token] = &adminSession{
		Email:     email,
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	adminSessionsMu.Unlock()
	return token
}

func validateAdminToken(token string) (string, bool) {
	adminSessionsMu.RLock()
	s, ok := adminSessions[token]
	adminSessionsMu.RUnlock()
	if !ok || time.Now().After(s.ExpiresAt) {
		if ok {
			adminSessionsMu.Lock()
			delete(adminSessions, token)
			adminSessionsMu.Unlock()
		}
		return "", false
	}
	return s.Email, true
}

func AdminAuthMiddleware(c fiber.Ctx) error {
	auth := c.Get("Authorization")
	token := strings.TrimPrefix(auth, "Bearer ")
	if token == "" || token == auth {
		return c.Status(401).JSON(fiber.Map{"success": false, "message": "Unauthorized"})
	}
	email, ok := validateAdminToken(token)
	if !ok {
		return c.Status(401).JSON(fiber.Map{"success": false, "message": "Invalid or expired token"})
	}
	c.Locals("admin_email", email)
	return c.Next()
}

func Login(c fiber.Ctx) error {
	email := c.FormValue("email")
	pass := c.FormValue("password")
	if email == "" || pass == "" {
		return c.Status(401).JSON(fiber.Map{"success": false, "message": "Invalid credentials"})
	}
	if email != os.Getenv("ADMIN_USER") {
		return c.Status(401).JSON(fiber.Map{"success": false, "message": "Invalid credentials"})
	}
	hash := os.Getenv("ADMIN_PASS_HASH")
	plain := os.Getenv("ADMIN_PASS")
	if hash != "" {
		if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(pass)); err != nil {
			return c.Status(401).JSON(fiber.Map{"success": false, "message": "Invalid credentials"})
		}
	} else if pass != plain {
		return c.Status(401).JSON(fiber.Map{"success": false, "message": "Invalid credentials"})
	}
	token := createAdminSession(email)
	return c.JSON(fiber.Map{"success": true, "message": "Login successful", "token": token})
}
