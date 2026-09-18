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

import React from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@douyinfe/semi-ui';
import { Sparkles } from 'lucide-react';
import { renderQuota } from '../../../helpers';

/** 右栏底部固定条：预计消耗 + 提交 */
const SubmitBar = ({
  estimatedQuota,
  billingLabel,
  disabled,
  submitting,
  onSubmit,
}) => {
  const { t } = useTranslation();

  return (
    <div className='wb-submit'>
      <div className='wb-submit__row'>
        <div className='wb-submit__price'>
          <span className='wb-submit__price-label'>{t('预计消耗')}</span>
          <span className='wb-submit__price-value'>
            {renderQuota(estimatedQuota)}
          </span>
        </div>

        {billingLabel && (
          <span className='wb-submit__billing'>{billingLabel}</span>
        )}
      </div>

      <Button
        theme='solid'
        type='primary'
        block
        size='large'
        icon={<Sparkles size={15} />}
        loading={submitting}
        disabled={disabled || submitting}
        onClick={onSubmit}
      >
        {t('提交任务')}
      </Button>
    </div>
  );
};

export default SubmitBar;
