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
 * 工作台使用的令牌（密钥）选择。
 *
 * 工作台里生成视频/图片、以及提示词优化，用的都是**用户自己的令牌**，
 * 所以进入工作台先要选一把密钥，使用过程中也允许随时切换。
 *
 * 令牌列表取自 /api/token/，真实 key 需要按 id 单独取（/api/token/:id/key），
 * 因此这里先取列表再批量取 key。
 */

import { useCallback, useEffect, useMemo, useState } from 'react';
import { API, showError } from '../../../helpers';

const STORAGE_KEY = 'workbench_token_id';

async function loadTokens() {
  const res = await API.get('/api/token/?p=1&size=100');
  const { success, data, message } = res.data || {};
  if (!success) {
    throw new Error(message || '加载密钥列表失败');
  }

  const items = Array.isArray(data) ? data : data.items || [];
  const active = items.filter((token) => token.status === 1);
  if (active.length === 0) return [];

  const ids = active.map((token) => token.id);
  let keyMap = {};
  try {
    const keyRes = await API.post('/api/token/batch/keys', { ids });
    if (keyRes.data?.success) {
      keyMap = keyRes.data.data?.keys || {};
    }
  } catch {
    // 批量取 key 失败时退化为逐个取
  }

  const result = [];
  for (const token of active) {
    let key = keyMap[token.id];
    if (!key) {
      try {
        const one = await API.post(`/api/token/${token.id}/key`);
        if (one.data?.success) key = one.data.data?.key;
      } catch {
        key = '';
      }
    }
    if (key) {
      result.push({ id: token.id, name: token.name, key });
    }
  }
  return result;
}

export function useWorkbenchToken() {
  const [tokens, setTokens] = useState([]);
  const [loading, setLoading] = useState(true);
  const [selectedId, setSelectedId] = useState(() => {
    const raw = localStorage.getItem(STORAGE_KEY);
    return raw ? Number(raw) : null;
  });

  useEffect(() => {
    let cancelled = false;
    loadTokens()
      .then((list) => {
        if (cancelled) return;
        setTokens(list);
        // 之前选的令牌若已失效/被删，自动回退到第一把
        setSelectedId((prev) => {
          if (prev && list.some((token) => token.id === prev)) return prev;
          return list.length > 0 ? list[0].id : null;
        });
      })
      .catch((error) => {
        if (!cancelled) showError(error?.message || '加载密钥列表失败');
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  const selectToken = useCallback((id) => {
    setSelectedId(id);
    if (id == null) {
      localStorage.removeItem(STORAGE_KEY);
    } else {
      localStorage.setItem(STORAGE_KEY, String(id));
    }
  }, []);

  const token = useMemo(
    () => tokens.find((item) => item.id === selectedId) || null,
    [tokens, selectedId],
  );

  return { tokens, token, loading, selectToken };
}
