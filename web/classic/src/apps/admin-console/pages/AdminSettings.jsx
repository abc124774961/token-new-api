import React from 'react';
import { useTranslation } from 'react-i18next';
import {
  AlertTriangle,
  CreditCard,
  Gauge,
  LockKeyhole,
  ServerCog,
  Settings2,
  ShieldCheck,
} from 'lucide-react';
import Setting from '../../../pages/Setting';
import { isRoot } from '../../../helpers';

const AdminSettings = () => {
  const { t } = useTranslation();
  const root = isRoot();

  return (
    <div className='aurora-admin-page aurora-settings-governance-page'>
      <section className='aurora-overview-hero'>
        <div>
          <div className='aurora-overview-kicker'>{t('系统治理')}</div>
          <h1>{t('系统设置')}</h1>
          <p>
            {t(
              '集中维护运营、支付、模型、倍率、限流和性能等全局配置，所有保存动作都会影响线上控制台行为。',
            )}
          </p>
          <div className='aurora-overview-meta'>
            <span>{t('访问级别')} Role 100</span>
            <span>
              {t('变更范围')} {t('全局生效')}
            </span>
          </div>
        </div>
        <div
          className={`aurora-overview-status ${
            root ? 'aurora-status-success' : 'aurora-status-warning'
          }`}
        >
          <span>{t('当前权限')}</span>
          <strong>{root ? t('超级管理员') : t('仅可查看说明')}</strong>
          <em>{root ? t('可编辑全局配置') : t('需要超级管理员权限')}</em>
        </div>
      </section>

      <section className='aurora-source-grid aurora-settings-scope-grid'>
        <div className='aurora-source-item is-ok'>
          <span className='aurora-source-state'>
            <Settings2 size={14} />
            {t('运营配置')}
          </span>
          <strong>{t('公告、签到、日志、监控')}</strong>
          <small>{t('影响普通用户控制台曝光、审计保留和运营自动化。')}</small>
        </div>
        <div className='aurora-source-item'>
          <span className='aurora-source-state'>
            <CreditCard size={14} />
            {t('商业配置')}
          </span>
          <strong>{t('支付、倍率、套餐联动')}</strong>
          <small>{t('影响充值入账、模型扣费、利润监控和订阅售卖。')}</small>
        </div>
        <div className='aurora-source-item'>
          <span className='aurora-source-state'>
            <ServerCog size={14} />
            {t('模型配置')}
          </span>
          <strong>{t('模型、部署、工具能力')}</strong>
          <small>{t('影响模型可见性、部署入口、工具调用和上游兼容。')}</small>
        </div>
        <div className='aurora-source-item'>
          <span className='aurora-source-state'>
            <Gauge size={14} />
            {t('运行配置')}
          </span>
          <strong>{t('限流与性能')}</strong>
          <small>{t('影响请求保护、并发策略、缓存和后台任务表现。')}</small>
        </div>
      </section>

      {root ? (
        <section className='aurora-panel ct-admin-settings-governance'>
          <div className='ct-admin-embed-head'>
            <div className='ct-admin-embed-title'>
              <span className='ct-admin-embed-icon'>
                <ShieldCheck size={16} />
              </span>
              <span>
                <strong>{t('超级管理员配置台')}</strong>
                <p>
                  {t(
                    '只对 Role 100 开放；保存前请确认影响范围、回滚方式和运营窗口。',
                  )}
                </p>
              </span>
            </div>
            <span className='ct-admin-embed-badge'>{t('Root only')}</span>
          </div>
          <div className='ct-admin-settings-governance-body'>
            <Setting variant='admin' />
          </div>
        </section>
      ) : (
        <section className='aurora-panel aurora-root-only-callout'>
          <span className='aurora-root-only-icon'>
            <LockKeyhole size={22} />
          </span>
          <div>
            <span className='aurora-status-pill is-warning'>
              <AlertTriangle size={13} />
              {t('Root only')}
            </span>
            <h2>{t('系统设置仅限超级管理员编辑')}</h2>
            <p>
              {t(
                '当前账号可以进入管理员后台，但不能编辑全局系统配置。请让超级管理员处理支付、模型、倍率、限流和性能等变更。',
              )}
            </p>
          </div>
        </section>
      )}
    </div>
  );
};

export default AdminSettings;
