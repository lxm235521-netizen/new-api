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
import { Button, Spin } from '@douyinfe/semi-ui';
import { AudioLines, ImagePlus, Upload, Video, X } from 'lucide-react';
import { showError } from '../../../helpers';
import { uploadWorkbenchAsset } from '../upload';
import { formatFileSize } from '../utils';

const KIND_ICONS = {
  image: ImagePlus,
  audio: AudioLines,
  video: Video,
};

function matchesKind(file, kind) {
  return file.type.startsWith(`${kind}/`);
}

let assetSeq = 0;

/**
 * 参考素材上传区。
 *
 * 拖拽或点选文件后立即上传，拿到 URL 后再交给父级保存。
 */
const ReferenceUpload = ({
  kind,
  labelKey,
  hintKey,
  chooseLabelKey,
  accept,
  maxCount,
  maxBytes,
  assets,
  onChange,
  disabled = false,
}) => {
  const { t } = useTranslation();
  const inputRef = useRef(null);
  const previewUrlsRef = useRef(new Set());
  const assetsRef = useRef(assets);
  const [queued, setQueued] = useState([]);
  const [dragging, setDragging] = useState(false);

  // 上传是异步的，直接基于 props.assets 追加会在两次批次重叠时丢结果
  useEffect(() => {
    assetsRef.current = assets;
  }, [assets]);

  // 预览用的 object URL 必须回收
  useEffect(() => {
    const urls = previewUrlsRef.current;
    return () => {
      urls.forEach((url) => URL.revokeObjectURL(url));
      urls.clear();
    };
  }, []);

  const releasePreview = (url) => {
    URL.revokeObjectURL(url);
    previewUrlsRef.current.delete(url);
  };

  const KindIcon = KIND_ICONS[kind];
  const remaining = Math.max(maxCount - assets.length, 0);
  const isFull = remaining === 0;

  const handleFiles = async (fileList) => {
    if (!fileList || fileList.length === 0) return;

    const accepted = [];
    for (const file of Array.from(fileList)) {
      if (accepted.length >= remaining) {
        showError(t('最多可上传 {{count}} 个文件', { count: maxCount }));
        break;
      }
      if (!matchesKind(file, kind)) {
        showError(t('不支持的文件类型'));
        continue;
      }
      if (file.size > maxBytes) {
        showError(
          t('{{name}} 超过 {{size}} 上限', {
            name: file.name,
            size: formatFileSize(maxBytes),
          }),
        );
        continue;
      }
      const previewUrl = URL.createObjectURL(file);
      previewUrlsRef.current.add(previewUrl);
      assetSeq += 1;
      accepted.push({ id: `asset-${assetSeq}`, file, previewUrl });
    }

    if (accepted.length === 0) return;

    setQueued((prev) => [
      ...prev,
      ...accepted.map((item) => ({
        id: item.id,
        previewUrl: item.previewUrl,
      })),
    ]);

    const uploaded = [];
    await Promise.all(
      accepted.map(async (item) => {
        try {
          const result = await uploadWorkbenchAsset(item.file);
          uploaded.push({
            id: item.id,
            name: result.filename || item.file.name,
            url: result.url,
            size: result.size ?? item.file.size,
            // 保留原始文件：提示词优化需要的是 base64 data URL，
            // 从本地文件转可以绕开图床的 CORS 限制。
            file: item.file,
          });
        } catch (error) {
          showError(error?.message || t('上传失败'));
        } finally {
          releasePreview(item.previewUrl);
          setQueued((prev) => prev.filter((entry) => entry.id !== item.id));
        }
      }),
    );

    if (uploaded.length > 0) {
      const next = [...assetsRef.current, ...uploaded];
      assetsRef.current = next;
      onChange(next);
    }
  };

  const removeAsset = (id) => {
    const next = assets.filter((asset) => asset.id !== id);
    assetsRef.current = next;
    onChange(next);
  };

  return (
    <div className='wb-field'>
      <div className='wb-field__label'>
        <span>{t(labelKey)}</span>
        <span className='wb-field__hint'>
          {t('已上传 {{count}}/{{max}}', {
            count: assets.length,
            max: maxCount,
          })}
        </span>
      </div>

      <div
        className={[
          'wb-dropzone',
          dragging ? 'wb-dropzone--active' : '',
          disabled || isFull ? 'wb-dropzone--disabled' : '',
        ].join(' ')}
        onDragOver={(event) => {
          event.preventDefault();
          if (!disabled && !isFull) setDragging(true);
        }}
        onDragLeave={() => setDragging(false)}
        onDrop={(event) => {
          event.preventDefault();
          setDragging(false);
          if (disabled || isFull) return;
          handleFiles(event.dataTransfer.files);
        }}
      >
        <div className='wb-dropzone__inner'>
          <div className='wb-dropzone__icon'>
            <KindIcon size={16} aria-hidden='true' />
          </div>

          <div className='wb-dropzone__body'>
            <span className='wb-dropzone__title'>{t(hintKey)}</span>
            <span className='wb-dropzone__sub'>
              {t('拖拽文件至此，或从本机选择')}
            </span>
            <div>
              <Button
                theme='light'
                size='small'
                icon={<Upload size={14} />}
                disabled={disabled || isFull}
                onClick={() => inputRef.current?.click()}
              >
                {t(chooseLabelKey)}
              </Button>
            </div>
          </div>
        </div>

        <input
          ref={inputRef}
          type='file'
          multiple
          hidden
          accept={accept}
          onChange={(event) => {
            handleFiles(event.target.files);
            event.target.value = '';
          }}
        />
      </div>

      {(assets.length > 0 || queued.length > 0) && (
        <div className='wb-thumbs'>
          {assets.map((asset) => (
            <div key={asset.id} className='wb-thumb'>
              {kind === 'image' ? (
                <img src={asset.url} alt={asset.name} />
              ) : (
                <KindIcon size={18} aria-hidden='true' />
              )}
              <button
                type='button'
                aria-label={t('移除 {{name}}', { name: asset.name })}
                onClick={() => removeAsset(asset.id)}
                className='wb-thumb__remove'
              >
                <X size={12} aria-hidden='true' />
              </button>
            </div>
          ))}

          {queued.map((item) => (
            <div key={item.id} className='wb-thumb'>
              {kind === 'image' ? (
                <img src={item.previewUrl} alt='' />
              ) : (
                <KindIcon size={18} aria-hidden='true' />
              )}
              <span className='wb-thumb__mask'>
                <Spin size='small' />
              </span>
            </div>
          ))}
        </div>
      )}
    </div>
  );
};

export default ReferenceUpload;
