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

import React, { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button, Progress, Tag, Tooltip } from '@douyinfe/semi-ui';
import {
  AlertTriangle,
  Download,
  Eye,
  Image as ImageIcon,
  Music,
  Play,
  RotateCcw,
  Video as VideoIcon,
} from 'lucide-react';
import { renderQuota } from '../../../helpers';
import { MEDIA_KIND_LABELS, TASK_STATUS_PRESENTATION } from '../constants';
import {
  formatClipDuration,
  formatDateTime,
  formatElapsed,
  resolvePlaybackUrl,
  resolveThumbUrl,
  simplifyRatio,
  taskCostQuota,
} from '../utils';

const MEDIA_KIND_ICONS = {
  video: VideoIcon,
  image: ImageIcon,
  audio: Music,
};

const PENDING_STATUSES = ['QUEUED', 'SUBMITTED', 'NOT_START', 'IN_PROGRESS'];

/**
 * 左侧媒体区。
 *
 * 封面有两个来源：
 *   1. 参考图（图生视频）—— 直接当封面；
 *   2. 结果视频的首帧 —— 懒挂一个 muted 的 <video preload="metadata">，
 *      只在卡片滚进视口时才挂（IntersectionObserver），避免 100+ 张卡片
 *      同时向上游要视频。首帧就绪前显示参考图/占位，不会闪黑。
 *
 * 顺带把播放器报出来的真实时长/分辨率回传给卡片 —— 这比读提交参数靠谱，
 * 老任务没有参数快照也能显示「视频时长 / 视频比例」。
 */
function TaskThumbnail({ task, onMediaInfo }) {
  const holderRef = useRef(null);
  const [inView, setInView] = useState(false);
  const [frameReady, setFrameReady] = useState(false);
  const [previewFailed, setPreviewFailed] = useState(false);

  const cover = task.thumbnailUrl || '';
  const src = resolvePlaybackUrl(task);
  // 卡片上的图片用缩略图，别回 2~3MB 的原图
  const thumbSrc = resolveThumbUrl(task);
  const canPreview =
    Boolean(src) &&
    task.kind === 'video' &&
    task.status === 'SUCCESS' &&
    !previewFailed;

  useEffect(() => {
    if (!canPreview || inView) return undefined;
    const node = holderRef.current;
    if (!node) return undefined;
    if (typeof IntersectionObserver === 'undefined') {
      setInView(true);
      return undefined;
    }
    const observer = new IntersectionObserver(
      (entries) => {
        if (entries.some((entry) => entry.isIntersecting)) setInView(true);
      },
      { rootMargin: '200px' },
    );
    observer.observe(node);
    return () => observer.disconnect();
  }, [canPreview, inView]);

  // 图片任务：直接展示生成结果（拿不到结果时退回参考图/占位）
  if (task.kind === 'image' && thumbSrc) {
    return (
      <div className='wb-card__holder' ref={holderRef}>
        <img
          src={thumbSrc}
          alt=''
          loading='lazy'
          decoding='async'
          onError={() => setPreviewFailed(true)}
        />
        {previewFailed && <div className='wb-card__placeholder' />}
      </div>
    );
  }

  const KindIcon = MEDIA_KIND_ICONS[task.kind] || VideoIcon;

  return (
    <div className='wb-card__holder' ref={holderRef}>
      {cover ? (
        <img src={cover} alt='' loading='lazy' decoding='async' />
      ) : (
        <div className='wb-card__placeholder'>
          <KindIcon size={22} aria-hidden='true' />
        </div>
      )}

      {canPreview && inView && (
        <video
          className={`wb-card__preview${frameReady ? ' wb-card__preview--ready' : ''}`}
          src={src}
          muted
          playsInline
          preload='metadata'
          onLoadedData={(event) => {
            setFrameReady(true);
            const video = event.currentTarget;
            onMediaInfo?.({
              duration: video.duration,
              width: video.videoWidth,
              height: video.videoHeight,
            });
          }}
          onError={() => setPreviewFailed(true)}
        />
      )}
    </div>
  );
}

/** 信息区的小标签：产出类型 / 时长 / 比例 / 参考素材 / 扣费 */
function buildChips(task, mediaInfo, t) {
  const chips = [t(MEDIA_KIND_LABELS[task.kind] || '视频')];

  const duration =
    mediaInfo?.duration && Number.isFinite(mediaInfo.duration)
      ? mediaInfo.duration
      : task.durationSeconds;
  if (duration > 0) chips.push(formatClipDuration(Math.round(duration)));

  const ratio =
    simplifyRatio(mediaInfo?.width, mediaInfo?.height) || task.aspectRatio;
  if (ratio) chips.push(ratio);

  const references =
    (task.referenceImages || 0) +
    (task.referenceAudios || 0) +
    (task.referenceVideos || 0);
  if (references > 0) chips.push(t('参考 {{count}}', { count: references }));

  // 失败的任务预扣费已退回，费用显示为 0
  chips.push(renderQuota(taskCostQuota(task)));

  return chips;
}

