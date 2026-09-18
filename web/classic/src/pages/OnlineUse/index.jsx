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

import React, { useEffect, useState } from 'react';
import { API, showError } from '../../helpers';
import { marked } from 'marked';
import { useTranslation } from 'react-i18next';
import Loading from '../../components/common/ui/Loading';

function detectIsHtml(content) {
  return (
    /^\s*<!DOCTYPE\s+html/i.test(content.trim()) ||
    /^\s*<html[\s>]/i.test(content.trim())
  );
}

function detectIsUrl(content) {
  try {
    const url = new URL(content);
    return url.protocol === 'http:' || url.protocol === 'https:';
  } catch {
    return false;
  }
}

/**
 * 「在线使用」页面：内容由后台配置（Markdown / HTML / 外链），
 * 外链与 HTML 都用 iframe 承载，避免污染后台自己的样式与脚本环境。
 *
 * embedded = true 时不带自己的整页定位与顶部留白，供工作台的「推理模型」tab 内嵌使用。
 */
const OnlineUse = ({ embedded = false }) => {
  const { t } = useTranslation();
  const [content, setContent] = useState('');
  const [loaded, setLoaded] = useState(false);

  const displayContent = async () => {
    setContent(localStorage.getItem('online_use_page_content') || '');
    const res = await API.get('/api/online_use_page_content');
    const { success, message, data } = res.data;
    if (success && data) {
      const isUrl = detectIsUrl(data);
      const isHtml = !isUrl && detectIsHtml(data);
      const onlineUseContent = isUrl || isHtml ? data : marked.parse(data);
      setContent(onlineUseContent);
      localStorage.setItem('online_use_page_content', onlineUseContent);
    } else if (!success && message) {
      showError(message);
      setContent('');
    } else {
      setContent('');
      localStorage.removeItem('online_use_page_content');
    }
    setLoaded(true);
  };

  useEffect(() => {
    displayContent().then();
  }, []);

  if (!loaded) {
    return <Loading />;
  }

  // 内嵌时铺满父容器；独立成页时占满内容区（顶部留出应用头栏高度）
  const shellClass = embedded
    ? 'wb-embed'
    : 'classic-page-fill flex flex-col overflow-x-hidden w-full pt-[60px]';

  if (!content) {
    return (
      <div className={embedded ? 'wb-embed' : shellClass}>
        <div className='flex flex-1 justify-center items-center p-8'>
          <p>{t('暂无内容')}</p>
        </div>
      </div>
    );
  }

  const isUrl = detectIsUrl(content);
  const isHtml = !isUrl && detectIsHtml(content);

  return (
    <div className={shellClass}>
      {isUrl ? (
        <iframe
          src={content}
          className='w-full flex-1 min-h-0 border-none'
          title={t('在线使用')}
          sandbox='allow-scripts allow-same-origin allow-downloads allow-popups'
        />
      ) : isHtml ? (
        <iframe
          srcDoc={content}
          className='w-full flex-1 min-h-0 border-none'
          title={t('在线使用')}
          sandbox='allow-scripts allow-same-origin allow-downloads allow-popups'
        />
      ) : (
        <div className='px-2' dangerouslySetInnerHTML={{ __html: content }} />
      )}
    </div>
  );
};

export default OnlineUse;
