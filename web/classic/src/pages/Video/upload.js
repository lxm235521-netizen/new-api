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
 * 参考素材上传适配层。
 *
 * 请求契约固定，即使日后更换后端服务也不得更改：
 *
 *   POST <endpoint>
 *   Content-Type: multipart/form-data
 *   表单字段：file=<二进制>
 *
 *   => { "success": true, "url": "https://.../x.png", ... }
 *
 * 默认走**同源后端代理** `/api/workbench/upload`：
 * 现用图床（files.mmg.lat）不返回任何 CORS 响应头，浏览器直传必被拦下；
 * 交给服务端转发同时也让图床 Token 只留在服务端，不下发到浏览器。
 *
 * 确实要绕过代理直传某个第三方图床时，把 VITE_MEDIA_UPLOAD_ENDPOINT 设成
 * 它的完整 URL 即可 —— 那种情况下绝不携带本系统的任何凭据。
 */
import axios from 'axios';
import { API } from '../../helpers';

/** 默认上传地址：本服务的转发接口 */
const DEFAULT_UPLOAD_ENDPOINT = '/api/workbench/upload';

/** 契约要求的表单字段名 */
const UPLOAD_FORM_FIELD = 'file';

export function getUploadEndpoint() {
  const configured = import.meta.env.VITE_MEDIA_UPLOAD_ENDPOINT;
  if (typeof configured === 'string' && configured.trim() !== '') {
    return configured.trim();
  }
  return DEFAULT_UPLOAD_ENDPOINT;
}

/**
 * 只有以 `/` 开头的相对路径才认定为本服务的接口。
 *
 * 判定刻意保守：写死完整域名的第三方图床一律当外部处理，
 * 宁可漏带凭据，也不能把 New-API-User / 会话泄露给外部域名。
 */
function isSameOriginEndpoint(endpoint) {
  return endpoint.startsWith('/');
}

/**
 * 外部图床专用客户端。
 *
 * 刻意不复用 helpers 里的 API 实例：那个实例会带上 `New-API-User`
 * 请求头，凭据绝不能泄露给第三方域名。
 */
const mediaClient = axios.create({
  withCredentials: false,
  timeout: 120000,
});

/**
 * 把各家图床的响应统一成 `{ success, message, url, filename, size }`。
 *
 * 兼容三种形态：
 *   - 后端代理：     { success: true, data: { url, ... } }
 *   - 旧图床 JSON：  { success: true, url, ... }
 *   - 纯文本图床：   响应体就是一行 https://.../x.png
 */
function normalizeUploadPayload(raw) {
  if (typeof raw === 'string') {
    const text = raw.trim();
    if (/^https?:\/\//i.test(text)) {
      return { success: true, message: '', url: text };
    }
    return { success: false, message: text };
  }

  if (!raw || typeof raw !== 'object') {
    return { success: false, message: '' };
  }

  const data = raw.data && typeof raw.data === 'object' ? raw.data : raw;
  return {
    success: raw.success !== false,
    message: raw.message || data.message || '',
    url: data.url || '',
    filename: data.filename || '',
    size: data.size,
  };
}

/** 从 axios 错误里取出后端给的可读信息（代理失败时是 { success:false, message }） */
function describeUploadError(error) {
  const data = error?.response?.data;
  if (typeof data === 'string' && data.trim() !== '') {
    return data.trim();
  }
  if (data && typeof data === 'object') {
    const message = data.message || data.error?.message || data.error;
    if (typeof message === 'string' && message.trim() !== '') {
      return message.trim();
    }
  }
  return error?.message || '上传失败';
}

/**
 * 上传单个参考素材并返回其公开 URL。
 *
 * @throws {Error} 传输失败，或服务返回 success=false / 缺少 url 时抛出。
 */
export async function uploadWorkbenchAsset(file) {
  const formData = new FormData();
  formData.append(UPLOAD_FORM_FIELD, file);

  let raw;
  try {
    // 刻意不设置 Content-Type：必须由浏览器生成带 boundary 的
    // multipart/form-data，写死该头会导致服务端无法解析。
    const endpoint = getUploadEndpoint();
    if (isSameOriginEndpoint(endpoint)) {
      // skipErrorHandler：这里自己报错，避免全局拦截器再弹一次
      const response = await API.post(endpoint, formData, {
        skipErrorHandler: true,
      });
      raw = response.data;
    } else {
      const response = await mediaClient.post(endpoint, formData);
      raw = response.data;
    }
  } catch (error) {
    throw new Error(describeUploadError(error));
  }

  const payload = normalizeUploadPayload(raw);
  if (!payload.success) {
    throw new Error(payload.message || '上传失败');
  }
  if (!payload.url) {
    throw new Error('上传服务未返回 URL');
  }

  return payload;
}
