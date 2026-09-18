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
 * 提示词优化。
 *
 * 走标准 relay 接口 `/v1/chat/completions`，用**用户自己的令牌**鉴权
 * （与工作台生成视频/图片用的是同一把密钥），因此：
 *   - 消耗计在用户自己的令牌上
 *   - 模型由本系统路由，前端不接触任何上游地址
 *
 * 上游（h3-prompt-writing）返回结构实测为：
 *
 *   integrated_multimodal_description: ...
 *   overall_soundscape: ...
 *   non_diegetic_music: N/A
 *
 *   <!-- H3 Prompt Writing | mode: T2VA | duration: 8s | ratio: 16:9 | shots: 3 -->
 *   说明：按纯文本 T2VA 处理，8 秒三镜……
 *
 * 即：正文 + HTML 注释元信息 + 一段「说明」。注释并不在最后，
 * 所以按**第一个** `<!--` 切分，之前是提示词正文，之后的都当元信息展示。
 */

import { SSE } from 'sse.js';
import { t } from 'i18next';

/** 上游对图片数量的上限 */
export const OPTIMIZE_MAX_IMAGES = 9;

/** 上游对单张图片体积的上限（8MB） */
export const OPTIMIZE_MAX_IMAGE_BYTES = 8 * 1024 * 1024;

/** 未配置自定义指令时使用的内置提示 */
export const DEFAULT_OPTIMIZE_SYSTEM_PROMPT =
  '你是视频生成提示词工程师。在保留原始台词与关键信息的前提下，把用户提供的内容改写成适合视频生成模型使用的高质量提示词。直接输出改写后的提示词，不要解释、不要加引号。';

const COMMENT_START = '<!--';

/**
 * 拆分「提示词正文」与「元信息」。
 *
 * @param {string} raw
 * @param {{ partial?: boolean }} options partial=true 用于流式过程中：
 *   此时注释可能只吐了一半，仍然按第一个 `<!--` 截断，不产出 meta。
 */
export function splitPromptMeta(raw, options = {}) {
  if (!raw) return { text: '', meta: '' };

  const index = raw.indexOf(COMMENT_START);
  if (index === -1) {
    return { text: raw.trim(), meta: '' };
  }

  const text = raw.slice(0, index).trim();
  if (options.partial) {
    return { text, meta: '' };
  }

  const rest = raw.slice(index);
  const matched = rest.match(/^<!--([\s\S]*?)-->/);
  if (!matched) {
    return { text, meta: rest.trim() };
  }

  // 注释之后的「说明」也是元信息，一并展示
  const trailing = rest.slice(matched[0].length).trim();
  return {
    text,
    meta: [matched[1].trim(), trailing].filter(Boolean).join('\n\n'),
  };
}

/** 把本地文件读成 data URL（绕开图床 CORS，且符合上游要求） */
function fileToDataUrl(file) {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(String(reader.result || ''));
    reader.onerror = () =>
      reject(new Error(t('读取图片失败：{{name}}', { name: file.name })));
    reader.readAsDataURL(file);
  });
}

/**
 * 组装 OpenAI 多模态 user content。
 *
 * 实测：上游**只接受 base64 data URL**，传公开 https 地址会报
 * 「第 N 张图片不是合法的 data URL」。所以这里统一把本地文件转成 data URL。
 * 无图时 content 用普通字符串。
 *
 * @throws {Error} 当某张图片超过上游体积上限时抛出，避免静默丢图导致结果偏差
 */
export async function buildOptimizeContent(prompt, images) {
  const usable = (images || []).filter((item) => item?.file);
  if (usable.length === 0) {
    return prompt;
  }

  const selected = usable.slice(0, OPTIMIZE_MAX_IMAGES);
  const tooLarge = selected.find(
    (item) => item.file.size > OPTIMIZE_MAX_IMAGE_BYTES,
  );
  if (tooLarge) {
    throw new Error(
      t('{{name}} 超过 {{size}}MB，无法用于提示词优化', {
        name: tooLarge.name || tooLarge.file.name,
        size: Math.round(OPTIMIZE_MAX_IMAGE_BYTES / 1024 / 1024),
      }),
    );
  }

  const dataUrls = await Promise.all(
    selected.map((item) => fileToDataUrl(item.file)),
  );

  return [
    { type: 'text', text: prompt },
    ...dataUrls.map((url) => ({ type: 'image_url', image_url: { url } })),
  ];
}

/**
 * 发起一次流式优化。
 *
 * 参考图需要先转成 data URL（异步），因此内部是异步的，但对外仍然
 * 同步返回一个可用于中断的句柄。
 *
 * @param {object} options
 * @param {string} options.tokenKey 用户选择的令牌（不含 sk- 前缀）
 * @param {Array}  options.images   参考图资源（含原始 File）
 * @returns {{ close: () => void }} 可用于中断
 */
export function optimizePrompt(options) {
  const {
    tokenKey,
    model,
    prompt,
    images = [],
    systemPrompt = '',
    onDelta,
    onDone,
    onError,
  } = options;

  let source = null;
  let raw = '';
  let settled = false;
  let cancelled = false;

  const finish = (partial = false) => {
    if (settled) return;
    settled = true;
    source?.close();
    onDone?.(splitPromptMeta(raw, partial ? { partial: true } : {}));
  };

  const fail = (message) => {
    if (settled) return;
    settled = true;
    source?.close();
    onError?.(message);
  };

  if (!tokenKey) {
    fail(t('请先选择用于调用的密钥'));
    return { close: () => {} };
  }

  const run = async () => {
    let content;
    try {
      content = await buildOptimizeContent(prompt, images);
    } catch (error) {
      fail(error?.message || t('读取参考图失败'));
      return;
    }
    if (settled || cancelled) return;

    const messages = [];
    const instruction = (systemPrompt || '').trim();
    if (instruction) {
      messages.push({ role: 'system', content: instruction });
    }
    messages.push({ role: 'user', content });

    const stream = new SSE('/v1/chat/completions', {
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer sk-${tokenKey}`,
      },
      method: 'POST',
      payload: JSON.stringify({ model, stream: true, messages }),
    });
    source = stream;

    stream.addEventListener('message', (event) => {
      if (event.data === '[DONE]') {
        finish();
        return;
      }

      let chunk;
      try {
        chunk = JSON.parse(event.data);
      } catch {
        return;
      }

      if (chunk?.error) {
        fail(chunk.error.message || t('提示词优化失败'));
        return;
      }

      const delta = chunk?.choices?.[0]?.delta?.content;
      if (typeof delta === 'string' && delta !== '') {
        raw += delta;
        // 流式过程中也按第一个 <!-- 截断，避免输入框里出现注释
        onDelta?.(splitPromptMeta(raw, { partial: true }).text);
      }
    });

    stream.addEventListener('error', () => {
      fail(t('提示词优化连接中断'));
    });
  };

  run();

  return {
    close: () => {
      cancelled = true;
      // 用户主动中断时保留已收到的正文
      if (settled) return;
      finish(true);
    },
  };
}
