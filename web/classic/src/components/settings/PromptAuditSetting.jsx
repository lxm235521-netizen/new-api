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
import { Card, Spin } from '@douyinfe/semi-ui';
import SettingsPromptAudit from '../../pages/Setting/Operation/SettingsPromptAudit';
import { API, showError, toBoolean } from '../../helpers';

const PromptAuditSetting = () => {
  let [inputs, setInputs] = useState({
    'prompt_audit_setting.enabled': false,
    'prompt_audit_setting.users': '',
    'prompt_audit_setting.models': '',
    'prompt_audit_setting.max_bytes': 0,
    'prompt_audit_setting.skip_output': false,
  });

  let [loading, setLoading] = useState(false);

  const getOptions = async () => {
    const res = await API.get('/api/option/');
    const { success, message, data } = res.data;
    if (success) {
      let newInputs = { ...inputs };
      data.forEach((item) => {
        if (!Object.prototype.hasOwnProperty.call(inputs, item.key)) {
          return;
        }
        if (typeof inputs[item.key] === 'boolean') {
          newInputs[item.key] = toBoolean(item.value);
        } else {
          newInputs[item.key] = item.value;
        }
      });
      setInputs(newInputs);
    } else {
      showError(message);
    }
  };

  async function onRefresh() {
    try {
      setLoading(true);
      await getOptions();
    } catch (error) {
      showError('刷新失败');
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    onRefresh();
  }, []);

  return (
    <>
      <Spin spinning={loading} size='large'>
        {/* 请求审计 */}
        <Card style={{ marginTop: '10px' }}>
          <SettingsPromptAudit options={inputs} refresh={onRefresh} />
        </Card>
      </Spin>
    </>
  );
};

export default PromptAuditSetting;
