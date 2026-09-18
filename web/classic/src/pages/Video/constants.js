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
 * 视频生成工作台的静态配置与占位数据。
 *
 * WORKBENCH_MODELS 与 MOCK_WORKBENCH_TASKS 仅用于当前骨架评审阶段：
 * 模型目录后续改为由管理员在后台配置（选择展示哪些模型 + 分组 + 计费方式），
 * 任务列表后续接入 /api/task/self。两者都从本文件导出，替换时只需改这一处。
 */

// ============================================================================
// 历史任务分页
// ============================================================================

/** 每页 12 条（4 列 × 3 行），分页由后端 /api/task/self 完成 */
export const HISTORY_PAGE_SIZE = 12;

// ============================================================================
// 任务状态展示
// ============================================================================

/** 终态：不会再变化，轮询可以停止 */
export const TERMINAL_STATUSES = ['SUCCESS', 'FAILURE'];

export const TASK_STATUS_PRESENTATION = {
  NOT_START: { labelKey: '等待中', tagColor: 'grey' },
  SUBMITTED: { labelKey: '已提交', tagColor: 'blue' },
  QUEUED: { labelKey: '排队中', tagColor: 'amber' },
  IN_PROGRESS: { labelKey: '生成中', tagColor: 'blue' },
  SUCCESS: { labelKey: '已完成', tagColor: 'green' },
  FAILURE: { labelKey: '失败', tagColor: 'red' },
};

export const STATUS_FILTER_OPTIONS = [
  { labelKey: '全部状态', value: 'all' },
  { labelKey: '已完成', value: 'SUCCESS' },
  { labelKey: '生成中', value: 'IN_PROGRESS' },
  { labelKey: '排队中', value: 'QUEUED' },
  { labelKey: '失败', value: 'FAILURE' },
];

// ============================================================================
// 产出类型筛选
// ============================================================================

export const MEDIA_FILTER_OPTIONS = [
  { labelKey: '全部产出', value: 'all' },
  { labelKey: '图片', value: 'image' },
  { labelKey: '视频', value: 'video' },
];

export const MEDIA_KIND_LABELS = {
  video: '视频',
  image: '图片',
  audio: '音频',
};

// ============================================================================
// 模型分组与计费方式
// ============================================================================

/** 模型下拉的分组顺序 */
export const MODEL_GROUPS = [
  { key: 'video', labelKey: '视频' },
  { key: 'image', labelKey: '图片' },
];

/**
 * 计费方式。
 *
 * per_second：按秒计费，预计消耗 = 每秒单价 × 时长
 * per_call：按次计费，预计消耗 = 单次价格
 */
export const BILLING_MODE = {
  PER_SECOND: 'per_second',
  PER_CALL: 'per_call',
};

export const BILLING_MODE_LABELS = {
  [BILLING_MODE.PER_SECOND]: '按秒计费',
  [BILLING_MODE.PER_CALL]: '按次计费',
};

// ============================================================================
// 新增模型时的默认参数（后台配置页预填用）
// ============================================================================

/** 时长模式：固定候选值 / 连续区间 */
export const DURATION_MODE = {
  LIST: 'list',
  RANGE: 'range',
};

/**
 * 按分组预填的默认参数。
 *
 * 这些只是"新增模型时的初始值"，最终以管理员在「工作台配置」页保存的为准。
 * 约定：空数组 = 前端不渲染该项（所以图片模型默认没有时长与分辨率），
 * 参考素材则用 enabled 开关控制。
 */
export const GROUP_DEFAULTS = {
  video: {
    duration: { mode: DURATION_MODE.LIST, values: [5, 10] },
    resolutions: ['480p', '720p', '1080p'],
    aspectRatios: ['16:9', '9:16', '1:1'],
    references: {
      image: { enabled: true, max: 4 },
      audio: { enabled: false, max: 0 },
      video: { enabled: false, max: 0 },
    },
    promptOptimize: { enabled: false, model: '', systemPrompt: '' },
  },
  image: {
    duration: { mode: '', values: [] },
    resolutions: [],
    aspectRatios: ['1:1', '9:16', '16:9'],
    references: {
      image: { enabled: true, max: 4 },
      audio: { enabled: false, max: 0 },
      video: { enabled: false, max: 0 },
    },
    promptOptimize: { enabled: false, model: '', systemPrompt: '' },
  },
};

/**
 * 参考素材的三种类型。顺序即前端渲染顺序；
 * 某一类是否出现完全由模型配置里的 references 决定。
 */
export const REFERENCE_KINDS = [
  {
    key: 'image',
    labelKey: '参考图片',
    hintKey: '支持常见图片格式，单个文件不超过 50MB。',
    chooseLabelKey: '选择图片',
    accept: 'image/*',
    maxBytes: 50 * 1024 * 1024,
  },
  {
    key: 'audio',
    labelKey: '参考音频',
    hintKey: '支持常见音频格式，单个文件不超过 50MB。',
    chooseLabelKey: '选择音频',
    accept: 'audio/*',
    maxBytes: 50 * 1024 * 1024,
  },
  {
    key: 'video',
    labelKey: '参考视频',
    hintKey: '支持常见视频格式，单个文件不超过 50MB。',
    chooseLabelKey: '选择视频',
    accept: 'video/*',
    maxBytes: 50 * 1024 * 1024,
  },
];
