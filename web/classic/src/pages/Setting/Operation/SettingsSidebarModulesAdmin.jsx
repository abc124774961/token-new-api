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

import React, { useState, useEffect, useContext } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Card,
  Form,
  Button,
  Switch,
  Row,
  Col,
  Typography,
} from '@douyinfe/semi-ui';
import { API, showSuccess, showError } from '../../../helpers';
import { StatusContext } from '../../../context/Status';
import {
  DEFAULT_ADMIN_CONFIG,
  mergeAdminConfig,
} from '../../../hooks/common/useSidebar';

const { Text } = Typography;

export default function SettingsSidebarModulesAdmin(props) {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(false);
  const [statusState, statusDispatch] = useContext(StatusContext);

  // 左侧边栏模块管理状态（管理员全局控制）
  const [sidebarModulesAdmin, setSidebarModulesAdmin] = useState(() =>
    mergeAdminConfig(null),
  );

  // 处理功能级别开关变更
  function handleModuleChange(sectionKey, moduleKey) {
    return (checked) => {
      setSidebarModulesAdmin((prev) => {
        const currentSection =
          prev[sectionKey] || DEFAULT_ADMIN_CONFIG[sectionKey] || {};

        return {
          ...prev,
          [sectionKey]: {
            ...currentSection,
            enabled: checked ? true : currentSection.enabled !== false,
            [moduleKey]: checked,
          },
        };
      });
    };
  }

  function handleModulesChange(modules) {
    return (checked) => {
      setSidebarModulesAdmin((prev) => {
        const nextModules = mergeAdminConfig(prev);

        modules.forEach(({ sectionKey, moduleKey }) => {
          const currentSection =
            nextModules[sectionKey] ||
            DEFAULT_ADMIN_CONFIG[sectionKey] ||
            {};

          nextModules[sectionKey] = {
            ...currentSection,
            enabled: checked ? true : currentSection.enabled !== false,
            [moduleKey]: checked,
          };
        });

        return nextModules;
      });
    };
  }

  // 重置为默认配置
  function resetSidebarModules() {
    setSidebarModulesAdmin(mergeAdminConfig(null));
    showSuccess(t('已重置为默认配置'));
  }

  // 保存配置
  async function onSubmit() {
    setLoading(true);
    try {
      const mergedModules = mergeAdminConfig(sidebarModulesAdmin);
      const res = await API.put('/api/option/', {
        key: 'SidebarModulesAdmin',
        value: JSON.stringify(mergedModules),
      });
      const { success, message } = res.data;
      if (success) {
        showSuccess(t('保存成功'));

        // 立即更新StatusContext中的状态
        statusDispatch({
          type: 'set',
          payload: {
            ...statusState.status,
            SidebarModulesAdmin: JSON.stringify(mergedModules),
          },
        });

        // 刷新父组件状态
        if (props.refresh) {
          await props.refresh();
        }
      } else {
        showError(message);
      }
    } catch (error) {
      showError(t('保存失败，请重试'));
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    // 从 props.options 中获取配置
    if (props.options && props.options.SidebarModulesAdmin) {
      try {
        const modules = JSON.parse(props.options.SidebarModulesAdmin);
        setSidebarModulesAdmin(mergeAdminConfig(modules));
      } catch (error) {
        setSidebarModulesAdmin(mergeAdminConfig(null));
      }
    }
  }, [props.options]);

  const isModuleEnabled = (sectionKey, moduleKey) =>
    sidebarModulesAdmin[sectionKey]?.enabled !== false &&
    sidebarModulesAdmin[sectionKey]?.[moduleKey] !== false;

  const getEnabledCount = (modules) =>
    modules.filter(({ sectionKey, moduleKey }) =>
      isModuleEnabled(sectionKey, moduleKey),
    ).length;

  const navigationGroups = [
    {
      key: 'user-console',
      title: t('普通用户控制台'),
      description: t('控制普通用户侧栏展示的概览、接入、日志和费用入口。'),
      accent: '#0f766e',
      categories: [
        {
          key: 'user-overview',
          title: t('概览'),
          description: t('面向用户的数据与服务状态'),
          modules: [
            {
              sectionKey: 'console',
              moduleKey: 'detail',
              title: t('数据看板'),
              description: t('系统数据统计'),
            },
            {
              sectionKey: 'console',
              moduleKey: 'channel_status',
              title: t('服务状态'),
              description: t('按分组监控渠道访问状态'),
            },
          ],
        },
        {
          key: 'developer-access',
          title: t('开发接入'),
          description: t('API 调试、令牌与调用记录'),
          modules: [
            {
              sectionKey: 'chat',
              moduleKey: 'playground',
              title: t('操练场'),
              description: t('AI模型测试环境'),
            },
            {
              sectionKey: 'chat',
              moduleKey: 'chat',
              title: t('聊天'),
              description: t('聊天会话管理'),
            },
            {
              sectionKey: 'console',
              moduleKey: 'token',
              title: t('令牌管理'),
              description: t('API令牌管理'),
            },
            {
              sectionKey: 'console',
              moduleKey: 'log',
              title: t('使用日志'),
              description: t('API使用记录'),
            },
            {
              sectionKey: 'console',
              moduleKey: 'midjourney',
              title: t('绘图日志'),
              description: t('绘图任务记录'),
            },
            {
              sectionKey: 'console',
              moduleKey: 'task',
              title: t('任务日志'),
              description: t('系统任务记录'),
            },
          ],
        },
        {
          key: 'billing-center',
          title: t('费用中心'),
          description: t('充值、套餐和邀请奖励'),
          modules: [
            {
              sectionKey: 'personal',
              moduleKey: 'recharge',
              title: t('账户充值'),
              description: t('余额充值与账单管理'),
            },
            {
              sectionKey: 'personal',
              moduleKey: 'subscription_plans',
              title: t('套餐订阅'),
              description: t('当前套餐与订阅购买'),
            },
            {
              sectionKey: 'personal',
              moduleKey: 'affiliate',
              title: t('邀请有奖'),
              description: t('邀请链接与奖励划转'),
            },
          ],
        },
      ],
    },
    {
      key: 'admin-console',
      title: t('管理员后台'),
      description: t('控制管理员后台侧栏展示的运营、模型、商业和系统治理入口。'),
      accent: '#2563eb',
      categories: [
        {
          key: 'operations-home',
          title: t('运营首页'),
          description: t('经营总览、实时监控与渠道预警'),
          modules: [
            {
              sectionKey: 'admin',
              moduleKey: 'overview',
              title: t('经营总览'),
              description: t('后台默认落点和核心运营指标'),
            },
            {
              sectionKey: 'admin',
              moduleKey: 'realtime_monitor',
              title: t('实时监控'),
              description: t('请求、渠道和系统实时态势'),
            },
            {
              sectionKey: 'admin',
              moduleKey: 'channel_alerts',
              title: t('渠道预警'),
              description: t('渠道异常、余额和健康风险'),
            },
          ],
        },
        {
          key: 'channel-operations',
          title: t('渠道运营'),
          description: t('渠道、账号池与健康巡检'),
          modules: [
            {
              sectionKey: 'admin',
              moduleKey: 'channel',
              title: t('渠道管理'),
              description: t('API渠道配置'),
            },
            {
              sectionKey: 'admin',
              moduleKey: 'channel_account',
              title: t('账号池管理'),
              description: t('全渠道账号与归档池'),
            },
            {
              sectionKey: 'admin',
              moduleKey: 'channel_balance_monitor',
              title: t('渠道余额监控'),
              description: t('账号余额告警和倍率同步'),
            },
            {
              sectionKey: 'admin',
              moduleKey: 'channel_health_check',
              title: t('渠道健康检测'),
              description: t('待检查队列和探活历史'),
            },
            {
              sectionKey: 'admin',
              moduleKey: 'channel_proxy',
              title: t('代理管理'),
              description: t('渠道账号代理资源管理'),
            },
          ],
        },
        {
          key: 'models-routing',
          title: t('模型与路由'),
          description: t('模型网关、模型管理与部署'),
          modules: [
            {
              sectionKey: 'console',
              moduleKey: 'model_gateway',
              title: t('智能模型网关'),
              description: t('智能调度与渠道观测'),
            },
            {
              sectionKey: 'admin',
              moduleKey: 'models',
              title: t('模型管理'),
              description: t('AI模型配置'),
            },
            {
              sectionKey: 'admin',
              moduleKey: 'deployment',
              title: t('模型部署'),
              description: t('模型部署管理'),
            },
            {
              sectionKey: 'admin',
              moduleKey: 'route_policy',
              title: t('路由策略'),
              description: t('模型路由规则与兜底策略'),
            },
            {
              sectionKey: 'admin',
              moduleKey: 'ratio_config',
              title: t('倍率配置'),
              description: t('模型倍率和计费策略配置'),
            },
          ],
        },
        {
          key: 'commercial-operations',
          title: t('商业运营'),
          description: t('利润、订阅、结算和消费'),
          modules: [
            {
              sectionKey: 'admin',
              moduleKey: 'profit_monitor',
              title: t('盈利监控台'),
              description: t('经营利润和资源成本监控'),
            },
            {
              sectionKey: 'admin',
              moduleKey: 'subscription',
              title: t('订阅管理'),
              description: t('订阅套餐管理'),
            },
            {
              sectionKey: 'admin',
              moduleKey: 'redemption',
              title: t('兑换码管理'),
              description: t('兑换码生成管理'),
            },
            {
              sectionKey: 'admin',
              moduleKey: 'settlements',
              title: t('结算记录'),
              description: t('用户扣费与补扣费结算记录'),
            },
            {
              sectionKey: 'admin',
              moduleKey: 'consumption',
              title: t('消费明细'),
              description: t('请求消费与计费明细'),
            },
          ],
        },
        {
          key: 'user-operations',
          title: t('用户运营'),
          description: t('用户账号与运营分层'),
          modules: [
            {
              sectionKey: 'admin',
              moduleKey: 'user',
              title: t('用户管理'),
              description: t('用户账户管理'),
            },
            {
              sectionKey: 'admin',
              moduleKey: 'user_segments',
              title: t('用户分层'),
              description: t('用户分层与运营标签'),
            },
            {
              sectionKey: 'admin',
              moduleKey: 'risk_records',
              title: t('风控记录'),
              description: t('用户风险事件与拦截记录'),
            },
            {
              sectionKey: 'admin',
              moduleKey: 'invite_rebates',
              title: t('邀请返佣'),
              description: t('邀请奖励和返佣管理'),
            },
          ],
        },
        {
          key: 'system-governance',
          title: t('系统治理'),
          description: t('系统设置与治理能力'),
          modules: [
            {
              sectionKey: 'admin',
              moduleKey: 'setting',
              title: t('系统设置'),
              description: t('系统参数配置'),
            },
            {
              sectionKey: 'admin',
              moduleKey: 'roles',
              title: t('权限角色'),
              description: t('后台角色与权限范围'),
            },
            {
              sectionKey: 'admin',
              moduleKey: 'audit_logs',
              title: t('审计日志'),
              description: t('后台操作审计和追踪'),
            },
            {
              sectionKey: 'admin',
              moduleKey: 'background_tasks',
              title: t('后台任务'),
              description: t('异步任务和系统队列'),
            },
          ],
        },
      ],
    },
  ];

  return (
    <Card style={{ borderRadius: '12px' }}>
      <Form.Section
        text={t('全局导航模块')}
        extraText={t(
          '按新控制台信息架构统一控制普通端和管理端导航显示；关闭后用户侧栏和管理后台都会实时收敛。',
        )}
      >
        <div
          style={{
            marginBottom: '20px',
            padding: '12px 16px',
            borderRadius: '10px',
            border: '1px solid rgba(15, 118, 110, 0.16)',
            background:
              'linear-gradient(135deg, rgba(15, 118, 110, 0.08), rgba(37, 99, 235, 0.05))',
          }}
        >
          <Text
            type='secondary'
            size='small'
            style={{ display: 'block', lineHeight: 1.6 }}
          >
            {t('个人设置已从侧栏配置中移除，统一放在右上角用户信息菜单。')}
          </Text>
          <Text
            type='secondary'
            size='small'
            style={{ display: 'block', lineHeight: 1.6 }}
          >
            {t('开关会保持旧配置字段兼容，当前不会改动后端结构。')}
          </Text>
        </div>

        {navigationGroups.map((group) => (
          <div
            key={group.key}
            style={{
              marginBottom: '24px',
              border: '1px solid var(--semi-color-border)',
              borderRadius: '12px',
              overflow: 'hidden',
              background: '#fff',
            }}
          >
            <div
              style={{
                display: 'flex',
                justifyContent: 'space-between',
                alignItems: 'center',
                gap: '16px',
                padding: '16px 18px',
                backgroundColor: 'var(--semi-color-fill-0)',
                borderLeft: `4px solid ${group.accent}`,
              }}
            >
              <div>
                <div
                  style={{
                    fontWeight: 700,
                    fontSize: '16px',
                    color: 'var(--semi-color-text-0)',
                    marginBottom: '4px',
                  }}
                >
                  {group.title}
                </div>
                <Text
                  type='secondary'
                  size='small'
                  style={{
                    fontSize: '12px',
                    color: 'var(--semi-color-text-2)',
                    lineHeight: '1.4',
                  }}
                >
                  {group.description}
                </Text>
              </div>
            </div>

            <div style={{ padding: '18px' }}>
              {group.categories.map((category) => {
                const enabledCount = getEnabledCount(category.modules);
                const totalCount = category.modules.length;
                const categoryEnabled = enabledCount === totalCount;

                return (
                  <div
                    key={category.key}
                    style={{
                      marginBottom: '18px',
                      paddingBottom: '18px',
                      borderBottom:
                        category.key ===
                        group.categories[group.categories.length - 1].key
                          ? 'none'
                          : '1px solid var(--semi-color-border)',
                    }}
                  >
                    <div
                      style={{
                        display: 'flex',
                        justifyContent: 'space-between',
                        alignItems: 'flex-start',
                        gap: '16px',
                        marginBottom: '12px',
                      }}
                    >
                      <div>
                        <div
                          style={{
                            fontWeight: 650,
                            fontSize: '14px',
                            color: 'var(--semi-color-text-0)',
                            marginBottom: '4px',
                          }}
                        >
                          {category.title}
                        </div>
                        <Text
                          type='secondary'
                          size='small'
                          style={{
                            fontSize: '12px',
                            color: 'var(--semi-color-text-2)',
                            lineHeight: '1.4',
                            display: 'block',
                          }}
                        >
                          {category.description}
                        </Text>
                      </div>
                      <div
                        style={{
                          display: 'flex',
                          alignItems: 'center',
                          gap: '10px',
                          minWidth: '132px',
                          justifyContent: 'flex-end',
                        }}
                      >
                        <Text type='secondary' size='small'>
                          {enabledCount}/{totalCount} {t('已启用')}
                        </Text>
                        <Switch
                          checked={categoryEnabled}
                          onChange={handleModulesChange(category.modules)}
                          size='default'
                        />
                      </div>
                    </div>

                    <Row gutter={[12, 12]}>
                      {category.modules.map((module) => {
                        const checked = isModuleEnabled(
                          module.sectionKey,
                          module.moduleKey,
                        );

                        return (
                          <Col
                            key={`${module.sectionKey}-${module.moduleKey}`}
                            xs={24}
                            sm={12}
                            md={8}
                            lg={8}
                            xl={6}
                          >
                            <div
                              style={{
                                display: 'flex',
                                alignItems: 'center',
                                justifyContent: 'space-between',
                                minHeight: '76px',
                                gap: '12px',
                                padding: '12px 14px',
                                borderRadius: '10px',
                                border: checked
                                  ? `1px solid ${group.accent}33`
                                  : '1px solid var(--semi-color-border)',
                                backgroundColor: checked
                                  ? `${group.accent}0D`
                                  : '#fff',
                                transition:
                                  'border-color 0.2s ease, background-color 0.2s ease',
                              }}
                            >
                              <div
                                style={{
                                  flex: 1,
                                  minWidth: 0,
                                  textAlign: 'left',
                                  paddingLeft: '10px',
                                  borderLeft: `3px solid ${
                                    checked ? group.accent : 'transparent'
                                  }`,
                                }}
                              >
                                <div
                                  style={{
                                    fontWeight: 650,
                                    fontSize: '13px',
                                    color: 'var(--semi-color-text-0)',
                                    marginBottom: '4px',
                                    whiteSpace: 'nowrap',
                                    overflow: 'hidden',
                                    textOverflow: 'ellipsis',
                                  }}
                                >
                                  {module.title}
                                </div>
                                <Text
                                  type='secondary'
                                  size='small'
                                  style={{
                                    display: 'block',
                                    fontSize: '12px',
                                    lineHeight: 1.45,
                                  }}
                                >
                                  {module.description}
                                </Text>
                              </div>
                              <Switch
                                checked={checked}
                                onChange={handleModuleChange(
                                  module.sectionKey,
                                  module.moduleKey,
                                )}
                                size='default'
                              />
                            </div>
                          </Col>
                        );
                      })}
                    </Row>
                  </div>
                );
              })}
            </div>
          </div>
        ))}

        <div
          style={{
            display: 'flex',
            gap: '12px',
            justifyContent: 'flex-start',
            alignItems: 'center',
            paddingTop: '8px',
            borderTop: '1px solid var(--semi-color-border)',
          }}
        >
          <Button
            size='default'
            type='tertiary'
            onClick={resetSidebarModules}
            style={{
              borderRadius: '6px',
              fontWeight: '500',
            }}
          >
            {t('重置为默认')}
          </Button>
          <Button
            size='default'
            type='primary'
            onClick={onSubmit}
            loading={loading}
            style={{
              borderRadius: '6px',
              fontWeight: '500',
              minWidth: '100px',
            }}
          >
            {t('保存设置')}
          </Button>
        </div>
      </Form.Section>
    </Card>
  );
}
