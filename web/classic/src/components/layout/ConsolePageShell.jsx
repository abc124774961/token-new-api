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

export const ConsolePageHeader = ({
  eyebrow,
  title,
  subtitle,
  badge,
  actions,
  className = '',
  children,
}) => {
  if (!title && !subtitle && !eyebrow && !badge && !actions && !children) {
    return null;
  }

  return (
    <div className={`ct-console-page-header ${className}`.trim()}>
      <div className='ct-console-page-heading'>
        {eyebrow && <div className='ct-console-page-eyebrow'>{eyebrow}</div>}
        {(title || badge) && (
          <div className='ct-console-page-title-row'>
            {title && <h1 className='ct-console-page-title'>{title}</h1>}
            {badge && <div className='ct-console-page-badge'>{badge}</div>}
          </div>
        )}
        {subtitle && <p className='ct-console-page-subtitle'>{subtitle}</p>}
        {children}
      </div>
      {actions && <div className='ct-console-page-actions'>{actions}</div>}
    </div>
  );
};

const ConsolePageShell = ({
  title,
  subtitle,
  eyebrow,
  badge,
  actions,
  metrics,
  children,
  className = '',
  bodyClassName = '',
}) => (
  <main className={`ct-dashboard-shell ct-console-page-shell ${className}`.trim()}>
    {(title || subtitle || eyebrow || badge || actions) && (
      <ConsolePageHeader
        title={title}
        subtitle={subtitle}
        eyebrow={eyebrow}
        badge={badge}
        actions={actions}
      />
    )}
    {metrics && <div className='ct-console-page-metrics'>{metrics}</div>}
    <div className={`ct-console-page-body ${bodyClassName}`.trim()}>
      {children}
    </div>
  </main>
);

export default ConsolePageShell;
