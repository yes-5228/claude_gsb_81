package refx

import (
	"context"

	"gorm.io/gorm"

	"github.com/drainage/desilting/internal/shared/num"
)

// SludgeSummary 一批任务下全部清淤记录的合计。
//
// 清淤量挂在清淤记录表上，而筛选条件（片区、道路、班组、计划时间等）挂在任务/管段表上，
// 因此这里统一用一条聚合 SQL 跨表统计，保证与列表、导出属于同一批数据。
type SludgeSummary struct {
	TaskCount      int64   `json:"taskCount"`
	RecordCount    int64   `json:"recordCount"`
	SludgeVolumeM3 float64 `json:"sludgeVolumeM3"`
	CleanedLengthM float64 `json:"cleanedLengthM"`
}

// SludgeSummaryForTasks 按给定的任务筛选子查询汇总其全部清淤记录。
//
// taskFilter 必须是基于 cleaning_tasks 表、且已带齐全部列表筛选条件的语句，
// 与列表分页使用的筛选条件完全一致：列表只取当前页而汇总取全量，
// 二者口径相同，翻页不会改变合计。
func SludgeSummaryForTasks(ctx context.Context, db *gorm.DB, taskFilter *gorm.DB) (SludgeSummary, error) {
	var summary SludgeSummary
	err := db.WithContext(ctx).Table(TableCleaningRecords+" AS r").
		Select(`COUNT(DISTINCT r.task_id) AS task_count,
			COUNT(*) AS record_count,
			COALESCE(SUM(r.sludge_volume_m3), 0) AS sludge_volume_m3,
			COALESCE(SUM(r.length_m), 0) AS cleaned_length_m`).
		Where("r.task_id IN (?)", taskFilter.Session(&gorm.Session{}).Select("id")).
		Scan(&summary).Error
	summary.SludgeVolumeM3 = num.Round2(summary.SludgeVolumeM3)
	summary.CleanedLengthM = num.Round2(summary.CleanedLengthM)
	return summary, err
}
