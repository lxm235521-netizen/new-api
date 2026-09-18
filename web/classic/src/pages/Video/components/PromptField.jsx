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
import { Button, Modal, TextArea, Typography } from '@douyinfe/semi-ui';
import { LoaderCircle, Maximize2, Wand } from 'lucide-react';
import { showError } from '../../../helpers';
import { OPTIMIZE_MAX_IMAGES, optimizePrompt } from '../optimize';

const { Text } = Typography;

const PROMPT_PLACEHOLDER = '描述你想参考素材生成的内容…';

/** 提示词输入 + 放大编辑 + 提示词优化 */
const PromptField = ({
  value,
  onChange,
  optimize,
  referenceImages = [],
  tokenKey = '',
  disabled = false,
}) => {
  const { t } = useTranslation();
  const [expanded, setExpanded] = useState(false);
  const [draft, setDraft] = useState(value);
  const [optimizing, setOptimizing] = useState(false);
  const [optimizeMeta, setOptimizeMeta] = useState('');
  const sessionRef = useRef(null);
  // 记录优化前的内容，用于中断/失败时回退
  const originalRef = useRef('');

  // 组件卸载时中断进行中的流
  useEffect(() => {
    return () => sessionRef.current?.close?.();
  }, []);

  const openExpanded = () => {
    setDraft(value);
    setExpanded(true);
  };

  const applyDraft = () => {
    onChange(draft);
    setExpanded(false);
  };

  const optimizeModel = optimize?.enabled ? optimize.model : '';
  // 上游只接受 base64 data URL，因此传资源本身（含原始 File）给 optimize
  const attachedImages = (referenceImages || [])
    .filter((asset) => asset?.file)
    .slice(0, OPTIMIZE_MAX_IMAGES);
  const canOptimize =
    Boolean(optimizeModel) &&
    Boolean(tokenKey) &&
    value.trim() !== '' &&
    !disabled &&
    !optimizing;

  const stopOptimize = () => {
    sessionRef.current?.close?.();
    sessionRef.current = null;
    setOptimizing(false);
  };

  const runOptimize = () => {
    if (optimizing) {
      stopOptimize();
      return;
    }
    if (!optimizeModel) return;
    if (!tokenKey) {
      showError(t('请先选择用于调用的密钥'));
      return;
    }
    if (value.trim() === '') {
      showError(t('请先填写提示词'));
      return;
    }

    originalRef.current = value;
    setOptimizeMeta('');
    setOptimizing(true);

    sessionRef.current = optimizePrompt({
      tokenKey,
      model: optimizeModel,
      systemPrompt: optimize.systemPrompt,
      prompt: value,
      images: attachedImages,
      // 流式写入输入框，边生成边看
      onDelta: (text) => onChange(text),
      onDone: ({ text, meta }) => {
        sessionRef.current = null;
        setOptimizing(false);
        setOptimizeMeta(meta);
        if (text) {
          onChange(text);
        } else {
          // 一个字都没收到，回退原内容
          onChange(originalRef.current);
        }
      },
      onError: (message) => {
        sessionRef.current = null;
        setOptimizing(false);
        onChange(originalRef.current);
        showError(message || t('提示词优化失败'));
      },
    });
  };

  return (
    <div className='wb-field'>
      <div className='wb-field__label'>
        <span>{t('提示词')}</span>
        <span className='wb-field__actions'>
          {/* 优化按钮的显示/隐藏与调用模型完全由模型配置决定 */}
          {optimizeModel && (
            <Button
              theme='light'
              type='primary'
              size='small'
              icon={
                optimizing ? (
                  <LoaderCircle size={13} className='wb-spin' />
                ) : (
                  <Wand size={13} />
                )
              }
              disabled={!optimizing && !canOptimize}
              onClick={runOptimize}
            >
              {optimizing ? t('中断优化') : t('优化提示词')}
            </Button>
          )}
          <Button
            theme='borderless'
            size='small'
            icon={<Maximize2 size={13} />}
            disabled={disabled}
            onClick={openExpanded}
          >
            {t('放大编辑')}
          </Button>
        </span>
      </div>

      <TextArea
        value={value}
        onChange={onChange}
        disabled={disabled || optimizing}
        placeholder={t(PROMPT_PLACEHOLDER)}
        autosize={{ minRows: 5, maxRows: 12 }}
      />

      {optimizeModel && (
        <div className='wb-optimize-hint'>
          <span>
            {optimizing
              ? t('正在调用 {{model}} 优化…', { model: optimizeModel })
              : t('优化模型：{{model}}', { model: optimizeModel })}
          </span>
          {attachedImages.length > 0 && (
            <span>
              {t('附参考图 {{count}} 张', { count: attachedImages.length })}
            </span>
          )}
          {!tokenKey && <span>{t('需先选择密钥才能优化')}</span>}
        </div>
      )}

      {/* 上游返回的元信息（模式/时长/校验 + 说明）单独展示，不污染提示词 */}
      {optimizeMeta && !optimizing && (
        <div className='wb-optimize-meta'>
          <Text size='small' type='tertiary' className='whitespace-pre-wrap'>
            {optimizeMeta}
          </Text>
        </div>
      )}

      <Modal
        title={t('提示词')}
        visible={expanded}
        onCancel={() => setExpanded(false)}
        onOk={applyDraft}
        okText={t('应用')}
        cancelText={t('取消')}
        width={720}
      >
        <TextArea
          value={draft}
          onChange={setDraft}
          autosize={{ minRows: 14, maxRows: 22 }}
          placeholder={t(PROMPT_PLACEHOLDER)}
        />
      </Modal>
    </div>
  );
};

export default PromptField;
