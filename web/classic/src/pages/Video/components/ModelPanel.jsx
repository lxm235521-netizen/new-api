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

import React, { useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Button,
  InputNumber,
  Popover,
  Select,
  Slider,
} from '@douyinfe/semi-ui';
import { ChevronDown } from 'lucide-react';
import { DURATION_MODE, MODEL_GROUPS, REFERENCE_KINDS } from '../constants';
import { referenceMax } from '../utils';
import PromptField from './PromptField';
import ReferenceUpload from './ReferenceUpload';

/** 标签 + 下拉的通用参数项 */
const ParamSelect = ({ label, value, options, renderOption, onChange }) => (
  <div className='wb-param'>
    <span className='wb-param__label'>{label}</span>
    <Select
      value={value}
      onChange={onChange}
      optionList={options.map((option) => ({
        value: option,
        label: renderOption ? renderOption(option) : option,
      }))}
      style={{ width: '100%' }}
    />
  </div>
);

/** 连续时长：点开下拉，用滑杆选或直接输入 */
const RangeSelect = ({ label, value, min, max, step, unit, onChange }) => {
  const [open, setOpen] = useState(false);

  const panel = (
    <div className='wb-range__panel'>
      <div className='wb-range__row'>
        <Slider
          className='wb-range__slider'
          min={min}
          max={max}
          step={step}
          value={value}
          onChange={onChange}
        />
        <InputNumber
          className='wb-range__input'
          min={min}
          max={max}
          step={step}
          value={value}
          onChange={(next) => onChange(Number(next) || min)}
        />
        <span className='wb-range__unit'>{unit}</span>
      </div>
      <div className='wb-range__presets'>
        {[min, Math.round((min + max) / 2), max].map((preset) => (
          <Button
            key={preset}
            theme={value === preset ? 'solid' : 'borderless'}
            type='primary'
            size='small'
            onClick={() => onChange(preset)}
          >
            {preset}
            {unit}
          </Button>
        ))}
      </div>
    </div>
  );

  return (
    <div className='wb-param wb-param--wide'>
      <span className='wb-param__label'>{label}</span>
      <Popover
        content={panel}
        trigger='click'
        position='bottomLeft'
        visible={open}
        onVisibleChange={setOpen}
        contentClassName='wb-range__popover'
      >
        {/* 触发器与其它下拉框同一套外观 */}
        <button type='button' className='wb-range-trigger'>
          <span>
            {value}
            {unit}
          </span>
          <ChevronDown size={14} aria-hidden='true' />
        </button>
      </Popover>
      <span className='wb-range__hint'>
        {min}~{max}
        {unit}
      </span>
    </div>
  );
};

/**
 * 按「视频 / 图片」分组的模型下拉。
 *
 * 注意：分组必须用 JSX 子节点 + Select.OptGroup。Semi 的 optionList 会被
 * 压平进一个无标题分组（getOptionsFromChildren 里写死了 label: ''），
 * 在 optionList 里放 children 不会渲染出分组标题。
 */
const ModelSelect = ({ models, value, onChange }) => {
  const { t } = useTranslation();

  return (
    <Select
      className='wb-model-select'
      value={value}
      onChange={onChange}
      style={{ width: '100%' }}
    >
      {MODEL_GROUPS.map((group) => {
        const groupModels = models.filter((m) => m.group === group.key);
        if (groupModels.length === 0) return null;
        return (
          <Select.OptGroup key={group.key} label={t(group.labelKey)}>
            {groupModels.map((m) => (
              <Select.Option key={m.value} value={m.value} label={m.label} />
            ))}
          </Select.OptGroup>
        );
      })}
    </Select>
  );
};

/** 模型能力摘要（计费方式 + 参考素材上限），让用户一眼看到这个模型要什么 */
const CapabilitySummary = ({ model, billingLabel }) => {
  const { t } = useTranslation();
  const chips = [];

  if (billingLabel) chips.push(t(billingLabel));
  REFERENCE_KINDS.forEach((kind) => {
    if (!model.references?.[kind.key]?.enabled) return;
    chips.push(`${t(kind.labelKey)} ≤${referenceMax(model, kind.key)}`);
  });

  if (chips.length === 0) return null;

  return (
    <div className='wb-chips'>
      {chips.map((chip) => (
        <span key={chip} className='wb-chip'>
          {chip}
        </span>
      ))}
    </div>
  );
};

/**
 * 右栏：模型与参数。
 *
 * 所有控件都由所选模型的配置驱动 —— 管理员没配的项直接不渲染，
 * 因此组件里不含任何"图片模型不显示分辨率"之类的硬编码判断。
 */
const ModelPanel = ({
  models,
  model,
  form,
  onChange,
  billingLabel,
  tokenKey,
}) => {
  const { t } = useTranslation();

  if (!model) return null;

  const duration = model.duration;
  const hasDuration =
    duration.mode === DURATION_MODE.RANGE || duration.values.length > 0;

  return (
    <div className='wb-sections'>
      <div className='wb-field'>
        <span className='wb-field__label'>{t('选择模型')}</span>

        <ModelSelect
          models={models}
          value={form.model}
          onChange={(value) => onChange({ model: value })}
        />

        <CapabilitySummary model={model} billingLabel={billingLabel} />
      </div>

      {REFERENCE_KINDS.map((kind) => {
        if (!model.references?.[kind.key]?.enabled) return null;
        return (
          <ReferenceUpload
            key={kind.key}
            kind={kind.key}
            labelKey={kind.labelKey}
            hintKey={kind.hintKey}
            chooseLabelKey={kind.chooseLabelKey}
            accept={kind.accept}
            maxCount={referenceMax(model, kind.key)}
            maxBytes={kind.maxBytes}
            assets={form.references[kind.key] || []}
            onChange={(assets) =>
              onChange({
                references: { ...form.references, [kind.key]: assets },
              })
            }
          />
        );
      })}

      <PromptField
        value={form.prompt}
        onChange={(prompt) => onChange({ prompt })}
        optimize={model.promptOptimize}
        referenceImages={form.references.image || []}
        tokenKey={tokenKey}
      />

      {(hasDuration ||
        model.resolutions.length > 0 ||
        model.aspectRatios.length > 0) && (
        <div className='wb-params'>
          {hasDuration &&
            (duration.mode === DURATION_MODE.RANGE ? (
              <RangeSelect
                label={t('时长（秒）')}
                value={form.duration}
                min={duration.min}
                max={duration.max}
                step={duration.step}
                unit={t('秒')}
                onChange={(value) => onChange({ duration: value })}
              />
            ) : (
              <ParamSelect
                label={t('时长（秒）')}
                value={String(form.duration)}
                options={duration.values.map(String)}
                renderOption={(option) => `${option}秒`}
                onChange={(value) => onChange({ duration: Number(value) })}
              />
            ))}

          {/* 分辨率为空即不渲染（图片类模型就是这样） */}
          {model.resolutions.length > 0 && (
            <ParamSelect
              label={t('分辨率')}
              value={form.resolution}
              options={model.resolutions}
              onChange={(resolution) => onChange({ resolution })}
            />
          )}

          {model.aspectRatios.length > 0 && (
            <ParamSelect
              label={t('比例')}
              value={form.aspectRatio}
              options={model.aspectRatios}
              onChange={(aspectRatio) => onChange({ aspectRatio })}
            />
          )}
        </div>
      )}
    </div>
  );
};

export default ModelPanel;
