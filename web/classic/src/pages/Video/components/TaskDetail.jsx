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
import { useTranslation } from 'react-i18next';
import { Button, Modal, Progress, Tag, Typography } from '@douyinfe/semi-ui';
import {
  AlertTriangle,
  AudioLines,
  Download,
  Image as ImageIcon,
  RotateCcw,
  Video as VideoIcon,
} from 'lucide-react';
import { renderQuota, showError } from '../../../helpers';
import { fetchTaskPlaybackUrl } from '../api';
import { downloadTaskResult } from '../download';
import { TASK_STATUS_PRESENTATION } from '../constants';
import {
  countReferences,
  describeTaskMode,
  formatDateTime,
  formatElapsed,
  resolvePlaybackUrl,
  resolveProxyPlaybackUrl,
  taskCostQuota,
} from '../utils';

const { Text } = Typography;

const REFERENCE_KINDS = [
  { key: 'image', labelKey: '参考图片', icon: ImageIcon },
  { key: 'audio', labelKey: '参考音频', icon: AudioLines },
  { key: 'video', labelKey: '参考视频', icon: VideoIcon },
];

const MODE_LABELS = {
  text2video: { video: '文生视频', image: '文生图' },
  image2video: { video: '图生视频', image: '图生图' },
  reference: { video: '参考生视频', image: '参考生图' },
};

/** 模式文案：优先用提交时的 mode，否则按参考素材推断 */
function modeLabel(task, t) {
  const mode = describeTaskMode(task);
  const entry = MODE_LABELS[mode];
  if (!entry) return t(mode);
  return t(entry[task.kind === 'image' ? 'image' : 'video']);
}

function buildParams(task, t) {
  const params = [];

  // 模式只有拿到提交快照才可靠（老任务没有参考素材记录，猜出来的会是错的）
  if (task.requestKnown) {
    params.push({ labelKey: '模式', value: modeLabel(task, t) });
  }

  params.push(
    { labelKey: '宽高比', value: task.aspectRatio || '-' },
    {
      labelKey: '时长',
      value:
        task.durationSeconds > 0
          ? t('{{count}} 秒', { count: task.durationSeconds })
          : '-',
    },
    { labelKey: '分辨率', value: task.resolution || '-' },
    { labelKey: '模型', value: task.model || '-' },
    { labelKey: '调用模型', value: task.upstreamModel || task.model || '-' },
    { labelKey: '参考素材', value: String(countReferences(task)) },
    // 失败的任务预扣费已退回，费用显示为 0
    { labelKey: '扣费', value: renderQuota(taskCostQuota(task)) },
  );

  if (task.elapsedSeconds > 0) {
    params.push({
      labelKey: '耗时',
      value: formatElapsed(task.elapsedSeconds),
    });
  }

  params.push(
    { labelKey: '提交时间', value: formatDateTime(task.createdAt) },
    {
      labelKey: '完成时间',
      value: task.finishTime ? formatDateTime(task.finishTime) : '-',
    },
  );

  return params;
}

/** 左侧：结果预览（视频 / 图片 / 进度 / 失败原因） */
function DetailPreview({ task, t, tokenKey }) {
  const [fetchedUrl, setFetchedUrl] = useState('');
  const [useProxy, setUseProxy] = useState(false);

  // 打开详情时查一次 /v1/videos/{task_id}，优先拿 result_url 直链；
  // 查不到就用任务列表里缓存的直链/代理地址。图片任务不查直链。
  useEffect(() => {
    if (task.kind === 'image') return undefined;

    let cancelled = false;
    setFetchedUrl('');
    setUseProxy(false);

    fetchTaskPlaybackUrl(task, tokenKey).then((url) => {
      if (cancelled || !url) return;
      setFetchedUrl(url);
      // 拿到新直链后重新尝试直链，避免旧直链先报错把状态锁在代理上
      setUseProxy(false);
    });

    return () => {
      cancelled = true;
    };
  }, [task.taskId, task.kind, tokenKey]);

  const isPending = [
    'QUEUED',
    'SUBMITTED',
    'NOT_START',
    'IN_PROGRESS',
  ].includes(task.status);

  if (task.status === 'FAILURE') {
    return (
      <div className='wb-detail__fallback wb-detail__fallback--fail'>
        <AlertTriangle size={22} aria-hidden='true' />
        <Text type='danger' size='small'>
          {task.failReason || t('生成失败')}
        </Text>
      </div>
    );
  }

  const playbackUrl = useProxy
    ? resolveProxyPlaybackUrl(task)
    : fetchedUrl || resolvePlaybackUrl(task);

  const handleMediaError = () => {
    // 直链过期/被上游拒绝时，回退到同源代理地址
    if (!useProxy) setUseProxy(true);
  };

  if (task.resultUrl && task.kind === 'image') {
    return <img className='wb-detail__media' src={playbackUrl} alt='' />;
  }

  if (task.resultUrl || task.directUrl) {
    return (
      <video
        className='wb-detail__media'
        src={playbackUrl}
        poster={task.thumbnailUrl || undefined}
        controls
        playsInline
        preload='metadata'
        onError={handleMediaError}
      />
    );
  }

  if (isPending) {
    return (
      <div className='wb-detail__fallback'>
        <span className='wb-detail__progress'>
          {t('生成中')} {task.progress || 0}%
        </span>
        <Progress percent={Math.max(task.progress || 0, 4)} showInfo={false} />
      </div>
    );
  }

  return (
    <div className='wb-detail__fallback'>
      <VideoIcon size={22} aria-hidden='true' />
      <Text type='tertiary' size='small'>
        {t('暂无结果')}
      </Text>
    </div>
  );
}

