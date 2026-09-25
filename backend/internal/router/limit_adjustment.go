package router

import (
	"github.com/gin-gonic/gin"

	"radiation-dose-budget-control/backend/internal/constants"
	"radiation-dose-budget-control/backend/internal/handler"
	"radiation-dose-budget-control/backend/internal/middleware"
)

func registerLimitAdjustmentRoutes(group *gin.RouterGroup, target *handler.LimitAdjustmentHandler) {
	group.GET("/workers/:id/adjustments", target.List)
	group.POST("/workers/:id/adjustments", middleware.RBAC(constants.RolePlanner, constants.RoleAdmin), target.Create)
	group.POST("/adjustments/:id/review", middleware.RBAC(constants.RoleRPOReviewer, constants.RoleAdmin), target.Review)
}
