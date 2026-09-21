package cleaningtask

import (
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

// Register 注册清淤任务路由，并返回 service 供其他模块装配依赖。
func Register(router fiber.Router, db *gorm.DB, segments SegmentGateway) *Service {
	svc := NewService(NewRepository(db), segments)
	handler := NewHandler(svc)

	group := router.Group("/cleaning-tasks")
	// 固定路径要注册在 /:id 之前，避免被参数路由抢先匹配。
	group.Get("/options", handler.FilterOptions)
	group.Get("/export", handler.Export)
	group.Get("", handler.List)
	group.Post("", handler.Create)
	group.Get("/:id", handler.Detail)
	group.Put("/:id", handler.Update)
	group.Delete("/:id", handler.Delete)
	group.Post("/:id/start", handler.Start)
	group.Post("/:id/complete", handler.Complete)
	group.Post("/:id/cancel", handler.Cancel)

	return svc
}
