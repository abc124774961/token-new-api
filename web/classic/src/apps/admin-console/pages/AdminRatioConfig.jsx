import React from 'react';
import { useTranslation } from 'react-i18next';
import { Calculator, Coins, Database, RefreshCw } from 'lucide-react';
import RatioSetting from '../../../components/settings/RatioSetting';

const AdminRatioConfig = () => {
  const { t } = useTranslation();

  return (
    <div className='aurora-admin-page aurora-ratio-config-page'>
      <section className='aurora-overview-hero'>
        <div>
          <div className='aurora-overview-kicker'>{t('模型与路由')}</div>
          <h1>{t('倍率配置')}</h1>
          <p>
            {t(
              '维护模型价格、分组倍率、缓存倍率、工具调用定价和上游价格同步策略。',
            )}
          </p>
          <div className='aurora-overview-meta'>
            <span>{t('配置来源')} /api/option</span>
            <span>{t('影响扣费和利润监控')}</span>
          </div>
        </div>
        <div className='aurora-overview-status aurora-status-warning'>
          <span>{t('计费策略')}</span>
          <strong>{t('核心配置')}</strong>
          <em>{t('保存前请复核')}</em>
        </div>
      </section>

      <section className='aurora-source-grid'>
        <div className='aurora-source-item is-ok'>
          <span className='aurora-source-state'>
            <Calculator size={14} />
            {t('模型定价')}
          </span>
          <strong>{t('价格和倍率')}</strong>
          <small>
            {t('维护模型基础价格、补全倍率、缓存倍率和多模态倍率。')}
          </small>
        </div>
        <div className='aurora-source-item'>
          <span className='aurora-source-state'>
            <Database size={14} />
            {t('分组倍率')}
          </span>
          <strong>{t('用户分组')}</strong>
          <small>{t('控制分组折扣、可选分组和跨分组特殊倍率。')}</small>
        </div>
        <div className='aurora-source-item'>
          <span className='aurora-source-state'>
            <RefreshCw size={14} />
            {t('上游价格同步')}
          </span>
          <strong>{t('成本校准')}</strong>
          <small>{t('同步供应商价格变化，减少手工维护成本。')}</small>
        </div>
        <div className='aurora-source-item'>
          <span className='aurora-source-state'>
            <Coins size={14} />
            {t('工具定价')}
          </span>
          <strong>{t('工具调用')}</strong>
          <small>{t('为联网、代码执行等工具调用配置独立计费策略。')}</small>
        </div>
      </section>

      <section className='aurora-panel ct-admin-settings-embed'>
        <div className='ct-admin-embed-head'>
          <div className='ct-admin-embed-title'>
            <span className='ct-admin-embed-icon'>
              <Calculator size={16} />
            </span>
            <span>
              <strong>{t('计费工作台')}</strong>
              <p>
                {t(
                  '复用现有倍率配置组件，统一维护模型价格、分组倍率和工具调用定价。',
                )}
              </p>
            </span>
          </div>
          <span className='ct-admin-embed-badge'>{t('影响扣费')}</span>
        </div>
        <div className='ct-admin-embed-body'>
          <RatioSetting />
        </div>
      </section>
    </div>
  );
};

export default AdminRatioConfig;
