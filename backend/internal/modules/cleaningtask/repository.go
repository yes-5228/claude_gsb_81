package cleaningtask

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/drainage/desilting/internal/shared/num"
	"github.com/drainage/desilting/internal/shared/refx"
)

// ErrNotFound 任务不存在。
var ErrNotFound = errors.New("清淤任务不存在")

// ErrStateConflict 并发操作导致状态已变化。
var ErrStateConflict = errors.New("任务状态已变化，请刷新后重试")

// Repository 清淤任务数据访问。
type Repository struct {
	db *gorm.DB
}

// NewRepository 构造仓储。
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// DB 暴露底层连接，供 service 做反向引用检查。
func (r *Repository) DB() *gorm.DB {
	return r.db
}

// Create 新增任务。
func (r *Repository) Create(ctx context.Context, task *CleaningTask) error {
	return r.db.WithContext(ctx).Create(task).Error
}

// Save 保存任务全部字段。
func (r *Repository) Save(ctx context.Context, task *CleaningTask) error {
	return r.db.WithContext(ctx).Save(task).Error
}

// Delete 物理删除任务。
func (r *Repository) Delete(ctx context.Context, id uint) error {
	result := r.db.WithContext(ctx).Delete(&CleaningTask{}, id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// FindByID 按主键查询任务。
func (r *Repository) FindByID(ctx context.Context, id uint) (*CleaningTask, error) {
	var task CleaningTask
	err := r.db.WithContext(ctx).First(&task, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &task, nil
}

// MaxCodeWithPrefix 查询某个前缀下已使用的最大任务编号，用于生成流水号。
func (r *Repository) MaxCodeWithPrefix(ctx context.Context, prefix string) (string, error) {
	var code string
	err := r.db.WithContext(ctx).Model(&CleaningTask{}).
		Where("code LIKE ?", prefix+"-%").
		Order("code DESC").
		Limit(1).
		Pluck("code", &code).Error
	return code, err
}

// Transition 在指定前置状态下更新任务状态，避免并发下出现非法流转。
func (r *Repository) Transition(ctx context.Context, id uint, from, to string, extra map[string]any) error {
	return r.TransitionTx(ctx, nil, id, from, to, extra)
}

// TransitionTx 在给定事务中执行状态流转，供验收模块与验收记录写入保持原子性。
//
// tx 可以为 nil，此时直接使用仓储自身的连接。
func (r *Repository) TransitionTx(ctx context.Context, tx *gorm.DB, id uint, from, to string, extra map[string]any) error {
	db := r.db
	if tx != nil {
		db = tx
	}
	updates := map[string]any{
		"status":     to,
		"updated_at": time.Now(),
	}
	for key, value := range extra {
		updates[key] = value
	}
	result := db.WithContext(ctx).Model(&CleaningTask{}).
		Where("id = ? AND status = ?", id, from).
		Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrStateConflict
	}
	return nil
}

// List 分页查询任务。
func (r *Repository) List(ctx context.Context, query ListQuery) ([]CleaningTask, int64, error) {
	query.Page.Normalize()
	var total int64
	if err := r.filtered(ctx, query).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	tasks := make([]CleaningTask, 0)
	err := r.baseList(ctx, query).
		Offset(query.Page.Offset()).
		Limit(query.Page.PageSize).
		Find(&tasks).Error
	if err != nil {
		return nil, 0, err
	}
	return tasks, total, nil
}

// ListAll 不分页查询全部命中任务，按列表相同顺序返回，供导出使用。
func (r *Repository) ListAll(ctx context.Context, query ListQuery) ([]CleaningTask, error) {
	tasks := make([]CleaningTask, 0)
	err := r.baseList(ctx, query).Find(&tasks).Error
	if err != nil {
		return nil, err
	}
	return tasks, nil
}

// ListBatch keyset 分批查询命中任务（t.id > afterID），专供导出流式写出。
//
// 导出按 id 升序稳定翻页：相比 OFFSET 深分页，每批的查询代价不随已导出
// 条数增长，任务攒到较大规模时导出仍然稳定。
func (r *Repository) ListBatch(ctx context.Context, query ListQuery, afterID uint, limit int) ([]CleaningTask, error) {
	if limit <= 0 || limit > 5000 {
		limit = exportBatchSize
	}
	tasks := make([]CleaningTask, 0, limit)
	tx := r.filtered(ctx, query).Select("t.*")
	if afterID > 0 {
		tx = tx.Where("t.id > ?", afterID)
	}
	err := tx.Order("t.id ASC").Limit(limit).Find(&tasks).Error
	if err != nil {
		return nil, err
	}
	return tasks, nil
}

func (r *Repository) baseList(ctx context.Context, query ListQuery) *gorm.DB {
	// JOIN 会同时带出管段的同名列（id / code / created_at 等），
	// 必须显式只取任务列，避免 Scan 时同名字段互相覆盖。
	return r.filtered(ctx, query).Select("t.*").Order("t.plan_start_date DESC, t.id DESC")
}

// Summary 汇总当前筛选条件命中的任务数量与这些任务下的清淤量合计。
//
// 汇总与列表共用 filtered() 的 WHERE 条件，且统计的是命中任务的全集，
// 不随分页变化；清淤量通过任务集合子查询聚合，保证口径一致。
func (r *Repository) Summary(ctx context.Context, query ListQuery) (ListSummary, error) {
	var summary ListSummary

	taskIDs := r.filtered(ctx, query).Select("t.id")
	if err := r.db.WithContext(ctx).Table(refx.TableCleaningRecords).
		Where("task_id IN (?)", taskIDs).
		Select(`COUNT(*) AS record_count,
			COALESCE(SUM(sludge_volume_m3), 0) AS sludge_volume_m3,
			COALESCE(SUM(length_m), 0) AS cleaned_length_m`).
		Scan(&summary).Error; err != nil {
		return ListSummary{}, err
	}

	if err := r.filtered(ctx, query).Count(&summary.TaskCount).Error; err != nil {
		return ListSummary{}, err
	}
	summary.SludgeVolumeM3 = num.Round2(summary.SludgeVolumeM3)
	summary.CleanedLengthM = num.Round2(summary.CleanedLengthM)
	return summary, nil
}

// Teams 返回已有任务中全部去重后的实施班组名称。
func (r *Repository) Teams(ctx context.Context) ([]string, error) {
	teams := make([]string, 0)
	err := r.db.WithContext(ctx).Model(&CleaningTask{}).
		Where("team_name <> ''").
		Distinct().
		Order("team_name ASC").
		Pluck("team_name", &teams).Error
	return teams, err
}

// filtered 构造组合筛选查询。
//
// 片区与道路都挂在管段台账上，统一用一次 INNER JOIN 实现，
// 避免子查询 IN 在任务规模增大后性能劣化；任务建立时管段必填，
// INNER JOIN 不会丢任务。
func (r *Repository) filtered(ctx context.Context, query ListQuery) *gorm.DB {
	tx := r.db.WithContext(ctx).
		Table(refx.TableCleaningTasks + " AS t").
		Joins("JOIN " + refx.TablePipeSegments + " AS s ON s.id = t.pipe_segment_id")
	if keyword := strings.ToLower(strings.TrimSpace(query.Keyword)); keyword != "" {
		like := "%" + keyword + "%"
		tx = tx.Where(
			"LOWER(t.code) LIKE ? OR LOWER(t.title) LIKE ? OR LOWER(t.team_name) LIKE ? OR LOWER(t.leader_name) LIKE ?",
			like, like, like, like,
		)
	}
	if query.Status != "" {
		tx = tx.Where("t.status = ?", query.Status)
	}
	if query.Priority != "" {
		tx = tx.Where("t.priority = ?", query.Priority)
	}
	if query.Source != "" {
		tx = tx.Where("t.source = ?", query.Source)
	}
	if query.TeamName != "" {
		tx = tx.Where("t.team_name = ?", query.TeamName)
	}
	if query.PipeSegmentID > 0 {
		tx = tx.Where("t.pipe_segment_id = ?", query.PipeSegmentID)
	}
	if query.District != "" {
		tx = tx.Where("s.district = ?", query.District)
	}
	if query.RoadName != "" {
		tx = tx.Where("s.road_name = ?", query.RoadName)
	}
	// 计划时间段按区间重叠匹配：任务计划周期与所选时间段有交集即命中。
	if query.PlanFrom != nil {
		tx = tx.Where("t.plan_end_date >= ?", query.PlanFrom.Time)
	}
	if query.PlanTo != nil {
		tx = tx.Where("t.plan_start_date <= ?", query.PlanTo.Time)
	}
	return tx
}

// HasRecords 任务下是否已经有清淤记录。
func (r *Repository) HasRecords(ctx context.Context, taskID uint) (bool, error) {
	return refx.HasRecordsForTask(ctx, r.db, taskID)
}

// HasAcceptance 任务下是否已经有验收记录。
func (r *Repository) HasAcceptance(ctx context.Context, taskID uint) (bool, error) {
	return refx.HasAcceptanceForTask(ctx, r.db, taskID)
}

// CountByStatus 统计各状态任务数量。
func (r *Repository) CountByStatus(ctx context.Context) (map[string]int64, error) {
	type row struct {
		Status string
		Total  int64
	}
	rows := make([]row, 0)
	err := r.db.WithContext(ctx).Model(&CleaningTask{}).
		Select("status, COUNT(*) AS total").
		Group("status").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	result := make(map[string]int64, len(rows))
	for _, item := range rows {
		result[item.Status] = item.Total
	}
	return result, nil
}
