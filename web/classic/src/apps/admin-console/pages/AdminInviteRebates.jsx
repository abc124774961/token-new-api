import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Gift,
  RefreshCw,
  TrendingUp,
  UserCheck,
  Users,
  WalletCards,
} from 'lucide-react';
import { API, renderQuota, timestamp2string } from '../../../helpers';

const SAMPLE_SIZE = 200;

function formatNumber(value) {
  const numeric = Number(value);
  if (!Number.isFinite(numeric)) return '0';
  return new Intl.NumberFormat('zh-CN').format(numeric);
}

function buildInviteSummary(users, total) {
  const inviters = users.filter((user) => Number(user.aff_count || 0) > 0);
  const invitedUsers = users.filter((user) => Number(user.inviter_id || 0) > 0);
  const totalInviteCount = users.reduce(
    (sum, user) => sum + Number(user.aff_count || 0),
    0,
  );
  const totalRewardQuota = users.reduce(
    (sum, user) => sum + Number(user.aff_history_quota || 0),
    0,
  );
  const pendingQuota = users.reduce(
    (sum, user) => sum + Number(user.aff_quota || 0),
    0,
  );

  const topInviters = [...users]
    .filter(
      (user) =>
        Number(user.aff_count || 0) > 0 ||
        Number(user.aff_history_quota || 0) > 0,
    )
    .sort((a, b) => {
      const rewardDelta =
        Number(b.aff_history_quota || 0) - Number(a.aff_history_quota || 0);
      if (rewardDelta !== 0) return rewardDelta;
      return Number(b.aff_count || 0) - Number(a.aff_count || 0);
    })
    .slice(0, 8);

  return {
    sampleCount: users.length,
    totalUsers: total || users.length,
    inviters: inviters.length,
    invitedUsers: invitedUsers.length,
    totalInviteCount,
    totalRewardQuota,
    pendingQuota,
    topInviters,
  };
}

const MetricCard = ({ icon: Icon, label, value, helper, tone = '' }) => (
  <div className={`aurora-metric-card ${tone}`}>
    <span className='aurora-metric-icon'>
      <Icon size={20} />
    </span>
    <span>
      <span className='aurora-metric-label'>{label}</span>
      <strong className='aurora-metric-value'>{value}</strong>
      <small className='aurora-metric-helper'>{helper}</small>
    </span>
  </div>
);

