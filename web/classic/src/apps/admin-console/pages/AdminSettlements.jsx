import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button, Input, Modal, Toast } from '@douyinfe/semi-ui';
import { IconSearch } from '@douyinfe/semi-icons';
import {
  CheckCircle2,
  Clock3,
  CreditCard,
  ReceiptText,
  RefreshCw,
  WalletCards,
  XCircle,
} from 'lucide-react';
import { API, timestamp2string } from '../../../helpers';
import { AdminPermissionButton } from '../permissions/AdminPermissionAction';
import { ADMIN_PERMISSION_KEYS } from '../permissions/adminPermissions.config';

const PAGE_SIZE = 20;

const statusMeta = {
  success: { label: '成功', className: 'is-success', icon: CheckCircle2 },
  pending: { label: '待支付', className: 'is-warning', icon: Clock3 },
  failed: { label: '失败', className: 'is-danger', icon: XCircle },
  expired: { label: '已过期', className: 'is-danger', icon: XCircle },
};

const paymentMethodMap = {
  alipay: '支付宝',
  creem: 'Creem',
  stripe: 'Stripe',
  waffo: 'Waffo',
  waffo_pancake: 'Waffo Pancake',
  wxpay: '微信',
};

function formatNumber(value) {
  const numeric = Number(value);
  if (!Number.isFinite(numeric)) return '0';
  return new Intl.NumberFormat('zh-CN').format(numeric);
}

function formatMoney(value) {
  const numeric = Number(value);
  if (!Number.isFinite(numeric)) return '¥0.00';
  return `¥${numeric.toFixed(2)}`;
}

