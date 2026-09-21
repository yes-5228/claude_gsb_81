// 清淤任务列表：支持片区、道路、状态、来源、实施班组、计划时间段组合筛选，
// 可保存常用筛选；列表数量与清淤量汇总始终与筛选结果同口径。
import { useEffect, useMemo, useState } from 'react';
import { Link, useNavigate, useSearchParams } from 'react-router-dom';
import { toErrorMessage } from '../../api/client';
import { taskApi, type TaskFilterPreset, type TaskQuery } from '../../api/tasks';
import { ConfirmDialog } from '../../components/ConfirmDialog';
import { DataTable, type Column } from '../../components/DataTable';
import { PageHeader } from '../../components/PageHeader';
import { Pagination } from '../../components/Pagination';
import { SectionCard } from '../../components/SectionCard';
import { StatusTag } from '../../components/StatusTag';
import { useToast } from '../../components/Toast';
import { useAsync } from '../../hooks/useAsync';
import { useMeta } from '../../providers/MetaProvider';
import type { TaskListItem } from '../../types/domain';
import { formatDate, formatNumber, formatVolume } from '../../utils/format';
import { SaveFilterDialog } from './SaveFilterDialog';
import { loadSavedFilters, removeSavedFilter, saveFilter, type SavedFilter } from './savedFilters';

const PAGE_SIZE = 10;

/** 参与组合筛选并可保存的查询参数。 */
const FILTER_KEYS = [
  'keyword',
  'status',
  'district',
  'roadName',
  'priority',
  'source',
  'teamName',
  'planFrom',
  'planTo'
] as const;

type FilterKey = (typeof FILTER_KEYS)[number];

/** 从 URL 查询串读取当前已应用的筛选条件。 */
function readFilters(params: URLSearchParams): TaskFilterPreset {
  const filters: TaskFilterPreset = {};
  FILTER_KEYS.forEach((key) => {
    const value = params.get(key);
    if (value) {
      filters[key] = value;
    }
  });
  return filters;
}

/** 判断当前组合条件是否自洽，返回需要在页面上直接说明的冲突。 */
function detectConflicts(
  filters: TaskFilterPreset,
  roadPairs: { district: string; roadName: string }[],
  roadOptionsReady: boolean
): string[] {
  const conflicts: string[] = [];
  if (filters.planFrom && filters.planTo && filters.planTo < filters.planFrom) {
    conflicts.push(`计划时间段存在冲突：开始日期 ${filters.planFrom} 晚于截止日期 ${filters.planTo}，请重新选择。`);
  }
  // 片区与道路的归属关系以后端选项为准；选项加载失败时不做误判，
  // 直接交给后端查询，避免把有效组合错误地拦下来。
  if (roadOptionsReady && filters.district && filters.roadName) {
    const belongs = roadPairs.some(
      (pair) => pair.district === filters.district && pair.roadName === filters.roadName
    );
    if (!belongs) {
      conflicts.push(
        `片区与道路条件冲突：道路「${filters.roadName}」不属于片区「${filters.district}」，组合后不会有任务命中，请清除其中一项。`
      );
    }
  }
  return conflicts;
}

