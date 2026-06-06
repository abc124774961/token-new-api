import React, { useContext, useMemo } from 'react';
import { Link, useLocation, useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { Dropdown } from '@douyinfe/semi-ui';
import {
  Activity,
  Bell,
  Bot,
  ChevronDown,
  ChevronRight,
  CircleDollarSign,
  ClipboardList,
  Code2,
  CreditCard,
  Database,
  Gauge,
  Gift,
  Globe2,
  HeartPulse,
  Home,
  KeyRound,
  Languages,
  LayoutDashboard,
  LogOut,
  Network,
  Package,
  ReceiptText,
  Search,
  Settings,
  ShieldCheck,
  SlidersHorizontal,
  Sparkles,
  UserCog,
  Users,
  WalletCards,
} from 'lucide-react';
import { API, isAdmin, showSuccess } from '../../../helpers';
import { UserContext } from '../../../context/User';
import '../console-ui.css';

const iconMap = {
  activity: Activity,
  bot: Bot,
  channel: Network,
  code: Code2,
  creditCard: CreditCard,
  database: Database,
  dashboard: LayoutDashboard,
  dollar: CircleDollarSign,
  gift: Gift,
  health: HeartPulse,
  home: Home,
  key: KeyRound,
  logs: ClipboardList,
  model: Bot,
  network: Network,
  package: Package,
  receipt: ReceiptText,
  settings: Settings,
  shield: ShieldCheck,
  sliders: SlidersHorizontal,
  sparkles: Sparkles,
  users: Users,
  wallet: WalletCards,
};

const getIcon = (icon) => {
  return iconMap[icon] || Gauge;
};

const findActiveItem = (groups, pathname) => {
  for (const group of groups) {
    for (const item of group.items || []) {
      const paths = [item.path, ...(item.aliases || [])].filter(Boolean);
      if (paths.some((path) => pathname === path)) {
        return { group, item };
      }
    }
  }
  return null;
};

const ConsoleShell = ({
  variant = 'user',
  title,
  subtitle,
  baseLabel,
  envLabel,
  navGroups,
  routeMeta = {},
  children,
}) => {
  const { t } = useTranslation();
  const location = useLocation();
  const navigate = useNavigate();
  const [userState, userDispatch] = useContext(UserContext);

  const active = useMemo(
    () => findActiveItem(navGroups, location.pathname),
    [location.pathname, navGroups],
  );
  const currentRouteMeta = routeMeta[location.pathname] || {};

  const initials = variant === 'admin' ? 'A' : 'C';
  const currentGroup =
    active?.group?.label || currentRouteMeta.groupLabel || baseLabel;
  const currentItem = active?.item?.label || currentRouteMeta.itemLabel || '';
  const user = userState?.user || {};
  const username = user.username || t('用户');
  const avatarText = username?.[0]?.toUpperCase?.() || 'U';
  const isAdminUser = isAdmin();

  const handleLogout = async () => {
    try {
      await API.get('/api/user/logout');
      showSuccess(t('注销成功!'));
    } finally {
      userDispatch({ type: 'logout' });
      localStorage.removeItem('user');
      navigate('/login');
    }
  };

  return (
    <div className={`aurora-console aurora-console-${variant}`}>
      <div className='aurora-console-shell'>
        <aside className='aurora-brand-rail' aria-label={t('品牌导航')}>
          <div className='aurora-brand-mark'>{initials}</div>
          <div className='aurora-rail-button'>
            <LayoutDashboard size={21} />
          </div>
          <div className='aurora-rail-button'>
            <Activity size={21} />
          </div>
          <div className='aurora-rail-button'>
            <Settings size={21} />
          </div>
        </aside>

        <aside className='aurora-side'>
          <div className='aurora-side-brand'>
            <div>
              <div className='aurora-side-title'>{t(title)}</div>
              <div className='aurora-side-subtitle'>{t(subtitle)}</div>
            </div>
          </div>

          <nav className='aurora-nav' aria-label={t('控制台导航')}>
            {navGroups.map((group) => {
              const ParentIcon = getIcon(group.icon);
              return (
                <section className='aurora-nav-group' key={group.key}>
                  <div className='aurora-nav-parent'>
                    <span className='aurora-nav-parent-left'>
                      <span className='aurora-nav-icon'>
                        <ParentIcon size={18} strokeWidth={2.2} />
                      </span>
                      {t(group.label)}
                    </span>
                    <ChevronDown size={16} />
                  </div>
                  {(group.items || []).map((item) => {
                    const ItemIcon = getIcon(item.icon || group.icon);
                    const paths = [item.path, ...(item.aliases || [])].filter(
                      Boolean,
                    );
                    const active = paths.some(
                      (path) => path === location.pathname,
                    );

                    return (
                      <Link
                        className={`aurora-nav-link ${
                          active ? 'is-active' : ''
                        }`}
                        key={item.key}
                        to={item.path}
                      >
                        <span className='aurora-nav-link-left'>
                          <span className='aurora-nav-icon'>
                            <ItemIcon size={17} strokeWidth={2.15} />
                          </span>
                          {t(item.label)}
                        </span>
                      </Link>
                    );
                  })}
                </section>
              );
            })}
          </nav>
        </aside>

        <header className='aurora-topbar'>
          <div className='aurora-breadcrumb'>
            <span>{t(baseLabel)}</span>
            <ChevronRight size={15} />
            <span>{t(currentGroup)}</span>
            {currentItem && (
              <>
                <ChevronRight size={15} />
                <strong>{t(currentItem)}</strong>
              </>
            )}
            {envLabel && <span className='aurora-env-pill'>{envLabel}</span>}
          </div>

          <div className='aurora-top-actions'>
            {variant === 'admin' && (
              <button className='aurora-icon-button' type='button'>
                <Search size={19} />
              </button>
            )}
            <button className='aurora-icon-button' type='button'>
              <Bell size={19} />
              {variant === 'admin' && <span className='aurora-notice-dot' />}
            </button>
            {variant === 'admin' && (
              <button className='aurora-icon-button' type='button'>
                <ReceiptText size={19} />
              </button>
            )}
            <button className='aurora-language' type='button'>
              <Languages size={18} />
              {t('简体中文')}
              <ChevronDown size={15} />
            </button>
            <Dropdown
              position='bottomRight'
              render={
                <Dropdown.Menu className='aurora-user-menu'>
                  <Dropdown.Item onClick={() => navigate('/console/personal')}>
                    <span className='aurora-user-menu-item'>
                      <UserCog size={16} />
                      {t('个人设置')}
                    </span>
                  </Dropdown.Item>
                  <Dropdown.Item onClick={() => navigate('/console/token')}>
                    <span className='aurora-user-menu-item'>
                      <KeyRound size={16} />
                      {t('令牌管理')}
                    </span>
                  </Dropdown.Item>
                  <Dropdown.Item onClick={() => navigate('/console/recharge')}>
                    <span className='aurora-user-menu-item'>
                      <CreditCard size={16} />
                      {t('账户充值')}
                    </span>
                  </Dropdown.Item>
                  {isAdminUser && (
                    <Dropdown.Item onClick={() => navigate('/admin/overview')}>
                      <span className='aurora-user-menu-item'>
                        <Settings size={16} />
                        {t('进入管理员后台')}
                      </span>
                    </Dropdown.Item>
                  )}
                  <Dropdown.Item onClick={handleLogout}>
                    <span className='aurora-user-menu-item is-danger'>
                      <LogOut size={16} />
                      {t('退出')}
                    </span>
                  </Dropdown.Item>
                </Dropdown.Menu>
              }
            >
              <button className='aurora-user-button' type='button'>
                <span className='aurora-avatar'>{avatarText}</span>
                <span>{username}</span>
                <ChevronDown size={15} />
              </button>
            </Dropdown>
          </div>
        </header>

        <main className='aurora-content'>
          <div className='aurora-content-card'>{children}</div>
        </main>
      </div>
    </div>
  );
};

export default ConsoleShell;
