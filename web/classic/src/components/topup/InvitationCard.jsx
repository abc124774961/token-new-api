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
import { Card, Button, Input, Tag } from '@douyinfe/semi-ui';
import { QRCodeSVG } from 'qrcode.react';
import { SiWechat } from 'react-icons/si';
import {
  ArrowRight,
  CheckCircle2,
  ChevronLeft,
  ChevronRight,
  Copy,
  DollarSign,
  Gift,
  Link2,
  MoreHorizontal,
  QrCode,
  ReceiptText,
  Sparkles,
  Users,
  BarChart2,
  Zap,
} from 'lucide-react';
import { timestamp2string } from '../../helpers';

const InvitationMetric = ({ icon: Icon, label, value, tone }) => (
  <div className={`ct-topup-invite-metric ct-topup-invite-metric-${tone}`}>
    <span className='ct-topup-invite-metric-icon'>
      <Icon size={16} />
    </span>
    <span className='ct-topup-invite-metric-copy'>
      <em>{label}</em>
      <strong>{value}</strong>
    </span>
  </div>
);

const InvitationCard = ({
  t,
  userState,
  renderQuota,
  setOpenTransfer,
  affLink,
  affiliateDashboard,
  affiliateLoading,
  affiliatePage,
  onAffiliatePageChange,
  handleAffLinkClick,
}) => {
  const summary = affiliateDashboard?.summary || {};
  const pagination = affiliateDashboard?.pagination || {};
  const rewardRecords = affiliateDashboard?.records || [];
  const canTransfer =
    (summary.aff_quota ?? userState?.user?.aff_quota) &&
    (summary.aff_quota ?? userState?.user?.aff_quota) > 0;
  const affQuota = summary.aff_quota ?? userState?.user?.aff_quota ?? 0;
  const affHistoryQuota =
    summary.aff_history_quota ?? userState?.user?.aff_history_quota ?? 0;
  const affCount = summary.aff_count ?? userState?.user?.aff_count ?? 0;
  const conversionRate = `${Number(summary.conversion_rate || 0).toFixed(1)}%`;
  const recordTotal = pagination.total ?? affCount;
  const pageSize = pagination.page_size || 10;
  const currentPage = pagination.page || affiliatePage || 1;
  const totalPages = Math.max(1, Math.ceil(recordTotal / pageSize));
  const visiblePages = Array.from(
    { length: Math.min(totalPages, 3) },
    (_, index) => {
      if (totalPages <= 3) return index + 1;
      if (currentPage <= 2) return index + 1;
      if (currentPage >= totalPages - 1) return totalPages - 2 + index;
      return currentPage - 1 + index;
    },
  );
  const displayAffLink =
    affLink ||
    (userState?.user?.aff_code
      ? `${window.location.origin}/register?aff=${userState.user.aff_code}`
      : `${window.location.origin}/register`);
  const inviteRules = [
    t('好友通过你的邀请链接注册后会自动绑定邀请关系'),
    t('邀请奖励按站点配置在注册后自动计入邀请余额'),
    t('消费和充值状态按受邀用户的真实账户数据统计'),
    t('可划转奖励可随时划转到账户余额'),
  ];
  const steps = [
    { icon: Users, title: t('邀请好友'), desc: t('分享链接或二维码') },
    { icon: CheckCircle2, title: t('好友注册'), desc: t('完成账号注册') },
    { icon: ReceiptText, title: t('真实统计'), desc: t('跟踪消费和充值') },
    { icon: Gift, title: t('奖励到账'), desc: t('奖励计入余额') },
  ];
  const renderRecordStatus = (record) =>
    record.status === 'converted'
      ? { label: t('已转化'), color: 'green' }
      : { label: t('已注册'), color: 'cyan' };
  const renderRecordName = (record) =>
    record.display_name || record.username || record.email || `#${record.user_id}`;
  const renderRecordSubline = (record) => {
    if (record.email && record.email !== renderRecordName(record)) {
      return record.email;
    }
    return `${t('请求次数')}: ${record.request_count || 0}`;
  };

  return (
    <div className='ct-topup-invite-page'>
      <div className='ct-topup-invite-metrics ct-topup-invite-metrics-wide'>
        <InvitationMetric
          icon={DollarSign}
          label={t('可划转奖励')}
          value={renderQuota(affQuota)}
          tone='teal'
        />
        <InvitationMetric
          icon={BarChart2}
          label={t('累计收益')}
          value={renderQuota(affHistoryQuota)}
          tone='blue'
        />
        <InvitationMetric
          icon={Users}
          label={t('邀请人数')}
          value={affCount}
          tone='green'
        />
        <InvitationMetric
          icon={Sparkles}
          label={t('注册转化率')}
          value={conversionRate}
          tone='violet'
        />
      </div>

      <div className='ct-topup-invite-grid'>
        <Card className='ct-topup-panel ct-topup-invite-link-panel'>
          <div className='ct-topup-section-heading'>
            <div>
              <strong>{t('邀请链接')}</strong>
              <span>{t('分享链接或二维码，好友注册后会自动绑定邀请关系')}</span>
            </div>
            <Tag shape='circle' className='ct-topup-aff-tag'>
              AFF
              {userState?.user?.aff_code
                ? `-${userState.user.aff_code.slice(0, 2).toUpperCase()}`
                : '--'}
            </Tag>
          </div>
          <div className='ct-topup-invite-link'>
            <Input
              value={displayAffLink}
              placeholder={t('邀请链接')}
              readonly
              suffix={
                <Button
                  theme='borderless'
                  type='tertiary'
                  onClick={handleAffLinkClick}
                  icon={<Copy size={14} />}
                  aria-label={t('复制邀请链接')}
                  className='ct-topup-input-copy-button'
                />
              }
            />
          </div>
          <div className='ct-topup-share-actions'>
            <Button
              type='primary'
              theme='solid'
              icon={<Copy size={14} />}
              onClick={handleAffLinkClick}
              className='ct-topup-primary-button ct-topup-share-primary'
            >
              {t('复制邀请链接')}
            </Button>
            <Button icon={<SiWechat size={14} />} onClick={handleAffLinkClick}>
              {t('微信')}
            </Button>
            <Button icon={<Link2 size={14} />} onClick={handleAffLinkClick}>
              {t('复制链接')}
            </Button>
            <Button
              icon={<Sparkles size={14} />}
              onClick={handleAffLinkClick}
              className='ct-topup-share-shortlink'
            >
              {t('生成短链')}
            </Button>
          </div>
        </Card>

        <Card className='ct-topup-panel ct-topup-qr-panel'>
          <div className='ct-topup-section-heading'>
            <div>
              <strong>{t('邀请二维码')}</strong>
              <span>{t('用于社群、文档或客服场景分享')}</span>
            </div>
            <QrCode size={18} />
          </div>
          <div className='ct-topup-qr-content'>
            <div className='ct-topup-qr-box'>
              <QRCodeSVG
                value={displayAffLink || window.location.origin}
                size={144}
                marginSize={1}
              />
            </div>
            <div className='ct-topup-qr-copy'>
              <strong>{t('使用二维码分享')}</strong>
              <span>{t('适合社群、文档和客服场景')}</span>
            </div>
          </div>
        </Card>
      </div>

      <Card className='ct-topup-panel ct-topup-invite-steps'>
        {steps.map((step, index) => {
          const Icon = step.icon;
          return (
            <React.Fragment key={step.title}>
              <div className='ct-topup-step-item'>
                <span className='ct-topup-step-index'>{index + 1}</span>
                <span className='ct-topup-step-icon'>
                  <Icon size={22} />
                </span>
                <span>
                  <strong>{step.title}</strong>
                  <em>{step.desc}</em>
                </span>
              </div>
              {index < steps.length - 1 && (
                <ArrowRight className='ct-topup-step-arrow' size={20} />
              )}
            </React.Fragment>
          );
        })}
      </Card>

      <div className='ct-topup-invite-bottom-grid'>
        <Card className='ct-topup-panel ct-topup-record-panel'>
          <div className='ct-topup-section-heading'>
            <div>
              <strong>{t('邀请记录')}</strong>
              <span>{t('展示真实受邀用户、消费和充值状态')}</span>
            </div>
            <Tag shape='circle'>{recordTotal} {t('条')}</Tag>
          </div>
          <div className='ct-topup-simple-table'>
            <div className='ct-topup-simple-table-head ct-topup-affiliate-table-head'>
              <span>{t('受邀用户')}</span>
              <span>{t('注册时间')}</span>
              <span>{t('已消费额度')}</span>
              <span>{t('成功充值')}</span>
              <span>{t('状态')}</span>
            </div>
            {rewardRecords.length > 0 ? (
              rewardRecords.map((record) => {
                const status = renderRecordStatus(record);
                return (
                  <div
                    className='ct-topup-simple-table-row ct-topup-affiliate-table-row'
                    key={record.user_id}
                  >
                    <span className='ct-topup-affiliate-user'>
                      <i>{record.initial || 'U'}</i>
                      <b>
                        <strong>{renderRecordName(record)}</strong>
                        <em>{renderRecordSubline(record)}</em>
                      </b>
                    </span>
                    <span>
                      {record.registered_at
                        ? timestamp2string(record.registered_at)
                        : '--'}
                    </span>
                    <span>{renderQuota(record.used_quota || 0)}</span>
                    <span>
                      {Number(record.topup_money || 0).toFixed(2)}
                      {record.topup_count > 0 ? ` / ${record.topup_count}` : ''}
                    </span>
                    <span>
                      <Tag shape='circle' color={status.color}>
                        {status.label}
                      </Tag>
                    </span>
                  </div>
                );
              })
            ) : (
              <div className='ct-topup-empty-row ct-topup-affiliate-empty-state'>
                <Users size={18} />
                <span>
                  {affiliateLoading ? t('加载中') : t('暂无真实邀请记录')}
                </span>
              </div>
            )}
            <div className='ct-topup-record-footer'>
              <span>
                {t('共')} {recordTotal} {t('条记录')}
              </span>
              <div className='ct-topup-record-pager'>
                <button type='button' disabled>{`${pageSize} ${t('条/页')}`}</button>
                <button
                  type='button'
                  disabled={currentPage <= 1 || affiliateLoading}
                  aria-label={t('上一页')}
                  onClick={() => onAffiliatePageChange(currentPage - 1)}
                >
                  <ChevronLeft size={14} />
                </button>
                {visiblePages.map((page) => (
                  <button
                    type='button'
                    key={page}
                    disabled={affiliateLoading}
                    className={
                      page === currentPage ? 'ct-topup-record-page-active' : ''
                    }
                    onClick={() => onAffiliatePageChange(page)}
                  >
                    {page}
                  </button>
                ))}
                <button
                  type='button'
                  disabled={currentPage >= totalPages || affiliateLoading}
                  aria-label={t('下一页')}
                  onClick={() => onAffiliatePageChange(currentPage + 1)}
                >
                  <ChevronRight size={14} />
                </button>
              </div>
            </div>
          </div>
        </Card>

        <div className='ct-topup-invite-side-stack'>
          <Card className='ct-topup-panel ct-topup-rule-card'>
            <div className='ct-topup-section-heading'>
              <div>
                <strong>{t('奖励规则')}</strong>
                <span>{t('奖励会根据站点配置自动计算')}</span>
              </div>
            </div>
            <div className='ct-topup-rule-list'>
              {inviteRules.map((rule) => (
                <div key={rule}>
                  <CheckCircle2 size={15} />
                  <span>{rule}</span>
                </div>
              ))}
            </div>
          </Card>

          <Card className='ct-topup-panel ct-topup-transfer-card'>
            <div>
              <span>{t('可划转奖励')}</span>
              <strong>{renderQuota(affQuota)}</strong>
              <p>{t('划转后将进入账户余额，可用于充值抵扣和 API 消费')}</p>
            </div>
            <Button
              type='primary'
              theme='solid'
              disabled={!canTransfer}
              onClick={() => setOpenTransfer(true)}
              icon={<Zap size={14} />}
              className='ct-topup-primary-button'
            >
              {t('立即划转')}
            </Button>
            <MoreHorizontal className='ct-topup-transfer-dots' size={18} />
          </Card>
        </div>
      </div>
    </div>
  );
};

export default InvitationCard;
