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
 * 工作台与后端的接口封装。
 *
 * 分两类：
 *   1. 控制台接口（session 鉴权）—— 模型目录、历史任务
 *   2. 生成接口（用户自己的令牌鉴权）—— /v1/videos 提交与查询
 *
 * 生成走标准 relay 路径，用工作台里选中的那把密钥，
 * 因此计费与权限都落在用户自己的令牌上。
 */

import { API } from '../../helpers';

/**
 * 拉取工作台模型目录。
 *
 * 返回的每一项已由后端按当前用户解析出 unit_quota（按秒：每秒额度；
 * 按次：每次额度），与 relay 预扣费用同一套公式。
 */
export async function getWorkbenchModels() {
  const res = await API.get('/api/workbench/models');
  const { success, message, data } = res.data;
  if (!success) {
    throw new Error(message || '加载模型列表失败');
  }
  return Array.isArray(data?.models) ? data.models : [];
}

/** 生成接口统一用用户的令牌鉴权；库里存的 key 不带 sk- 前缀 */
function authHeaders(tokenKey) {
  return {
    'Content-Type': 'application/json',
    Authorization: `Bearer sk-${tokenKey}`,
  };
}

/** 从 relay 的错误响应里取出可读信息（relay 可能返回 {error:{message}} 或 {message}） */
async function extractError(res) {
  try {
    const body = await res.json();
    return (
      body?.error?.message ||
      body?.message ||
      body?.error ||
      `请求失败（HTTP ${res.status}）`
    );
  } catch {
    return `请求失败（HTTP ${res.status}）`;
  }
}

/**
 * 提交视频生成任务：POST /v1/videos
 *
 * 请求体按网关约定：
 *   { model, prompt, seconds(字符串), resolution, aspect_ratio,
 *     images: [url...], audios: [url...], videos: [url...] }
 * 参考项按模型能力决定传哪几个（没有就不传）。
 */
export async function submitVideoTask(tokenKey, payload) {
  const res = await fetch('/v1/videos', {
    method: 'POST',
    headers: authHeaders(tokenKey),
    body: JSON.stringify(payload),
  });

  if (!res.ok) {
    throw new Error(await extractError(res));
  }

  const data = await res.json();
  return {
    id: data.id || data.task_id,
    taskId: data.task_id || data.id,
    status: data.status,
    progress: Number(data.progress) || 0,
    model: data.model,
    createdAt: data.created_at,
  };
}

/**
 * 提交图片生成任务（异步）：POST /v1/images/generations
 *
 * 请求体里带 `async: true`，new-api 会立刻回一个 task_id，后台再调上游的同步接口；
 * 图片上游一次调用要几十秒，同步返回会把连接一直挂着（浏览器/nginx/CF 都会超时）。
 * 计费、历史记录、失败退款都沿用任务那套。
 */
export async function submitImageTask(tokenKey, payload) {
  const res = await fetch('/v1/images/generations', {
    method: 'POST',
    headers: authHeaders(tokenKey),
    body: JSON.stringify(payload),
  });

  if (!res.ok) {
    throw new Error(await extractError(res));
  }

  const data = await res.json();
  return {
    id: data.id || data.task_id,
    taskId: data.task_id || data.id,
    status: data.status || 'QUEUED',
    progress: Number(data.progress) || 0,
    model: data.model,
    createdAt: data.created_at,
  };
}

/**
 * 查询异步图片任务：GET /v1/images/tasks/{task_id}
 *
 * 图片上游是同步接口（没有可轮询的上游 id），进度只在自己库里，
 * 所以用图片自己的查询接口，而不是 /v1/videos/{id}。
 */
export async function fetchImageTask(tokenKey, taskId) {
  const res = await fetch(`/v1/images/tasks/${encodeURIComponent(taskId)}`, {
    method: 'GET',
    headers: authHeaders(tokenKey),
  });

  if (!res.ok) {
    throw new Error(await extractError(res));
  }

  const data = await res.json();
  return {
    id: data.task_id || taskId,
    taskId: data.task_id || taskId,
    status: data.status,
    progress: Number(data.progress) || 0,
    model: data.model,
    // 与 fetchVideoTask 保持一致：上层用 videoUrl 字段接收结果地址
    videoUrl: data.result_url || '',
    failReason: data.fail_reason || '',
  };
}

/**
 * 查询单个任务：GET /v1/videos/{task_id}
 *
 * 完成时返回里带 video_url —— 直接用它播放，**不把视频下载到本服务器**。
 */
export async function fetchVideoTask(tokenKey, taskId) {
  const res = await fetch(`/v1/videos/${encodeURIComponent(taskId)}`, {
    method: 'GET',
    headers: authHeaders(tokenKey),
  });

  if (!res.ok) {
    throw new Error(await extractError(res));
  }

  const data = await res.json();
  return {
    id: data.id || data.task_id || taskId,
    taskId: data.task_id || data.id || taskId,
    status: data.status,
    progress: Number(data.progress) || 0,
    model: data.model,
    createdAt: data.created_at,
    videoUrl: data.video_url || '',
  };
}

/**
 * 历史任务（控制台接口，落库记录）。
 *
 * 产出类型必须交给服务端过滤，不能只在前端筛：分页是服务端做的，
 * 前端筛会出现「这一页 9 条里只有 1 条视频」这种看着像丢数据的情况，
 * 总数（共 N 条）也对不上。
 *
 * 图片任务落库时 platform=image；视频任务的 platform 是渠道类型（"1" 之类），
 * 所以视频这一侧用 exclude_platform=image 表达，而不是 platform=video。
 */
export async function getUserTasks(params = {}) {
  const { mediaFilter, ...rest } = params;
  const query = { p: 1, page_size: 50, ...rest };

  if (mediaFilter === 'image') {
    query.platform = 'image';
  } else if (mediaFilter === 'video') {
    query.exclude_platform = 'image';
  }

  const res = await API.get('/api/task/self', { params: query });
  const { success, message, data } = res.data;
  if (!success) {
    throw new Error(message || '加载历史任务失败');
  }
  return {
    items: Array.isArray(data?.items) ? data.items : [],
    total: data?.total ?? 0,
  };
}
