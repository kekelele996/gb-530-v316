package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"radiation-dose-budget-control/backend/internal/dto"
	"radiation-dose-budget-control/backend/internal/service"
)

type TemporaryLimitAdjustmentHandler struct {
	service *service.TemporaryLimitAdjustmentService
}

func NewTemporaryLimitAdjustmentHandler(service *service.TemporaryLimitAdjustmentService) *TemporaryLimitAdjustmentHandler {
	return &TemporaryLimitAdjustmentHandler{service: service}
}

func (handler *TemporaryLimitAdjustmentHandler) List(context *gin.Context) {
	page, pageSize := Pagination(context)
	items, meta, err := handler.service.List(page, pageSize, context.Query("worker_id"), context.Query("status"))
	if err != nil {
		WriteError(context, err)
		return
	}
	WritePage(context, items, meta)
}

func (handler *TemporaryLimitAdjustmentHandler) Get(context *gin.Context) {
	id, err := PathID(context)
	if err != nil {
		WriteError(context, err)
		return
	}
	item, err := handler.service.Get(id)
	if err != nil {
		WriteError(context, err)
		return
	}
	WriteData(context, http.StatusOK, item)
}

func (handler *TemporaryLimitAdjustmentHandler) Create(context *gin.Context) {
	var request dto.CreateTemporaryLimitAdjustmentRequest
	if err := BindAndValidate(context, &request); err != nil {
		WriteError(context, err)
		return
	}
	item, err := handler.service.Create(request, Actor(context), RequestID(context))
	if err != nil {
		WriteError(context, err)
		return
	}
	WriteData(context, http.StatusCreated, item)
}

func (handler *TemporaryLimitAdjustmentHandler) Review(context *gin.Context) {
	id, err := PathID(context)
	if err != nil {
		WriteError(context, err)
		return
	}
	var request dto.ReviewTemporaryLimitAdjustmentRequest
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
