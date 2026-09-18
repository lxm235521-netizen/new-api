/*
Copyright (C) 2025 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

import React from 'react';
import { useTranslation } from 'react-i18next';
import { Pagination, Select, Switch, Tabs } from '@douyinfe/semi-ui';
import { LayoutGrid } from 'lucide-react';
import { MEDIA_FILTER_OPTIONS, STATUS_FILTER_OPTIONS } from '../constants';
import TaskCard from './TaskCard';

const TabPane = Tabs.TabPane;

/**
 * 左栏：历史任务。
 *
 * 工具栏（产出/状态筛选、仅看当前模型、总数）+ 12 条一页的卡片网格（4 列 × 3 行）
 * + 底部分页器。分页由后端完成，`total` 是后端给的总数。
 */
const HistoryPanel = ({
  tasks,
  total,
  page,
  pageSize,
  onPageChange,
  mediaFilter,
  statusFilter,
  onlyCurrentModel,
  onMediaFilterChange,
  onStatusFilterChange,
  onOnlyCurrentModelChange,
  onRetryTask,
  onOpenTask,
}) => {
  const { t } = useTranslation();

  const statusOptions = STATUS_FILTER_OPTIONS.map((option) => ({
    label: t(option.labelKey),
    value: option.value,
  }));

  return (
    <div className='wb-main'>
      <div className='wb-toolbar'>
        <div className='wb-toolbar__row'>
          <div className='wb-toolbar__title'>{t('全部历史任务')}</div>

          <div className='flex flex-wrap items-center gap-2'>
            <Tabs
              type='button'
              size='small'
              activeKey={mediaFilter}
              onChange={onMediaFilterChange}
            >
              {MEDIA_FILTER_OPTIONS.map((option) => (
                <TabPane
                  key={option.value}
                  itemKey={option.value}
                  tab={t(option.labelKey)}
                />
              ))}
            </Tabs>

            <Select
              value={statusFilter}
              onChange={onStatusFilterChange}
              optionList={statusOptions}
              style={{ width: 132 }}
              size='small'
            />
          </div>
        </div>

        <div className='wb-toolbar__meta'>
          <div className='wb-toolbar__switch'>
            <Switch
              size='small'
              checked={onlyCurrentModel}
              onChange={onOnlyCurrentModelChange}
            />
            <span>{t('仅查看当前模型')}</span>
          </div>

          <span className='wb-count'>{t('共 {{total}} 条', { total })}</span>
        </div>
      </div>

      {tasks.length === 0 ? (
        <div className='wb-grid'>
          <div className='wb-empty'>
            <span className='wb-empty__icon'>
              <LayoutGrid size={22} aria-hidden='true' />
            </span>
            <span className='wb-empty__title'>
              {t('没有符合当前筛选条件的任务')}
            </span>
          </div>
        </div>
      ) : (
        <div className='wb-grid'>
          {tasks.map((task) => (
            <TaskCard
              key={task.taskId}
              task={task}
              onRetry={onRetryTask}
              onOpen={onOpenTask}
            />
          ))}
        </div>
      )}

      <div className='wb-pager'>
        <span className='wb-pager__hint'>
          {t('本页 {{count}} 条', { count: tasks.length })}
        </span>
        <Pagination
          currentPage={page}
          pageSize={pageSize}
          total={total}
          onPageChange={onPageChange}
          showSizeChanger={false}
        />
      </div>
    </div>
  );
};

export default HistoryPanel;
