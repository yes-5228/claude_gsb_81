package cleaningtask_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/drainage/desilting/internal/modules/cleaningrecord"
	"github.com/drainage/desilting/internal/modules/cleaningtask"
	"github.com/drainage/desilting/internal/shared/date"
	"github.com/drainage/desilting/internal/testsupport"
)

// TestListAndExportRemainStableAtScale 任务攒到较大规模后：
// 组合筛选稳定返回、汇总不随分页变化、keyset 分批导出无重复无遗漏。
func TestListAndExportRemainStableAtScale(t *testing.T) {
	if testing.Short() {
		t.Skip("规模测试默认跳过")
	}
	fixture := testsupport.NewFixture(t)
	ctx := context.Background()

	const taskTotal = 2500
	tasks := make([]cleaningtask.CleaningTask, 0, taskTotal)
	records := make([]cleaningrecord.CleaningRecord, 0, taskTotal)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	for i := 0; i < taskTotal; i++ {
		// 一半任务落在城东 + 班组甲 + 来源巡查，构造高选择性的组合条件。
		district := "城东片区"
		team := "班组甲"
		source := cleaningtask.SourceInspection
		if i%2 == 1 {
			district = "城西片区"
			team = "班组乙"
			source = cleaningtask.SourcePlan
		}
		segment := fixture.CreateSegmentOnRoad(t,
			fmt.Sprintf("PS-S-%04d", i), district,
			map[bool]string{true: "中山路", false: "解放路"}[i%2 == 0])
		start := base.AddDate(0, 0, i)
		tasks = append(tasks, cleaningtask.CleaningTask{
			Code:          fmt.Sprintf("QX-SCALE-%05d", i),
			Title:         fmt.Sprintf("规模测试任务 %d", i),
			PipeSegmentID: segment.ID,
			Priority:      cleaningtask.PriorityNormal,
			Source:        source,
			PlanStartDate: date.New(start),
			PlanEndDate:   date.New(start.AddDate(0, 0, 5)),
			TeamName:      team,
			Status:        cleaningtask.StatusInProgress,
		})
	}
	if err := fixture.DB.CreateInBatches(tasks, 500).Error; err != nil {
		t.Fatalf("批量创建任务失败: %v", err)
	}
	for i := range tasks {
		records = append(records, cleaningrecord.CleaningRecord{
			Code:           fmt.Sprintf("JL-SCALE-%05d", i),
			TaskID:         tasks[i].ID,
			CleanedAt:      date.New(base.AddDate(0, 0, i)),
			LengthM:        10,
			SludgeVolumeM3: 1.5,
			WaterVolumeM3:  3,
			PersonnelCount: 2,
			Method:         cleaningtask.MethodHighPressure,
			RecorderName:   "规模测试",
		})
	}
	if err := fixture.DB.CreateInBatches(records, 500).Error; err != nil {
		t.Fatalf("批量创建清淤记录失败: %v", err)
	}

	query := cleaningtask.ListQuery{
		District: "城东片区",
		RoadName: "中山路",
		TeamName: "班组甲",
		Source:   cleaningtask.SourceInspection,
		PlanFrom: date.Ptr("2026-02-01"),
		PlanTo:   date.Ptr("2026-06-30"),
	}

	// 各分页的汇总必须完全一致。
	first, total, summary, err := fixture.Tasks.ListPage(ctx, query)
	testsupport.RequireNoError(t, err)
	if total != summary.TaskCount {
		t.Fatalf("total %d 与汇总任务数 %d 不一致", total, summary.TaskCount)
	}
	for page := 2; page <= 5; page++ {
		q := query
		q.Page.Page = page
		q.Page.PageSize = 50
		_, pageTotal, pageSummary, err := fixture.Tasks.ListPage(ctx, q)
		testsupport.RequireNoError(t, err)
		if pageTotal != total || pageSummary != summary {
			t.Fatalf("第 %d 页口径漂移: total=%d summary=%+v (首页 total=%d summary=%+v)",
				page, pageTotal, pageSummary, total, summary)
		}
	}

	// 命中集合里的任务各有 1 条 1.5 m³ 的记录。
	if summary.SludgeVolumeM3 != float64(summary.TaskCount)*1.5 {
		t.Fatalf("清淤量合计 %v 与任务数 %d 不符", summary.SludgeVolumeM3, summary.TaskCount)
	}
	if len(first) != 10 {
		t.Fatalf("默认每页 10 条，实际 %d", len(first))
	}

	// keyset 分批导出：无重复、无遗漏，且与汇总任务数一致。
	seen := make(map[uint]bool, summary.TaskCount)
	var lastID uint
	batches := 0
	for {
		items, err := fixture.Tasks.ExportBatch(ctx, query, lastID, 137)
		testsupport.RequireNoError(t, err)
		if len(items) == 0 {
			break
		}
		for _, item := range items {
			if seen[item.ID] {
				t.Fatalf("分批导出出现重复任务 %d", item.ID)
			}
			seen[item.ID] = true
		}
		lastID = items[len(items)-1].ID
		batches++
		if batches > taskTotal {
			t.Fatalf("分批导出未能收敛")
		}
	}
	if int64(len(seen)) != summary.TaskCount {
		t.Fatalf("导出任务数 %d 与汇总 %d 不一致", len(seen), summary.TaskCount)
	}

	// 一次性导出同样与汇总一致。
	allRows, exportSummary, err := fixture.Tasks.ExportRows(ctx, query)
	testsupport.RequireNoError(t, err)
	if int64(len(allRows)) != summary.TaskCount || exportSummary != summary {
		t.Fatalf("导出全集与列表汇总不一致: rows=%d export=%+v list=%+v",
			len(allRows), exportSummary, summary)
	}
}
