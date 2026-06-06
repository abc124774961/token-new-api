import React from 'react';
import { useTranslation } from 'react-i18next';
import { Bot, Route, ShieldCheck } from 'lucide-react';
import SettingsModelGatewayScheduler from '../../../pages/Setting/Operation/SettingsModelGatewayScheduler';

const AdminRoutePolicy = () => {
  const { t } = useTranslation();

  return (
    <div className='aurora-admin-page aurora-route-policy-page'>
      <section className='aurora-overview-hero'>
        <div>
          <div className='aurora-overview-kicker'>{t('模型与路由')}</div>
          <h1>{t('路由策略')}</h1>
          <p>
            {t(
              '集中配置智能模型网关的调度策略、候选筛选、资源保护、动态倍率和兜底规则。',
            )}
          </p>
          <div className='aurora-overview-meta'>
            <span>{t('策略来源')} Model Gateway Scheduler</span>
            <span>{t('配置变更会影响实时调度')}</span>
          </div>
        </div>
        <div className='aurora-overview-status aurora-status-warning'>
          <span>{t('配置级别')}</span>
          <strong>{t('全局')}</strong>
          <em>{t('请谨慎保存')}</em>
        </div>
      </section>

      <section className='aurora-source-grid'>
        <div className='aurora-source-item is-ok'>
          <span className='aurora-source-state'>
            <Route size={14} />
            {t('调度策略')}
          </span>
          <strong>{t('候选和兜底')}</strong>
          <small>{t('控制模型路由候选、优先级、自动模式和失败兜底。')}</small>
        </div>
        <div className='aurora-source-item'>
          <span className='aurora-source-state'>
            <ShieldCheck size={14} />
            {t('资源保护')}
          </span>
          <strong>{t('队列和熔断')}</strong>
          <small>{t('配置并发保护、排队深度和异常渠道恢复策略。')}</small>
        </div>
        <div className='aurora-source-item'>
          <span className='aurora-source-state'>
            <Bot size={14} />
            {t('动态策略')}
          </span>
          <strong>{t('智能切换')}</strong>
          <small>{t('结合成功率、成本和体验信号进行路由决策。')}</small>
        </div>
      </section>

      <section className='aurora-panel ct-admin-settings-embed'>
        <div className='ct-admin-embed-head'>
          <div className='ct-admin-embed-title'>
            <span className='ct-admin-embed-icon'>
              <Route size={16} />
            </span>
            <span>
              <strong>{t('配置工作台')}</strong>
              <p>
                {t(
                  '复用现有调度配置组件，统一承载候选、熔断、兜底和动态策略。',
                )}
              </p>
            </span>
          </div>
          <span className='ct-admin-embed-badge'>{t('全局生效')}</span>
        </div>
        <div className='ct-admin-embed-body'>
          <SettingsModelGatewayScheduler />
        </div>
      </section>
    </div>
  );
};

export default AdminRoutePolicy;
