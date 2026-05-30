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

export const DEFAULT_HEADER_NAV_MODULES = {
  home: true,
  console: true,
  integrationDocs: true,
  pricing: {
    enabled: true,
    requireAuth: false,
  },
  subscriptionPlans: true,
  docs: true,
  about: true,
};

export function normalizeHeaderNavModules(modules = {}) {
  const source =
    modules && typeof modules === 'object' && !Array.isArray(modules)
      ? modules
      : {};
  const pricing =
    typeof source.pricing === 'boolean'
      ? {
          enabled: source.pricing,
          requireAuth: false,
        }
      : {
          ...DEFAULT_HEADER_NAV_MODULES.pricing,
          ...(source.pricing || {}),
        };

  return {
    ...DEFAULT_HEADER_NAV_MODULES,
    ...source,
    pricing,
    integrationDocs: source.integrationDocs ?? source.integration_docs ?? true,
    subscriptionPlans:
      source.subscriptionPlans ?? source.subscription_plans ?? true,
  };
}

export function parseHeaderNavModulesConfig(config) {
  if (!config) {
    return normalizeHeaderNavModules();
  }

  try {
    return normalizeHeaderNavModules(JSON.parse(config));
  } catch (error) {
    console.error('解析顶栏模块配置失败:', error);
    return normalizeHeaderNavModules();
  }
}

export function isHeaderNavModuleEnabled(modules, key) {
  const normalized = normalizeHeaderNavModules(modules);
  if (key === 'pricing') {
    return normalized.pricing?.enabled !== false;
  }
  if (key === 'subscriptionPlans') {
    return normalized.subscriptionPlans !== false;
  }
  return normalized[key] !== false;
}

export function isPricingAuthRequired(modules) {
  return normalizeHeaderNavModules(modules).pricing?.requireAuth === true;
}
