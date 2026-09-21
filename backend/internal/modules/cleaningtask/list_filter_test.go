package cleaningtask_test

import (
	"context"
	"testing"

	"github.com/drainage/desilting/internal/httpx"
	"github.com/drainage/desilting/internal/modules/cleaningtask"
	"github.com/drainage/desilting/internal/shared/date"
	"github.com/drainage/desilting/internal/testsupport"
)

// baseTaskRequest 构造可按需覆盖字段的任务请求。
func baseTaskRequest(segmentID uint, title, team string) cleaningtask.SaveRequest {
	return cleaningtask.SaveRequest{
		Title:         title,
		PipeSegmentID: segmentID,
		Priority:      cleaningtask.PriorityNormal,
		Source:        cleaningtask.SourcePlan,
		Method:        cleaningtask.MethodHighPressure,
		PlanStartDate: date.Today().AddDays(-10),
		PlanEndDate:   date.Today().AddDays(10),
		TeamName:      team,
		LeaderName:    "负责人",
	}
}

// setupFilterFixture 构造两个片区 / 两条道路、多个班组的任务集合。
func setupFilterFixture(t *testing.T) *testsupport.Fixture {
	t.Helper()
	fixture := testsupport.NewFixture(t)

	eastRoad := fixture.CreateSegmentOnRoad(t, "PS-E-1", "城东片区", "中山路")
	westRoad := fixture.CreateSegmentOnRoad(t, "PS-W-1", "城西片区", "解放路")

	// 城东 / 中山路 / 班组甲，计划周期落在本月，两条任务各录一条记录。
	task1 := fixture.CreateTaskWith(t, func() cleaningtask.SaveRequest {
		req := baseTaskRequest(eastRoad.ID, "城东巡查任务", "班组甲")
		req.Source = cleaningtask.SourceInspection
		return req
	}())
	task2 := fixture.CreateTaskWith(t, func() cleaningtask.SaveRequest {
		req := baseTaskRequest(eastRoad.ID, "城东投诉任务", "班组甲")
		req.Source = cleaningtask.SourceComplaint
		req.PlanStartDate = date.Today().AddDays(-40)
		req.PlanEndDate = date.Today().AddDays(-30)
		return req
	}())
	fixture.CreateRecord(t, task1.ID, 8)
	fixture.CreateRecord(t, task2.ID, 4)

	// 城西 / 解放路 / 班组乙，无清淤记录。
	fixture.CreateTaskWith(t, func() cleaningtask.SaveRequest {
		req := baseTaskRequest(westRoad.ID, "城西计划任务", "班组乙")
		req.Source = cleaningtask.SourcePlan
		return req
	}())

	return fixture
}

func TestListCombinedFilters(t *testing.T) {
	fixture := setupFilterFixture(t)
	ctx := context.Background()

	cases := []struct {
		name      string
		query     cleaningtask.ListQuery
		wantCount int64
	}{
		{"无条件", cleaningtask.ListQuery{}, 3},
		{"按片区", cleaningtask.ListQuery{District: "城东片区"}, 2},
		{"按道路", cleaningtask.ListQuery{RoadName: "解放路"}, 1},
		{"片区+道路组合", cleaningtask.ListQuery{District: "城东片区", RoadName: "中山路"}, 2},
		{"来源", cleaningtask.ListQuery{Source: cleaningtask.SourceInspection}, 1},
		{"班组", cleaningtask.ListQuery{TeamName: "班组甲"}, 2},
		{"片区+班组+来源组合", cleaningtask.ListQuery{
			District: "城东片区", TeamName: "班组甲", Source: cleaningtask.SourceComplaint,
		}, 1},
		{"不存在的片区与道路组合", cleaningtask.ListQuery{District: "城东片区", RoadName: "解放路"}, 0},
		{"关键字命中班组", cleaningtask.ListQuery{Keyword: "班组乙"}, 1},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, total, err := fixture.Tasks.List(ctx, tc.query)
			testsupport.RequireNoError(t, err)
			if total != tc.wantCount {
				t.Fatalf("期望命中 %d 条，实际 %d 条", tc.wantCount, total)
			}
		})
	}
}

func TestListPlanPeriodOverlap(t *testing.T) {
	fixture := setupFilterFixture(t)
	ctx := context.Background()

	// 时间段只覆盖本月：城西任务与城东巡查任务命中，已结束的投诉任务不命中。
	query := cleaningtask.ListQuery{
		PlanFrom: date.Ptr(date.Today().AddDays(-5).String()),
		PlanTo:   date.Ptr(date.Today().AddDays(5).String()),
	}
	_, total, err := fixture.Tasks.List(ctx, query)
	testsupport.RequireNoError(t, err)
	if total != 2 {
		t.Fatalf("时间段重叠筛选期望命中 2 条，实际 %d 条", total)
	}
}

