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

import React, { useState } from 'react';
import { Avatar, Button, Card, Spin, Typography } from '@douyinfe/semi-ui';
import { ExternalLink, ShoppingCart } from 'lucide-react';

const ExternalRechargeCard = ({ t, topUpLink }) => {
  const [loading, setLoading] = useState(true);

  return (
    <Card className='!rounded-2xl shadow-sm border-0 w-full'>
      <div className='flex items-center justify-between gap-3 mb-4'>
        <div className='flex items-center min-w-0'>
          <Avatar size='small' color='orange' className='mr-3 shadow-md'>
            <ShoppingCart size={16} />
          </Avatar>
          <div className='min-w-0'>
            <Typography.Text className='text-lg font-medium'>
              {t('在线自主充值')}
            </Typography.Text>
          </div>
        </div>
        <Button
          icon={<ExternalLink size={16} />}
          theme='solid'
          onClick={() => window.open(topUpLink, '_blank', 'noopener,noreferrer')}
        >
          {t('在新标签页中打开')}
        </Button>
      </div>

      <div
        className='relative overflow-hidden border rounded-lg bg-white'
        style={{ height: 720, borderColor: 'var(--semi-color-border)' }}
      >
        {loading && (
          <div className='absolute inset-0 z-10 flex items-center justify-center bg-white'>
            <Spin tip={t('正在加载充值页面...')} />
          </div>
        )}
        <iframe
          title={t('在线自主充值')}
          src={topUpLink}
          className='block w-full h-full border-0'
          onLoad={() => setLoading(false)}
        />
      </div>
    </Card>
  );
};

export default ExternalRechargeCard;
