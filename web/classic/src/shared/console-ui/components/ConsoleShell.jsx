import React, { useCallback, useContext, useEffect, useMemo, useState } from 'react';
import { Link, useLocation, useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { Dropdown, Input, Modal } from '@douyinfe/semi-ui';
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
import { normalizeLanguage, supportedLanguages } from '../../../i18n/language';
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

const languageLabels = {
  'zh-CN': '简体中文',
  'zh-TW': '繁體中文',
  en: 'English',
  fr: 'Français',
  ru: 'Русский',
  ja: '日本語',
  vi: 'Tiếng Việt',
};

const flattenNavItems = (groups) =>
  (groups || []).flatMap((group) =>
    (group.items || []).map((item) => ({
      ...item,
      groupKey: group.key,
      groupLabel: group.label,
    })),
  );

const findNavItem = (items, key) => items.find((item) => item.key === key);

const findFirstInGroup = (items, groupKey) =>
  items.find((item) => item.groupKey === groupKey);

const readCollapsedGroupKeys = (storageKey) => {
  try {
    const parsed = JSON.parse(localStorage.getItem(storageKey) || '[]');
    return Array.isArray(parsed) ? parsed.filter(Boolean) : [];
  } catch (error) {
    return [];
  }
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
  const { t, i18n } = useTranslation();
  const location = useLocation();
  const navigate = useNavigate();
  const [userState, userDispatch] = useContext(UserContext);
  const collapsedStorageKey = `aurora-console-${variant}-collapsed-groups`;
  const [collapsedGroups, setCollapsedGroups] = useState(() =>
    readCollapsedGroupKeys(collapsedStorageKey),
  );
  const [commandVisible, setCommandVisible] = useState(false);
  const [commandQuery, setCommandQuery] = useState('');

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
  const currentLang = normalizeLanguage(i18n.language) || 'zh-CN';
  const navItems = useMemo(() => flattenNavItems(navGroups), [navGroups]);
  const commandItems = useMemo(() => {
    const keyword = commandQuery.trim().toLowerCase();
    if (!keyword) return navItems;
    return navItems.filter((item) =>
      [item.label, item.groupLabel, item.path]
        .filter(Boolean)
        .some((value) => t(value).toLowerCase().includes(keyword)),
    );
  }, [commandQuery, navItems, t]);
  const railItems = useMemo(() => {
    const first = navItems[0];
    if (variant === 'admin') {
      return [
        {
          key: 'overview',
          icon: LayoutDashboard,
          label: t('经营总览'),
          target:
            findNavItem(navItems, 'business-overview') ||
            findFirstInGroup(navItems, 'operations-home') ||
            first,
        },
        {
          key: 'monitor',
          icon: Activity,
          label: t('实时监控'),
          target:
            findNavItem(navItems, 'realtime-monitor') ||
            findFirstInGroup(navItems, 'operations-home') ||
            first,
        },
        {
          key: 'settings',
          icon: Settings,
          label: t('系统设置'),
          target:
            findNavItem(navItems, 'settings') ||
            findFirstInGroup(navItems, 'system-governance') ||
            first,
        },
      ];
    }
    return [
      {
        key: 'dashboard',
        icon: LayoutDashboard,
        label: t('数据看板'),
        target: findNavItem(navItems, 'dashboard') || first,
      },
      {
        key: 'status',
        icon: Activity,
        label: t('服务状态'),
        target: findNavItem(navItems, 'service-status') || first,
      },
      {
        key: 'personal',
        icon: Settings,
        label: t('个人设置'),
        target: { path: '/console/personal' },
      },
    ];
  }, [navItems, t, variant]);

  useEffect(() => {
    localStorage.setItem(collapsedStorageKey, JSON.stringify(collapsedGroups));
  }, [collapsedGroups, collapsedStorageKey]);

  useEffect(() => {
    setCollapsedGroups((keys) =>
      active?.group?.key ? keys.filter((key) => key !== active.group.key) : keys,
    );
  }, [active?.group?.key]);

  const toggleGroup = useCallback(
    (groupKey) => {
      if (!groupKey || active?.group?.key === groupKey) return;
      setCollapsedGroups((keys) =>
        keys.includes(groupKey)
          ? keys.filter((key) => key !== groupKey)
          : [...keys, groupKey],
      );
    },
    [active?.group?.key],
  );

  const navigateToItem = useCallback(
    (item) => {
      if (!item?.path) return;
      navigate(item.path);
      setCommandVisible(false);
      setCommandQuery('');
    },
    [navigate],
  );

  const handleLanguageChange = useCallback(
    async (lang) => {
      const normalized = normalizeLanguage(lang) || 'zh-CN';
      const previousLang = normalizeLanguage(i18n.language);
      i18n.changeLanguage(normalized);
      localStorage.setItem('i18nextLng', normalized);
      if (!userState?.user?.id) return;

      try {
        const res = await API.put('/api/user/self', {
          language: normalized,
        });
        if (!res?.data?.success) return;

        let settings = {};
        if (userState.user.setting) {
          try {
            settings = JSON.parse(userState.user.setting) || {};
          } catch (error) {
            settings = {};
          }
        }
        settings.language = normalized;
        const nextUser = {
          ...userState.user,
          setting: JSON.stringify(settings),
        };
        userDispatch({ type: 'login', payload: nextUser });
        localStorage.setItem('user', JSON.stringify(nextUser));
      } catch (error) {
        if (previousLang) {
          i18n.changeLanguage(previousLang);
          localStorage.setItem('i18nextLng', previousLang);
        }
      }
    },
    [i18n, userDispatch, userState],
  );

  const notificationTarget =
    variant === 'admin'
      ? findNavItem(navItems, 'channel-alerts') || navItems[0]
      : findNavItem(navItems, 'service-status') || navItems[0];
  const receiptTarget =
    findNavItem(navItems, 'audit-logs') ||
    findNavItem(navItems, 'consumption') ||
    navItems[0];

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
          {railItems.map((item) => {
            const Icon = item.icon;
            const path = item.target?.path;
            const activeRail = path && path === location.pathname;
            if (!path) {
              return (
                <button
                  key={item.key}
                  className='aurora-rail-button is-disabled'
                  type='button'
                  aria-label={item.label}
                  disabled
                >
                  <Icon size={21} />
                </button>
              );
            }
            return (
              <Link
                key={item.key}
                className={`aurora-rail-button ${
                  activeRail ? 'is-active' : ''
                }`}
                to={path}
                aria-label={item.label}
                title={item.label}
              >
                <Icon size={21} />
              </Link>
            );
          })}
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
                  {(() => {
                    const collapsed =
                      collapsedGroups.includes(group.key) &&
                      active?.group?.key !== group.key;
                    return (
                      <>
                        <button
                          className={`aurora-nav-parent ${
                            collapsed ? 'is-collapsed' : ''
                          }`}
                          type='button'
                          aria-expanded={!collapsed}
                          onClick={() => toggleGroup(group.key)}
                        >
                          <span className='aurora-nav-parent-left'>
                            <span className='aurora-nav-icon'>
                              <ParentIcon size={18} strokeWidth={2.2} />
                            </span>
                            {t(group.label)}
                          </span>
                          {collapsed ? (
                            <ChevronRight size={16} />
                          ) : (
                            <ChevronDown size={16} />
                          )}
                        </button>
                        {!collapsed &&
                          (group.items || []).map((item) => {
                            const ItemIcon = getIcon(item.icon || group.icon);
                            const paths = [
                              item.path,
                              ...(item.aliases || []),
                            ].filter(Boolean);
                            const itemActive = paths.some(
                              (path) => path === location.pathname,
                            );

                            return (
                              <Link
                                className={`aurora-nav-link ${
                                  itemActive ? 'is-active' : ''
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
                      </>
                    );
                  })()}
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
            <button
              className='aurora-icon-button'
              type='button'
              aria-label={t('搜索页面')}
              title={t('搜索页面')}
              onClick={() => setCommandVisible(true)}
            >
              <Search size={19} />
            </button>
            <button
              className='aurora-icon-button'
              type='button'
              aria-label={t('查看通知')}
              title={t('查看通知')}
              onClick={() => navigateToItem(notificationTarget)}
            >
              <Bell size={19} />
              {variant === 'admin' && <span className='aurora-notice-dot' />}
            </button>
            {variant === 'admin' && (
              <button
                className='aurora-icon-button'
                type='button'
                aria-label={t('查看审计或消费明细')}
                title={t('查看审计或消费明细')}
                onClick={() => navigateToItem(receiptTarget)}
              >
                <ReceiptText size={19} />
              </button>
            )}
            <Dropdown
              position='bottomRight'
              render={
                <Dropdown.Menu className='aurora-user-menu aurora-language-menu'>
                  {supportedLanguages.map((lang) => (
                    <Dropdown.Item
                      key={lang}
                      onClick={() => handleLanguageChange(lang)}
                    >
                      <span
                        className={`aurora-user-menu-item ${
                          currentLang === lang ? 'is-active' : ''
                        }`}
                      >
                        {languageLabels[lang] || lang}
                      </span>
                    </Dropdown.Item>
                  ))}
                </Dropdown.Menu>
              }
            >
              <button
                className='aurora-language'
                type='button'
                aria-label={t('common.changeLanguage')}
              >
                <Languages size={18} />
                {languageLabels[currentLang] || t('简体中文')}
                <ChevronDown size={15} />
              </button>
            </Dropdown>
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
      <Modal
        visible={commandVisible}
        title={t('搜索页面')}
        footer={null}
        width={560}
        onCancel={() => setCommandVisible(false)}
      >
        <div className='aurora-command-palette'>
          <Input
            autoFocus
            value={commandQuery}
            prefix={<Search size={15} />}
            placeholder={t('输入页面名称或分组')}
            onChange={setCommandQuery}
          />
          <div className='aurora-command-list'>
            {commandItems.length ? (
              commandItems.map((item) => {
                const Icon = getIcon(item.icon);
                return (
                  <button
                    key={`${item.groupKey}-${item.key}`}
                    className='aurora-command-item'
                    type='button'
                    onClick={() => navigateToItem(item)}
                  >
                    <span className='aurora-command-icon'>
                      <Icon size={16} />
                    </span>
                    <span>
                      <strong>{t(item.label)}</strong>
                      <small>{t(item.groupLabel)}</small>
                    </span>
                  </button>
                );
              })
            ) : (
              <div className='aurora-command-empty'>{t('暂无匹配页面')}</div>
            )}
          </div>
        </div>
      </Modal>
    </div>
  );
};

export default ConsoleShell;
