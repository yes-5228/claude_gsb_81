package cleaningtask

import (
	"encoding/csv"
	"strconv"

	"github.com/gofiber/fiber/v2"

	"github.com/drainage/desilting/internal/httpx"
	"github.com/drainage/desilting/internal/shared/num"
	"github.com/drainage/desilting/internal/shared/option"
)

// exportBatchSize 导出时每批查询 / 写出的任务数量，
// 任务规模很大时内存占用也保持在一批以内。
const exportBatchSize = 500

// exportHeaders CSV 表头，顺序与 taskExportRow 保持一致。
var exportHeaders = []string{
	"任务编号", "任务标题", "所属片区", "所在道路", "管段编号", "管段名称",
	"任务状态", "优先级", "任务来源", "实施班组", "现场负责人", "联系电话",
	"计划开始日期", "计划完成日期",
	"清淤记录数(条)", "清淤量(m³)", "清淤长度(m)", "最近清淤日期",
}

// writeExport 把当前筛选条件命中的全部任务写成 CSV。
//
// 导出与列表接口共用 ListQuery 与汇总逻辑：导出文件的数据行数等于 summary 中
// 的任务数，末尾合计行与页面顶部的汇总数字来自同一次聚合。
// 分批按 id 向后翻（keyset pagination），避免深分页 OFFSET 在大数据量下变慢。
func (h *Handler) writeExport(c *fiber.Ctx, query ListQuery) error {
	summary, err := h.svc.repo.Summary(c.UserContext(), query)
	if err != nil {
		return httpx.WrapInternal("统计清淤任务汇总失败", err)
	}

	filename := "attachment; filename=cleaning-tasks.csv; filename*=UTF-8''cleaning-tasks.csv"
	c.Set(fiber.HeaderContentType, "text/csv; charset=utf-8")
	c.Set(fiber.HeaderContentDisposition, filename)

	// Excel 打开无 BOM 的 UTF-8 CSV 会把中文识别成乱码。
	if _, err := c.Write([]byte{0xEF, 0xBB, 0xBF}); err != nil {
		return err
	}

	writer := csv.NewWriter(c)
	if err := writer.Write(exportHeaders); err != nil {
		return err
	}

	var lastID uint
	remaining := summary.TaskCount
	for remaining > 0 {
		items, err := h.svc.ExportBatch(c.UserContext(), query, lastID, exportBatchSize)
		if err != nil {
			return err
		}
		if len(items) == 0 {
			break
		}
		for i := range items {
			if err := writer.Write(taskExportRow(items[i])); err != nil {
				return err
			}
		}
		// 每批立即刷给客户端，避免响应体在内存里堆积。
		writer.Flush()
		if err := writer.Error(); err != nil {
			return err
		}
		lastID = items[len(items)-1].ID
		remaining -= int64(len(items))
		if len(items) < exportBatchSize {
			break
		}
	}

	// 合计行：条数与合计与页面筛选汇总严格一致。
	if err := writer.Write([]string{
		"合计", strconv.FormatInt(summary.TaskCount, 10), "", "", "", "",
		"", "", "", "", "", "", "", "",
		strconv.FormatInt(summary.RecordCount, 10),
		strconv.FormatFloat(summary.SludgeVolumeM3, 'f', 2, 64),
		strconv.FormatFloat(summary.CleanedLengthM, 'f', 2, 64),
		"",
	}); err != nil {
		return err
	}
	writer.Flush()
	return writer.Error()
}

// taskExportRow 把单条任务转成 CSV 行，空值统一输出空串。
func taskExportRow(item ListItem) []string {
	segmentCode, segmentName, district, road := "", "", "", ""
	if item.Segment != nil {
		segmentCode = item.Segment.Code
		segmentName = item.Segment.Name
		district = item.Segment.District
		road = item.Segment.RoadName
	}
	return []string{
		item.Code,
		item.Title,
		district,
		road,
		segmentCode,
		segmentName,
		option.Label(StatusOptions(), item.Status),
		option.Label(PriorityOptions(), item.Priority),
		option.Label(SourceOptions(), item.Source),
		item.TeamName,
		item.LeaderName,
		item.LeaderPhone,
		item.PlanStartDate.String(),
		item.PlanEndDate.String(),
		strconv.FormatInt(item.RecordTotals.RecordCount, 10),
		strconv.FormatFloat(num.Round2(item.RecordTotals.SludgeVolumeM3), 'f', 2, 64),
		strconv.FormatFloat(num.Round2(item.RecordTotals.CleanedLengthM), 'f', 2, 64),
		item.RecordTotals.LatestCleanedAt.String(),
	}
}
