package cleaningtask_test

import (
	"context"
	"testing"

	"github.com/drainage/desilting/internal/httpx"
	"github.com/drainage/desilting/internal/modules/cleaningrecord"
	"github.com/drainage/desilting/internal/modules/cleaningtask"
	"github.com/drainage/desilting/internal/modules/pipesegment"
	"github.com/drainage/desilting/internal/shared/date"
	"github.com/drainage/desilting/internal/testsupport"
)

// createSegmentWith 在指定片区与道路建档，用于组合筛选测试。
func createSegmentWith(t *testing.T, fixture *testsupport.Fixture, code, district, road string) *pipesegment.PipeSegment {
	t.Helper()
	segment, err := fixture.Segments.Create(context.Background(), pipesegment.SaveRequest{
		Code:         code,
		Name:         "测试管段 " + code,
		District:     district,
		RoadName:     road,
		PipeType:     pipesegment.TypeRainwater,
		Material:     "concrete",
		DiameterMm:   600,
		LengthM:      100,
		DepthM:       2,
		StartManhole: "S-1",
		EndManhole:   "S-2",
		BuildYear:    2020,
	})
	testsupport.RequireNoError(t, err)
	return segment
}

// createTaskSpec 按指定规格创建任务，覆盖组合筛选所需的各类属性。
func createTaskSpec(t *testing.T, fixture *testsupport.Fixture, spec cleaningtask.SaveRequest) *cleaningtask.CleaningTask {
	t.Helper()
	task, err := fixture.Tasks.Create(context.Background(), spec)
	testsupport.RequireNoError(t, err)
	return task
}

// addRecord 给任务录入一条指定清淤量的记录。
func addRecord(t *testing.T, fixture *testsupport.Fixture, taskID uint, sludge float64) {
	t.Helper()
	_, err := fixture.Records.Create(context.Background(), cleaningrecord.SaveRequest{
		TaskID:         taskID,
		CleanedAt:      date.Today(),
		LengthM:        50,
		SludgeVolumeM3: sludge,
		WaterVolumeM3:  10,
		PersonnelCount: 3,
		Method:         cleaningtask.MethodHighPressure,
		Weather:        cleaningrecord.WeatherSunny,
		RecorderName:   "记录员",
	})
	testsupport.RequireNoError(t, err)
}

func baseSpec(segmentID uint, title string) cleaningtask.SaveRequest {
	return cleaningtask.SaveRequest{
		Title:         title,
		PipeSegmentID: segmentID,
		Priority:      cleaningtask.PriorityNormal,
		Source:        cleaningtask.SourcePlan,
		Method:        cleaningtask.MethodHighPressure,
		PlanStartDate: date.Today().AddDays(-2),
		PlanEndDate:   date.Today().AddDays(2),
		TeamName:      "城东养护一班",
		LeaderName:    "负责人",
	}
}