func TestListRejectsInvertedPlanPeriod(t *testing.T) {
	query := cleaningtask.ListQuery{
		PlanFrom: date.Ptr(date.Today().String()),
		PlanTo:   date.Ptr(date.Today().AddDays(-1).String()),
	}
	if err := query.Validate(); err == nil {
		t.Fatal("起止颠倒的计划时间段应返回校验错误")
	} else {
		testsupport.RequireAppError(t, err, httpx.CodeBadRequest)
	}
}

func TestListSummaryMatchesFilterAcrossPages(t *testing.T) {
	fixture := setupFilterFixture(t)
	ctx := context.Background()

	query := cleaningtask.ListQuery{
		District: "城东片区",
		TeamName: "班组甲",
		Page:     httpx.PageQuery{Page: 1, PageSize: 1}, // 只取 1 条，验证汇总不受分页影响
	}
	items, total, summary, err := fixture.Tasks.ListPage(ctx, query)
	testsupport.RequireNoError(t, err)
	if total != 2 {
		t.Fatalf("城东班组甲应有 2 条任务，实际 %d", total)
	}
	if len(items) != 1 {
		t.Fatalf("pageSize=1 时应只返回 1 行，实际 %d", len(items))
	}
	if summary.TaskCount != 2 {
		t.Fatalf("汇总任务数应为 2（不随分页变），实际 %d", summary.TaskCount)
	}
	if summary.RecordCount != 2 {
		t.Fatalf("汇总记录数应为 2，实际 %d", summary.RecordCount)
	}
	// 两条记录分别 8 和 4，合计 12。
	if summary.SludgeVolumeM3 != 12 {
		t.Fatalf("汇总清淤量应为 12，实际 %v", summary.SludgeVolumeM3)
	}

	// 翻到第 2 页，汇总口径必须保持一致。
	query.Page.Page = 2
	_, _, summaryPage2, err := fixture.Tasks.ListPage(ctx, query)
	testsupport.RequireNoError(t, err)
	if summaryPage2 != summary {
		t.Fatalf("翻页后汇总发生变化：page1=%+v page2=%+v", summary, summaryPage2)
	}
}

func TestExportRowsMatchListFilter(t *testing.T) {
	fixture := setupFilterFixture(t)
	ctx := context.Background()

	query := cleaningtask.ListQuery{Source: cleaningtask.SourcePlan}
	items, summary, err := fixture.Tasks.ExportRows(ctx, query)
	testsupport.RequireNoError(t, err)
	if int64(len(items)) != summary.TaskCount {
		t.Fatalf("导出行数 %d 与汇总任务数 %d 不一致", len(items), summary.TaskCount)
	}
	if summary.TaskCount != 1 {
		t.Fatalf("年度计划来源只有 1 条任务，实际 %d", summary.TaskCount)
	}

	// keyset 分批导出与一次性导出结果一致（按 id 升序）。
	batch1, err := fixture.Tasks.ExportBatch(ctx, cleaningtask.ListQuery{}, 0, 2)
	testsupport.RequireNoError(t, err)
	batch2, err := fixture.Tasks.ExportBatch(ctx, cleaningtask.ListQuery{}, batch1[len(batch1)-1].ID, 2)
	testsupport.RequireNoError(t, err)
	if len(batch1)+len(batch2) != 3 {
		t.Fatalf("分批导出总数应为 3，实际 %d + %d", len(batch1), len(batch2))
	}
	if batch2[0].ID <= batch1[len(batch1)-1].ID {
		t.Fatal("keyset 翻页应严格按 id 递增")
	}
}

func TestFilterOptions(t *testing.T) {
	fixture := setupFilterFixture(t)
	ctx := context.Background()

	options, err := fixture.Tasks.FilterOptions(ctx, "")
	testsupport.RequireNoError(t, err)
	if len(options.Districts) != 2 {
		t.Fatalf("应有 2 个片区，实际 %v", options.Districts)
	}
	if len(options.Teams) != 2 {
		t.Fatalf("应有 2 个班组，实际 %v", options.Teams)
	}

	eastOptions, err := fixture.Tasks.FilterOptions(ctx, "城东片区")
	testsupport.RequireNoError(t, err)
	// 默认夹具在城东片区还有一条「测试道路」，这里验证按片区过滤后
	// 只返回城东片区的道路，且包含中山路。
	if len(eastOptions.Roads) != 2 {
		t.Fatalf("城东片区应有 2 条道路，实际 %+v", eastOptions.Roads)
	}
	hasZhongshan := false
	for _, road := range eastOptions.Roads {
		if road.District != "城东片区" {
			t.Fatalf("按片区过滤后仍返回了其他片区：%+v", road)
		}
		if road.RoadName == "中山路" {
			hasZhongshan = true
		}
	}
	if !hasZhongshan {
		t.Fatalf("城东片区道路中应包含中山路，实际 %+v", eastOptions.Roads)
	}
}
