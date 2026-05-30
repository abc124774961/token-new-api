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
import { Skeleton, Tag } from '@douyinfe/semi-ui';
import { useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';

const buildSparklinePoints = (values, width = 100, height = 40) => {
  const source = (Array.isArray(values) ? values : [])
    .map((value) => Number(value))
    .filter((value) => Number.isFinite(value));
  const data = source.length > 1 ? source : [0, 0];
  const min = Math.min(...data);
  const max = Math.max(...data);
  const range = Math.max(max - min, 1);
  const padX = 2;
  const padY = 4;
  const step = (width - padX * 2) / Math.max(data.length - 1, 1);

  return data
    .map((value, index) => {
      const normalized = max === min ? 0.5 : (value - min) / range;
      const x = padX + index * step;
      const y = height - padY - normalized * (height - padY * 2);
      return `${x.toFixed(1)},${y.toFixed(1)}`;
    })
    .join(' ');
};

const TrendSparkline = ({ data, color, compact = false }) => (
  <svg
    className='ct-command-sparkline'
    viewBox='0 0 100 40'
    preserveAspectRatio='none'
    aria-hidden='true'
  >
    <polyline
      points={buildSparklinePoints(data)}
      fill='none'
      stroke={color || '#14b8a6'}
      strokeWidth={compact ? 2.4 : 2.8}
      strokeLinecap='round'
      strokeLinejoin='round'
      vectorEffect='non-scaling-stroke'
    />
  </svg>
);

const StatsCards = ({
  groupedStatsData,
  loading,
}) => {
  const navigate = useNavigate();
  const { t } = useTranslation();

  const renderValue = (value) => (
    <Skeleton
      loading={loading}
      active
      placeholder={
        <Skeleton.Paragraph
          active
          rows={1}
          style={{
            width: '72px',
            height: '26px',
            marginTop: '4px',
          }}
        />
      }
    >
      {value}
    </Skeleton>
  );

  return (
    <div className='ct-command-metric-grid'>
      {groupedStatsData.map((group, idx) => {
        const primary = group.items?.[0];
        const secondaryItems = group.items?.slice(1) || [];
        return (
          <section
            key={idx}
            className={`ct-command-metric-card ct-command-metric-card-${group.tone || 'default'}`}
          >
            <div className='ct-command-metric-head'>
              <div className='ct-command-metric-title'>{group.title}</div>
              <span className='ct-command-metric-index'>
                {String(idx + 1).padStart(2, '0')}
              </span>
            </div>

            {primary && (
              <div
                className='ct-command-metric-primary'
                onClick={primary.onClick}
              >
                <span
                  className={`ct-command-metric-icon ct-command-metric-icon-${primary.iconTone || 'teal'}`}
                >
                  {primary.icon}
                </span>
                <span className='ct-command-metric-copy'>
                  <span className='ct-command-metric-label'>
                    {primary.title}
                  </span>
                  <span
                    className={`ct-command-metric-value ct-command-tone-${primary.iconTone || 'teal'}`}
                  >
                    {renderValue(primary.value)}
                  </span>
                </span>
                {primary.title === t('当前余额') ? (
                  <Tag
                    color='green'
                    shape='circle'
                    size='large'
                    className='ct-command-recharge-tag'
                    onClick={(e) => {
                      e.stopPropagation();
                      navigate('/console/recharge');
                    }}
                  >
                    {t('充值')}
                  </Tag>
                ) : (
                  (loading ||
                    (primary.trendData && primary.trendData.length > 0)) && (
                    <span className='ct-command-metric-trend'>
                      <TrendSparkline
                        data={primary.trendData || []}
                        color={primary.trendColor}
                      />
                    </span>
                  )
                )}
              </div>
            )}

            <div className='ct-command-metric-subgrid'>
              {secondaryItems.map((item, itemIdx) => (
                <div
                  key={itemIdx}
                  className='ct-command-metric-secondary'
                  onClick={item.onClick}
                >
                  <span
                    className={`ct-command-metric-mini-icon ct-command-metric-icon-${item.iconTone || 'teal'}`}
                  >
                    {item.icon}
                  </span>
                  <span className='ct-command-metric-secondary-copy'>
                    <span className='ct-command-metric-label'>
                      {item.title}
                    </span>
                    <span
                      className={`ct-command-metric-secondary-value ct-command-tone-${item.iconTone || 'teal'}`}
                    >
                      {renderValue(item.value)}
                    </span>
                  </span>
                  {(loading || (item.trendData && item.trendData.length > 0)) &&
                    item.title !== t('当前余额') && (
                      <span className='ct-command-metric-mini-trend'>
                        <TrendSparkline
                          data={item.trendData || []}
                          color={item.trendColor}
                          compact
                        />
                      </span>
                    )}
                </div>
              ))}
            </div>
          </section>
        );
      })}
    </div>
  );
};

export default StatsCards;
