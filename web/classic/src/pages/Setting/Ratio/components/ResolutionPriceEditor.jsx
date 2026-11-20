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
import { Banner, Button, Card, Input, Switch } from '@douyinfe/semi-ui';
import { IconDelete, IconPlus } from '@douyinfe/semi-icons';

const QUICK_TIERS = ['480p', '720p', '768p', '1080p', '4k'];

/**
 * 按分辨率定价的可选编辑器。
 *
 * 只有固定价格（按次/按秒）模型才会渲染；关闭开关时不写入任何配置，
 * 计费完全沿用原来的按次/按秒单价。
 */
export default function ResolutionPriceEditor({
  model,
  enabled,
  unitLabel,
  hasFixedPrice,
  onToggle,
  onChange,
  onAdd,
  onRemove,
  t,
}) {
  const rows = Array.isArray(model?.resolutionPrices)
    ? model.resolutionPrices
    : [];
  const usedTiers = new Set(
    rows.map((row) =>
      String(row.resolution || '')
        .trim()
        .toLowerCase(),
    ),
  );
  const quickTiers = QUICK_TIERS.filter((tier) => !usedTiers.has(tier));

  return (
    <Card
      bodyStyle={{ padding: 16 }}
      style={{ marginBottom: 16, background: 'var(--semi-color-fill-0)' }}
    >
      <div className='flex items-start justify-between gap-3'>
        <div>
          <div className='font-medium'>{t('按分辨率定价（可选）')}</div>
          <div className='mt-1 text-xs text-gray-500'>
            {t(
              '未指定的分辨率按固定价格计费；不填这里则完全沿用原来的按次/按秒计费。',
            )}
          </div>
        </div>
        <Switch size='small' checked={enabled} onChange={onToggle} />
      </div>

      {enabled ? (
        <div style={{ marginTop: 12 }}>
          {rows.length === 0 ? (
            <div className='text-sm text-gray-500'>
              {t('还没有分辨率档位，点下方按钮添加。')}
            </div>
          ) : (
            rows.map((row, index) => (
              <div
                key={index}
                className='flex items-center gap-2'
                style={{ marginBottom: 8 }}
              >
                <Input
                  value={row.resolution}
                  placeholder='720p'
                  style={{ width: 120 }}
                  onChange={(value) => onChange(index, 'resolution', value)}
                />
                <Input
                  value={row.price}
                  placeholder={t('单价')}
                  prefix='$'
                  suffix={unitLabel}
                  style={{ flex: 1 }}
                  onChange={(value) => onChange(index, 'price', value)}
                />
                <Button
                  type='danger'
                  theme='borderless'
                  icon={<IconDelete />}
                  onClick={() => onRemove(index)}
                  aria-label={t('删除')}
                />
              </div>
            ))
          )}

          <div className='mt-2 flex flex-wrap items-center gap-2'>
            <Button size='small' icon={<IconPlus />} onClick={() => onAdd('')}>
              {t('添加档位')}
            </Button>
            {quickTiers.map((tier) => (
              <Button
                key={tier}
                size='small'
                theme='light'
                type='tertiary'
                onClick={() => onAdd(tier)}
              >
                {tier}
              </Button>
            ))}
          </div>

          {!hasFixedPrice ? (
            <Banner
              type='warning'
              bordered
              fullMode={false}
              closeIcon={null}
              style={{ marginTop: 12 }}
              description={t(
                '未设置固定价格：未指定分辨率的请求会退回按模型倍率计费。',
              )}
            />
          ) : null}
        </div>
      ) : null}
    </Card>
  );
}
