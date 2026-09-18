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
import {
  Button,
  Input,
  InputNumber,
  Select,
  SideSheet,
  Switch,
  TextArea,
  Typography,
} from '@douyinfe/semi-ui';
import { DURATION_MODE } from '../../Video/constants';

const { Text, Title } = Typography;

const Row = ({ label, hint, children }) => (
  <div className='wbcfg-row'>
    <div className='wbcfg-row__label'>
      <Text size='small'>{label}</Text>
      {hint && (
        <Text size='small' type='tertiary' className='wbcfg-row__hint'>
          {hint}
        </Text>
      )}
    </div>
    <div className='wbcfg-row__control'>{children}</div>
  </div>
);

/** 参考素材一行：开关 + 数量上限 */
const ReferenceRow = ({ label, enabled, max, onToggle, onMax }) => {
  const { t } = useTranslation();
  return (
    <div className='wbcfg-ref'>
      <Switch size='small' checked={enabled} onChange={onToggle} />
      <span className='wbcfg-ref__name'>{label}</span>
      {enabled ? (
        <InputNumber
          size='small'
          min={1}
          max={99}
          value={max}
          onChange={(value) => onMax(Number(value) || 1)}
          style={{ width: 90 }}
          prefix={t('最多')}
        />
      ) : (
        <Text size='small' type='tertiary'>
          {t('不启用')}
        </Text>
      )}
    </div>
  );
};

/**
 * 单模型配置抽屉。
 *
 * 时长是最需要区分的一块：有些模型只有固定几档（5/10 秒），
 * 有些是连续区间（1~30 秒），因此用「模式」先分流再渲染不同控件。
 */