function getStatusMeta(status) {
  return (
    statusMeta[status] || {
      label: status || '未知状态',
      className: 'is-muted',
      icon: ReceiptText,
    }
  );
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

const StatusPill = ({ status, t }) => {
  const meta = getStatusMeta(status);
  const Icon = meta.icon;
  return (
    <span className={`aurora-status-pill ${meta.className}`}>
      <Icon size={13} />
      {t(meta.label)}
    </span>
  );
};

const AdminSettlements = () => {
  const { t } = useTranslation();
  const [records, setRecords] = useState([]);
  const [page, setPage] = useState(1);
  const [total, setTotal] = useState(0);
  const [keyword, setKeyword] = useState('');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');

  const loadRecords = useCallback(
    async (targetPage = page) => {
      setLoading(true);
      setError('');
      try {
        const query = new URLSearchParams({
          p: String(targetPage),
          page_size: String(PAGE_SIZE),
        });
        if (keyword.trim()) {
          query.set('keyword', keyword.trim());
        }
        const response = await API.get(`/api/user/topup?${query.toString()}`, {
          disableDuplicate: true,
          skipErrorHandler: true,
        });
        if (response?.data?.success === false) {
          throw new Error(response?.data?.message || t('加载失败'));
        }
        const payload = response?.data?.data || {};
        setRecords(Array.isArray(payload.items) ? payload.items : []);
        setTotal(Number(payload.total || 0));
        setPage(Number(payload.page || targetPage));
      } catch (err) {
        setRecords([]);
        setTotal(0);
        setError(err?.response?.data?.message || err?.message || t('加载失败'));
      } finally {
        setLoading(false);
      }
    },
    [keyword, page, t],
  );

  useEffect(() => {
    loadRecords(1);
  }, []);

  const summary = useMemo(() => {
    return records.reduce(
      (acc, record) => {
        const money = Number(record.money || 0);
        const amount = Number(record.amount || 0);
        acc.money += Number.isFinite(money) ? money : 0;
        acc.amount += Number.isFinite(amount) ? amount : 0;
        acc[record.status] = (acc[record.status] || 0) + 1;
        return acc;
      },
      { money: 0, amount: 0, success: 0, pending: 0, failed: 0, expired: 0 },
    );
  }, [records]);

  const confirmComplete = (tradeNo) => {
    Modal.confirm({
      title: t('确认补单'),
      content: t('是否将该订单标记为成功并为用户入账？'),
      onOk: async () => {
        try {
          const response = await API.post('/api/user/topup/complete', {
            trade_no: tradeNo,
          });
          if (response?.data?.success) {
            Toast.success({ content: t('补单成功') });
            await loadRecords(page);
          } else {
            Toast.error({
              content: response?.data?.message || t('补单失败'),
            });
          }
        } catch (err) {
          Toast.error({ content: t('补单失败') });
        }
      },
    });
  };

  const handleSearch = () => {
    setPage(1);
    loadRecords(1);
  };

  return (
    <div className='aurora-admin-page aurora-settlements-page'>
      <section className='aurora-overview-hero'>
        <div>
          <div className='aurora-overview-kicker'>{t('商业运营')}</div>
          <h1>{t('结算记录')}</h1>
          <p>
            {t('查看充值、订阅入账和人工补单状态，快速定位待支付或异常订单。')}
          </p>
          <div className='aurora-overview-meta'>
            <span>
              {t('记录总数')} {formatNumber(total)}
            </span>
            <span>
              {t('当前页')} {formatNumber(records.length)}
            </span>
          </div>
        </div>
        <div className='aurora-overview-status aurora-status-success'>
          <span>{t('订单状态')}</span>
          <strong>{formatNumber(summary.success)}</strong>
          <em>{t('当前页成功订单')}</em>
          <button
            className='aurora-refresh-button'
            type='button'
            onClick={() => loadRecords(page)}
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
          icon={CreditCard}
          label={t('支付金额')}
          value={formatMoney(summary.money)}
          helper={t('当前页订单汇总')}
          tone='aurora-tone-money'
        />
        <MetricCard
          icon={WalletCards}
          label={t('入账额度')}
          value={formatNumber(summary.amount)}
          helper={t('当前页充值额度')}
        />
        <MetricCard
          icon={Clock3}
          label={t('待支付')}
          value={formatNumber(summary.pending)}
          helper={t('可人工核对补单')}
          tone='aurora-tone-warning'
        />
        <MetricCard
          icon={XCircle}
          label={t('失败或过期')}
          value={formatNumber(summary.failed + summary.expired)}
          helper={t('需要运营复核')}
          tone='aurora-tone-danger'
        />
      </section>

      <section className='aurora-source-grid aurora-settlement-flow'>
        <div className='aurora-source-item is-ok'>
          <span className='aurora-source-state'>
            <CreditCard size={14} />
            {t('支付轨道')}
          </span>
          <strong>{t('金额与方式')}</strong>
          <small>{t('对照支付渠道、交易号和创建时间。')}</small>
        </div>
        <div className='aurora-source-item'>
          <span className='aurora-source-state'>
            <WalletCards size={14} />
            {t('入账对象')}
          </span>
          <strong>{t('用户与额度')}</strong>
          <small>{t('核对用户 ID、入账额度和充值来源。')}</small>
        </div>
        <div className='aurora-source-item'>
          <span className='aurora-source-state'>
            <ReceiptText size={14} />
            {t('订单状态')}
          </span>
          <strong>{t('成功/待支付/失败')}</strong>
          <small>{t('待支付可补单，失败或过期进入人工复核。')}</small>
        </div>
        <div className='aurora-source-item'>
          <span className='aurora-source-state'>
            <CheckCircle2 size={14} />
            {t('补单动作')}
          </span>
          <strong>{t('仅待支付订单')}</strong>
          <small>{t('人工补单前先核对支付平台流水。')}</small>
        </div>
      </section>

      <section className='aurora-panel'>
        <div className='aurora-section-head'>
          <div>
            <h2>{t('订单列表')}</h2>
            <p>{t('按订单号搜索充值、订阅和补单记录。')}</p>
          </div>
          <div className='aurora-table-toolbar'>
            <Input
              prefix={<IconSearch />}
              placeholder={t('订单号')}
              value={keyword}
              onChange={setKeyword}
              onEnterPress={handleSearch}
              showClear
            />
            <Button type='primary' onClick={handleSearch} loading={loading}>
              {t('查询')}
            </Button>
          </div>
        </div>

        <div className='aurora-simple-table-wrap'>
          <table className='aurora-simple-table aurora-settlements-table'>
            <thead>
              <tr>
                <th>{t('订单号')}</th>
                <th>{t('用户ID')}</th>
                <th>{t('支付方式')}</th>
                <th>{t('支付金额')}</th>
                <th>{t('入账额度')}</th>
                <th>{t('状态')}</th>
                <th>{t('创建时间')}</th>
                <th>{t('操作')}</th>
              </tr>
            </thead>
            <tbody>
              {records.map((record) => (
                <tr
                  className={`aurora-settlement-row is-${record.status || 'unknown'}`}
                  key={record.id}
                >
                  <td>
                    <span className='aurora-table-identity'>
                      <strong>{record.trade_no || '-'}</strong>
                      <small>
                        {t('内部ID')} {record.id}
                      </small>
                    </span>
                  </td>
                  <td>
                    <span className='aurora-table-identity'>
                      <strong>{record.user_id ?? '-'}</strong>
                      <small>{t('入账对象')}</small>
                    </span>
                  </td>
                  <td>
                    <span className='aurora-payment-chip'>
                      {t(
                        paymentMethodMap[record.payment_method] ||
                          record.payment_method ||
                          '-',
                      )}
                    </span>
                  </td>
                  <td>
                    <span className='aurora-table-number'>
                      {formatMoney(record.money)}
                    </span>
                  </td>
                  <td>
                    <span className='aurora-table-number'>
                      {formatNumber(record.amount)}
                    </span>
                  </td>
                  <td>
                    <StatusPill status={record.status} t={t} />
                  </td>
                  <td>
                    <span className='aurora-table-time'>
                      {record.create_time
                        ? timestamp2string(record.create_time)
                        : '-'}
                    </span>
                  </td>
                  <td>
                    {record.status === 'pending' ? (
                      <AdminPermissionButton
                        size='small'
                        theme='outline'
                        type='primary'
                        dangerPermission={
                          ADMIN_PERMISSION_KEYS.commercialSettlementComplete
                        }
                        fallbackTooltip={t(
                          '没有人工补单权限，请联系财务管理员或超级管理员。',
                        )}
                        onClick={() => confirmComplete(record.trade_no)}
                      >
                        {t('补单')}
                      </AdminPermissionButton>
                    ) : (
                      <span className='aurora-muted-text'>-</span>
                    )}
                  </td>
                </tr>
              ))}
              {records.length === 0 && (
                <tr>
                  <td colSpan={8}>
                    <div className='aurora-empty-state'>
                      {loading ? t('加载中') : t('暂无充值记录')}
                    </div>
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>

        <div className='aurora-table-pagination'>
          <Button
            disabled={loading || page <= 1}
            onClick={() => loadRecords(page - 1)}
          >
            {t('上一页')}
          </Button>
          <span>
            {t('第')} {formatNumber(page)} {t('页')}
          </span>
          <Button
            disabled={loading || page * PAGE_SIZE >= total}
            onClick={() => loadRecords(page + 1)}
          >
            {t('下一页')}
          </Button>
        </div>
      </section>
    </div>
  );
};

export default AdminSettlements;
