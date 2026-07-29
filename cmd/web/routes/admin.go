package routes

import (
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/yourorg/ecommerce/handlers"
)

func RegisterAdmin(admin fiber.Router) {
	admin.Post("/login", handlers.RateLimit(5, 1*time.Minute), handlers.Login)

	admin.Use(handlers.AdminAuthMiddleware)

	admin.Get("/dashboard", handlers.AdminDashboard)
	admin.Get("/orders", handlers.AdminOrders)
	admin.Get("/products", handlers.AdminProducts)
	admin.Get("/customers", handlers.AdminCustomers)
	admin.Get("/notifications", handlers.AdminNotifications)

	admin.Get("/brands", handlers.AdminBrands)
	admin.Get("/categories", handlers.AdminCategories)
	admin.Post("/brands", handlers.CreateBrand)
	admin.Post("/categories", handlers.CreateCategory)

	admin.Get("/products/detail/:id", handlers.AdminProductDetail)
	admin.Post("/products", handlers.CreateProduct)
	admin.Put("/products/:id", handlers.UpdateProduct)
	admin.Patch("/products/:id/status", handlers.UpdateProductStatus)
	admin.Delete("/products/:id", handlers.DeleteProduct)

	admin.Post("/upload", handlers.AdminUpload)

	admin.Put("/store-info", handlers.UpdateStoreInfo)

	admin.Get("/packs", handlers.AdminGetPacks)
	admin.Post("/packs", handlers.CreatePack)
	admin.Put("/packs/:id", handlers.UpdatePack)
	admin.Delete("/packs/:id", handlers.DeletePack)

	admin.Put("/orders/:id/status", handlers.UpdateOrderStatus)
}
