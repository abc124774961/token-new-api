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
import {
  Copy,
  Users,
  BarChart2,
  TrendingUp,
  Gift,
  Zap,
  Share2,
} from 'lucide-react';

const { Text } = Typography;

const InvitationMetric = ({ icon: Icon, label, value, tone }) => (
  <div className={`ct-topup-invite-metric ct-topup-invite-metric-${tone}`}>
    <span className='ct-topup-invite-metric-icon'>
      <Icon size={16} />
    </span>
    <span>
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
  handleAffLinkClick,
}) => {
  const canTransfer =
    userState?.user?.aff_quota && userState?.user?.aff_quota > 0;

  return (
    <Card className='ct-topup-invite-hero'>
      <div className='ct-topup-invite-main'>
        <div className='ct-topup-title-wrap'>
          <div className='ct-topup-invite-logo'>
            <Gift size={20} />
          </div>
          <div>
            <div className='ct-topup-panel-kicker'>{t('邀请奖励')}</div>
            <Typography.Title heading={3} className='ct-topup-hero-title'>
              {t('邀请好友获得额外奖励')}
            </Typography.Title>
            <div className='ct-topup-panel-subtitle'>
              {t('邀请好友注册，好友充值后您可获得相应奖励')}
            </div>
          </div>
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
      </div>

      <div className='ct-topup-invite-side'>
        <div className='ct-topup-invite-metrics'>
          <InvitationMetric
            icon={TrendingUp}
            label={t('待使用收益')}
            value={renderQuota(userState?.user?.aff_quota || 0)}
            tone='teal'
          />
          <InvitationMetric
            icon={BarChart2}
            label={t('总收益')}
            value={renderQuota(userState?.user?.aff_history_quota || 0)}
            tone='blue'
          />
          <InvitationMetric
            icon={Users}
            label={t('邀请人数')}
            value={userState?.user?.aff_count || 0}
            tone='green'
          />
        </div>
        <div className='ct-topup-invite-actions'>
          <Button
            type='primary'
            theme='solid'
            disabled={!canTransfer}
            onClick={() => setOpenTransfer(true)}
            icon={<Zap size={14} />}
            className='ct-topup-primary-button'
          >
            {t('划转到余额')}
          </Button>
          <div className='ct-topup-invite-rules'>
            <Tag shape='circle'>{t('奖励说明')}</Tag>
            <Text type='tertiary' size='small'>
              {t('通过划转功能将奖励额度转入到您的账户余额中')}
            </Text>
          </div>
        </div>
      </div>
    </Card>
  );
};

export default InvitationCard;
