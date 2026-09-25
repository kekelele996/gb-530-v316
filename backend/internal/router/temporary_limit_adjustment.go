package router

import (
	"github.com/gin-gonic/gin"

	"radiation-dose-budget-control/backend/internal/constants"
	"radiation-dose-budget-control/backend/internal/handler"
	"radiation-dose-budget-control/backend/internal/middleware"
)

func registerTemporaryLimitAdjustmentRoutes(group *gin.RouterGroup, target *handler.TemporaryLimitAdjustmentHandler) {
	routes := group.Group("/limit-adjustments")
	routes.GET("", target.List)
	routes.GET("/:id", target.Get)
	routes.POST("", middleware.RBAC(constants.RolePlanner, constants.RoleAdmin), target.Create)
	routes.POST("/:id/review", middleware.RBAC(constants.RoleRPOReviewer, constants.RoleAdmin), target.Review)
}
