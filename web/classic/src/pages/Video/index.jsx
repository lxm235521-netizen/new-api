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

/**
 * 视频生成工作台。
 *
 * 布局对应当前经典主题的两栏形态：左侧历史任务网格，右侧模型与参数面板。
 * 右侧所有参数项都由「工作台配置」里每个模型的配置驱动。
 *
 * 当前阶段：模型目录与参数已接后端；任务列表仍是内存占位。接线计划：
 *   - 历史任务 -> GET /api/task/self（轮询仅针对非终态任务）
 *   - 提交任务 -> POST /pg/videos（走控制台会话鉴权）
 *   - 预计消耗 -> 后端干跑预扣费计算
 *   - 删除/重试 -> 专用任务接口
 */

import React, {
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
} from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router-dom';
import { Button, Select } from '@douyinfe/semi-ui';
import { Clapperboard, KeyRound, Wallet } from 'lucide-react';
import { UserContext } from '../../context/User';
import {
  API,
  isAdmin,
  renderQuota,
  setUserData,
  showError,
  showSuccess,
} from '../../helpers';
import {
  fetchImageTask,
  fetchVideoTask,
  getUserTasks,
  getWorkbenchModels,
  submitImageTask,
  submitVideoTask,
} from './api';
import HistoryPanel from './components/HistoryPanel';
import ModelPanel from './components/ModelPanel';
import SubmitBar from './components/SubmitBar';
import TaskDetail from './components/TaskDetail';
import { BILLING_MODE_LABELS, HISTORY_PAGE_SIZE } from './constants';
import { useWorkbenchToken } from './hooks/useWorkbenchToken';
import { buildWorkbenchModels } from './modelCatalog';
import {
  buildImagePayload,
  buildInitialParams,
  buildSubmitPayload,
  effectiveBilling,
  estimateTaskQuota,
  filterTasks,
  isReferenceEnabled,
  isSubmitDisabled,
  isTerminalStatus,
  mapGatewayStatus,
  mapTaskDtoToTask,
  referenceMax,
} from './utils';
import './workbench.css';

/** 新建表单：参数取所选模型的第一个合法值 */
function createForm(model) {
  return {
    model: model.value,
    prompt: '',
    references: { image: [], audio: [], video: [] },
    ...buildInitialParams(model),
  };
}

let localTaskSeq = 0;

/** 顶栏副标题：跟着当前产出类型走，别在图片下还写着「生成视频」 */
const SUBTITLE_KEYS = {
  video: '使用提示词和参考素材生成视频',
  image: '使用提示词和参考素材生成图片',
};

