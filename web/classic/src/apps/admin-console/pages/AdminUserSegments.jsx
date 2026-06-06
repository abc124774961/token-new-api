import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Activity,
  Crown,
  RefreshCw,
  ShieldCheck,
  Users,
  WalletCards,
} from 'lucide-react';
import { API, renderQuota, timestamp2string } from '../../../helpers';

const SAMPLE_SIZE = 200;

const roleMeta = {
  1: { label: '普通用户', className: 'is-info' },
  10: { label: '管理员', className: 'is-warning' },
  100: { label: '超级管理员', className: 'is-danger' },
};

function formatNumber(value) {
  const numeric = Number(value);
  if (!Number.isFinite(numeric)) return '0';
  return new Intl.NumberFormat('zh-CN').format(numeric);
}

function getRoleMeta(role) {
  return roleMeta[role] || { label: '未知身份', className: 'is-muted' };
}

function isDeletedUser(user) {
  return user?.DeletedAt !== null && user?.DeletedAt !== undefined;
}

function buildSummary(users, total) {
  const nowSeconds = Math.floor(Date.now() / 1000);
  const thirtyDays = 30 * 24 * 60 * 60;
  const activeUsers = users.filter(
    (user) => user.status === 1 && !isDeletedUser(user),
  );
  const disabledUsers = users.filter(
    (user) => user.status !== 1 || isDeletedUser(user),
  );
  const adminUsers = users.filter((user) => Number(user.role) >= 10);
  const rootUsers = users.filter((user) => Number(user.role) >= 100);
  const recentLoginUsers = users.filter(
    (user) =>
      Number(user.last_login_at) > 0 &&
      nowSeconds - Number(user.last_login_at) <= thirtyDays,
  );
  const billableUsers = users.filter(
    (user) => Number(user.quota || 0) > 0 || Number(user.used_quota || 0) > 0,
  );

  const groups = users.reduce((acc, user) => {
    const groupName = user.group || 'default';
    acc[groupName] = (acc[groupName] || 0) + 1;
    return acc;
  }, {});

  const groupSegments = Object.entries(groups)
    .map(([name, count]) => ({
      name,
      count,
      percent: users.length ? Math.round((count / users.length) * 100) : 0,
    }))
    .sort((a, b) => b.count - a.count)
    .slice(0, 6);

  return {
    totalUsers: total || users.length,
    sampleCount: users.length,
    activeUsers: activeUsers.length,
    disabledUsers: disabledUsers.length,
    adminUsers: adminUsers.length,
    rootUsers: rootUsers.length,
    recentLoginUsers: recentLoginUsers.length,
    billableUsers: billableUsers.length,
    groupSegments,
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

const StatusPill = ({ children, className = '' }) => (
  <span className={`aurora-status-pill ${className}`}>{children}</span>
);

const AdminUserSegments = () => {
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
      const payload = response?.data?.data || {};
      if (response?.data?.success === false) {
        throw new Error(response?.data?.message || t('加载失败'));
      }
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

  const summary = useMemo(() => buildSummary(users, total), [users, total]);
  const recentUsers = useMemo(() => users.slice(0, 8), [users]);

  return (
    <div className='aurora-admin-page aurora-user-segments'>
      <section className='aurora-overview-hero'>
        <div>
          <div className='aurora-overview-kicker'>{t('用户运营')}</div>
          <h1>{t('用户分层')}</h1>
          <p>
            {t(
              '按角色、状态、分组和额度识别用户结构，辅助运营判断重点用户与异常账号。',
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
          <span>{t('数据来源')}</span>
          <strong>{t('用户列表')}</strong>
          <em>{t('实时抽样')}</em>
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
          label={t('有效用户')}
          value={formatNumber(summary.activeUsers)}
          helper={t('状态启用且未注销')}
        />
        <MetricCard
          icon={ShieldCheck}
          label={t('后台账号')}
          value={formatNumber(summary.adminUsers)}
          helper={`${t('超级管理员')} ${formatNumber(summary.rootUsers)}`}
          tone='aurora-tone-warning'
        />
        <MetricCard
          icon={Activity}
          label={t('近 30 天登录')}
          value={formatNumber(summary.recentLoginUsers)}
          helper={t('按最后登录时间统计')}
          tone='aurora-tone-success'
        />
        <MetricCard
          icon={WalletCards}
          label={t('有额度用户')}
          value={formatNumber(summary.billableUsers)}
          helper={`${t('禁用或注销')} ${formatNumber(summary.disabledUsers)}`}
          tone='aurora-tone-money'
        />
      </section>

      <section className='aurora-overview-grid'>
        <div className='aurora-panel aurora-panel-main'>
          <div className='aurora-section-head'>
            <div>
              <h2>{t('分组分布')}</h2>
              <p>{t('按用户分组统计当前样本占比。')}</p>
            </div>
          </div>
          <div className='aurora-segment-list'>
            {summary.groupSegments.map((segment) => (
              <div className='aurora-segment-row' key={segment.name}>
                <div className='aurora-segment-row-head'>
                  <strong>{segment.name}</strong>
                  <span>
                    {formatNumber(segment.count)} · {segment.percent}%
                  </span>
                </div>
                <div className='aurora-segment-bar'>
                  <span style={{ width: `${Math.max(segment.percent, 3)}%` }} />
                </div>
              </div>
            ))}
            {summary.groupSegments.length === 0 && (
              <div className='aurora-empty-state'>{t('暂无用户分组数据')}</div>
            )}
          </div>
        </div>

        <div className='aurora-panel'>
          <div className='aurora-section-head'>
            <div>
              <h2>{t('角色结构')}</h2>
              <p>{t('后台账号与普通用户边界。')}</p>
            </div>
          </div>
          <div className='aurora-role-stack'>
            {[1, 10, 100].map((role) => {
              const meta = getRoleMeta(role);
              const count = users.filter((user) => user.role === role).length;
              return (
                <div className='aurora-role-item' key={role}>
                  <span className='aurora-role-icon'>
                    {role === 100 ? <Crown size={18} /> : <Users size={18} />}
                  </span>
                  <span>
                    <strong>{t(meta.label)}</strong>
                    <small>
                      Role {role} · {formatNumber(count)}
                    </small>
                  </span>
                  <StatusPill className={meta.className}>
                    {formatNumber(count)}
                  </StatusPill>
                </div>
              );
            })}
          </div>
        </div>
      </section>

      <section className='aurora-panel'>
        <div className='aurora-section-head'>
          <div>
            <h2>{t('用户样本')}</h2>
            <p>{t('展示最新用户，用于快速确认分层是否符合预期。')}</p>
          </div>
        </div>
        <div className='aurora-simple-table-wrap'>
          <table className='aurora-simple-table'>
            <thead>
              <tr>
                <th>{t('用户')}</th>
                <th>{t('角色')}</th>
                <th>{t('分组')}</th>
                <th>{t('状态')}</th>
                <th>{t('剩余额度')}</th>
                <th>{t('最后登录')}</th>
              </tr>
            </thead>
            <tbody>
              {recentUsers.map((user) => {
                const meta = getRoleMeta(user.role);
                return (
                  <tr key={user.id}>
                    <td>
                      <strong>{user.username || '-'}</strong>
                      <small>ID {user.id}</small>
                    </td>
                    <td>
                      <StatusPill className={meta.className}>
                        {t(meta.label)}
                      </StatusPill>
                    </td>
                    <td>{user.group || 'default'}</td>
                    <td>
                      <StatusPill
                        className={
                          user.status === 1 && !isDeletedUser(user)
                            ? 'is-success'
                            : 'is-danger'
                        }
                      >
                        {user.status === 1 && !isDeletedUser(user)
                          ? t('已启用')
                          : t('已禁用')}
                      </StatusPill>
                    </td>
                    <td>{renderQuota(user.quota || 0)}</td>
                    <td>
                      {user.last_login_at
                        ? timestamp2string(user.last_login_at)
                        : '-'}
                    </td>
                  </tr>
                );
              })}
              {recentUsers.length === 0 && (
                <tr>
                  <td colSpan={6}>
                    <div className='aurora-empty-state'>
                      {loading ? t('加载中') : t('暂无用户数据')}
                    </div>
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      </section>
    </div>
  );
};

export default AdminUserSegments;
