import React, { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { API, copy, renderQuota, showError, showSuccess, timestamp2string } from '../../helpers';
import EditRedemptionModal from '../../components/table/redemptions/modals/EditRedemptionModal';
import { Button, Card, Form, Modal, Space, Table, Tag, Typography } from '@douyinfe/semi-ui';

const { Text, Title } = Typography;

const Audit = () => {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(false);
  const [data, setData] = useState({ items: [], total: 0 });
  const [stat, setStat] = useState({});
  const [page, setPage] = useState(1);
  const [filters, setFilters] = useState({ start: null, end: null, creatorId: '', creatorName: '', usedUserId: '', usedUserName: '', status: '' });
  const [selectedRows, setSelectedRows] = useState([]);
  const [editingRedemption, setEditingRedemption] = useState({ id: undefined });
  const [showCreate, setShowCreate] = useState(false);

  const query = (includePage = true) => {
    const params = new URLSearchParams();
    if (includePage) {
      params.set('p', page);
      params.set('page_size', 10);
    }
    if (filters.start) params.set('start_timestamp', Math.floor(filters.start.getTime() / 1000));
    if (filters.end) params.set('end_timestamp', Math.floor(filters.end.getTime() / 1000) + 86399);
    if (filters.creatorId) params.set('creator_id', filters.creatorId);
    if (filters.creatorName) params.set('creator_name', filters.creatorName);
    if (filters.usedUserId) params.set('used_user_id', filters.usedUserId);
    if (filters.usedUserName) params.set('used_user_name', filters.usedUserName);
    if (filters.status) params.set('status', filters.status);
    return params.toString();
  };

  const load = async () => {
    setLoading(true);
    try {
      const [listRes, statRes] = await Promise.all([
        API.get(`/api/redemption/audit/?${query(true)}`),
        API.get(`/api/redemption/audit/stat?${query(false)}`),
      ]);
      if (!listRes.data.success || !statRes.data.success) throw new Error(listRes.data.message || statRes.data.message);
      setData(listRes.data.data);
      setStat(statRes.data.data || {});
      setSelectedRows([]);
    } catch (e) {
      showError(e.message);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => { load(); }, [page, filters]);

  const refresh = () => load();

  const copyKeys = async (rows) => {
    if (rows.length === 0) {
      showError(t('请至少选择一个兑换码！'));
      return;
    }
    try {
      const response = await API.post('/api/redemption/audit/keys', {
        ids: rows.map((row) => row.id),
      });
      if (!response.data.success) throw new Error(response.data.message);
      const text = (response.data.data || [])
        .map((redemption) => redemption.key)
        .join('\n');
      if (!text) {
        showError(t('没有可复制的兑换码'));
        return;
      }
      if (await copy(text)) {
        showSuccess(t('已复制到剪贴板！'));
      } else {
        Modal.error({
          title: t('无法复制到剪贴板，请手动复制'),
          content: text,
          size: 'large',
        });
      }
    } catch (e) {
      showError(e.message);
    }
  };

  const deleteRows = (rows) => {
    if (rows.length === 0) {
      showError(t('请至少选择一个兑换码！'));
      return;
    }
    Modal.confirm({
      title: t('确认删除选中的兑换码？'),
      content: t('已使用的兑换码不会被删除，此操作不可撤销。'),
      onOk: async () => {
        setLoading(true);
        try {
          const response = await API.post('/api/redemption/audit/delete', {
            ids: rows.map((row) => row.id),
          });
          if (!response.data.success) throw new Error(response.data.message);
          const result = response.data.data || {};
          if (result.deleted_ids?.length) showSuccess(t('兑换码删除成功'));
          if (result.rejected?.length) {
            showError(t('部分兑换码无法删除，请检查其状态或权限'));
          }
          await load();
        } catch (e) {
          showError(e.message);
          setLoading(false);
        }
      },
    });
  };

  const columns = [
    { title: t('名称'), dataIndex: 'name' },
    { title: t('创建者 ID'), dataIndex: 'user_id' },
    { title: t('创建者账户'), dataIndex: 'creator_name', render: (value) => value || '-' },
    { title: t('额度'), dataIndex: 'quota', render: (value) => renderQuota(value) },
    { title: t('创建时间'), dataIndex: 'created_time', render: (value) => timestamp2string(value) },
    { title: t('状态'), dataIndex: 'status', render: (value) => {
      const labels = { 1: t('未使用'), 2: t('已禁用'), 3: t('已兑换') };
      const colors = { 1: 'blue', 2: 'orange', 3: 'green' };
      return <Tag color={colors[value] || 'grey'}>{labels[value] || t('未知')}</Tag>;
    } },
    { title: t('兑换时间'), dataIndex: 'redeemed_time', render: (value) => value ? timestamp2string(value) : '-' },
    { title: t('兑换用户 ID'), dataIndex: 'used_user_id', render: (value) => value || '-' },
    { title: t('兑换者账户'), dataIndex: 'used_user_name', render: (value) => value || '-' },
    { title: t('过期时间'), dataIndex: 'expired_time', render: (value) => value ? timestamp2string(value) : t('永不过期') },
    {
      title: t('操作'),
      render: (_, record) => (
        <Space>
          <Button size='small' onClick={() => copyKeys([record])}>{t('复制')}</Button>
          <Button size='small' type='danger' disabled={record.status === 3} onClick={() => deleteRows([record])}>{t('删除')}</Button>
        </Space>
      ),
    },
  ];

  const applyFilters = (values) => {
    setPage(1);
    setFilters({
      start: values.range?.[0] || null,
      end: values.range?.[1] || null,
      creatorId: values.creator_id || '',
      creatorName: values.creator_name || '',
      usedUserId: values.used_user_id || '',
      usedUserName: values.used_user_name || '',
      status: values.status || '',
    });
  };

  return (
    <div className='mt-[60px] px-2'>
      <div className='flex items-center justify-between mb-3'>
        <Title heading={4} className='m-0'>{t('兑换码管理')}</Title>
        <Space>
          <Button disabled={selectedRows.length === 0} onClick={() => copyKeys(selectedRows)}>{t('复制所选兑换码')}</Button>
          <Button type='danger' disabled={selectedRows.length === 0} onClick={() => deleteRows(selectedRows)}>{t('删除所选')}</Button>
          <Button theme='solid' onClick={() => { setEditingRedemption({ id: undefined }); setShowCreate(true); }}>{t('创建兑换码')}</Button>
        </Space>
      </div>
      <Card className='mb-3'>
        <Form layout='horizontal' onSubmit={applyFilters} initValues={{ status: '' }}>
          <Form.DatePicker field='range' type='dateRange' label={t('创建时间')} style={{ width: 260 }} />
          <Form.Input field='creator_id' label={t('创建者 ID')} placeholder={t('管理员可筛选')} />
          <Form.Input field='creator_name' label={t('创建者名称')} placeholder={t('用户名或显示名称')} />
          <Form.Input field='used_user_id' label={t('兑换者 ID')} placeholder={t('用户 ID')} />
          <Form.Input field='used_user_name' label={t('兑换者名称')} placeholder={t('用户名或显示名称')} />
          <Form.Select field='status' label={t('状态')} style={{ width: 130 }} optionList={[{ label: t('全部'), value: '' }, { label: t('未使用'), value: '1' }, { label: t('已兑换'), value: '3' }]} />
          <Button htmlType='submit' theme='solid'>{t('查询')}</Button>
        </Form>
      </Card>
      <Space wrap className='mb-3'>
        {[['created_quota', '创建总额度'], ['redeemed_quota', '已兑换使用总额度'], ['unused_quota', '未使用总额度']].map(([key, label]) => <Card key={key} style={{ minWidth: 180 }}><Text type='tertiary'>{t(label)}</Text><div className='text-xl font-semibold'>{renderQuota(stat[key] || 0)}</div></Card>)}
      </Space>
      <Table
        loading={loading}
        columns={columns}
        dataSource={data.items || []}
        rowSelection={{ onChange: (_, rows) => setSelectedRows(rows) }}
        pagination={{ currentPage: page, pageSize: 10, total: data.total || 0, onPageChange: setPage }}
        rowKey='id'
      />
      <EditRedemptionModal
        editingRedemption={editingRedemption}
        visiable={showCreate}
        handleClose={() => setShowCreate(false)}
        refresh={refresh}
        createUrl='/api/redemption/audit/create'
      />
    </div>
  );
};

export default Audit;