// TestTaskListCombinedFilters 验证片区 + 道路 + 状态 + 来源 + 班组 + 计划时间段组合筛选。
func TestTaskListCombinedFilters(t *testing.T) {
	fixture := testsupport.NewFixture(t)
	ctx := context.Background()

	east := createSegmentWith(t, fixture, "PS-E-001", "城东片区", "中山北路")
	west := createSegmentWith(t, fixture, "PS-W-001", "城西片区", "解放东路")
	south := createSegmentWith(t, fixture, "PS-S-001", "城南片区", "长江南路")

	// 命中条件：城东 / 中山北路 / 投诉举报 / 城东班组。
	match := createTaskSpec(t, fixture, func() cleaningtask.SaveRequest {
		s := baseSpec(east.ID, "城东投诉任务")
		s.Source = cleaningtask.SourceComplaint
		s.PlanStartDate = date.Today()
		s.PlanEndDate = date.Today().AddDays(3)
		return s
	}())
	// 同片区同道路但来源不同，不应命中。
	createTaskSpec(t, fixture, func() cleaningtask.SaveRequest {
		s := baseSpec(east.ID, "城东年度计划任务")
		s.Source = cleaningtask.SourcePlan
		return s
	}())
	// 道路不同。
	createTaskSpec(t, fixture, func() cleaningtask.SaveRequest {
		s := baseSpec(west.ID, "城西投诉任务")
		s.Source = cleaningtask.SourceComplaint
		s.TeamName = "城西养护二班"
		return s
	}())
	// 片区不同。
	createTaskSpec(t, fixture, func() cleaningtask.SaveRequest {
		s := baseSpec(south.ID, "城南投诉任务")
		s.Source = cleaningtask.SourceComplaint
		return s
	}())

	addRecord(t, fixture, match.ID, 12.5)
	addRecord(t, fixture, match.ID, 7.25)
	// 录入记录后任务自动进入「清淤中」，状态维度由此生效。
	if got := fixture.Reload(t, match.ID).Status; got != cleaningtask.StatusInProgress {
		t.Fatalf("期望任务为清淤中，实际 %s", got)
	}

	result, err := fixture.Tasks.List(ctx, cleaningtask.ListQuery{
		District: "城东片区",
		RoadName: "中山北路",
		Status:   cleaningtask.StatusInProgress,
		Source:   cleaningtask.SourceComplaint,
		TeamName: "城东",
		PlanFrom: ptrDate(date.Today().AddDays(-1)),
		PlanTo:   ptrDate(date.Today().AddDays(1)),
		Page:     httpx.PageQuery{Page: 1, PageSize: 10},
	})
	testsupport.RequireNoError(t, err)

	if result.Total != 1 {
		t.Fatalf("组合筛选应只命中 1 条任务，实际 %d", result.Total)
	}
	if len(result.Items) != 1 || result.Items[0].ID != match.ID {
		t.Fatalf("命中的应为任务 %d，实际 %+v", match.ID, result.Items)
	}
	if result.Summary.RecordCount != 2 {
		t.Fatalf("应有 2 条清淤记录，实际 %d", result.Summary.RecordCount)
	}
	if result.Summary.SludgeVolumeM3 != 19.75 {
		t.Fatalf("清淤量合计应为 19.75，实际 %v", result.Summary.SludgeVolumeM3)
	}
	if result.Summary.TaskCount != 1 {
		t.Fatalf("有清淤记录的任务数应为 1，实际 %d", result.Summary.TaskCount)
	}
}

// TestTaskListSummaryStableAcrossPages 翻页不能改变顶部汇总，且汇总不能只算当前页。
func TestTaskListSummaryStableAcrossPages(t *testing.T) {
	fixture := testsupport.NewFixture(t)
	ctx := context.Background()

	const taskCount = 5
	for i := 0; i < taskCount; i++ {
		task := createTaskSpec(t, fixture, func() cleaningtask.SaveRequest {
			s := baseSpec(fixture.Segment.ID, "跨页任务")
			s.PlanStartDate = date.Today().AddDays(-i)
			return s
		}())
		addRecord(t, fixture, task.ID, 2.0)
	}

	first, err := fixture.Tasks.List(ctx, cleaningtask.ListQuery{
		Keyword: "跨页任务",
		Page:    httpx.PageQuery{Page: 1, PageSize: 2},
	})
	testsupport.RequireNoError(t, err)
	second, err := fixture.Tasks.List(ctx, cleaningtask.ListQuery{
		Keyword: "跨页任务",
		Page:    httpx.PageQuery{Page: 3, PageSize: 2},
	})
	testsupport.RequireNoError(t, err)

	if first.Total != taskCount {
		t.Fatalf("总数应为 %d，实际 %d", taskCount, first.Total)
	}
	if len(first.Items) != 2 || len(second.Items) != 1 {
		t.Fatalf("第 1 页应有 2 条、第 3 页应有 1 条，实际 %d / %d", len(first.Items), len(second.Items))
	}
	if first.Summary.SludgeVolumeM3 != 10.0 || second.Summary.SludgeVolumeM3 != 10.0 {
		t.Fatalf("翻页前后汇总都应为 10.00 m³，实际 %v / %v",
			first.Summary.SludgeVolumeM3, second.Summary.SludgeVolumeM3)
	}
	if first.Summary.RecordCount != 5 || second.Summary.RecordCount != 5 {
		t.Fatalf("翻页前后记录数合计都应为 5，实际 %d / %d",
			first.Summary.RecordCount, second.Summary.RecordCount)
	}
}