const ModelEditor = ({
  visible,
  draft,
  modelOptions,
  onChange,
  onCancel,
  onSave,
}) => {
  const { t } = useTranslation();

  if (!draft) return null;

  const patch = (key, value) => onChange({ ...draft, [key]: value });

  const durationModeOptions = [
    { label: t('不需要时长'), value: '' },
    { label: t('固定候选值'), value: DURATION_MODE.LIST },
    { label: t('连续区间'), value: DURATION_MODE.RANGE },
  ];

  // 没有时长项的模型（图片类）只能是按次计费，见下面计费方式那一项
  const durationDisabled = draft.durationMode === '';

  return (
    <SideSheet
      placement='right'
      width={560}
      visible={visible}
      onCancel={onCancel}
      title={
        <Title heading={5} style={{ margin: 0 }}>
          {draft.model || t('新增模型')}
        </Title>
      }
      footer={
        <div className='wbcfg-footer'>
          <Button theme='light' onClick={onCancel}>
            {t('取消')}
          </Button>
          <Button theme='solid' type='primary' onClick={onSave}>
            {t('确定')}
          </Button>
        </div>
      }
    >
      <div className='wbcfg-sections'>
        {/* 基础 */}
        <div className='wbcfg-group'>
          <div className='wbcfg-group__title'>{t('基础')}</div>
          <Row label={t('分组')}>
            <Select
              value={draft.group}
              onChange={(value) => patch('group', value)}
              optionList={[
                { label: t('视频'), value: 'video' },
                { label: t('图片'), value: 'image' },
              ]}
              style={{ width: '100%' }}
            />
          </Row>
          <Row
            label={t('计费方式')}
            hint={t('按秒：预计消耗 = 单价×时长；按次：单次价格')}
          >
            <Select
              // 没有时长项的模型（图片类）只能是按次：按秒会乘出 0，
              // 而后端本来就是按 model_price 每次扣费
              value={durationDisabled ? 'per_call' : draft.billing}
              onChange={(value) => patch('billing', value)}
              disabled={durationDisabled}
              optionList={[
                { label: t('按秒计费'), value: 'per_second' },
                { label: t('按次计费'), value: 'per_call' },
              ]}
              style={{ width: '100%' }}
            />
          </Row>
        </div>

        {/* 参数 */}
        <div className='wbcfg-group'>
          <div className='wbcfg-group__title'>{t('生成参数')}</div>

          <Row label={t('时长模式')}>
            <Select
              value={draft.durationMode}
              onChange={(value) => patch('durationMode', value)}
              optionList={durationModeOptions}
              style={{ width: '100%' }}
            />
          </Row>

          {draft.durationMode === DURATION_MODE.LIST && (
            <Row label={t('候选时长')} hint={t('逗号分隔，例：5, 10')}>
              <Input
                value={draft.durationValues}
                onChange={(value) => patch('durationValues', value)}
                placeholder='5, 10'
              />
            </Row>
          )}

          {draft.durationMode === DURATION_MODE.RANGE && (
            <Row label={t('区间与步长')} hint={t('秒')}>
              <div className='wbcfg-range'>
                <InputNumber
                  min={0}
                  value={draft.durationMin}
                  onChange={(value) => patch('durationMin', Number(value) || 0)}
                  prefix={t('最小')}
                />
                <InputNumber
                  min={0}
                  value={draft.durationMax}
                  onChange={(value) => patch('durationMax', Number(value) || 0)}
                  prefix={t('最大')}
                />
                <InputNumber
                  min={1}
                  value={draft.durationStep}
                  onChange={(value) =>
                    patch('durationStep', Number(value) || 1)
                  }
                  prefix={t('步长')}
                />
              </div>
            </Row>
          )}

          <Row label={t('分辨率')} hint={t('逗号分隔；留空则不显示该项')}>
            <Input
              value={draft.resolutions}
              onChange={(value) => patch('resolutions', value)}
              placeholder='480p, 720p, 1080p'
            />
          </Row>

          <Row label={t('比例')} hint={t('逗号分隔，例：16:9, 9:16')}>
            <Input
              value={draft.aspectRatios}
              onChange={(value) => patch('aspectRatios', value)}
              placeholder='16:9, 9:16, 1:1'
            />
          </Row>
        </div>

        {/* 参考素材 */}
        <div className='wbcfg-group'>
          <div className='wbcfg-group__title'>{t('参考素材')}</div>
          <ReferenceRow
            label={t('参考图片')}
            enabled={draft.imageEnabled}
            max={draft.imageMax}
            onToggle={(value) => patch('imageEnabled', value)}
            onMax={(value) => patch('imageMax', value)}
          />
          <ReferenceRow
            label={t('参考音频')}
            enabled={draft.audioEnabled}
            max={draft.audioMax}
            onToggle={(value) => patch('audioEnabled', value)}
            onMax={(value) => patch('audioMax', value)}
          />
          <ReferenceRow
            label={t('参考视频')}
            enabled={draft.videoEnabled}
            max={draft.videoMax}
            onToggle={(value) => patch('videoEnabled', value)}
            onMax={(value) => patch('videoMax', value)}
          />
          <Text size='small' type='tertiary'>
            {t('参考视频能否生效取决于上游适配器是否支持；不支持时请勿开启。')}
          </Text>
        </div>

        {/* 提示词优化 */}
        <div className='wbcfg-group'>
          <div className='wbcfg-group__title'>{t('提示词优化')}</div>
          <Row label={t('启用')}>
            <Switch
              checked={draft.optimizeEnabled}
              onChange={(value) => patch('optimizeEnabled', value)}
            />
          </Row>
          {draft.optimizeEnabled && (
            <>
              <Row
                label={t('优化用的推理模型')}
                hint={t('可直接输入列表里还没有的模型名')}
              >
                <Select
                  value={draft.optimizeModel || undefined}
                  onChange={(value) => patch('optimizeModel', value)}
                  optionList={modelOptions}
                  filter
                  allowCreate
                  placeholder={t('选择或输入用于改写提示词的模型')}
                  style={{ width: '100%' }}
                />
              </Row>
              <Row
                label={t('自定义优化指令')}
                hint={t('留空则使用内置默认指令')}
              >
                <TextArea
                  value={draft.optimizePrompt}
                  onChange={(value) => patch('optimizePrompt', value)}
                  autosize={{ minRows: 3, maxRows: 8 }}
                  placeholder={t('例如：只输出中文提示词，保留原始台词')}
                />
              </Row>
            </>
          )}
        </div>
      </div>
    </SideSheet>
  );
};

export default ModelEditor;
