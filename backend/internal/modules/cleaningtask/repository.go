package cleaningtask

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"

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
	filter := r.filtered(ctx, query)

	var total int64
	if err := filter.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	tasks := make([]CleaningTask, 0)
	err := r.filtered(ctx, query).
		Order("plan_start_date DESC, id DESC").
		Offset(query.Page.Offset()).
		Limit(query.Page.PageSize).
		Find(&tasks).Error
	if err != nil {
		return nil, 0, err
	}
	return tasks, total, nil
}

// Count 返回筛选条件下的任务总数（不带分页）。
func (r *Repository) Count(ctx context.Context, query ListQuery) (int64, error) {
	var total int64
	err := r.filtered(ctx, query).Count(&total).Error
	return total, err
}

// Summary 按与 List 完全相同的筛选条件整体汇总清淤量，不分页。
//
// 单独调用 r.filtered 生成语句，不与分页查询共用 *gorm.DB，
// 保证分页 LIMIT/OFFSET 不会串到汇总语句里。
func (r *Repository) Summary(ctx context.Context, query ListQuery) (refx.SludgeSummary, error) {
	return refx.SludgeSummaryForTasks(ctx, r.db, r.filtered(ctx, query))
}

// MaxExportRows 单次导出允许的最大条数，避免筛选范围过大时一次性把库拉爆。
const MaxExportRows = 10000

// All 返回符合筛选条件的全部任务（最多 MaxExportRows 条），导出时使用，
// 排序与分页列表一致，保证导出的就是页面同一批数据。
func (r *Repository) All(ctx context.Context, query ListQuery) ([]CleaningTask, error) {
	tasks := make([]CleaningTask, 0)
	err := r.filtered(ctx, query).
		Order("plan_start_date DESC, id DESC").
		Limit(MaxExportRows).
		Find(&tasks).Error
	if err != nil {
		return nil, err
	}
	return tasks, nil
}

func (r *Repository) filtered(ctx context.Context, query ListQuery) *gorm.DB {
	tx := r.db.WithContext(ctx).Model(&CleaningTask{})
	if keyword := strings.ToLower(strings.TrimSpace(query.Keyword)); keyword != "" {
		like := "%" + keyword + "%"
		tx = tx.Where(
			"LOWER(code) LIKE ? OR LOWER(title) LIKE ? OR LOWER(team_name) LIKE ? OR LOWER(leader_name) LIKE ?",
			like, like, like, like,
		)
	}
	if query.Status != "" {
		tx = tx.Where("status = ?", query.Status)
	}
	if query.Priority != "" {
		tx = tx.Where("priority = ?", query.Priority)
	}
	if query.Source != "" {
		tx = tx.Where("source = ?", query.Source)
	}
	if query.PipeSegmentID > 0 {
		tx = tx.Where("pipe_segment_id = ?", query.PipeSegmentID)
	}
	// 片区、道路都挂在管段台账上，通过 EXISTS 子查询过滤，
	// 避免 JOIN 管段表后任务行被放大，影响 COUNT 与清淤量汇总。
	if query.District != "" {
		tx = tx.Where(
			"EXISTS (SELECT 1 FROM "+refx.TablePipeSegments+" s WHERE s.id = "+refx.TableCleaningTasks+".pipe_segment_id AND s.district = ?)",
			query.District,
		)
	}
	if road := strings.TrimSpace(query.RoadName); road != "" {
		like := "%" + strings.ToLower(road) + "%"
		tx = tx.Where(
			"EXISTS (SELECT 1 FROM "+refx.TablePipeSegments+" s WHERE s.id = "+refx.TableCleaningTasks+".pipe_segment_id AND LOWER(s.road_name) LIKE ?)",
			like,
		)
	}
	if team := strings.TrimSpace(query.TeamName); team != "" {
		tx = tx.Where("LOWER(team_name) LIKE ?", "%"+strings.ToLower(team)+"%")
	}
	if query.PlanFrom != nil {
		tx = tx.Where("plan_start_date >= ?", query.PlanFrom.Time)
	}
	if query.PlanTo != nil {
		tx = tx.Where("plan_start_date <= ?", query.PlanTo.Time)
	}
	return tx
}

// FilterOptions 返回任务筛选下拉所需的片区、道路与实施班组。
//
// district 非空时道路只返回该片区下已建档的道路，供前端「片区 -> 道路」联动，
// 避免选了片区又选到其他片区道路这种必然为空的组合；班组取自任务表中实际出现过的值。
func (r *Repository) FilterOptions(ctx context.Context, district string) (FilterOptionsResponse, error) {
	var options FilterOptionsResponse
	err := r.db.WithContext(ctx).Table(refx.TablePipeSegments).
		Where("district <> ''").
		Distinct().
		Order("district ASC").
		Pluck("district", &options.Districts).Error
	if err != nil {
		return FilterOptionsResponse{}, err
	}
	roadQuery := r.db.WithContext(ctx).Table(refx.TablePipeSegments).
		Where("road_name <> ''")
	if district = strings.TrimSpace(district); district != "" {
		roadQuery = roadQuery.Where("district = ?", district)
	}
	err = roadQuery.Distinct().
		Order("road_name ASC").
		Pluck("road_name", &options.Roads).Error
	if err != nil {
		return FilterOptionsResponse{}, err
	}
	err = r.db.WithContext(ctx).Model(&CleaningTask{}).
		Where("team_name <> ''").
		Distinct().
		Order("team_name ASC").
		Pluck("team_name", &options.Teams).Error
	if err != nil {
		return FilterOptionsResponse{}, err
	}
	return options, nil
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
