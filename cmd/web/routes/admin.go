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
	admin.Put("/brands/:id", handlers.UpdateBrand)
	admin.Delete("/brands/:id", handlers.DeleteBrand)
	admin.Put("/categories/:id", handlers.UpdateCategory)
	admin.Delete("/categories/:id", handlers.DeleteCategory)

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

	admin.Get("/reviews", handlers.AdminGetReviews)
	admin.Put("/reviews/:id/approve", handlers.AdminApproveReview)
	admin.Delete("/reviews/:id", handlers.AdminDeleteReview)

	admin.Get("/staff", handlers.AdminGetStaff)
	admin.Post("/staff", handlers.AdminCreateStaff)
	admin.Put("/staff/:id", handlers.AdminUpdateStaff)
	admin.Delete("/staff/:id", handlers.AdminDeleteStaff)

	admin.Get("/content", handlers.AdminGetContent)
	admin.Post("/content", handlers.AdminUpsertContent)
	admin.Put("/content/:id", handlers.AdminUpsertContent)
	admin.Delete("/content/:id", handlers.AdminDeleteContent)

	admin.Get("/products/:id/variants", handlers.GetProductVariants)
	admin.Post("/variants", handlers.AdminCreateVariant)
	admin.Put("/variants/:id", handlers.AdminUpdateVariant)
	admin.Delete("/variants/:id", handlers.AdminDeleteVariant)

	admin.Get("/customers/search", handlers.AdminSearchCustomers)
	admin.Get("/customers/:id", handlers.AdminGetCustomerDetail)
}
