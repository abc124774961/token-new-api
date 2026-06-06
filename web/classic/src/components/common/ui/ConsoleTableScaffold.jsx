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
import ConsolePageShell from '../../layout/ConsolePageShell';

export const ConsoleMetricCard = ({
  icon,
  label,
  value,
  helper,
  tone = 'teal',
}) => (
  <div className={`ct-console-metric-card ct-console-metric-${tone}`}>
    <div className='ct-console-metric-copy'>
      <span>{label}</span>
      <strong>{value}</strong>
      {helper && <small>{helper}</small>}
    </div>
    {icon && <div className='ct-console-metric-icon'>{icon}</div>}
  </div>
);

const renderMetrics = (metrics) => {
  if (!metrics) return null;
  if (React.isValidElement(metrics)) return metrics;

  return metrics.map((metric) => (
    <ConsoleMetricCard
      key={metric.key || metric.label}
      icon={metric.icon}
      label={metric.label}
      value={metric.value}
      helper={metric.helper}
      tone={metric.tone}
    />
  ));
};

const ConsoleTableScaffold = ({
  eyebrow,
  title,
  subtitle,
  badge,
  actions,
  metrics,
  tableTitle,
  tableSubtitle,
  tableIcon,
  tableMeta,
  toolbar,
  tabs,
  children,
  className = '',
  bodyClassName = '',
  workbenchClassName = '',
}) => (
  <ConsolePageShell
    eyebrow={eyebrow}
    title={title}
    subtitle={subtitle}
    badge={badge}
    actions={actions}
    metrics={renderMetrics(metrics)}
    className={`ct-console-table-page ${className}`.trim()}
    bodyClassName={bodyClassName}
  >
    {tabs && <section className='ct-console-table-tabs'>{tabs}</section>}
    <section
      className={`ct-console-table-workbench ${workbenchClassName}`.trim()}
    >
      {(tableTitle || tableSubtitle || tableMeta || toolbar) && (
        <div className='ct-console-table-workbench-head'>
          {(tableTitle || tableSubtitle) && (
            <div className='ct-console-table-title'>
              {tableIcon && (
                <span className='ct-console-table-title-icon'>{tableIcon}</span>
              )}
              <div>
                {tableTitle && <strong>{tableTitle}</strong>}
                {tableSubtitle && <span>{tableSubtitle}</span>}
              </div>
            </div>
          )}
          {(tableMeta || toolbar) && (
            <div className='ct-console-table-head-actions'>
              {tableMeta && (
                <div className='ct-console-table-meta'>{tableMeta}</div>
              )}
              {toolbar && <div className='ct-console-table-toolbar'>{toolbar}</div>}
            </div>
          )}
        </div>
      )}
      {children}
    </section>
  </ConsolePageShell>
);

export default ConsoleTableScaffold;
