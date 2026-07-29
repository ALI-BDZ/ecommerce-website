package routes

import (
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/yourorg/ecommerce/handlers"
)

func RegisterPublic(api fiber.Router) {
	api.Get("/store-info", handlers.GetStoreInfo)
	api.Get("/products", handlers.GetProducts)
	api.Get("/packs", handlers.GetPacks)
	api.Get("/products/:slug", handlers.GetProduct)
	api.Post("/cart/add", handlers.AddToCart)
	api.Post("/orders", handlers.RateLimit(10, 1*time.Minute), handlers.CreateOrder)
	api.Get("/orders/:id", handlers.GetOrder)
}
