package cleaningtask

import (
	"github.com/gofiber/fiber/v2"

	"github.com/drainage/desilting/internal/httpx"
)

// Handler 清淤任务 HTTP 接口。
type Handler struct {
	svc *Service
}

// NewHandler 构造处理器。
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// List 任务列表（含与筛选结果同口径的数量 / 清淤量汇总）。
func (h *Handler) List(c *fiber.Ctx) error {
	query, err := ParseListQuery(c)
	if err != nil {
		return err
	}
	if err := query.Validate(); err != nil {
		return err
	}
	items, total, summary, err := h.svc.ListPage(c.UserContext(), query)
	if err != nil {
		return err
	}
	return httpx.OK(c, ListResponse{
		List:     items,
		Total:    total,
		Page:     query.Page.Page,
		PageSize: query.Page.PageSize,
		Summary:  summary,
	})
}

// FilterOptions 任务筛选栏可选项。
func (h *Handler) FilterOptions(c *fiber.Ctx) error {
	options, err := h.svc.FilterOptions(c.UserContext(), httpx.TrimmedQuery(c, "district"))
	if err != nil {
		return err
	}
	return httpx.OK(c, options)
}

// Export 按当前筛选条件导出全部任务为 CSV（含合计行）。
func (h *Handler) Export(c *fiber.Ctx) error {
	query, err := ParseListQuery(c)
	if err != nil {
		return err
	}
	if err := query.Validate(); err != nil {
		return err
	}
	return h.writeExport(c, query)
}

// Create 登记任务。
func (h *Handler) Create(c *fiber.Ctx) error {
	var req SaveRequest
	if err := httpx.BindAndValidate(c, &req); err != nil {
		return err
	}
	task, err := h.svc.Create(c.UserContext(), req)
	if err != nil {
		return err
	}
	return httpx.Created(c, task)
}

// Detail 任务详情。
func (h *Handler) Detail(c *fiber.Ctx) error {
	id, err := httpx.PathID(c, "id", "任务")
	if err != nil {
		return err
	}
	detail, err := h.svc.Detail(c.UserContext(), id)
	if err != nil {
		return err
	}
	return httpx.OK(c, detail)
}

// Update 修改任务。
func (h *Handler) Update(c *fiber.Ctx) error {
	id, err := httpx.PathID(c, "id", "任务")
	if err != nil {
		return err
	}
	var req SaveRequest
	if err := httpx.BindAndValidate(c, &req); err != nil {
		return err
	}
	task, err := h.svc.Update(c.UserContext(), id, req)
	if err != nil {
		return err
	}
	return httpx.Message(c, "任务已更新", task)
}

// Delete 删除任务。
func (h *Handler) Delete(c *fiber.Ctx) error {
	id, err := httpx.PathID(c, "id", "任务")
	if err != nil {
		return err
	}
	if err := h.svc.Delete(c.UserContext(), id); err != nil {
		return err
	}
	return httpx.Message(c, "任务已删除", fiber.Map{"id": id})
}

// Start 开工。
func (h *Handler) Start(c *fiber.Ctx) error {
	id, err := httpx.PathID(c, "id", "任务")
	if err != nil {
		return err
	}
	task, err := h.svc.Start(c.UserContext(), id)
	if err != nil {
		return err
	}
	return httpx.Message(c, "任务已开工", task)
}

// Complete 完工报验。
func (h *Handler) Complete(c *fiber.Ctx) error {
	id, err := httpx.PathID(c, "id", "任务")
	if err != nil {
		return err
	}
	task, err := h.svc.Complete(c.UserContext(), id)
	if err != nil {
		return err
	}
	return httpx.Message(c, "任务已提交完工报验，等待验收", task)
}

// Cancel 取消任务。
func (h *Handler) Cancel(c *fiber.Ctx) error {
	id, err := httpx.PathID(c, "id", "任务")
	if err != nil {
		return err
	}
	var req CancelRequest
	if err := httpx.BindAndValidate(c, &req); err != nil {
		return err
	}
	task, err := h.svc.Cancel(c.UserContext(), id, req.Reason)
	if err != nil {
		return err
	}
	return httpx.Message(c, "任务已取消", task)
}
