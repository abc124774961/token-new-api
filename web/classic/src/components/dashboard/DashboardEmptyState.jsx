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
import { PiTrayDuotone } from 'react-icons/pi';

const DashboardEmptyState = ({ title, description, className = '' }) => (
  <div className={`ct-command-empty ${className}`}>
    <div className='ct-command-empty-icon'>
      <PiTrayDuotone size={24} />
    </div>
    <div className='ct-command-empty-title'>{title}</div>
    {description && (
      <div className='ct-command-empty-description'>{description}</div>
    )}
  </div>
);

export default DashboardEmptyState;
