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
 * 只有 endpoint 可通过 VITE_MEDIA_UPLOAD_ENDPOINT 配置。当目标图床没有
 * 放开跨域时，把该变量指向同源反向代理路径即可（例如
 * `/image-bed/api/upload`），无需改代码。
 */
import axios from 'axios';

/** 默认图床地址 */
const DEFAULT_UPLOAD_ENDPOINT = 'https://wgspai.cn/image-bed/api/upload';

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
 * 图床专用客户端。
 *
 * 刻意不复用 helpers 里的 API 实例：那个实例会带上 `New-API-User`
 * 请求头，凭据绝不能泄露给第三方域名。
 */
const mediaClient = axios.create({
  withCredentials: false,
  timeout: 120000,
});

/**
 * 上传单个参考素材并返回其公开 URL。
 *
 * @throws {Error} 传输失败，或服务返回 success=false / 缺少 url 时抛出。
 */
export async function uploadWorkbenchAsset(file) {
  const formData = new FormData();
  formData.append(UPLOAD_FORM_FIELD, file);

  let payload;
  try {
    // 刻意不设置 Content-Type：必须由浏览器生成带 boundary 的
    // multipart/form-data，写死该头会导致服务端无法解析。
    const response = await mediaClient.post(getUploadEndpoint(), formData);
    payload = response.data || {};
  } catch (error) {
    throw new Error(error?.message || '上传失败');
  }

  if (payload.success === false) {
    throw new Error(payload.message || '上传失败');
  }
  if (!payload.url) {
    throw new Error('上传服务未返回 URL');
  }

  return payload;
}
