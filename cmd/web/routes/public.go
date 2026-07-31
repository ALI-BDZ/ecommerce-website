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
	api.Get("/categories", handlers.PublicCategories)
	api.Get("/brands", handlers.PublicBrands)
	api.Get("/products/:slug", handlers.GetProduct)
	api.Post("/cart/add", handlers.AddToCart)
	api.Post("/orders", handlers.RateLimit(10, 1*time.Minute), handlers.CreateOrder)
	api.Get("/orders/:id", handlers.GetOrder)

	api.Get("/products/:id/reviews", handlers.GetProductReviews)
	api.Post("/reviews", handlers.SubmitReview)

	api.Get("/content", handlers.GetContentPages)
	api.Get("/content/:slug", handlers.GetContentBySlug)

	api.Get("/products/:id/variants", handlers.GetProductVariants)
}
