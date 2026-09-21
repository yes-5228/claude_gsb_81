package cleaningtask

import (
	"encoding/csv"
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/drainage/desilting/internal/httpx"
	"github.com/drainage/desilting/internal/shared/option"
)

// Handler 清淤任务 HTTP 接口。
type Handler struct {
	svc *Service
}

// NewHandler 构造处理器。
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// List 任务列表：分页数据 + 与筛选结果一致的任务数和清淤量汇总。
func (h *Handler) List(c *fiber.Ctx) error {
	query, err := ParseListQuery(c)
	if err != nil {
		return err
	}
	result, err := h.svc.List(c.UserContext(), query)
	if err != nil {
		return err
	}
	return httpx.OK(c, ListResponse{
		List:     result.Items,
		Total:    result.Total,
		Page:     query.Page.Page,
		PageSize: query.Page.PageSize,
		Summary:  result.Summary,
	})
}

// FilterOptions 任务筛选下拉选项（片区、道路、实施班组）。
//
// 带 district 参数时只返回该片区的道路，用于前端片区与道路联动。
func (h *Handler) FilterOptions(c *fiber.Ctx) error {
	options, err := h.svc.FilterOptions(c.UserContext(), httpx.TrimmedQuery(c, "district"))
	if err != nil {
		return err
	}
	return httpx.OK(c, options)
}

// Export 按当前筛选条件导出任务 CSV，条数与合计与列表、汇总同源。
func (h *Handler) Export(c *fiber.Ctx) error {
	query, err := ParseListQuery(c)
	if err != nil {
		return err
	}
	rows, err := h.svc.Export(c.UserContext(), query)
	if err != nil {
		return err
	}

	c.Set(fiber.HeaderContentType, "text/csv; charset=utf-8")
	filename := url.PathEscape(fmt.Sprintf("清淤任务_%s.csv", time.Now().Format("20060102150405")))
	c.Set(fiber.HeaderContentDisposition, fmt.Sprintf("attachment; filename*=UTF-8''%s", filename))
	if rows.Truncated {
		c.Set("X-Export-Truncated", "1")
	}
	c.Set("X-Export-Total", strconv.FormatInt(rows.Total, 10))

	// UTF-8 BOM，避免 Excel 打开中文乱码。
	if _, err := c.Write([]byte{0xEF, 0xBB, 0xBF}); err != nil {
		return err
	}
	writer := csv.NewWriter(c)
	records := make([][]string, 0, len(rows.Items)+3)
	records = append(records, []string{
		"任务编号", "任务标题", "所属片区", "所在道路", "管段编号", "管段名称",
		"任务状态", "优先级", "任务来源", "实施班组", "现场负责人",
		"计划开始日期", "计划完成日期", "清淤记录数", "清淤量(m³)", "清淤长度(m)",
	})
	for _, item := range rows.Items {
		records = append(records, exportRow(item))
	}
	// 合计行：口径与列表顶部汇总完全一致，不受分页影响。
	records = append(records, []string{
		"合计",
		fmt.Sprintf("任务 %d 条", rows.Summary.TaskCount),
		"", "", "", "", "", "", "", "", "", "",
		strconv.FormatInt(rows.Summary.RecordCount, 10),
		formatExportFloat(rows.Summary.SludgeVolumeM3),
		formatExportFloat(rows.Summary.CleanedLengthM),
	})
	if rows.Truncated {
		records = append(records, []string{
			fmt.Sprintf("注：符合条件的任务共 %d 条，导出上限 %d 条，仅导出前 %d 条，请缩小筛选范围后重新导出。",
				rows.Total, MaxExportRows, len(rows.Items)),
		})
	}
	return writer.WriteAll(records)
}

func exportRow(item ListItem) []string {
	district, road, segmentCode, segmentName := "", "", "", ""
	if item.Segment != nil {
		district = item.Segment.District
		road = item.Segment.RoadName
		segmentCode = item.Segment.Code
		segmentName = item.Segment.Name
	}
	return []string{
		item.Code,
		item.Title,
		district,
		road,
		segmentCode,
		segmentName,
		StatusLabel(item.Status),
		option.Label(PriorityOptions(), item.Priority),
		option.Label(SourceOptions(), item.Source),
		item.TeamName,
		item.LeaderName,
		item.PlanStartDate.String(),
		item.PlanEndDate.String(),
		strconv.FormatInt(item.RecordTotals.RecordCount, 10),
		formatExportFloat(item.RecordTotals.SludgeVolumeM3),
		formatExportFloat(item.RecordTotals.CleanedLengthM),
	}
}

func formatExportFloat(value float64) string {
	return strconv.FormatFloat(value, 'f', 2, 64)
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
