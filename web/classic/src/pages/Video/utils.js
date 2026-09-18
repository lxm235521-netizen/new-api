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

import { BILLING_MODE, DURATION_MODE, TERMINAL_STATUSES } from './constants';

/** 把秒数格式化为 mm:ss */
export function formatClipDuration(seconds) {
  if (!seconds || seconds <= 0) return '-';
  const minutes = Math.floor(seconds / 60);
  const rest = seconds % 60;
  return `${String(minutes).padStart(2, '0')}:${String(rest).padStart(2, '0')}`;
}

export function formatFileSize(bytes) {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

export function isTerminalStatus(status) {
  return TERMINAL_STATUSES.includes(status);
}

/**
 * 历史任务筛选（页面内）。
 *
 * 状态筛选已经交给后端（/api/task/self?status=...），这样分页 total 才准确；
 * 这里只处理后端不支持的「产出类型」和「仅看当前模型」。
 */
export function filterTasks(tasks, { mediaFilter, model }) {
  return tasks.filter((task) => {
    if (mediaFilter !== 'all' && task.kind !== mediaFilter) return false;
    if (model && task.model !== model) return false;
    return true;
  });
}

/**
 * 切换模型时该用的初始参数。
 *
 * 时长：固定值取第一个；区间取最小值。
 * 分辨率/比例：可能为空（该项不适用），此时留空并由 UI 隐藏。
 */
export function buildInitialParams(model) {
  if (!model) {
    return { duration: 0, resolution: '', aspectRatio: '' };
  }

  let duration = 0;
  if (model.duration.mode === DURATION_MODE.RANGE) {
    duration = model.duration.min;
  } else if (model.duration.mode === DURATION_MODE.LIST) {
    duration = model.duration.values[0] ?? 0;
  }

  return {
    duration,
    resolution: model.resolutions[0] ?? '',
    aspectRatio: model.aspectRatios[0] ?? '',
  };
}

/** 模型是否支持某类参考素材 */
export function isReferenceEnabled(model, kind) {
  return Boolean(model?.references?.[kind]?.enabled);
}

export function referenceMax(model, kind) {
  return model?.references?.[kind]?.max ?? 0;
}

/** 模型是否配置了时长项（图片模型没有，视频模型才有） */
export function hasDurationCapability(model) {
  if (!model?.duration) return false;
  const { mode, values, max } = model.duration;
  if (mode === DURATION_MODE.RANGE) return (max || 0) > 0;
  if (mode === DURATION_MODE.LIST) return (values || []).length > 0;
  return false;
}

/**
 * 实际生效的计费方式。
 *
 * 配置里可能给图片模型留了「按秒计费」，但图片模型根本没有时长项，
 * 乘出来永远是 0 —— 而后端是按 model_price（每次价格）扣的。
 * 所以：没有时长项就按「按次」处理，避免预估显示成 0。
 */
export function effectiveBilling(model) {
  if (!model) return BILLING_MODE.PER_CALL;
  if (!hasDurationCapability(model)) return BILLING_MODE.PER_CALL;
  return model.billing === BILLING_MODE.PER_CALL
    ? BILLING_MODE.PER_CALL
    : BILLING_MODE.PER_SECOND;
}

/**
 * 提交前的预估消耗。
 *
 * unitQuota 由后端按「model_price × QuotaPerUnit × 分组倍率」解析，
 * 与 relay 预扣费同一套公式，因此：
 *   按秒 -> unitQuota × 时长
 *   按次 -> unitQuota
 *
 * 已知偏差：部分供应商的适配器还会在提交时叠加尺寸/折扣倍率
 * （例如 Sora 的尺寸倍率），这部分无法在提交前得知，预估可能略低于实扣。
 */
export function estimateTaskQuota(model, form) {
  if (!model) return 0;
  const unit = model.unitQuota || 0;
  if (effectiveBilling(model) === BILLING_MODE.PER_CALL) return unit;
  return Math.round(unit * (form.duration || 0));
}

export function isSubmitDisabled(model, form) {
  if (!model) return true;
  if (!form.prompt || form.prompt.trim() === '') return true;

  return Object.keys(form.references || {}).some((kind) => {
    if (!isReferenceEnabled(model, kind)) return false;
    return (form.references[kind]?.length ?? 0) > referenceMax(model, kind);
  });
}

// ============================================================================
// 提交与状态映射（对接 /v1/videos）
// ============================================================================

/**
 * 组装提交体。
 *
 * 网关约定：
 *   { model, prompt, seconds(字符串), resolution, aspect_ratio,
 *     images: [url...], audios: [url...] }
 * 参考项按模型能力决定传哪几个；没配的项一律不传，避免上游收到空数组。
 *
 * 说明：文档只列了 images / audios。视频参考按同样的命名推断为 videos，
 * 若上游字段名不同，改这一处即可。
 */
export function buildSubmitPayload(model, form) {
  const payload = {
    model: model.value,
    prompt: form.prompt,
  };

  if (form.duration > 0) {
    payload.seconds = String(form.duration);
  }
  if (form.resolution) {
    payload.resolution = form.resolution;
  }
  if (form.aspectRatio) {
    payload.aspect_ratio = form.aspectRatio;
  }

  const references = form.references || {};
  const urlList = (kind) =>
    (references[kind] || []).map((asset) => asset.url).filter(Boolean);

  const fields = { image: 'images', audio: 'audios', video: 'videos' };
  Object.entries(fields).forEach(([kind, field]) => {
    if (!isReferenceEnabled(model, kind)) return;
    const urls = urlList(kind);
    if (urls.length > 0) payload[field] = urls;
  });

  return payload;
}

/**
 * 组装图片生成的提交体。
 *
 * 图片上游是同步接口 `POST /v1/images/generations`：
 *   { model, prompt, quality, n, size, response_format, images? }
 *
 * 其中 quality=high / n=1 / response_format=url 是固定参数，由后端注入；
 * 前端只负责：
 *   - async: true —— 让 new-api 立刻回 task_id，后台再调上游（否则请求要挂几十秒）
 *   - size —— 取「比例」（图片模型没有分辨率这一项）
 *   - images —— 图生图的参考图 URL，文生图不传这个字段
 */
export function buildImagePayload(model, form) {
  const payload = {
    model: model.value,
    prompt: form.prompt,
    async: true,
  };

  if (form.aspectRatio) {
    payload.size = form.aspectRatio;
  }

  const images = (form.references?.image || [])
    .map((asset) => asset.url)
    .filter(Boolean);
  if (images.length > 0) {
    payload.images = images;
  }

  return payload;
}

/** 网关状态 -> 工作台状态 */
export function mapGatewayStatus(status) {
  switch (String(status || '').toLowerCase()) {
    case 'queued':
    case 'pending':
      return 'QUEUED';
    case 'processing':
    case 'running':
    case 'in_progress':
      return 'IN_PROGRESS';
    case 'completed':
    case 'succeeded':
    case 'success':
      return 'SUCCESS';
    case 'failed':
    case 'error':
    case 'cancelled':
      return 'FAILURE';
    default:
      return 'SUBMITTED';
  }
}

/** 把 "30%" / 30 / "30" 统一成 0-100 的整数 */
export function parseProgress(progress) {
  if (typeof progress === 'number' && Number.isFinite(progress)) {
    return Math.max(0, Math.min(100, Math.round(progress)));
  }
  const parsed = parseInt(String(progress || '').replace('%', ''), 10);
  return Number.isFinite(parsed) ? Math.max(0, Math.min(100, parsed)) : 0;
}

/**
 * 把后端历史任务记录（dto.TaskDto）转成卡片 / 详情需要的形状。
 *
 * resultUrl 直接用后端给的地址（完成时是 video_url / 代理地址），
 * 前端只负责播放，不把视频下载到服务器。
 *
 * 提交参数来自 properties.request（老任务没有该字段，全部按空值处理）。
 */
export function mapTaskDtoToTask(dto, models = []) {
  const properties = dto.properties || {};
  const request = properties.request || {};
  const modelName = properties.origin_model_name || '';
  const matched = models.find((item) => item.value === modelName);

  const asList = (value) => (Array.isArray(value) ? value : []);
  const images = asList(request.images);
  const audios = asList(request.audios);
  const videos = asList(request.videos);

  const submitTime = (dto.submit_time || dto.created_at || 0) * 1000;
  const finishTime = (dto.finish_time || 0) * 1000;
  // 耗时：已完成用 完成-提交，进行中的实时算（列表每 6 秒刷新一次）
  const elapsedSeconds = submitTime
    ? Math.max(0, Math.round(((finishTime || Date.now()) - submitTime) / 1000))
    : 0;

  const isImage = dto.platform === 'image' || matched?.group === 'image';

  return {
    id: dto.id || 0,
    taskId: dto.task_id,
    // 产出类型：图片任务自报 platform=image；否则看模型在工作台配置里的分组
    kind: isImage ? 'image' : 'video',
    status: dto.status || 'SUBMITTED',
    prompt: properties.input || '',
    model: modelName,
    upstreamModel: properties.upstream_model_name || '',
    // 有参考图时拿第一张当封面（视频首帧由 <video preload="metadata"> 自己出）
    thumbnailUrl: images[0] || '',
    resultUrl: dto.result_url || '',
    durationSeconds: request.duration || 0,
    // 图片模型的「比例」是走 size 字段提交的（16:9 这种），视频模型才用 aspect_ratio
    aspectRatio: request.aspect_ratio || (isImage ? request.size || '' : ''),
    resolution: isImage ? '' : request.resolution || request.size || '',
    mode: request.mode || '',
    // 提交参数快照是否存在：老任务没有，模式/参数一律不猜
    requestKnown: Boolean(properties.request),
    references: { image: images, audio: audios, video: videos },
    createdAt: submitTime,
    finishTime,
    elapsedSeconds,
    referenceImages: images.length,
    referenceAudios: audios.length,
    referenceVideos: videos.length,
    quota: dto.quota || 0,
    progress: parseProgress(dto.progress),
    failReason: dto.status === 'FAILURE' ? dto.fail_reason || '' : '',
  };
}

/**
 * 任务模式的显示文案。
 *
 * 后端只透传客户端给的 mode（工作台默认不传），因此按参考素材兜底推断：
 * 有图 = 图生视频，有音/视频 = 参考生视频，都没有 = 文生视频。
 */
export function describeTaskMode(task) {
  const raw = String(task?.mode || '').trim();
  if (raw) return raw;
  if ((task?.referenceAudios || 0) > 0 || (task?.referenceVideos || 0) > 0) {
    return 'reference';
  }
  if ((task?.referenceImages || 0) > 0) return 'image2video';
  return 'text2video';
}

/** 参考素材总数（详情页展示用） */
export function countReferences(task) {
  return (
    (task?.referenceImages ?? 0) +
    (task?.referenceAudios ?? 0) +
    (task?.referenceVideos ?? 0)
  );
}

/**
 * 卡片/详情展示用的扣费金额。
 *
 * 任务失败时预扣费已经退回（轮询的失败分支会 RefundTaskQuota），但 tasks.quota
 * 列里留的还是当初的预扣金额，直接显示会让人以为白花了钱 —— 失败一律按 0 展示。
 */
export function taskCostQuota(task) {
  if (!task || task.status === 'FAILURE') return 0;
  return task.quota || 0;
}

/**
 * 播放/显示/下载用的地址。
 *
 * 后端把完成的任务统一代理到自己身上：视频是 `/v1/videos/{task_id}/content`，
 * 图片是 `/v1/images/tasks/{task_id}/content`；库里存的是带域名的绝对地址。
 * 而这两个接口都要登录态 —— 面板如果不在那个域名下（比如临时用 IP:端口 访问），
 * 浏览器不会带 cookie 就是 401；集群里还有别的节点在跑旧代码时，那个域名上
 * 甚至没有图片路由（直接 404、图裂）。
 *
 * 所以：只要发现是本站的代理地址，就改写成**同源相对路径**，一切跟着当前访问地址走。
 */
export function resolvePlaybackUrl(task) {
  const url = task?.resultUrl || '';
  if (!url) return '';

  const matched = url.match(
    /\/v1\/(videos|images\/tasks)\/([^/]+)\/content(?:\?.*)?$/,
  );
  if (matched) {
    const taskId = task?.taskId || matched[2];
    return `/v1/${matched[1]}/${encodeURIComponent(taskId)}/content`;
  }

  return url;
}

/**
 * 卡片预览用的缩略图地址。
 *
 * 生成出来的原图有 2~3MB，而卡片上只显示一两百像素宽 —— 直接回原图，首屏要白等
 * 好几秒。图片任务因此带 ?w=480（服务端解码缩放 + 内存缓存）；视频任务没有缩略图
 * 接口，保持原样。详情大图、下载仍用 resolvePlaybackUrl 取原图。
 */
export function resolveThumbUrl(task) {
  const url = resolvePlaybackUrl(task);
  if (!url || task?.kind !== 'image') return url;
  return `${url}${url.includes('?') ? '&' : '?'}w=480`;
}

/** 时间戳（毫秒）-> 本地时间文案 */
export function formatDateTime(timestamp) {
  if (!timestamp) return '-';
  const date = new Date(timestamp);
  if (Number.isNaN(date.getTime())) return '-';
  const pad = (value) => String(value).padStart(2, '0');
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(
    date.getDate(),
  )} ${pad(date.getHours())}:${pad(date.getMinutes())}`;
}

/** 生成耗时文案：68 -> "1m08s"、8 -> "8s" */
export function formatElapsed(seconds) {
  const total = Math.max(0, Math.round(seconds || 0));
  if (total < 60) return `${total}s`;
  const minutes = Math.floor(total / 60);
  return `${minutes}m${String(total % 60).padStart(2, '0')}s`;
}

/** 常见比例表：像素尺寸不可能刚好是 16:9，按容差归一到人类习惯的写法 */
const COMMON_RATIOS = [
  [16, 9],
  [9, 16],
  [1, 1],
  [4, 3],
  [3, 4],
  [3, 2],
  [2, 3],
  [21, 9],
  [9, 21],
];

/**
 * 由视频真实分辨率推算比例（864x480 -> 16:9）。
 * 认不出来时才退化成 "864:480" 这种原始写法。
 */
export function simplifyRatio(width, height) {
  if (!width || !height) return '';
  const value = width / height;
  let best = null;
  for (const [a, b] of COMMON_RATIOS) {
    const diff = Math.abs(value - a / b);
    if (!best || diff < best.diff) best = { label: `${a}:${b}`, diff };
  }
  if (best && best.diff <= 0.06) return best.label;
  return `${Math.round(width)}:${Math.round(height)}`;
}
