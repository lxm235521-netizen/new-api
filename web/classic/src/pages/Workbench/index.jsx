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

/**
 * 工作台配置（管理员）。
 *
 * 为视频生成工作台挑选模型，并为每个模型单独配置：
 * 分组、计费方式、时长（固定值/区间）、分辨率、比例、
 * 三类参考素材的开关与数量上限、提示词优化所用的推理模型与指令。
 *
 * 数据结构沿用 option `workbench_setting.models`，前端工作台通过
 * /api/workbench/models 读取（该接口还会按当前用户解析出单价）。
 */

import React, { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Button,
  InputNumber,
  Select,
  Spin,
  Typography,
} from '@douyinfe/semi-ui';
import { Pencil, Plus, Save, Trash2 } from 'lucide-react';
import { API, showError, showSuccess } from '../../helpers';
import { REFERENCE_KINDS } from '../Video/constants';
import ModelEditor from './components/ModelEditor';
import {
  IMAGE_CONCURRENCY_OPTION_KEY,
  IMAGE_ENDPOINT,
  PER_CALL_OPTION_KEY,
  WORKBENCH_OPTION_KEY,
  describeDuration,
  fromDraft,
  newDraft,
  parseList,
  toDraft,
} from './utils';
import './workbench-config.css';

const { Text, Title } = Typography;

function parseModelsOption(value) {
  if (!value) return [];
  try {
    const parsed = JSON.parse(value);
    return Array.isArray(parsed) ? parsed : [];
  } catch {
    return [];
  }
}

