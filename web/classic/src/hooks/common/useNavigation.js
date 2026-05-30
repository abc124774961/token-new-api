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

import { useMemo } from 'react';
import {
  isHeaderNavModuleEnabled,
  normalizeHeaderNavModules,
} from '../../constants/header-nav.constants';

export const useNavigation = (t, headerNavModules) => {
  const mainNavLinks = useMemo(() => {
    const modules = normalizeHeaderNavModules(headerNavModules);

    const allLinks = [
      {
        text: t('首页'),
        itemKey: 'home',
        to: '/',
      },
      {
        text: t('控制台'),
        itemKey: 'console',
        to: '/console',
      },
      {
        text: t('价格'),
        itemKey: 'pricing',
        to: '/#pricing',
      },
      {
        text: t('套餐'),
        itemKey: 'subscriptionPlans',
        to: '/#subscription-plans',
      },
      {
        text: t('接入文档'),
        itemKey: 'integrationDocs',
        to: '/integration-docs',
      },
    ];

    // 根据配置过滤导航链接
    return allLinks.filter((link) =>
      isHeaderNavModuleEnabled(modules, link.itemKey),
    );
  }, [t, headerNavModules]);

  return {
    mainNavLinks,
  };
};