/**
 * 历史任务卡片（横向：左媒体 + 右信息）。
 *
 * 整张卡片可点击打开详情，卡片内的按钮自行阻止冒泡。
 */
const TaskCard = ({ task, onRetry, onOpen }) => {
  const { t } = useTranslation();
  const [mediaInfo, setMediaInfo] = useState(null);
  const status =
    TASK_STATUS_PRESENTATION[task.status] || TASK_STATUS_PRESENTATION.NOT_START;
  const isPending = PENDING_STATUSES.includes(task.status);
  const isFailure = task.status === 'FAILURE';
  // 图片结果是静态图，没有「播放」这回事
  const playable =
    task.kind === 'video' &&
    Boolean(task.resultUrl) &&
    task.status === 'SUCCESS';

  const handleOpen = () => onOpen?.(task);
  const stop = (event) => event.stopPropagation();
  const chips = buildChips(task, mediaInfo, t);

  return (
    <article
      className={`wb-card${onOpen ? ' wb-card--clickable' : ''}`}
      role={onOpen ? 'button' : undefined}
      tabIndex={onOpen ? 0 : undefined}
      aria-label={onOpen ? t('查看任务详情') : undefined}
      onClick={onOpen ? handleOpen : undefined}
      onKeyDown={
        onOpen
          ? (event) => {
              if (event.key === 'Enter' || event.key === ' ') {
                event.preventDefault();
                handleOpen();
              }
            }
          : undefined
      }
    >
      <div className='wb-card__media'>
        <TaskThumbnail task={task} onMediaInfo={setMediaInfo} />

        {task.elapsedSeconds > 0 && (
          <span className='wb-card__badge wb-card__badge--spent'>
            {t('耗时')} {formatElapsed(task.elapsedSeconds)}
          </span>
        )}

        {isPending && (
          <span className='wb-card__media-mask'>
            <Progress
              type='circle'
              percent={Math.max(task.progress || 0, 4)}
              showInfo={false}
              size='small'
              stroke='var(--semi-color-primary)'
            />
          </span>
        )}

        {isFailure && (
          <span className='wb-card__media-mask wb-card__media-mask--fail'>
            <AlertTriangle size={20} aria-hidden='true' />
          </span>
        )}

        {playable && (
          <span className='wb-card__play'>
            <Play size={15} aria-hidden='true' />
          </span>
        )}
      </div>

      <div className='wb-card__body'>
        <div className='wb-card__head'>
          <Tag color={status.tagColor} size='small' shape='circle'>
            {t(status.labelKey)}
          </Tag>
          <span className='wb-card__id'>
            {task.id > 0 ? `#${task.id}` : t('刚提交')}
          </span>
        </div>

        {/* 提示词放在浅色细边框的盒子里，长提示词两行截断 */}
        <div className='wb-card__prompt-box'>
          {isFailure ? (
            <p className='wb-card__prompt wb-card__prompt--fail'>
              {task.failReason || t('生成失败')}
            </p>
          ) : (
            <p className='wb-card__prompt'>
              {task.prompt || task.model || t('（无提示词记录）')}
            </p>
          )}
        </div>

        {isPending && (
          <Progress
            percent={Math.max(task.progress || 0, 4)}
            showInfo={false}
            size='small'
            stroke='var(--semi-color-primary)'
          />
        )}

        <div className='wb-card__chips'>
          {chips.map((chip) => (
            <span key={chip} className='wb-chip'>
              {chip}
            </span>
          ))}
        </div>

        <div className='wb-card__foot'>
          {/* 只放时间本身：下方空间要留给右下角悬浮的操作按钮 */}
          <span
            className='wb-chip wb-chip--time'
            title={`${t('生成时间')} ${formatDateTime(task.createdAt)}`}
          >
            {formatDateTime(task.createdAt).slice(5)}
          </span>

          <div className='wb-card__actions' onClick={stop}>
            {onOpen && (
              <Tooltip content={t('查看任务详情')}>
                <Button
                  theme='borderless'
                  type='tertiary'
                  size='small'
                  icon={<Eye size={15} />}
                  aria-label={t('查看任务详情')}
                  onClick={handleOpen}
                />
              </Tooltip>
            )}
            {task.resultUrl && (
              <Tooltip content={t('下载')}>
                <Button
                  theme='borderless'
                  type='tertiary'
                  size='small'
                  icon={<Download size={15} />}
                  aria-label={t('下载')}
                  onClick={() =>
                    window.open(resolvePlaybackUrl(task), '_blank')
                  }
                />
              </Tooltip>
            )}
            {onRetry && (
              <Tooltip content={t('复用配置')}>
                <Button
                  theme='borderless'
                  type='tertiary'
                  size='small'
                  icon={<RotateCcw size={15} />}
                  aria-label={t('复用配置')}
                  onClick={() => onRetry(task)}
                />
              </Tooltip>
            )}
          </div>
        </div>
      </div>
    </article>
  );
};

export default TaskCard;
