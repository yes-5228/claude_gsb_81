// 清淤任务列表：支持片区、道路、状态、来源、实施班组与计划时间段组合筛选，
// 可保存常用条件；顶部任务数 / 清淤量汇总与筛选结果同源，翻页不变。
import { useEffect, useMemo, useState } from 'react';
import { Link, useNavigate, useSearchParams } from 'react-router-dom';
import { toErrorMessage } from '../../api/client';
import { taskApi, type TaskQuery } from '../../api/tasks';
import { ConfirmDialog } from '../../components/ConfirmDialog';
import { DataTable, type Column } from '../../components/DataTable';
import { Modal } from '../../components/Modal';
import { PageHeader } from '../../components/PageHeader';
import { Pagination } from '../../components/Pagination';
import { SectionCard } from '../../components/SectionCard';
import { StatCard } from '../../components/StatCard';
import { StatusTag } from '../../components/StatusTag';
import { useToast } from '../../components/Toast';
import { useAsync } from '../../hooks/useAsync';
import { useSavedTaskFilters } from '../../hooks/useSavedTaskFilters';
import { useMeta } from '../../providers/MetaProvider';
import type { TaskListItem } from '../../types/domain';
import { formatDate, formatNumber, formatVolume, isDateString } from '../../utils/format';

const PAGE_SIZE = 10;

// 参与组合筛选的查询参数（page 单独处理，切换这些条件时一律回到第一页）。
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

/** 从 URLSearchParams 读取当前筛选条件（不含分页）。 */
function readFilters(params: URLSearchParams): Record<FilterKey, string> {
  return {
    keyword: params.get('keyword') ?? '',
    status: params.get('status') ?? '',
    district: params.get('district') ?? '',
    roadName: params.get('roadName') ?? '',
    priority: params.get('priority') ?? '',
    source: params.get('source') ?? '',
    teamName: params.get('teamName') ?? '',
    planFrom: params.get('planFrom') ?? '',
    planTo: params.get('planTo') ?? ''
  };
}

/** 去掉空值，得到实际生效的条件，用于保存常用条件与判空。 */
function activeFilters(filters: Record<FilterKey, string>): Record<string, string> {
  const result: Record<string, string> = {};
  FILTER_KEYS.forEach((key) => {
    if (filters[key]) {
      result[key] = filters[key];
    }
  });
  return result;
}

