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
 * 任务结果下载。
 *
 * 按优先级尝试：
 *   1. GET /v1/videos/{task_id} 返回的 result_url（上游直链，如七牛签名地址）；
 *   2. 任务列表里 data.result_url 缓存的直链；
 *   3. 任务列表里的代理 result_url；
 *   4. 配置的 new-api 地址 + 标准代理路径 + 用户令牌；
 *   5. 当前站点同源相对路径（代理域名不可达时的兜底）。
 *
 * 直链走“HEAD 探活 + 浏览器直接跳转”：七牛带 Content-Disposition: attachment，
 * 浏览器会直接下载，不需要把 10MB+ 视频读进内存。
 * 直链探活失败时才退回代理，用 fetch 成 Blob 再 <a download>。
 *
 * 直链请求用 credentials: 'omit'（七牛 CORS 是 `*`，带 cookie 会被浏览器拒绝）；
 * 代理地址用 credentials: 'include' 走会话，并可按需带用户令牌。
 *
 * 图片任务保持原有逻辑：不读直链，继续走代理。
 */

import { getServerAddress } from '../../helpers';
import { fetchTaskPlaybackUrl } from './api';

const PROXY_CONTENT_RE = /^\/v1\/(?:videos|images\/tasks)\/[^/]+\/content\/?$/;

/** 任务结果在网关上的标准代理路径 */
export function taskProxyPath(task) {
  const taskId = String(task?.taskId || '').trim();
  if (!taskId) return '';
  return task?.kind === 'image'
    ? `/v1/images/tasks/${encodeURIComponent(taskId)}/content`
    : `/v1/videos/${encodeURIComponent(taskId)}/content`;
}

function toAbsolute(url) {
  const value = String(url || '').trim();
  if (!value) return '';
  if (/^(?:https?:|data:|blob:)/i.test(value)) return value;
  try {
    return new URL(value, window.location.origin).toString();
  } catch {
    return value;
  }
}

function isProxyContentUrl(url) {
  try {
    return PROXY_CONTENT_RE.test(new URL(url, window.location.origin).pathname);
  } catch {
    return false;
  }
}

function isSameOrigin(a, b) {
  try {
    return (
      new URL(a, window.location.origin).origin ===
      new URL(b, window.location.origin).origin
    );
  } catch {
    return false;
  }
}

function joinUrl(base, path) {
  const trimmed = String(base || '').replace(/\/+$/, '');
  return trimmed ? `${trimmed}${path}` : path;
}

/**
 * 收集可用的上游直链（视频任务）。图片任务保持走代理，不参与直链逻辑。
 */
export function collectDirectUrls(task, primaryUrl = '') {
  if (task?.kind === 'image') return [];

  const resultUrl = toAbsolute(task?.resultUrl);
  const seen = new Set();
  return [toAbsolute(primaryUrl), toAbsolute(task?.directUrl)].filter((url) => {
    if (!url || url === resultUrl || isProxyContentUrl(url) || seen.has(url)) {
      return false;
    }
    seen.add(url);
    return true;
  });
}

/**
 * 按优先级生成下载候选。
 * primaryUrl 是刚通过 /v1/videos/{task_id} 查到的 result_url（直链），排在最前。
 * withToken 为 true 的候选明确是 new-api 自己的代理接口，可以带用户令牌。
 */
export function buildDownloadCandidates(
  task,
  primaryUrl = '',
  { includeDirect = true } = {},
) {
  const path = taskProxyPath(task);
  const server = getServerAddress();
  const origin = window.location.origin;
  const candidates = [];

  const add = (url, withToken, credentials = 'include') => {
    const value = toAbsolute(url);
    if (!value) return;
    if (candidates.some((item) => item.url === value)) return;
    candidates.push({ url: value, withToken, credentials });
  };

  const resultUrl = toAbsolute(task?.resultUrl);

  // 上游直链（如七牛签名地址）优先：浏览器直连，不经过 new-api 代理。
  // 七牛的 CORS 是 `*`，不能带 credentials，否则浏览器会拒绝响应。
  if (includeDirect) {
    collectDirectUrls(task, primaryUrl).forEach((url) => {
      add(url, false, 'omit');
    });
  }

  if (resultUrl) {
    // result_url 是后端返回的代理地址；只有确认是 new-api 的代理路径、
    // 且和配置的服务器同源时才附带令牌，避免把令牌发给上游。
    add(
      resultUrl,
      Boolean(path) &&
        isProxyContentUrl(resultUrl) &&
        isSameOrigin(resultUrl, server),
    );
  }

  if (path && server) {
    add(joinUrl(server, path), true);
  }

  if (path && origin && origin !== server) {
    add(joinUrl(origin, path), true);
  }

  return candidates;
}

