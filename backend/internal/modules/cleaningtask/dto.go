package cleaningtask

import (
	"fmt"

	"github.com/gofiber/fiber/v2"

	"github.com/drainage/desilting/internal/httpx"
	"github.com/drainage/desilting/internal/modules/pipesegment"
	"github.com/drainage/desilting/internal/shared/date"
	"github.com/drainage/desilting/internal/shared/refx"
)

// SaveRequest 新增或修改清淤任务的请求体。
type SaveRequest struct {
	Title         string    `json:"title" label:"任务标题" validate:"required,max=128"`
	PipeSegmentID uint      `json:"pipeSegmentId" label:"关联管段" validate:"required"`
	Priority      string    `json:"priority" label:"优先级"`
	Source        string    `json:"source" label:"任务来源"`
	Method        string    `json:"method" label:"清淤方式"`
	PlanStartDate date.Date `json:"planStartDate" label:"计划开始日期"`
	PlanEndDate   date.Date `json:"planEndDate" label:"计划完成日期"`
	TeamName      string    `json:"teamName" label:"实施班组" validate:"max=64"`
	LeaderName    string    `json:"leaderName" label:"现场负责人" validate:"max=32"`
	LeaderPhone   string    `json:"leaderPhone" label:"联系电话" validate:"max=32"`
	Description   string    `json:"description" label:"任务说明" validate:"max=1000"`
}

// CancelRequest 取消任务请求体。
type CancelRequest struct {
	Reason string `json:"reason" label:"取消原因" validate:"required,max=255"`
}

// ListQuery 任务列表查询条件。
type ListQuery struct {
	Keyword       string
	Status        string
	District      string
	RoadName      string
	Priority      string
	Source        string
	TeamName      string
	PipeSegmentID uint
	PlanFrom      *date.Date
	PlanTo        *date.Date
	Page          httpx.PageQuery
}

// ParseListQuery 解析任务列表查询条件。
func ParseListQuery(c *fiber.Ctx) (ListQuery, error) {
	query := ListQuery{
		Keyword:       httpx.TrimmedQuery(c, "keyword"),
		Status:        httpx.TrimmedQuery(c, "status"),
		District:      httpx.TrimmedQuery(c, "district"),
		RoadName:      httpx.TrimmedQuery(c, "roadName"),
		Priority:      httpx.TrimmedQuery(c, "priority"),
		Source:        httpx.TrimmedQuery(c, "source"),
		TeamName:      httpx.TrimmedQuery(c, "teamName"),
		PipeSegmentID: uint(c.QueryInt("pipeSegmentId", 0)),
		Page:          httpx.ParsePage(c),
	}
	from, err := parseDateParam(c, "planFrom", "计划开始日期起")
	if err != nil {
		return ListQuery{}, err
	}
	to, err := parseDateParam(c, "planTo", "计划开始日期止")
	if err != nil {
		return ListQuery{}, err
	}
	query.PlanFrom = from
	query.PlanTo = to
	return query, nil
}

// Validate 校验组合条件本身是否自洽。
//
// 筛选条件不会因为组合冲突而报 5xx，但起止日期颠倒属于明确的错误请求，
// 直接返回 400 提示，避免静默返回空数据让用户误判。
func (q ListQuery) Validate() error {
	if q.PlanFrom != nil && q.PlanTo != nil && q.PlanTo.Before(*q.PlanFrom) {
		return httpx.BadRequest("计划时间段截止日期不能早于开始日期")
	}
	return nil
}

func parseDateParam(c *fiber.Ctx, key, label string) (*date.Date, error) {
	raw := httpx.TrimmedQuery(c, key)
	if raw == "" {
		return nil, nil
	}
	parsed, err := date.Parse(raw)
	if err != nil {
		return nil, httpx.BadRequest(fmt.Sprintf("%s格式不正确，应为 YYYY-MM-DD", label))
	}
	return &parsed, nil
}

// ListSummary 与筛选结果严格一致的任务 / 清淤量汇总。
//
// 统计口径：当前全部筛选条件命中的任务集合（不受分页影响），
// 清淤量取这些任务下全部清淤记录的合计。
type ListSummary struct {
	TaskCount      int64   `json:"taskCount"`
	RecordCount    int64   `json:"recordCount"`
	SludgeVolumeM3 float64 `json:"sludgeVolumeM3"`
	CleanedLengthM float64 `json:"cleanedLengthM"`
}

// ListResponse 任务列表：分页数据 + 同口径汇总。
type ListResponse struct {
	List     []ListItem  `json:"list"`
	Total    int64       `json:"total"`
	Page     int         `json:"page"`
	PageSize int         `json:"pageSize"`
	Summary  ListSummary `json:"summary"`
}

// FilterOptionsResponse 任务筛选栏可选项（片区 / 道路 / 班组）。
type FilterOptionsResponse struct {
	Districts []string                 `json:"districts"`
	Roads     []pipesegment.RoadOption `json:"roads"`
	Teams     []string                 `json:"teams"`
}

// ListItem 任务列表项：任务本体 + 管段信息 + 清淤汇总。
type ListItem struct {
	CleaningTask
	Segment      *pipesegment.Brief `json:"segment"`
	RecordTotals refx.RecordTotals  `json:"recordTotals"`
}

// DetailResponse 任务详情：任务 + 管段 + 清淤汇总 + 验收结论 + 可执行操作。
type DetailResponse struct {
	Task           *CleaningTask         `json:"task"`
	Segment        *pipesegment.Brief    `json:"segment"`
	RecordTotals   refx.RecordTotals     `json:"recordTotals"`
	Acceptance     *refx.AcceptanceBrief `json:"acceptance"`
	AllowedActions []string              `json:"allowedActions"`
}