export function TaskListPage() {
  const navigate = useNavigate();
  const toast = useToast();
  const { enums } = useMeta();
  const [params, setParams] = useSearchParams();
  const { filters: savedFilters, saveFilter, removeFilter } = useSavedTaskFilters();

  const filters = readFilters(params);
  const page = Math.max(1, Number(params.get('page') ?? '1') || 1);

  const [keywordInput, setKeywordInput] = useState(filters.keyword);
  useEffect(() => {
    setKeywordInput(filters.keyword);
  }, [filters.keyword]);

  const [saveOpen, setSaveOpen] = useState(false);
  const [saveName, setSaveName] = useState('');
  const [exporting, setExporting] = useState(false);

  const filterOptions = useAsync(
    // 片区切换时道路列表随片区收窄，只展示该片区已建档的道路。
    () => taskApi.filterOptions(filters.district || undefined),
    [filters.district]
  );

  // 计划时间段冲突 / 非法格式：前端先直接说明，不发出无效请求。
  const dateProblem = useMemo(() => {
    const { planFrom, planTo } = filters;
    if (planFrom && !isDateString(planFrom)) {
      return '计划开始日期起格式不正确，应为 YYYY-MM-DD';
    }
    if (planTo && !isDateString(planTo)) {
      return '计划开始日期止格式不正确，应为 YYYY-MM-DD';
    }
    if (planFrom && planTo && planFrom > planTo) {
      return '筛选条件冲突：计划开始日期起不能晚于计划开始日期止，请调整后重新查询';
    }
    return '';
  }, [filters.planFrom, filters.planTo]);

  const blocked = dateProblem !== '';

  const query: TaskQuery = {
    ...activeFilters(filters),
    page,
    pageSize: PAGE_SIZE
  };

  const list = useAsync(
    () => {
      if (blocked) {
        // 条件冲突时不发请求，避免页面上出现与条件无关的旧汇总。
        return Promise.reject(new Error(dateProblem));
      }
      return taskApi.list(query);
      // query 每次渲染都是新对象，依赖项显式列出，随条件变化重新加载。
      // eslint-disable-next-line react-hooks/exhaustive-deps
    },
    [
      filters.keyword,
      filters.status,
      filters.district,
      filters.roadName,
      filters.priority,
      filters.source,
      filters.teamName,
      filters.planFrom,
      filters.planTo,
      page,
      blocked,
      dateProblem
    ]
  );

  // 道路选项已由后端按当前片区收窄；若已选道路不在其中（例如从保存条件进入），
  // 说明片区与道路组合冲突，页面直接给出说明。
  const roadOptions = filterOptions.data?.roads ?? [];

  const roadDistrictMismatch = Boolean(filters.district && filters.roadName && !roadOptions.includes(filters.roadName));

  // 当前页码超出总页数（例如直接打开带 page 的链接）时自动回到第一页。
  const pageCount = list.data ? Math.max(1, Math.ceil(list.data.total / list.data.pageSize)) : 1;
  useEffect(() => {
    if (list.data && list.data.total > 0 && page > pageCount) {
      const next = new URLSearchParams(params);
      next.set('page', '1');
      setParams(next);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [list.data, page, pageCount]);

  /** 修改任意筛选条件：写入 URL 并强制回到第一页。 */
  const applyFilter = (patch: Partial<Record<FilterKey, string>>) => {
    const next = new URLSearchParams(params);
    Object.entries(patch).forEach(([key, value]) => {
      if (value) {
        next.set(key, value);
      } else {
        next.delete(key);
      }
    });
    // 道路与片区联动：切换片区后原道路必然不属于新片区，直接清空避免产生空组合。
    if (patch.district !== undefined && patch.district !== filters.district && filters.roadName) {
      next.delete('roadName');
    }
    next.set('page', '1');
    setParams(next);
  };

  const resetFilters = () => {
    setKeywordInput('');
    setParams(new URLSearchParams());
  };

  const goPage = (nextPage: number) => {
    const next = new URLSearchParams(params);
    next.set('page', String(nextPage));
    setParams(next);
  };

  const applySavedFilter = (saved: Record<string, string>) => {
    const next = new URLSearchParams();
    Object.entries(saved).forEach(([key, value]) => {
      if (FILTER_KEYS.includes(key as FilterKey) && value) {
        next.set(key, value);
      }
    });
    next.set('page', '1');
    setKeywordInput(saved.keyword ?? '');
    setParams(next);
  };

  const handleSaveFilter = () => {
    const active = activeFilters(filters);
    if (Object.keys(active).length === 0) {
      toast.error('当前没有任何筛选条件，无法保存');
      return;
    }
    if (saveFilter(saveName, active)) {
      toast.success(`已保存常用筛选「${saveName.trim()}」`);
      setSaveOpen(false);
      setSaveName('');
    } else {
      toast.error('请填写筛选条件名称');
    }
  };

  const handleExport = async () => {
    if (blocked) {
      toast.error(dateProblem);
      return;
    }
    setExporting(true);
    try {
      // GET 接口返回 Content-Disposition 附件，浏览器会直接触发下载，不会离开当前页。
      const response = await fetch(taskApi.exportUrl(activeFilters(filters)));
      if (!response.ok) {
        let message = `导出失败（HTTP ${response.status}）`;
        try {
          const envelope = (await response.json()) as { message?: string };
          if (envelope.message) {
            message = envelope.message;
          }
        } catch {
          // 非 JSON 错误时使用兜底文案。
        }
        throw new Error(message);
      }
      const blob = await response.blob();
      const disposition = response.headers.get('Content-Disposition') ?? '';
      const match = /filename\*=UTF-8''([^;]+)/i.exec(disposition);
      const filename = match ? decodeURIComponent(match[1]) : `清淤任务_${Date.now()}.csv`;
      const url = window.URL.createObjectURL(blob);
      const anchor = document.createElement('a');
      anchor.href = url;
      anchor.download = filename;
      document.body.appendChild(anchor);
      anchor.click();
      anchor.remove();
      window.URL.revokeObjectURL(url);
      if (response.headers.get('X-Export-Truncated') === '1') {
        toast.show('info', '符合条件的任务超过导出上限，仅导出了前 10000 条，请缩小筛选范围');
      } else {
        toast.success('导出完成，条数与合计与当前筛选结果一致');
      }
    } catch (cause: unknown) {
      toast.error(toErrorMessage(cause));
    } finally {
      setExporting(false);
    }
  };

  const [pendingDelete, setPendingDelete] = useState<TaskListItem | null>(null);
  const [deleting, setDeleting] = useState(false);
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

  const summary = list.data?.summary;
  const summaryTone = list.loading ? 'summary-loading' : '';

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
          <>
            <button type="button" className="btn btn-ghost" onClick={handleExport} disabled={exporting}>
              {exporting ? '导出中…' : '导出当前筛选'}
            </button>
            <button type="button" className="btn btn-primary" onClick={() => navigate('/tasks/new')}>
              登记任务
            </button>
          </>
        }
      />

      {/* 顶部汇总：与列表在同一个接口响应里返回，随筛选变化、不随翻页变化。 */}
      <div className={`stat-grid task-summary ${summaryTone}`}>
        <StatCard label="筛选命中任务" value={formatNumber(list.data?.total ?? 0, 0)} tone="primary" />
        <StatCard
          label="已录入清淤记录的任务"
          value={formatNumber(summary?.taskCount ?? 0, 0)}
          hint={`占命中任务的 ${list.data && list.data.total > 0 ? formatNumber(((summary?.taskCount ?? 0) / list.data.total) * 100, 1) : '0'}%`}
        />
        <StatCard label="清淤记录条数" value={formatNumber(summary?.recordCount ?? 0, 0)} hint="按全部命中任务统计，不受翻页影响" />
        <StatCard
          label="清淤量合计"
          value={formatVolume(summary?.sludgeVolumeM3 ?? 0)}
          hint={`累计清淤长度 ${formatNumber(summary?.cleanedLengthM ?? 0, 2)} m`}
          tone="success"
        />
      </div>

      <SectionCard title="任务清单" subtitle={`共 ${list.data?.total ?? 0} 条记录`}>
        <div className="card-body-flush">
          {savedFilters.length > 0 ? (
            <div className="saved-filter-bar">
              <span className="saved-filter-label">常用条件</span>
              {savedFilters.map((saved) => (
                <span key={saved.id} className="saved-filter-chip-group">
                  <button type="button" className="saved-filter-chip" onClick={() => applySavedFilter(saved.params)}>
                    {saved.name}
                  </button>
                  <button
                    type="button"
                    className="saved-filter-remove"
                    aria-label={`删除常用条件 ${saved.name}`}
                    onClick={() => removeFilter(saved.id)}
                  >
                    ×
                  </button>
                </span>
              ))}
            </div>
          ) : null}

          <div className="filter-bar">
            <div className="filter-item" style={{ minWidth: 200 }}>
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
              <span className="filter-label">任务状态</span>
              <select className="select" value={filters.status} onChange={(event) => applyFilter({ status: event.target.value })}>
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
              <select className="select" value={filters.source} onChange={(event) => applyFilter({ source: event.target.value })}>
                <option value="">全部来源</option>
                {(enums?.taskSources ?? []).map((item) => (
                  <option key={item.value} value={item.value}>
                    {item.label}
                  </option>
                ))}
              </select>
            </div>
            <div className="filter-item">
              <span className="filter-label">所属片区</span>
              <select className="select" value={filters.district} onChange={(event) => applyFilter({ district: event.target.value })}>
                <option value="">全部片区</option>
                {(filterOptions.data?.districts ?? []).map((item) => (
                  <option key={item} value={item}>
                    {item}
                  </option>
                ))}
              </select>
            </div>
            <div className="filter-item">
              <span className="filter-label">所在道路</span>
              <select className="select" value={filters.roadName} onChange={(event) => applyFilter({ roadName: event.target.value })}>
                <option value="">全部道路</option>
                {roadOptions.map((item) => (
                  <option key={item} value={item}>
                    {item}
                  </option>
                ))}
              </select>
            </div>
            <div className="filter-item">
              <span className="filter-label">实施班组</span>
              <select className="select" value={filters.teamName} onChange={(event) => applyFilter({ teamName: event.target.value })}>
                <option value="">全部班组</option>
                {(filterOptions.data?.teams ?? []).map((item) => (
                  <option key={item} value={item}>
                    {item}
                  </option>
                ))}
              </select>
            </div>
            <div className="filter-item">
              <span className="filter-label">优先级</span>
              <select className="select" value={filters.priority} onChange={(event) => applyFilter({ priority: event.target.value })}>
                <option value="">全部优先级</option>
                {(enums?.taskPriorities ?? []).map((item) => (
                  <option key={item.value} value={item.value}>
                    {item.label}
                  </option>
                ))}
              </select>
            </div>
            <div className="filter-item filter-item-range">
              <span className="filter-label">计划开始日期</span>
              <div className="date-range">
                <input
                  type="date"
                  className="input"
                  value={filters.planFrom}
                  onChange={(event) => applyFilter({ planFrom: event.target.value })}
                />
                <span className="date-range-sep">至</span>
                <input
                  type="date"
                  className="input"
                  value={filters.planTo}
                  onChange={(event) => applyFilter({ planTo: event.target.value })}
                />
              </div>
            </div>
            <div className="filter-actions">
              <button type="button" className="btn btn-ghost" onClick={resetFilters}>
                重置
              </button>
              <button type="button" className="btn btn-ghost" onClick={() => setSaveOpen(true)}>
                保存条件
              </button>
              <button type="button" className="btn btn-primary" onClick={() => applyFilter({ keyword: keywordInput })}>
                查询
              </button>
            </div>
          </div>

          {dateProblem ? (
            <div className="alert alert-warn filter-alert">
              <p>{dateProblem}</p>
            </div>
          ) : null}
          {!dateProblem && roadDistrictMismatch ? (
            <div className="alert alert-info filter-alert">
              <p>
                筛选条件冲突：「{filters.roadName}」不属于「{filters.district}」，该组合下没有任务；
                请切换片区或清空道路条件。
              </p>
            </div>
          ) : null}

          <DataTable
            columns={columns}
            rows={blocked ? [] : list.data?.list ?? []}
            rowKey={(row) => row.id}
            loading={!blocked && list.loading}
            error={blocked ? '' : list.error}
            onRetry={list.reload}
            emptyText={blocked ? '当前筛选条件存在冲突' : '未找到符合条件的清淤任务'}
            emptyDescription={blocked ? '请调整上方标红 / 提示的条件后重新查询。' : '可以先登记任务，再录入清淤记录。'}
          />
          <Pagination
            total={list.data?.total ?? 0}
            page={page}
            pageSize={list.data?.pageSize ?? PAGE_SIZE}
            onChange={goPage}
          />
        </div>
      </SectionCard>

      <Modal
        open={saveOpen}
        title="保存常用筛选条件"
        onClose={() => setSaveOpen(false)}
        footer={
          <>
            <button type="button" className="btn btn-ghost" onClick={() => setSaveOpen(false)}>
              取消
            </button>
            <button type="button" className="btn btn-primary" onClick={handleSaveFilter}>
              保存
            </button>
          </>
        }
      >
        <p className="modal-tip">把当前的组合筛选条件命名保存，之后在列表上方一键套用。</p>
        <input
          className="input"
          placeholder="例如：城东汛期投诉任务"
          value={saveName}
          maxLength={20}
          autoFocus
          onChange={(event) => setSaveName(event.target.value)}
          onKeyDown={(event) => {
            if (event.key === 'Enter') {
              handleSaveFilter();
            }
          }}
        />
        <p className="modal-tip">
          将保存 {Object.keys(activeFilters(filters)).length} 个条件；
          分页页码不会保存。
        </p>
      </Modal>

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