const VideoWorkbench = () => {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const [userState, userDispatch] = useContext(UserContext);
  // 工作台的所有调用（生成视频/图片、提示词优化）都用用户自己的密钥
  const {
    tokens,
    token,
    loading: tokenLoading,
    selectToken,
  } = useWorkbenchToken();

  const [tasks, setTasks] = useState([]);
  const [totalTasks, setTotalTasks] = useState(0);
  const [page, setPage] = useState(1);
  const [models, setModels] = useState([]);
  const [loading, setLoading] = useState(true);
  const [form, setForm] = useState(null);
  const [mediaFilter, setMediaFilter] = useState('video');
  const [statusFilter, setStatusFilter] = useState('all');
  const [onlyCurrentModel, setOnlyCurrentModel] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [detailTask, setDetailTask] = useState(null);
  // 实时账户余额；null 表示还没拉到（先用本地缓存值兜着）
  const [balanceQuota, setBalanceQuota] = useState(null);

  /**
   * 刷新账户余额。
   *
   * 全局的 user 对象是 PageLayout 从 localStorage 读出来的「登录时快照」，
   * 生成扣费后不会自动更新 —— 直接读 user.quota 会一直显示登录那一刻的数字
   * （余额为 0 时登录的账号就永远是 0）。所以这里主动拉一次 /api/user/self，
   * 顺手写回全局与本地缓存，30 秒轮询一次。
   */
  const refreshBalance = useCallback(async () => {
    try {
      const res = await API.get('/api/user/self');
      const { success, data } = res.data;
      if (!success || !data) return;
      setBalanceQuota(data.quota ?? 0);
      if (userDispatch) {
        userDispatch({ type: 'login', payload: data });
      }
      setUserData(data);
    } catch (error) {
      // 拉取失败就继续用缓存值，不打扰用户
    }
  }, [userDispatch]);

  useEffect(() => {
    refreshBalance();
    const timer = setInterval(refreshBalance, 30000);
    return () => clearInterval(timer);
  }, [refreshBalance]);

  // 加载管理员配置的模型目录（含按当前用户解析的单价）
  useEffect(() => {
    let cancelled = false;
    getWorkbenchModels()
      .then((remote) => {
        if (cancelled) return;
        const next = buildWorkbenchModels(remote);
        setModels(next);
        if (next.length > 0) {
          setForm(createForm(next[0]));
        }
      })
      .catch((error) => {
        if (!cancelled) showError(error?.message || t('加载模型列表失败'));
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // 历史任务：取后端落库的记录（刷新后仍在）。
  // 分页、状态筛选、产出类型都在后端做，这样分页器的 total 与列表始终一致。
  const refreshHistory = useCallback(async () => {
    try {
      const { items, total } = await getUserTasks({
        p: page,
        page_size: HISTORY_PAGE_SIZE,
        mediaFilter,
        ...(statusFilter === 'all' ? {} : { status: statusFilter }),
      });
      setTotalTasks(total);
      setTasks((prev) => {
        const mapped = items.map((item) => mapTaskDtoToTask(item, models));
        // 网关返回的 task id 可能与 relay 落库的公开 id 不同，所以不能只按 id
        // 去重：再用「模型 + 提示词 + 时间接近」兜一层，否则刚提交的任务会
        // 同时出现本地卡片和服务端卡片。
        const isSameTask = (local, server) =>
          local.model === server.model &&
          local.prompt === server.prompt &&
          Math.abs((local.createdAt || 0) - (server.createdAt || 0)) < 120000;

        const localOnly = prev.filter(
          (task) =>
            task.isLocal &&
            !mapped.some(
              (item) => item.taskId === task.taskId || isSameTask(task, item),
            ),
        );
        return [...localOnly, ...mapped];
      });
    } catch (error) {
      showError(error?.message || t('加载历史任务失败'));
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [models, page, statusFilter, mediaFilter]);

  useEffect(() => {
    if (models.length === 0 && loading) return;
    refreshHistory();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [loading, models.length, page, statusFilter, mediaFilter]);

  // 轮询未结束的任务；终态不轮询。
  // 图片任务用自己的查询接口（/v1/images/tasks/:id），它没有上游任务可查。
  const pendingKey = useMemo(
    () =>
      tasks
        .filter((task) => !isTerminalStatus(task.status))
        .map(
          (task) =>
            `${task.kind === 'image' ? 'image' : 'video'}:${task.taskId}`,
        )
        .join(','),
    [tasks],
  );

  // 每 6 秒刷新一次任务进度。
  //
  // 一个循环里做两件事：
  //   1) 拉服务端历史 —— 这是跨页面/跨标签的“唯一事实来源”。切走再切回来时
  //      组件会重新挂载、本地状态丢失，只有靠它才能把进行中的任务找回来。
  //   2) 对进行中的任务按 id 单独查询 —— 进度比 relay 侧轮询更实时。
  useEffect(() => {
    if (loading) return undefined;

    let cancelled = false;

    const tick = async () => {
      if (cancelled) return;

      await refreshHistory();
      if (cancelled) return;

      const pending = pendingKey === '' ? [] : pendingKey.split(',');
      if (!token?.key || pending.length === 0) return;

      const updates = await Promise.all(
        pending.map(async (entry) => {
          const [kind, id] = entry.split(':');
          try {
            const data =
              kind === 'image'
                ? await fetchImageTask(token.key, id)
                : await fetchVideoTask(token.key, id);
            return { id, data };
          } catch {
            return null;
          }
        }),
      );
      if (cancelled) return;

      setTasks((prev) =>
        prev.map((task) => {
          const hit = updates.find((item) => item?.id === task.taskId);
          if (!hit) return task;
          return {
            ...task,
            status: mapGatewayStatus(hit.data.status),
            progress: hit.data.progress ?? task.progress,
            // 完成时直接用 video_url，不把视频下载到服务器
            resultUrl: hit.data.videoUrl || task.resultUrl,
            isLocal: false,
          };
        }),
      );
    };

    const timer = setInterval(tick, 6000);

    return () => {
      cancelled = true;
      clearInterval(timer);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [loading, pendingKey, token?.key, refreshHistory]);

  const model = useMemo(
    () => models.find((item) => item.value === form?.model),
    [models, form?.model],
  );

  // 当前 tab 允许的模型：视频只能选视频模型，图片只能选图片模型
  const modeModels = useMemo(
    () => models.filter((item) => item.group === mediaFilter),
    [models, mediaFilter],
  );

  const visibleTasks = useMemo(
    () =>
      filterTasks(tasks, {
        mediaFilter,
        model: onlyCurrentModel ? form?.model : undefined,
      }),
    [tasks, mediaFilter, onlyCurrentModel, form?.model],
  );

  const estimatedQuota = useMemo(
    () => estimateTaskQuota(model, form || {}),
    [model, form],
  );

  const submitDisabled = isSubmitDisabled(model, form || { references: {} });

  const billingLabel = model
    ? t(BILLING_MODE_LABELS[effectiveBilling(model)] || '')
    : '';

  const handleFormChange = (patch) => {
    setForm((prev) => {
      if (!prev) return prev;

      // 切换模型：参数取新模型的合法值，并清掉新模型不支持的参考素材
      if (patch.model && patch.model !== prev.model) {
        const nextModel = models.find((item) => item.value === patch.model);
        if (!nextModel) return prev;

        const references = { image: [], audio: [], video: [] };
        Object.keys(references).forEach((kind) => {
          if (isReferenceEnabled(nextModel, kind)) {
            references[kind] = prev.references[kind] || [];
          }
        });

        return {
          ...prev,
          model: nextModel.value,
          references,
          ...buildInitialParams(nextModel),
        };
      }

      return { ...prev, ...patch };
    });
  };

  // 提交到 /v1/videos（用户自己的密钥），随后由轮询跟进进度
  const handleSubmit = async () => {
    if (!model || !form) return;
    if (!token?.key) {
      showError(t('请先选择用于调用的密钥'));
      return;
    }

    setSubmitting(true);
    try {
      // 图片模型走同步上游（async 包一层），视频模型走任务接口
      const isImageModel = model.group === 'image';
      const created = isImageModel
        ? await submitImageTask(token.key, buildImagePayload(model, form))
        : await submitVideoTask(token.key, buildSubmitPayload(model, form));

      localTaskSeq += 1;
      const optimistic = {
        taskId: created.taskId || `task_local_${localTaskSeq}`,
        kind: model.group === 'image' ? 'image' : 'video',
        status: mapGatewayStatus(created.status),
        prompt: form.prompt,
        model: model.value,
        thumbnailUrl: form.references.image[0]?.url ?? '',
        resultUrl: '',
        durationSeconds: form.duration,
        aspectRatio: form.aspectRatio,
        resolution: form.resolution,
        createdAt: Date.now(),
        referenceImages: form.references.image.length,
        referenceAudios: form.references.audio.length,
        referenceVideos: form.references.video.length,
        quota: 0,
        progress: created.progress || 0,
        // 后端落库前先显示在列表里，刷新历史后由服务端记录取代
        isLocal: true,
      };

      setTasks((prev) => [optimistic, ...prev]);
      setForm((prev) => ({
        ...prev,
        prompt: '',
        references: { image: [], audio: [], video: [] },
      }));
      showSuccess(t('任务已提交'));
      // 新任务永远在最前面：回到第一页，稍等片刻再拉一次历史，
      // 让服务端记录尽早取代本地占位卡片；顺便刷新余额（提交时已预扣费）
      setPage(1);
      refreshBalance();
      setTimeout(() => {
        refreshHistory();
      }, 2000);
    } catch (error) {
      showError(error?.message || t('提交失败'));
    } finally {
      setSubmitting(false);
    }
  };

  /**
   * 把某个历史任务的配置载回右侧表单。
   *
   * 卡片上的「重新生成」和详情里的「复用配置」共用这一份逻辑：
   * 模型 + 提示词 + 时长/分辨率/比例 + 参考素材全量还原，
   * 新模型不支持的参数回落成该模型的默认值。
   */
  const loadTaskConfig = (task) => {
    const nextModel = models.find((item) => item.value === task.model) || model;
    if (!nextModel) {
      showError(t('该模型已不在工作台配置中，无法复用'));
      return;
    }

    const params = buildInitialParams(nextModel);
    const references = { image: [], audio: [], video: [] };
    Object.keys(references).forEach((kind) => {
      if (!isReferenceEnabled(nextModel, kind)) return;
      const limit = referenceMax(nextModel, kind);
      references[kind] = (task.references?.[kind] || [])
        .slice(0, limit)
        .map((url, index) => ({
          id: `${task.taskId}-${kind}-${index}`,
          name: url.split('/').pop() || url,
          url,
        }));
    });

    const durationValues = nextModel.duration?.values || [];
    setForm({
      model: nextModel.value,
      prompt: task.prompt || '',
      duration: durationValues.includes(task.durationSeconds)
        ? task.durationSeconds
        : params.duration,
      resolution: nextModel.resolutions.includes(task.resolution)
        ? task.resolution
        : params.resolution,
      aspectRatio: nextModel.aspectRatios.includes(task.aspectRatio)
        ? task.aspectRatio
        : params.aspectRatio,
      references,
    });
    setDetailTask(null);
    showSuccess(t('已载入该任务的配置'));
  };

  const quota = balanceQuota ?? userState?.user?.quota ?? 0;

  /**
   * 切换产出类型（视频 / 图片）。
   *
   * 两组模型和参数完全不同：切过去时把表单的模型换成该组第一个，并回到第一页，
   * 模型下拉也只列出当前这一组。
   */
  const handleModeChange = (mode) => {
    if (mode === mediaFilter) return;
    setMediaFilter(mode);
    setPage(1);

    const groupModels = models.filter((item) => item.group === mode);
    if (groupModels.length > 0) {
      setForm((prev) =>
        prev ? { ...prev, model: groupModels[0].value } : prev,
      );
    }
  };

  return (
    <div className='wb-root'>
      {/* 顶栏 */}
      <div className='wb-topbar'>
        <div className='min-w-0'>
          <div className='wb-topbar__title'>{t('工作台')}</div>
          <div className='wb-topbar__sub'>{t(SUBTITLE_KEYS[mediaFilter])}</div>
        </div>

        <div className='wb-topbar__actions'>
          {/* 密钥选择：进入工作台先选密钥，使用中可随时切换 */}
          {tokens.length > 0 ? (
            <Select
              value={token?.id}
              onChange={selectToken}
              optionList={tokens.map((item) => ({
                label: item.name,
                value: item.id,
              }))}
              prefix={<KeyRound size={13} />}
              placeholder={t('选择密钥')}
              style={{ width: 190 }}
              size='small'
            />
          ) : (
            !tokenLoading && (
              <Button
                theme='light'
                type='warning'
                size='small'
                icon={<KeyRound size={13} />}
                onClick={() => navigate('/console/token')}
              >
                {t('先创建密钥')}
              </Button>
            )
          )}

          <span className='wb-balance'>
            <Wallet size={14} aria-hidden='true' />
            <span className='wb-balance__label'>{t('账户余额')}</span>
            <span className='wb-balance__value'>{renderQuota(quota)}</span>
          </span>
          <Button
            theme='light'
            type='primary'
            size='small'
            onClick={() => navigate('/console/topup')}
          >
            {t('充值')}
          </Button>
        </div>
      </div>

      {/* 两栏：左侧历史任务 + 右侧模型参数 */}
      <div className='wb-body'>
        <HistoryPanel
          tasks={visibleTasks}
          total={totalTasks || tasks.length}
          page={page}
          pageSize={HISTORY_PAGE_SIZE}
          onPageChange={setPage}
          mediaFilter={mediaFilter}
          statusFilter={statusFilter}
          onlyCurrentModel={onlyCurrentModel}
          onMediaFilterChange={handleModeChange}
          onStatusFilterChange={(value) => {
            setStatusFilter(value);
            // 换筛选条件必须回到第一页，否则可能停在一个空页上
            setPage(1);
          }}
          onOnlyCurrentModelChange={setOnlyCurrentModel}
          onRetryTask={loadTaskConfig}
          onOpenTask={setDetailTask}
        />

        <div className='wb-side'>
          <div className='wb-side__scroll'>
            {!loading && modeModels.length === 0 ? (
              <div className='wb-empty'>
                <span className='wb-empty__icon'>
                  <Clapperboard size={22} aria-hidden='true' />
                </span>
                <span className='wb-empty__title'>
                  {t('管理员尚未配置工作台模型')}
                </span>
                {isAdmin() && (
                  <Button
                    theme='light'
                    type='primary'
                    size='small'
                    onClick={() => navigate('/console/workbench')}
                  >
                    {t('前往工作台配置')}
                  </Button>
                )}
              </div>
            ) : (
              form && (
                <ModelPanel
                  models={modeModels}
                  model={model}
                  form={form}
                  onChange={handleFormChange}
                  billingLabel={
                    model
                      ? BILLING_MODE_LABELS[effectiveBilling(model)] || ''
                      : ''
                  }
                  tokenKey={token?.key || ''}
                />
              )
            )}
          </div>

          <SubmitBar
            estimatedQuota={estimatedQuota}
            billingLabel={billingLabel}
            disabled={submitDisabled || modeModels.length === 0}
            submitting={submitting}
            onSubmit={handleSubmit}
          />
        </div>
      </div>

      <TaskDetail
        task={detailTask}
        visible={detailTask !== null}
        onClose={() => setDetailTask(null)}
        onReuse={loadTaskConfig}
      />
    </div>
  );
};

export default VideoWorkbench;
