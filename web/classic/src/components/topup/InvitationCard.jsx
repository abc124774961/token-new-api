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
import { Typography, Card, Button, Input, Tag } from '@douyinfe/semi-ui';
import { QRCodeSVG } from 'qrcode.react';
import {
  ArrowRight,
  CheckCircle2,
  Copy,
  Gift,
  Link2,
  QrCode,
  ReceiptText,
  ShieldCheck,
  Share2,
  Sparkles,
  Users,
  BarChart2,
  TrendingUp,
  Zap,
} from 'lucide-react';

const { Text } = Typography;

const InvitationMetric = ({ icon: Icon, label, value, tone }) => (
  <div className={`ct-topup-invite-metric ct-topup-invite-metric-${tone}`}>
    <span className='ct-topup-invite-metric-copy'>
      <em>{label}</em>
      <strong>{value}</strong>
    </span>
    <span className='ct-topup-invite-metric-icon'>
      <Icon size={16} />
    </span>
  </div>
);

const InvitationCard = ({
  t,
  userState,
  renderQuota,
  setOpenTransfer,
  affLink,
  handleAffLinkClick,
}) => {
  const canTransfer =
    userState?.user?.aff_quota && userState?.user?.aff_quota > 0;
  const affQuota = userState?.user?.aff_quota || 0;
  const affHistoryQuota = userState?.user?.aff_history_quota || 0;
  const affCount = userState?.user?.aff_count || 0;
  const inviteCode = affLink ? affLink.split('aff=').pop() : '--';
  const inviteRules = [
    t('好友通过你的邀请链接注册并完成邮箱验证'),
    t('好友首次充值并产生有效消费后可获得奖励'),
    t('奖励会在好友消费完成后自动计入邀请余额'),
    t('可划转奖励可随时划转到账户余额'),
  ];
  const steps = [
    { icon: Users, title: t('邀请好友'), desc: t('分享链接或二维码') },
    { icon: CheckCircle2, title: t('好友注册'), desc: t('完成账号注册') },
    { icon: ReceiptText, title: t('充值消费'), desc: t('产生有效消费') },
    { icon: Gift, title: t('奖励到账'), desc: t('奖励计入余额') },
  ];

  return (
    <div className='ct-topup-invite-page'>
      <div className='ct-topup-page-head'>
        <div>
          <div className='ct-topup-page-title-row'>
            <Typography.Title heading={2} className='ct-topup-page-title'>
              {t('邀请有奖')}
            </Typography.Title>
            <Tag
              color={canTransfer ? 'green' : 'cyan'}
              shape='circle'
              prefixIcon={<ShieldCheck size={12} />}
            >
              {canTransfer ? t('收益可划转') : t('等待收益')}
            </Tag>
          </div>
          <Text type='tertiary' className='ct-topup-page-subtitle'>
            {t('分享专属邀请链接，好友注册并消费后奖励自动计入邀请余额')}
          </Text>
        </div>
        <div className='ct-topup-page-actions'>
          <Button
            type='primary'
            theme='solid'
            onClick={handleAffLinkClick}
            icon={<Copy size={14} />}
            className='ct-topup-primary-button'
          >
            {t('复制邀请链接')}
          </Button>
          <Button
            theme='light'
            type='tertiary'
            disabled={!canTransfer}
            onClick={() => setOpenTransfer(true)}
            icon={<Zap size={14} />}
            className='ct-topup-panel-action'
          >
            {t('划转到余额')}
          </Button>
        </div>
      </div>

      <div className='ct-topup-invite-metrics ct-topup-invite-metrics-wide'>
        <InvitationMetric
          icon={TrendingUp}
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
          label={t('奖励状态')}
          value={canTransfer ? t('可划转') : t('待积累')}
          tone='blue'
        />
      </div>

      <div className='ct-topup-invite-grid'>
        <Card className='ct-topup-panel ct-topup-invite-link-panel'>
          <div className='ct-topup-section-heading'>
            <div>
              <strong>{t('邀请链接')}</strong>
              <span>{t('分享链接或二维码，好友注册后会自动绑定邀请关系')}</span>
            </div>
            <Tag shape='circle'>AFF {inviteCode}</Tag>
          </div>
          <div className='ct-topup-invite-link'>
            <Input
              value={affLink}
              readonly
              prefix={
                <span className='ct-topup-invite-link-prefix'>
                  <Share2 size={14} />
                  {t('邀请链接')}
                </span>
              }
              suffix={
                <Button
                  type='primary'
                  theme='solid'
                  onClick={handleAffLinkClick}
                  icon={<Copy size={14} />}
                  className='ct-topup-primary-button'
                >
                  {t('复制')}
                </Button>
              }
            />
          </div>
          <div className='ct-topup-share-actions'>
            <Button icon={<Share2 size={14} />} onClick={handleAffLinkClick}>
              {t('微信')}
            </Button>
            <Button icon={<Link2 size={14} />} onClick={handleAffLinkClick}>
              {t('复制链接')}
            </Button>
            <Button icon={<Sparkles size={14} />} onClick={handleAffLinkClick}>
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
          <div className='ct-topup-qr-box'>
            <QRCodeSVG
              value={affLink || window.location.origin}
              size={144}
              marginSize={1}
            />
          </div>
          <Button
            theme='light'
            type='tertiary'
            icon={<Copy size={14} />}
            onClick={handleAffLinkClick}
            className='ct-topup-panel-action'
          >
            {t('复制二维码链接')}
          </Button>
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
              <strong>{t('奖励记录')}</strong>
              <span>{t('后续接入明细接口后展示邀请用户、消费和奖励状态')}</span>
            </div>
            <Tag shape='circle'>{affCount} {t('人')}</Tag>
          </div>
          <div className='ct-topup-simple-table'>
            <div className='ct-topup-simple-table-head'>
              <span>{t('受邀用户')}</span>
              <span>{t('消费金额')}</span>
              <span>{t('奖励金额')}</span>
              <span>{t('状态')}</span>
            </div>
            <div className='ct-topup-empty-row'>
              <ReceiptText size={18} />
              <span>{t('暂无奖励明细记录')}</span>
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
          </Card>
        </div>
      </div>
    </div>
  );
};

export default InvitationCard;