/**
 * 任务详情。
 *
 * 成功后可在这里播放/下载结果，并把整份配置（模型 + 提示词 + 参数 + 参考素材）
 * 一键载回右侧表单重新生成。
 */
const TaskDetail = ({ task, visible, onClose, onReuse, tokenKey }) => {
  const { t } = useTranslation();
  const [downloading, setDownloading] = useState(false);

  if (!task) return null;

  const status =
    TASK_STATUS_PRESENTATION[task.status] || TASK_STATUS_PRESENTATION.NOT_START;
  const title = task.id ? t('任务 #{{id}}', { id: task.id }) : task.taskId;

  const handleDownload = async () => {
    if (!task.resultUrl && !task.directUrl) return;
    setDownloading(true);
    try {
      // 优先 result_url；失败后自动用「new-api 地址 + 令牌」重试，见 download.js
      await downloadTaskResult(task, { tokenKey });
    } catch (error) {
      showError(error?.message || t('下载失败'));
      window.open(resolvePlaybackUrl(task), '_blank');
    } finally {
      setDownloading(false);
    }
  };

  const references = REFERENCE_KINDS.map((kind) => ({
    ...kind,
    urls: task.references?.[kind.key] || [],
  })).filter((kind) => kind.urls.length > 0);

  return (
    <Modal
      visible={visible}
      onCancel={onClose}
      footer={null}
      width={1080}
      centered
      title={
        <span className='wb-detail__title'>
          {title}
          <Tag color={status.tagColor} size='small' shape='circle'>
            {t(status.labelKey)}
          </Tag>
        </span>
      }
      bodyStyle={{ padding: 0 }}
    >
      <div className='wb-detail'>
        <div className='wb-detail__left'>
          <DetailPreview
            key={task.taskId}
            task={task}
            t={t}
            tokenKey={tokenKey}
          />
        </div>

        <div className='wb-detail__right'>
          <div className='wb-detail__block'>
            <div className='wb-detail__label'>{t('输入内容')}</div>
            <div className='wb-detail__prompt'>
              {task.prompt || t('（无提示词记录）')}
            </div>
          </div>

          {references.length > 0 && (
            <div className='wb-detail__block'>
              <div className='wb-detail__label'>{t('输入媒体')}</div>
              <div className='wb-detail__refs'>
                {references.map((kind) => {
                  const RefIcon = kind.icon;
                  return (
                    <div key={kind.key} className='wb-detail__ref-group'>
                      <div className='wb-detail__ref-title'>
                        {t(kind.labelKey)} · {kind.urls.length}
                      </div>
                      <div className='wb-detail__ref-list'>
                        {kind.urls.map((url) => (
                          <a
                            key={url}
                            className='wb-detail__ref'
                            href={url}
                            target='_blank'
                            rel='noreferrer'
                            title={url}
                          >
                            {kind.key === 'image' ? (
                              <img src={url} alt='' />
                            ) : (
                              <RefIcon size={16} aria-hidden='true' />
                            )}
                          </a>
                        ))}
                      </div>
                    </div>
                  );
                })}
              </div>
            </div>
          )}

          <div className='wb-detail__block'>
            <div className='wb-detail__label'>{t('参数配置')}</div>
            <div className='wb-detail__params'>
              {buildParams(task, t).map((item) => (
                <div key={item.labelKey} className='wb-detail__param'>
                  <span className='wb-detail__param-label'>
                    {t(item.labelKey)}
                  </span>
                  <span className='wb-detail__param-value' title={item.value}>
                    {item.value}
                  </span>
                </div>
              ))}
            </div>
          </div>
        </div>
      </div>

      <div className='wb-detail__footer'>
        <Button
          theme='light'
          type='primary'
          icon={<RotateCcw size={14} />}
          onClick={() => onReuse(task)}
        >
          {t('复用配置')}
        </Button>
        <Button
          theme='solid'
          type='primary'
          icon={<Download size={14} />}
          loading={downloading}
          disabled={!task.resultUrl && !task.directUrl}
          onClick={handleDownload}
        >
          {t('下载结果')}
        </Button>
      </div>
    </Modal>
  );
};

export default TaskDetail;