export function TaskListPage() {
  const navigate = useNavigate();
  const toast = useToast();
  const { enums } = useMeta();
  const [params, setParams] = useSearchParams();

  const page = Math.max(1, Number(params.get('page') ?? '1') || 1);
  const appliedFilters = readFilters(params);
  const { keyword, status, district, roadName, priority, source, teamName, planFrom, planTo } = appliedFilters;

  // 关键字只在回车 / 点击查询时提交，输入过程不同步 URL。
  const [keywordInput, setKeywordInput] = useState(keyword ?? '');
  useEffect(() => {
    setKeywordInput(keyword ?? '');
  }, [keyword]);

  // 筛选栏可选项（片区 / 道路 / 班组），页面加载一次即可。
  const options = useAsync(() => taskApi.filterOptions(), []);
  const roadPairs = options.data?.roads ?? [];

  // 条件冲突时直接在页面说明，不发请求，避免返回空数据被误解为真的没任务。
  // 道路可选项尚未加载完成时先不判定片区/道路冲突，加载完成后再统一校验。
  const roadOptionsReady = !options.loading && !options.error;
  const conflicts = useMemo(
    () => detectConflicts(appliedFilters, roadPairs, roadOptionsReady),
    [appliedFilters, roadPairs, roadOptionsReady]
  );

  const query: TaskQuery = {
    keyword,
    status,
    district,
    roadName,
    priority,
    source,
    teamName,
    planFrom,
    planTo,
    page: conflicts.length > 0 ? 1 : page,
    pageSize: PAGE_SIZE
  };

  // 列表与汇总来自同一次接口响应，翻页、异步刷新都不会出现口径偏差。
  const list = useAsync(
    () => (conflicts.length > 0 ? Promise.resolve(null) : taskApi.list(query)),
    [keyword, status, district, roadName, priority, source, teamName, planFrom, planTo, page, conflicts.length]
  );

  const [pendingDelete, setPendingDelete] = useState<TaskListItem | null>(null);
  const [deleting, setDeleting] = useState(false);
  const [exporting, setExporting] = useState(false);

  const [savedFilters, setSavedFilters] = useState<SavedFilter[]>([]);
  const [saveDialogOpen, setSaveDialogOpen] = useState(false);
  useEffect(() => {
    setSavedFilters(loadSavedFilters());
  }, []);

  /** 修改任意筛选条件：写入 URL 并强制回到第 1 页。 */
  const applyFilter = (patch: Record<string, string>) => {
    const next = new URLSearchParams(params);
    Object.entries(patch).forEach(([key, value]) => {
      if (value) {
        next.set(key, value);
      } else {
        next.delete(key);
      }
    });
    next.set('page', '1');
    setParams(next);
  };

  const goPage = (nextPage: number) => {
    const next = new URLSearchParams(params);
    next.set('page', String(nextPage));
    setParams(next);
  };

  const resetAll = () => {
    setParams(new URLSearchParams());
  };

  const applyPreset = (preset: SavedFilter) => {
    const next = new URLSearchParams();
    Object.entries(preset.filter).forEach(([key, value]) => {
      if (typeof value === 'string' && value) {
        next.set(key, value);
      }
    });
    next.set('page', '1');
    setParams(next);
    setKeywordInput(preset.filter.keyword ?? '');
    toast.success(`已应用常用筛选「${preset.name}」`);
  };

  const handleSaveFilter = (name: string) => {
    const next = saveFilter(name, appliedFilters);
    setSavedFilters(next);
    setSaveDialogOpen(false);
    toast.success(`筛选条件已保存为「${name}」`);
  };

  const handleRemoveFilter = (id: string, name: string) => {
    setSavedFilters(removeSavedFilter(id));
    toast.success(`已删除常用筛选「${name}」`);
  };

  const hasActiveFilters = FILTER_KEYS.some((key: FilterKey) => Boolean(appliedFilters[key]));

  const handleExport = async () => {
    if (conflicts.length > 0) {
      toast.error('当前筛选条件存在冲突，请先调整后再导出');
      return;
    }
    setExporting(true);
    try {
      await taskApi.exportList(query);
      toast.success('已按当前筛选条件导出清淤任务');
    } catch (cause: unknown) {
      toast.error(toErrorMessage(cause));
    } finally {
      setExporting(false);
    }
  };

  const handleDelete = async () => {
    if (!pendingDelete) {
      return;
    }
    setDeleting(true);
    try {
      await taskApi.remove(pendingDelete.id);
      toast.success(`任务 ${pendingDelete.code} 已删除`);
      setPendingDelete(null);
      list.reload();
    } catch (cause: unknown) {
      toast.error(toErrorMessage(cause));
    } finally {
      setDeleting(false);
    }
  };

  // 片区切换后，若已选道路不属于新片区，下拉里直接提示联动冲突。
  const roadsInDistrict = useMemo(() => {
    if (!district) {
      return roadPairs;
    }
    return roadPairs.filter((pair) => pair.district === district);
  }, [district, roadPairs]);

  const roadNames = useMemo(() => {
    const names = roadsInDistrict.map((pair) => pair.roadName);
    return Array.from(new Set(names)).sort();
  }, [roadsInDistrict]);

  const summary = list.data?.summary ?? null;
  const total = list.data?.total ?? 0;

  const columns: Column<TaskListItem>[] = [
    {
      key: 'code',
      title: '任务编号',
      width: '180px',
      render: (row) => (
        <>
          <Link className="cell-main" to={`/tasks/${row.id}`}>
            {row.code}
          </Link>
          <span className="cell-sub">{row.title}</span>
        </>
      )
    },
    {
      key: 'segment',
      title: '关联管段',
      width: '170px',
      render: (row) => (
        <>
          <span>{row.segment?.code ?? '—'}</span>
          <span className="cell-sub">
            {row.segment ? `${row.segment.district} · ${row.segment.name}` : '管段已删除'}
          </span>
        </>
      )
    },
    { key: 'status', title: '状态', width: '100px', render: (row) => <StatusTag list="taskStatuses" value={row.status} /> },
    {
      key: 'priority',
      title: '优先级',
      width: '90px',
      render: (row) => <StatusTag list="taskPriorities" value={row.priority} />
    },
    { key: 'source', title: '来源', width: '110px', render: (row) => <StatusTag list="taskSources" value={row.source} /> },
    {
      key: 'team',
      title: '实施班组',
      width: '140px',
      render: (row) => (
        <>
          <span>{row.teamName || '—'}</span>
          <span className="cell-sub">{row.leaderName || '未指定负责人'}</span>
        </>
      )
    },
    {
      key: 'plan',
      title: '计划周期',
      width: '190px',
      render: (row) => `${formatDate(row.planStartDate)} ~ ${formatDate(row.planEndDate)}`
    },
    {
      key: 'totals',
      title: '清淤汇总',
      width: '130px',
      align: 'right',
      render: (row) => (
        <>
          <span className="cell-num">{formatVolume(row.recordTotals?.sludgeVolumeM3 ?? 0)}</span>
          <span className="cell-sub">记录 {formatNumber(row.recordTotals?.recordCount ?? 0, 0)} 条</span>
        </>
      )
    },
    {
      key: 'actions',
      title: '操作',
      width: '150px',
      render: (row) => (
        <div className="row-actions">
          <Link className="link" to={`/tasks/${row.id}`}>
            详情
          </Link>
          <Link className="link" to={`/tasks/${row.id}/edit`}>
            编辑
          </Link>
          <button type="button" className="btn-link" onClick={() => setPendingDelete(row)}>
            删除
          </button>
        </div>
      )
    }
  ];

  return (
    <div className="page">
      <PageHeader
        title="清淤任务"
        description="登记年度计划、巡查发现与投诉举报产生的清淤任务，驱动清淤记录录入与验收流程。"
        actions={
          <button type="button" className="btn btn-primary" onClick={() => navigate('/tasks/new')}>
            登记任务
          </button>
        }
      />

      <SectionCard title="任务清单" subtitle={conflicts.length > 0 ? '筛选条件存在冲突' : `共 ${total} 条记录`}>
        <div className="card-body-flush">
          <div className="filter-bar">
            <div className="filter-item" style={{ minWidth: 220 }}>
              <span className="filter-label">关键字</span>
              <input
                className="input"
                placeholder="任务编号 / 标题 / 班组"
                value={keywordInput}
                onChange={(event) => setKeywordInput(event.target.value)}
                onKeyDown={(event) => {
                  if (event.key === 'Enter') {
                    applyFilter({ keyword: keywordInput });
                  }
                }}
              />
            </div>
            <div className="filter-item">
              <span className="filter-label">所属片区</span>
              <select className="select" value={district ?? ''} onChange={(event) => applyFilter({ district: event.target.value, roadName: '' })}>
                <option value="">全部片区</option>
                {(options.data?.districts ?? []).map((item) => (
                  <option key={item} value={item}>
                    {item}
                  </option>
                ))}
              </select>
            </div>
            <div className="filter-item">
              <span className="filter-label">所在道路</span>
              <select className="select" value={roadName ?? ''} onChange={(event) => applyFilter({ roadName: event.target.value })}>
                <option value="">全部道路</option>
                {roadNames.map((item) => (
                  <option key={item} value={item}>
                    {item}
                  </option>
                ))}
              </select>
            </div>
            <div className="filter-item">
              <span className="filter-label">任务状态</span>
              <select className="select" value={status ?? ''} onChange={(event) => applyFilter({ status: event.target.value })}>
                <option value="">全部状态</option>
                {(enums?.taskStatuses ?? []).map((item) => (
                  <option key={item.value} value={item.value}>
                    {item.label}
                  </option>
                ))}
              </select>
            </div>
            <div className="filter-item">
              <span className="filter-label">任务来源</span>
              <select className="select" value={source ?? ''} onChange={(event) => applyFilter({ source: event.target.value })}>
                <option value="">全部来源</option>
                {(enums?.taskSources ?? []).map((item) => (
                  <option key={item.value} value={item.value}>
                    {item.label}
                  </option>
                ))}
              </select>
            </div>
            <div className="filter-item">
              <span className="filter-label">实施班组</span>
              <select className="select" value={teamName ?? ''} onChange={(event) => applyFilter({ teamName: event.target.value })}>
                <option value="">全部班组</option>
                {(options.data?.teams ?? []).map((item) => (
                  <option key={item} value={item}>
                    {item}
                  </option>
                ))}
              </select>
            </div>
            <div className="filter-item">
              <span className="filter-label">优先级</span>
              <select
                className="select"
                value={priority ?? ''}
                onChange={(event) => applyFilter({ priority: event.target.value })}
              >
                <option value="">全部优先级</option>
                {(enums?.taskPriorities ?? []).map((item) => (
                  <option key={item.value} value={item.value}>
                    {item.label}
                  </option>
                ))}
              </select>
            </div>
            <div className="filter-item">
              <span className="filter-label">计划时间段起</span>
              <input
                className="input"
                type="date"
                value={planFrom ?? ''}
                onChange={(event) => applyFilter({ planFrom: event.target.value })}
              />
            </div>
            <div className="filter-item">
              <span className="filter-label">计划时间段止</span>
              <input
                className="input"
                type="date"
                value={planTo ?? ''}
                onChange={(event) => applyFilter({ planTo: event.target.value })}
              />
            </div>
            <div className="filter-actions">
              <button type="button" className="btn btn-ghost" onClick={resetAll}>
                重置
              </button>
              <button type="button" className="btn btn-primary" onClick={() => applyFilter({ keyword: keywordInput })}>
                查询
              </button>
            </div>
          </div>

          {/* 常用筛选：保存当前组合，或一键应用 / 删除。 */}
          <div className="saved-filter-bar">
            <div className="saved-filter-group">
              <span className="saved-filter-label">常用筛选</span>
              <button
                type="button"
                className="btn btn-ghost btn-sm"
                disabled={!hasActiveFilters}
                title={hasActiveFilters ? '把当前筛选条件保存下来' : '请先设置至少一个筛选条件'}
                onClick={() => setSaveDialogOpen(true)}
              >
                ＋ 保存当前条件
              </button>
              {savedFilters.length === 0 ? (
                <span className="saved-filter-empty">暂无保存的条件</span>
              ) : (
                savedFilters.map((preset) => (
                  <span key={preset.id} className="saved-filter-chip">
                    <button type="button" className="saved-filter-apply" onClick={() => applyPreset(preset)}>
                      {preset.name}
                    </button>
                    <button
                      type="button"
                      className="saved-filter-remove"
                      aria-label={`删除 ${preset.name}`}
                      onClick={() => handleRemoveFilter(preset.id, preset.name)}
                    >
                      ×
                    </button>
                  </span>
                ))
              )}
            </div>
            <button type="button" className="btn btn-ghost btn-sm" disabled={exporting || conflicts.length > 0} onClick={handleExport}>
              {exporting ? '导出中…' : '导出当前结果'}
            </button>
          </div>

          {/* 条件冲突时在页面上直接说明，并阻止继续请求。 */}
          {conflicts.map((message) => (
            <div key={message} className="alert alert-warn filter-conflict">
              <p>{message}</p>
            </div>
          ))}

          {/* 汇总条：数字全部来自本次筛选响应的 summary，与列表 / 导出同口径。 */}
          <div className={`summary-strip${list.loading ? ' summary-strip-loading' : ''}`}>
            <div className="summary-item">
              <span className="summary-label">任务数量</span>
              <span className="summary-value">{conflicts.length > 0 ? '—' : formatNumber(summary?.taskCount ?? 0, 0)}</span>
              <span className="summary-unit">个</span>
            </div>
            <div className="summary-item">
              <span className="summary-label">清淤记录</span>
              <span className="summary-value">{conflicts.length > 0 ? '—' : formatNumber(summary?.recordCount ?? 0, 0)}</span>
              <span className="summary-unit">条</span>
            </div>
            <div className="summary-item">
              <span className="summary-label">清淤量合计</span>
              <span className="summary-value">
                {conflicts.length > 0 ? '—' : formatNumber(summary?.sludgeVolumeM3 ?? 0, 2)}
              </span>
              <span className="summary-unit">m³</span>
            </div>
            <div className="summary-item">
              <span className="summary-label">清淤长度合计</span>
              <span className="summary-value">
                {conflicts.length > 0 ? '—' : formatNumber(summary?.cleanedLengthM ?? 0, 2)}
              </span>
              <span className="summary-unit">m</span>
            </div>
            <span className="summary-note">
              {conflicts.length > 0
                ? '存在冲突条件，暂未查询'
                : list.loading
                  ? '正在按当前条件统计…'
                  : '按当前筛选条件合计，不受翻页影响'}
            </span>
          </div>

          <DataTable
            columns={columns}
            rows={conflicts.length > 0 ? [] : list.data?.list ?? []}
            rowKey={(row) => row.id}
            loading={conflicts.length === 0 && list.loading}
            error={conflicts.length === 0 ? list.error : ''}
            onRetry={list.reload}
            emptyText={conflicts.length > 0 ? '筛选条件存在冲突' : '未找到符合条件的清淤任务'}
            emptyDescription={conflicts.length > 0 ? '请按上方提示调整相互冲突的条件。' : '可以先登记任务，再录入清淤记录。'}
          />
          <Pagination
            total={total}
            page={page}
            pageSize={list.data?.pageSize ?? PAGE_SIZE}
            onChange={goPage}
          />
        </div>
      </SectionCard>

      <SaveFilterDialog
        open={saveDialogOpen}
        saving={false}
        onClose={() => setSaveDialogOpen(false)}
        onConfirm={handleSaveFilter}
      />

      <ConfirmDialog
        open={pendingDelete !== null}
        title="删除清淤任务"
        danger
        busy={deleting}
        confirmText="确认删除"
        message={
          <>
            <p>
              即将删除任务 <strong>{pendingDelete?.code}</strong>（{pendingDelete?.title}）。
            </p>
            <p>已录入清淤记录或已产生验收记录的任务不允许删除，以保证台账可追溯。</p>
          </>
        }
        onConfirm={handleDelete}
        onCancel={() => setPendingDelete(null)}
      />
    </div>
  );
}