const WorkbenchConfig = () => {
  const { t } = useTranslation();

  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [items, setItems] = useState([]);
  const [dirty, setDirty] = useState(false);
  const [candidates, setCandidates] = useState([]);
  const [endpointMap, setEndpointMap] = useState({});
  const [perCallSet, setPerCallSet] = useState(new Set());
  const [editingIndex, setEditingIndex] = useState(-1);
  const [draft, setDraft] = useState(null);
  // 异步图片生成：每个用户同时能跑几张（0 = 不限）
  const [imageConcurrency, setImageConcurrency] = useState(2);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);

    Promise.all([
      API.get('/api/option/'),
      API.get('/api/channel/models_enabled'),
      API.get('/api/models/', { params: { page: 1, page_size: 1000 } }),
    ])
      .then(([optionRes, enabledRes, metaRes]) => {
        if (cancelled) return;

        const options = optionRes.data?.data || [];
        const findOption = (key) =>
          options.find((option) => option.key === key)?.value;

        setItems(parseModelsOption(findOption(WORKBENCH_OPTION_KEY)));
        setPerCallSet(new Set(parseList(findOption(PER_CALL_OPTION_KEY))));
        const concurrency = Number(
          findOption(IMAGE_CONCURRENCY_OPTION_KEY) ?? 2,
        );
        setImageConcurrency(Number.isFinite(concurrency) ? concurrency : 2);
        setDirty(false);

        const enabled = enabledRes.data?.data;
        setCandidates(
          Array.isArray(enabled)
            ? enabled.filter((name) => typeof name === 'string')
            : [],
        );

        const metas = metaRes.data?.data?.items;
        const map = {};
        if (Array.isArray(metas)) {
          metas.forEach((meta) => {
            if (!meta?.model_name) return;
            map[meta.model_name] = meta.endpoints
              ? String(meta.endpoints)
                  .split(',')
                  .map((s) => s.trim())
              : [];
          });
        }
        setEndpointMap(map);
      })
      .catch((error) => {
        if (!cancelled) showError(error?.message || t('加载配置失败'));
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });

    return () => {
      cancelled = true;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const modelOptions = useMemo(
    () => candidates.map((name) => ({ label: name, value: name })),
    [candidates],
  );

  const addOptions = useMemo(
    () =>
      candidates
        .filter((name) => !items.some((item) => item.model === name))
        .map((name) => ({ label: name, value: name })),
    [candidates, items],
  );

  /** 依据 model_meta 的 endpoints 猜分组，管理员可在抽屉里改 */
  const guessGroup = (modelName) => {
    const endpoints = endpointMap[modelName];
    return Array.isArray(endpoints) && endpoints.includes(IMAGE_ENDPOINT)
      ? 'image'
      : 'video';
  };

  const handleAdd = (modelName) => {
    if (!modelName) return;
    const group = guessGroup(modelName);
    const next = newDraft(modelName, group);
    // 若该模型已在「任务按次计费设置」里，预填按次计费
    if (perCallSet.has(modelName)) next.billing = 'per_call';
    setDraft(next);
    setEditingIndex(-1);
  };

  const handleEdit = (index) => {
    setDraft(toDraft(items[index]));
    setEditingIndex(index);
  };

  const closeEditor = () => {
    setDraft(null);
    setEditingIndex(-1);
  };

  const handleConfirmEditor = () => {
    if (!draft) return;
    if (
      items.some(
        (item, idx) => item.model === draft.model && idx !== editingIndex,
      )
    ) {
      showError(t('该模型已在列表中'));
      return;
    }
    // 开了优化却没填模型：直接拦住。否则工作台按 enabled && model 判断，
    // 不会渲染按钮，管理员会以为"启用了却没生效"。
    if (draft.optimizeEnabled && !draft.optimizeModel.trim()) {
      showError(t('已开启提示词优化，请先选择或输入优化用的模型'));
      return;
    }

    const next = fromDraft(draft);
    setItems((prev) => {
      if (editingIndex >= 0) {
        const copy = [...prev];
        copy[editingIndex] = next;
        return copy;
      }
      return [...prev, next];
    });
    setDirty(true);
    closeEditor();
  };

  const handleRemove = (index) => {
    setItems((prev) => prev.filter((_, idx) => idx !== index));
    setDirty(true);
  };

  const handleSave = async () => {
    setSaving(true);
    try {
      const [modelsRes, concurrencyRes] = await Promise.all([
        API.put('/api/option/', {
          key: WORKBENCH_OPTION_KEY,
          value: JSON.stringify(items),
        }),
        API.put('/api/option/', {
          key: IMAGE_CONCURRENCY_OPTION_KEY,
          value: String(imageConcurrency),
        }),
      ]);
      if (modelsRes.data?.success && concurrencyRes.data?.success) {
        showSuccess(t('保存成功'));
        setDirty(false);
      } else {
        showError(t('保存失败，请重试'));
      }
    } catch (error) {
      showError(error?.message || t('保存失败，请重试'));
    } finally {
      setSaving(false);
    }
  };

  const renderReferenceSummary = (item) => {
    const refs = item.references || {};
    const parts = REFERENCE_KINDS.filter((kind) => refs[kind.key]?.enabled).map(
      (kind) => `${t(kind.labelKey)}×${refs[kind.key].max}`,
    );
    return parts.length ? parts.join(' / ') : t('无');
  };

  return (
    <div className='wbcfg-page'>
      <div className='wbcfg-header'>
        <div>
          <Title heading={5} style={{ margin: 0 }}>
            {t('工作台配置')}
          </Title>
          <Text size='small' type='tertiary'>
            {t(
              '选择视频生成工作台展示哪些模型，并为每个模型单独配置参数能力与提示词优化。',
            )}
          </Text>
        </div>

        <div className='wbcfg-header__actions'>
          {/* 异步图片生成：上游是同步接口，这里限制单个用户同时跑几张，
              挡住「一个人开一堆标签页把上游占满」；0 = 不限 */}
          <div className='wbcfg-concurrency'>
            <Text size='small' type='tertiary'>
              {t('图片并发/用户')}
            </Text>
            <InputNumber
              value={imageConcurrency}
              min={0}
              max={50}
              size='small'
              style={{ width: 78 }}
              disabled={loading}
              onChange={(value) => {
                const next = Number(value);
                setImageConcurrency(Number.isFinite(next) ? next : 0);
                setDirty(true);
              }}
            />
            <Text size='small' type='tertiary'>
              {imageConcurrency > 0 ? t('0 表示不限') : t('当前不限并发')}
            </Text>
          </div>

          <Select
            value={null}
            onChange={handleAdd}
            filter
            prefix={<Plus size={14} />}
            placeholder={t('搜索并添加模型')}
            optionList={addOptions}
            style={{ width: 260 }}
            disabled={loading}
          />
          <Button
            theme='solid'
            type='primary'
            icon={<Save size={14} />}
            loading={saving}
            disabled={!dirty || loading}
            onClick={handleSave}
          >
            {dirty ? t('保存配置') : t('已保存')}
          </Button>
        </div>
      </div>

      <Spin spinning={loading}>
        {items.length === 0 ? (
          <div className='wbcfg-empty'>
            <Text type='tertiary'>
              {t('还没有配置任何模型，用右上角搜索添加。')}
            </Text>
          </div>
        ) : (
          <div className='wbcfg-list'>
            <div className='wbcfg-list__head'>
              <span>{t('模型')}</span>
              <span>{t('分组')}</span>
              <span>{t('计费')}</span>
              <span>{t('时长')}</span>
              <span>{t('分辨率')}</span>
              <span>{t('比例')}</span>
              <span>{t('参考素材')}</span>
              <span>{t('优化')}</span>
              <span />
            </div>

            {items.map((item, index) => (
              <div className='wbcfg-list__row' key={item.model}>
                <span className='wbcfg-list__model'>{item.model}</span>
                <span>{item.group === 'image' ? t('图片') : t('视频')}</span>
                <span>
                  {item.billing === 'per_second'
                    ? t('按秒计费')
                    : t('按次计费')}
                </span>
                <span>{describeDuration(item)}</span>
                <span>{item.resolutions?.join('/') || '—'}</span>
                <span>{item.aspect_ratios?.join('/') || '—'}</span>
                <span>{renderReferenceSummary(item)}</span>
                <span>
                  {item.prompt_optimize?.enabled
                    ? item.prompt_optimize.model || (
                        <Text type='danger' size='small'>
                          {t('未填模型')}
                        </Text>
                      )
                    : '—'}
                </span>
                <span className='wbcfg-list__ops'>
                  <Button
                    theme='borderless'
                    size='small'
                    icon={<Pencil size={14} />}
                    aria-label={t('编辑')}
                    onClick={() => handleEdit(index)}
                  />
                  <Button
                    theme='borderless'
                    type='danger'
                    size='small'
                    icon={<Trash2 size={14} />}
                    aria-label={t('删除')}
                    onClick={() => handleRemove(index)}
                  />
                </span>
              </div>
            ))}
          </div>
        )}
      </Spin>

      <ModelEditor
        visible={draft !== null}
        draft={draft}
        modelOptions={modelOptions}
        onChange={setDraft}
        onCancel={closeEditor}
        onSave={handleConfirmEditor}
      />
    </div>
  );
};

export default WorkbenchConfig;