async function fetchResultBlob(candidate, tokenKey) {
  const headers = {};
  if (candidate.withToken && tokenKey) {
    headers.Authorization = `Bearer sk-${tokenKey}`;
  }

  const response = await fetch(candidate.url, {
    method: 'GET',
    credentials: candidate.credentials || 'include',
    headers,
  });

  if (!response.ok) {
    throw new Error(`HTTP ${response.status}`);
  }

  const contentType = (
    response.headers.get('content-type') || ''
  ).toLowerCase();
  // 鉴权失败/反代登录页有时会以 200 返回 JSON/HTML，别把它当视频保存下来。
  if (
    contentType.includes('application/json') ||
    contentType.includes('text/html')
  ) {
    throw new Error(`unexpected content type: ${contentType}`);
  }

  const blob = await response.blob();
  if (!blob.size) {
    throw new Error('empty response');
  }
  return blob;
}

function downloadExtension(blob, task) {
  const subtype = (blob.type.split('/')[1] || '')
    .split(';')[0]
    .split('+')[0]
    .trim()
    .toLowerCase();
  // octet-stream 说明上游没给具体类型，按任务产出类型给一个合理后缀
  if (subtype && subtype !== 'octet-stream') return subtype;
  return task?.kind === 'image' ? 'png' : 'mp4';
}

function triggerBlobDownload(blob, task, sourceUrl) {
  const extension = downloadExtension(blob, task);

  // 直链上通常带 attname（七牛会用它作为下载文件名），优先保留
  let filename = `${task?.taskId || 'result'}.${extension}`;
  try {
    const attname = new URL(sourceUrl, window.location.origin).searchParams.get(
      'attname',
    );
    if (attname) filename = attname;
  } catch {
    // 非法 URL 就继续用 taskId
  }

  const objectUrl = URL.createObjectURL(blob);

  const link = document.createElement('a');
  link.href = objectUrl;
  link.download = filename;
  document.body.appendChild(link);
  link.click();
  link.remove();

  // 立即 revoke 在部分浏览器里会把正在开始的下载打断。
  window.setTimeout(() => URL.revokeObjectURL(objectUrl), 1000);
}

/** 直链探活：HEAD 不下载正文。七牛对 HEAD 返回 200 + CORS `*`。 */
async function probeDirectUrl(url) {
  try {
    const response = await fetch(url, {
      method: 'HEAD',
      credentials: 'omit',
    });
    return response.ok;
  } catch {
    return false;
  }
}

/** 直链直接跳转：七牛带 attachment，浏览器会保存文件，不把视频读进内存。 */
function triggerDirectDownload(url) {
  window.location.href = url;
}

/**
 * 下载任务结果（视频/图片）。成功返回 true；全部候选失败时抛出最后一条错误。
 *
 * 视频：先查 /v1/videos/{task_id} 拿最新 result_url，HEAD 探活后直接跳转；
 *       直链不可用才退回代理 Blob 下载。
 * 图片：保持原逻辑，直接走代理 Blob 下载。
 */
export async function downloadTaskResult(task, { tokenKey = '' } = {}) {
  const isImage = task?.kind === 'image';
  const fetchedUrl = isImage ? '' : await fetchTaskPlaybackUrl(task, tokenKey);

  if (!isImage) {
    for (const url of collectDirectUrls(task, fetchedUrl)) {
      if (await probeDirectUrl(url)) {
        triggerDirectDownload(url);
        return true;
      }
    }
  }

  const candidates = buildDownloadCandidates(task, fetchedUrl, {
    includeDirect: false,
  });
  if (candidates.length === 0) {
    throw new Error('no download url');
  }

  let lastError = null;
  for (const candidate of candidates) {
    try {
      const blob = await fetchResultBlob(candidate, tokenKey);
      triggerBlobDownload(blob, task, candidate.url);
      return true;
    } catch (error) {
      lastError = error;
    }
  }

  throw lastError || new Error('download failed');
}