const AdminInviteRebates = () => {
  const { t } = useTranslation();
  const [users, setUsers] = useState([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');

  const fetchUsers = useCallback(async () => {
    setLoading(true);
    setError('');
    try {
      const response = await API.get(
        `/api/user/?p=0&page_size=${SAMPLE_SIZE}`,
        {
          disableDuplicate: true,
          skipErrorHandler: true,
        },
      );
      if (response?.data?.success === false) {
        throw new Error(response?.data?.message || t('加载失败'));
      }
      const payload = response?.data?.data || {};
      setUsers(Array.isArray(payload.items) ? payload.items : []);
      setTotal(Number(payload.total || 0));
    } catch (err) {
      setUsers([]);
      setTotal(0);
      setError(err?.response?.data?.message || err?.message || t('加载失败'));
    } finally {
      setLoading(false);
    }
  }, [t]);

  useEffect(() => {
    fetchUsers();
  }, [fetchUsers]);

  const summary = useMemo(
    () => buildInviteSummary(users, total),
    [users, total],
  );

  return (
    <div className='aurora-admin-page aurora-invite-rebates-page'>
      <section className='aurora-overview-hero'>
        <div>
          <div className='aurora-overview-kicker'>{t('用户运营')}</div>
          <h1>{t('邀请返佣')}</h1>
          <p>
            {t(
              '跟踪邀请人、被邀请用户、历史奖励和待划转额度，辅助运营判断拉新质量。',
            )}
          </p>
          <div className='aurora-overview-meta'>
            <span>
              {t('样本数')} {formatNumber(summary.sampleCount)}
            </span>
            <span>
              {t('用户总数')} {formatNumber(summary.totalUsers)}
            </span>
          </div>
        </div>
        <div className='aurora-overview-status aurora-status-success'>
          <span>{t('邀请用户')}</span>
          <strong>{formatNumber(summary.invitedUsers)}</strong>
          <em>{t('当前样本')}</em>
          <button
            className='aurora-refresh-button'
            type='button'
            onClick={fetchUsers}
            disabled={loading}
          >
            <RefreshCw size={14} className={loading ? 'is-spinning' : ''} />
            {t('刷新')}
          </button>
        </div>
      </section>

      {error && <div className='aurora-inline-error'>{error}</div>}

      <section className='aurora-metric-grid'>
        <MetricCard
          icon={Users}
          label={t('邀请人')}
          value={formatNumber(summary.inviters)}
          helper={t('产生过邀请的用户')}
        />
        <MetricCard
          icon={UserCheck}
          label={t('邀请人数')}
          value={formatNumber(summary.totalInviteCount)}
          helper={t('用户累计邀请计数')}
          tone='aurora-tone-success'
        />
        <MetricCard
          icon={Gift}
          label={t('历史奖励')}
          value={renderQuota(summary.totalRewardQuota)}
          helper={t('邀请历史收益')}
          tone='aurora-tone-money'
        />
        <MetricCard
          icon={WalletCards}
          label={t('待划转额度')}
          value={renderQuota(summary.pendingQuota)}
          helper={t('仍在邀请余额中')}
          tone='aurora-tone-warning'
        />
      </section>

      <section className='aurora-overview-grid'>
        <div className='aurora-panel aurora-panel-main'>
          <div className='aurora-section-head'>
            <div>
              <h2>{t('邀请排行')}</h2>
              <p>{t('按历史奖励和邀请人数排序，定位主要拉新贡献用户。')}</p>
            </div>
          </div>
          <div className='aurora-simple-table-wrap'>
            <table className='aurora-simple-table aurora-invite-table'>
              <thead>
                <tr>
                  <th>{t('用户')}</th>
                  <th>{t('邀请人数')}</th>
                  <th>{t('历史奖励')}</th>
                  <th>{t('待划转额度')}</th>
                  <th>{t('邀请人ID')}</th>
                  <th>{t('注册时间')}</th>
                </tr>
              </thead>
              <tbody>
                {summary.topInviters.map((user) => (
                  <tr key={user.id}>
                    <td>
                      <strong>{user.username || '-'}</strong>
                      <small>ID {user.id}</small>
                    </td>
                    <td>{formatNumber(user.aff_count)}</td>
                    <td>{renderQuota(user.aff_history_quota || 0)}</td>
                    <td>{renderQuota(user.aff_quota || 0)}</td>
                    <td>{user.inviter_id || '-'}</td>
                    <td>
                      {user.created_at
                        ? timestamp2string(user.created_at)
                        : '-'}
                    </td>
                  </tr>
                ))}
                {summary.topInviters.length === 0 && (
                  <tr>
                    <td colSpan={6}>
                      <div className='aurora-empty-state'>
                        {loading ? t('加载中') : t('暂无邀请返佣数据')}
                      </div>
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>
        </div>

        <div className='aurora-panel'>
          <div className='aurora-section-head'>
            <div>
              <h2>{t('运营判断')}</h2>
              <p>{t('通过样本结构判断拉新质量和奖励沉淀。')}</p>
            </div>
          </div>
          <div className='aurora-risk-list'>
            <div className='aurora-risk-item aurora-tone-success'>
              <span className='aurora-risk-icon'>
                <TrendingUp size={18} />
              </span>
              <span className='aurora-risk-body'>
                <span className='aurora-risk-title'>
                  {t('转化观察')}
                  <em>{formatNumber(summary.invitedUsers)}</em>
                </span>
                <span className='aurora-risk-desc'>
                  {t('被邀请用户可继续结合消费明细判断真实转化。')}
                </span>
              </span>
            </div>
            <div className='aurora-risk-item aurora-tone-warning'>
              <span className='aurora-risk-icon'>
                <WalletCards size={18} />
              </span>
              <span className='aurora-risk-body'>
                <span className='aurora-risk-title'>
                  {t('余额沉淀')}
                  <em>{renderQuota(summary.pendingQuota)}</em>
                </span>
                <span className='aurora-risk-desc'>
                  {t('待划转额度较高时，可检查活动规则和用户触达。')}
                </span>
              </span>
            </div>
          </div>
        </div>
      </section>
    </div>
  );
};

export default AdminInviteRebates;
