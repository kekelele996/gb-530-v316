package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"radiation-dose-budget-control/backend/internal/dto"
	"radiation-dose-budget-control/backend/internal/service"
)

type LimitAdjustmentHandler struct {
	service *service.LimitAdjustmentService
}

func NewLimitAdjustmentHandler(service *service.LimitAdjustmentService) *LimitAdjustmentHandler {
	return &LimitAdjustmentHandler{service: service}
}

func (handler *LimitAdjustmentHandler) List(context *gin.Context) {
	workerID, err := PathID(context)
	if err != nil {
		WriteError(context, err)
		return
	}
	items, err := handler.service.List(workerID)
	if err != nil {
		WriteError(context, err)
		return
	}
	WriteData(context, http.StatusOK, items)
}

func (handler *LimitAdjustmentHandler) Create(context *gin.Context) {
	workerID, err := PathID(context)
	if err != nil {
		WriteError(context, err)
		return
	}
	var request dto.CreateLimitAdjustmentRequest
	if err := BindAndValidate(context, &request); err != nil {
		WriteError(context, err)
		return
	}
	item, err := handler.service.Create(workerID, request, Actor(context), RequestID(context))
	if err != nil {
		WriteError(context, err)
		return
	}
	WriteData(context, http.StatusCreated, item)
}

func (handler *LimitAdjustmentHandler) Review(context *gin.Context) {
	id, err := PathID(context)
	if err != nil {
		WriteError(context, err)
		return
	}
	var request dto.ReviewLimitAdjustmentRequest
	if err := BindAndValidate(context, &request); err != nil {
		WriteError(context, err)
		return
	}
	item, err := handler.service.Review(id, request, Actor(context), RequestID(context))
	if err != nil {
		WriteError(context, err)
		return
	}
	WriteData(context, http.StatusOK, item)
}
