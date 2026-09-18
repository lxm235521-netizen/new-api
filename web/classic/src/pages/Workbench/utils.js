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
 * 工作台配置的读写辅助。
 *
 * 列表型字段（时长候选、分辨率、比例）在界面上用逗号分隔的文本编辑，
 * 这里统一负责文本 <-> 数组的转换，避免各组件各写一套解析。
 */

import { DURATION_MODE, GROUP_DEFAULTS } from '../Video/constants';

export const WORKBENCH_OPTION_KEY = 'workbench_setting.models';
// 异步图片生成时「单个用户同时可跑几张」（0 = 不限）
export const IMAGE_CONCURRENCY_OPTION_KEY =
  'workbench_setting.async_image_per_user';
export const PER_CALL_OPTION_KEY = 'task_per_call_billing_setting.model_names';

/** 图片类端点：命中则认为该模型属于「图片」分组 */
export const IMAGE_ENDPOINT = 'image-generation';

export function parseList(value) {
  if (!value) return [];
  return String(value)
    .split(/[,，\s]+/)
    .map((item) => item.trim())
    .filter(Boolean);
}

export function parseNumberList(value) {
  return parseList(value)
    .map((item) => Number(item))
    .filter((n) => Number.isFinite(n) && n >= 0);
}

export function joinList(list) {
  return Array.isArray(list) ? list.join(', ') : '';
}

function parseIntOr(value, fallback) {
  const n = Number(value);
  return Number.isFinite(n) ? n : fallback;
}

/** 从后端配置构造编辑草稿（补齐缺失字段，便于表单绑定） */
export function toDraft(item) {
  const duration = item?.duration || {};
  const mode =
    duration.mode === DURATION_MODE.RANGE
      ? DURATION_MODE.RANGE
      : Array.isArray(duration.values) && duration.values.length > 0
        ? DURATION_MODE.LIST
        : '';

  const references = item?.references || {};
  const optimize = item?.prompt_optimize || {};

  return {
    model: item?.model || '',
    group: item?.group === 'image' ? 'image' : 'video',
    billing: item?.billing === 'per_second' ? 'per_second' : 'per_call',
    durationMode: mode,
    durationValues: joinList(duration.values),
    durationMin: parseIntOr(duration.min, 1),
    durationMax: parseIntOr(duration.max, 30),
    durationStep: parseIntOr(duration.step, 1) || 1,
    resolutions: joinList(item?.resolutions),
    aspectRatios: joinList(item?.aspect_ratios),
    imageEnabled: Boolean(references.image?.enabled),
    imageMax: parseIntOr(references.image?.max, 0),
    audioEnabled: Boolean(references.audio?.enabled),
    audioMax: parseIntOr(references.audio?.max, 0),
    videoEnabled: Boolean(references.video?.enabled),
    videoMax: parseIntOr(references.video?.max, 0),
    optimizeEnabled: Boolean(optimize.enabled),
    optimizeModel: optimize.model || '',
    optimizePrompt: optimize.system_prompt || '',
  };
}

/** 编辑草稿 -> 后端配置结构 */
export function fromDraft(draft) {
  const duration =
    draft.durationMode === DURATION_MODE.RANGE
      ? {
          mode: DURATION_MODE.RANGE,
          min: draft.durationMin,
          max: draft.durationMax,
          step: draft.durationStep,
          values: [],
        }
      : draft.durationMode === DURATION_MODE.LIST
        ? {
            mode: DURATION_MODE.LIST,
            values: parseNumberList(draft.durationValues),
            min: 0,
            max: 0,
            step: 1,
          }
        : { mode: '', values: [], min: 0, max: 0, step: 1 };

  return {
    model: draft.model.trim(),
    group: draft.group,
    // 没有时长项时只能是按次：按秒会乘出 0，与实际扣费（model_price 每次）不符
    billing: duration.mode === '' ? 'per_call' : draft.billing,
    duration,
    resolutions: parseList(draft.resolutions),
    aspect_ratios: parseList(draft.aspectRatios),
    references: {
      image: { enabled: draft.imageEnabled, max: draft.imageMax },
      audio: { enabled: draft.audioEnabled, max: draft.audioMax },
      video: { enabled: draft.videoEnabled, max: draft.videoMax },
    },
    prompt_optimize: {
      enabled: draft.optimizeEnabled,
      model: draft.optimizeModel,
      system_prompt: draft.optimizePrompt,
    },
  };
}

/** 新增模型时的默认草稿：按分组预填，减少手工填写 */
export function newDraft(modelName, group = 'video') {
  const defaults = GROUP_DEFAULTS[group] || GROUP_DEFAULTS.video;
  return toDraft({
    model: modelName,
    group,
    billing: 'per_call',
    duration: defaults.duration,
    resolutions: defaults.resolutions,
    aspect_ratios: defaults.aspectRatios,
    references: defaults.references,
    prompt_optimize: defaults.promptOptimize,
  });
}

/** 列表里展示用的简短摘要 */
export function describeDuration(draftOrItem) {
  const duration = draftOrItem.duration || {
    mode: draftOrItem.durationMode,
    values: parseNumberList(draftOrItem.durationValues),
    min: draftOrItem.durationMin,
    max: draftOrItem.durationMax,
    step: draftOrItem.durationStep,
  };
  if (duration.mode === DURATION_MODE.RANGE) {
    return `${duration.min}~${duration.max}s`;
  }
  if (duration.mode === DURATION_MODE.LIST && duration.values?.length) {
    return duration.values.map((v) => `${v}s`).join('/');
  }
  return '—';
}
