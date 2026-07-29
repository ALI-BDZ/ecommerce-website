package main

import (
	"log"
	"os"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/cors"
	"github.com/gofiber/fiber/v3/middleware/static"
	"github.com/joho/godotenv"
	"github.com/yourorg/ecommerce/cmd/web/routes"
	"github.com/yourorg/ecommerce/database"
	"github.com/yourorg/ecommerce/handlers"
)

func main() {
	godotenv.Load()

	if err := database.Connect(); err != nil {
		log.Printf("WARNING: database not connected: %v", err)
		log.Println("Orders will not persist to database")
	} else {
		log.Println("database connected")
		defer database.Close()
		handlers.CleanupStoreInfo()
		handlers.FixProductData()
	}

	app := fiber.New(fiber.Config{
		BodyLimit: 15 * 1024 * 1024,
	})

	app.Use(cors.New())
	app.Use(handlers.SecurityHeaders)

	app.Get("/*", static.New("./public", static.Config{
		CacheDuration: 10 * time.Second,
	}))

	app.Get("/cart", func(c fiber.Ctx) error {
		return c.SendFile("./public/cart.html")
	})

	app.Get("/health", func(c fiber.Ctx) error {
		dbOK := database.DB != nil
		return c.JSON(fiber.Map{"status": "ok", "database": dbOK})
	})

	api := app.Group("/api")
	admin := api.Group("/admin")

	routes.RegisterAdmin(admin)
	routes.RegisterPublic(api)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("server starting on :%s", port)
	log.Fatal(app.Listen(":" + port))
}