// TestTaskExportMatchesFilter 导出条数与合计必须和筛选结果同源。
func TestTaskExportMatchesFilter(t *testing.T) {
	fixture := testsupport.NewFixture(t)
	ctx := context.Background()

	keep := createTaskSpec(t, fixture, func() cleaningtask.SaveRequest {
		s := baseSpec(fixture.Segment.ID, "保留的投诉任务")
		s.Source = cleaningtask.SourceComplaint
		return s
	}())
	createTaskSpec(t, fixture, baseSpec(fixture.Segment.ID, "排除的计划任务"))
	addRecord(t, fixture, keep.ID, 3.5)
	addRecord(t, fixture, keep.ID, 1.5)

	query := cleaningtask.ListQuery{
		Source: cleaningtask.SourceComplaint,
		Page:   httpx.PageQuery{Page: 1, PageSize: 10},
	}
	page, err := fixture.Tasks.List(ctx, query)
	testsupport.RequireNoError(t, err)
	export, err := fixture.Tasks.Export(ctx, query)
	testsupport.RequireNoError(t, err)

	if page.Total != 1 || len(export.Items) != 1 || export.Items[0].ID != keep.ID {
		t.Fatalf("导出数据应与页面一致，page=%d export=%d", page.Total, len(export.Items))
	}
	if export.Summary.SludgeVolumeM3 != 5.0 {
		t.Fatalf("导出合计应为 5.00 m³，实际 %v", export.Summary.SludgeVolumeM3)
	}
	if export.Summary != page.Summary {
		t.Fatalf("导出合计 %+v 与页面合计 %+v 不一致", export.Summary, page.Summary)
	}
	if export.Truncated {
		t.Fatal("数据量未达上限，不应标记截断")
	}
}

// TestTaskFilterOptions 筛选项应包含实际建档的片区、道路与实际派工的班组。
func TestTaskFilterOptions(t *testing.T) {
	fixture := testsupport.NewFixture(t)
	ctx := context.Background()

	createSegmentWith(t, fixture, "PS-E-002", "城东片区", "滨江大道")
	createSegmentWith(t, fixture, "PS-W-002", "城西片区", "解放东路")
	createTaskSpec(t, fixture, func() cleaningtask.SaveRequest {
		s := baseSpec(fixture.Segment.ID, "班组任务甲")
		s.TeamName = "滨江专项作业队"
		return s
	}())
	createTaskSpec(t, fixture, func() cleaningtask.SaveRequest {
		s := baseSpec(fixture.Segment.ID, "班组任务乙")
		s.TeamName = "城东养护一班"
		return s
	}())

	options, err := fixture.Tasks.FilterOptions(ctx, "")
	testsupport.RequireNoError(t, err)

	if !contains(options.Districts, "城东片区") || !contains(options.Districts, "城西片区") {
		t.Fatalf("片区选项不完整：%v", options.Districts)
	}
	if !contains(options.Roads, "滨江大道") || !contains(options.Roads, "解放东路") {
		t.Fatalf("道路选项不完整：%v", options.Roads)
	}
	if !contains(options.Teams, "滨江专项作业队") || !contains(options.Teams, "城东养护一班") {
		t.Fatalf("班组选项不完整：%v", options.Teams)
	}

	// 带 district 时道路应收窄到该片区，用于前端片区 -> 道路联动。
	eastOptions, err := fixture.Tasks.FilterOptions(ctx, "城东片区")
	testsupport.RequireNoError(t, err)
	if !contains(eastOptions.Roads, "滨江大道") {
		t.Fatalf("城东片区应包含滨江大道：%v", eastOptions.Roads)
	}
	if contains(eastOptions.Roads, "解放东路") {
		t.Fatalf("城东片区不应包含城西的解放东路：%v", eastOptions.Roads)
	}
}

func ptrDate(d date.Date) *date.Date {
	return &d
}

func contains(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}
