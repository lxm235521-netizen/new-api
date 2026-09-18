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
 * 把后端返回的模型目录整理成 UI 直接可用的结构。
 *
 * 后端已经做了规范化（补默认、去重、收敛非法值），这里只负责：
 *   - 统一字段名
 *   - 保证每个数组/对象都存在，避免渲染时报错
 *   - 过滤掉单价解析失败的条目（unit_quota 为 0 且没有价格配置）
 */

import { BILLING_MODE, DURATION_MODE } from './constants';

function normalizeDuration(duration) {
  if (!duration) return { mode: '', values: [], min: 0, max: 0, step: 1 };
  if (duration.mode === DURATION_MODE.RANGE) {
    return {
      mode: DURATION_MODE.RANGE,
      values: [],
      min: Number(duration.min) || 0,
      max: Number(duration.max) || 0,
      step: Number(duration.step) || 1,
    };
  }
  const values = Array.isArray(duration.values)
    ? duration.values.map(Number).filter((n) => Number.isFinite(n))
    : [];
  return {
    mode: values.length > 0 ? DURATION_MODE.LIST : '',
    values,
    min: 0,
    max: 0,
    step: 1,
  };
}

function normalizeReference(reference) {
  if (!reference || !reference.enabled) return { enabled: false, max: 0 };
  return { enabled: true, max: Math.max(Number(reference.max) || 0, 1) };
}

export function buildWorkbenchModels(remoteModels) {
  if (!Array.isArray(remoteModels)) return [];

  return remoteModels.map((remote) => {
    const group = remote.group === 'image' ? 'image' : 'video';
    const references = remote.references || {};

    return {
      value: remote.model,
      label: remote.model,
      group,
      billing:
        remote.billing === BILLING_MODE.PER_SECOND
          ? BILLING_MODE.PER_SECOND
          : BILLING_MODE.PER_CALL,
      // unitQuota：按秒为每秒额度，按次为每次额度（后端按当前用户解析）
      unitQuota: Number(remote.unit_quota) || 0,
      duration: normalizeDuration(remote.duration),
      resolutions: Array.isArray(remote.resolutions) ? remote.resolutions : [],
      aspectRatios: Array.isArray(remote.aspect_ratios)
        ? remote.aspect_ratios
        : [],
      references: {
        image: normalizeReference(references.image),
        audio: normalizeReference(references.audio),
        video: normalizeReference(references.video),
      },
      promptOptimize: {
        enabled: Boolean(remote.prompt_optimize?.enabled),
        model: remote.prompt_optimize?.model || '',
        systemPrompt: remote.prompt_optimize?.system_prompt || '',
      },
      usePrice: remote.use_price,
      modelPrice: remote.model_price,
      modelRatio: remote.model_ratio,
    };
  });
}

/** 该模型是否配置了任何可调参数（用于决定右栏是否显示参数区） */
export function hasAnyParamControl(model) {
  if (!model) return false;
  const hasDuration =
    model.duration.mode === DURATION_MODE.RANGE ||
    model.duration.values.length > 0;
  const hasResolution = model.resolutions.length > 0;
  const hasAspectRatio = model.aspectRatios.length > 0;
  return hasDuration || hasResolution || hasAspectRatio;
}
