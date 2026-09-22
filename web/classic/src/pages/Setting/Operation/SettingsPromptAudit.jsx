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
import {
  Banner,
  Button,
  Col,
  Descriptions,
  Form,
  Row,
  Spin,
  Typography,
} from '@douyinfe/semi-ui';
import {
  compareObjects,
  API,
  showError,
  showSuccess,
  showWarning,
} from '../../../helpers';
import { useTranslation } from 'react-i18next';

const { Text } = Typography;

const FIELD_KEYS = {
  enabled: 'prompt_audit_setting.enabled',
  users: 'prompt_audit_setting.users',
  models: 'prompt_audit_setting.models',
  maxBytes: 'prompt_audit_setting.max_bytes',
  skipOutput: 'prompt_audit_setting.skip_output',
};

const defaultInputs = {
  [FIELD_KEYS.enabled]: false,
  [FIELD_KEYS.users]: '',
  [FIELD_KEYS.models]: '',
  [FIELD_KEYS.maxBytes]: 0,
  [FIELD_KEYS.skipOutput]: false,
};

export default function SettingsPromptAudit(props) {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(false);
  const [logDir, setLogDir] = useState('');
  const [inputs, setInputs] = useState(defaultInputs);
  const refForm = useRef();
  const [inputsRow, setInputsRow] = useState(defaultInputs);

  function handleFieldChange(fieldName) {
    return (value) => {
      setInputs((inputs) => ({ ...inputs, [fieldName]: value }));
    };
  }

  function onSubmit() {
    const updateArray = compareObjects(inputs, inputsRow);
    if (!updateArray.length) return showWarning(t('你似乎并没有修改什么'));
    const requestQueue = updateArray.map((item) =>
      API.put('/api/option/', {
        key: item.key,
        value: String(inputs[item.key]),
      }),
    );
    setLoading(true);
    Promise.all(requestQueue)
      .then((res) => {
        if (res.includes(undefined)) {
          return showError(t('部分保存失败，请重试'));
        }
        showSuccess(t('保存成功'));
        props.refresh();
      })
      .catch(() => {
        showError(t('保存失败，请重试'));
      })
      .finally(() => {
        setLoading(false);
      });
  }

  async function fetchLogDir() {
    try {
      const res = await API.get('/api/performance/logs');
      if (res.data.success && res.data.data) {
        setLogDir(res.data.data.log_dir || '');
      }
    } catch (error) {
      // 日志目录仅用于提示，获取失败时忽略
    }
  }

  useEffect(() => {
    const currentInputs = {};
    for (let key in props.options) {
      if (!Object.keys(inputs).includes(key)) {
        continue;
      }
      if (typeof inputs[key] === 'boolean') {
        currentInputs[key] =
          props.options[key] === 'true' || props.options[key] === true;
      } else if (typeof inputs[key] === 'number') {
        const parsed = parseInt(props.options[key], 10);
        currentInputs[key] = Number.isNaN(parsed) ? inputs[key] : parsed;
      } else {
        currentInputs[key] = props.options[key];
      }
    }
    const merged = { ...inputs, ...currentInputs };
    setInputs(merged);
    setInputsRow(merged);
    if (refForm.current) {
      refForm.current.setValues(merged);
    }
    fetchLogDir();
  }, [props.options]);

  return (
    <Spin spinning={loading}>
      <Form
        values={inputs}
        getFormApi={(formAPI) => (refForm.current = formAPI)}
        style={{ marginBottom: 15 }}
      >
        <Form.Section text={t('请求审计')}>
          <Banner
            type='warning'
            description={t(
              '命中「监控用户 + 监控模型」的请求，其输入与输出会以 JSONL 追加写入日志目录；未命中的请求不产生任何记录。审计内容包含用户原始输入，属于敏感数据，请控制文件访问权限与留存时间。',
            )}
            style={{ marginBottom: 16 }}
          />
          <Row gutter={16}>
            <Col xs={24} sm={12} md={8} lg={8} xl={8}>
              <Form.Switch
                field={FIELD_KEYS.enabled}
                label={t('启用请求审计')}
                extraText={t('仅记录匹配的请求，默认关闭')}
                size='default'
                checkedText='｜'
                uncheckedText='〇'
                onChange={handleFieldChange(FIELD_KEYS.enabled)}
              />
            </Col>
            <Col xs={24} sm={12} md={8} lg={8} xl={8}>
              <Form.Switch
                field={FIELD_KEYS.skipOutput}
                label={t('只记录输入')}
                extraText={t('开启后不记录模型返回与流式分片')}
                size='default'
                checkedText='｜'
                uncheckedText='〇'
                onChange={handleFieldChange(FIELD_KEYS.skipOutput)}
                disabled={!inputs[FIELD_KEYS.enabled]}
              />
            </Col>
            <Col xs={24} sm={12} md={8} lg={8} xl={8}>
              <Form.InputNumber
                field={FIELD_KEYS.maxBytes}
                label={t('单条上限 (字节)')}
                extraText={t('0 表示不截断，保留完整输入')}
                min={0}
                onChange={handleFieldChange(FIELD_KEYS.maxBytes)}
                disabled={!inputs[FIELD_KEYS.enabled]}
              />
            </Col>
          </Row>
          <Row gutter={16}>
            <Col xs={24} sm={12} md={12} lg={12} xl={12}>
              <Form.Input
                field={FIELD_KEYS.users}
                label={t('监控用户')}
                extraText={t('用户名或用户 ID，逗号分隔，留空表示所有用户')}
                placeholder='ccx,42'
                onChange={handleFieldChange(FIELD_KEYS.users)}
                showClear
                disabled={!inputs[FIELD_KEYS.enabled]}
              />
            </Col>
            <Col xs={24} sm={12} md={12} lg={12} xl={12}>
              <Form.Input
                field={FIELD_KEYS.models}
                label={t('监控模型')}
                extraText={t('支持 * 通配，不带 * 时按前缀匹配，留空表示所有模型')}
                placeholder='gemini-3.1-pro*'
                onChange={handleFieldChange(FIELD_KEYS.models)}
                showClear
                disabled={!inputs[FIELD_KEYS.enabled]}
              />
            </Col>
          </Row>
          {logDir ? (
            <Descriptions
              data={[{ key: t('审计文件目录'), value: logDir }]}
              style={{ marginBottom: 16 }}
            />
          ) : null}
          <Row>
            <Button size='default' onClick={onSubmit}>
              {t('保存请求审计设置')}
            </Button>
          </Row>
          <Row style={{ marginTop: 12 }}>
            <Text type='tertiary'>
              {t(
                '环境变量 PROMPT_AUDIT_* 显式设置时会覆盖此处的配置，便于应急开关。',
              )}
            </Text>
          </Row>
        </Form.Section>
      </Form>
    </Spin>
  );
}
