import React, { useEffect, useMemo, useState } from 'react';
import ConsoleShell from '../../../shared/console-ui/components/ConsoleShell';
import { userConsoleNavGroups } from '../navigation/userConsoleNav.config';
import { useSidebar } from '../../../hooks/common/useSidebar';

const loadChatNavItems = () => {
  try {
    const rawChats = localStorage.getItem('chats');
    if (!rawChats) return [];

    const chats = JSON.parse(rawChats);
    if (!Array.isArray(chats)) return [];

    return chats
      .map((chat, index) => {
        if (!chat || typeof chat !== 'object') return null;

        for (const [name, link] of Object.entries(chat)) {
          if (typeof link !== 'string') continue;
          if (
            link.startsWith('fluent') ||
            link.startsWith('ccswitch') ||
            link.startsWith('deepchat')
          ) {
            return null;
          }

          return {
            key: `chat-${index}`,
            label: name,
            icon: 'bot',
            path: `/console/chat/${index}`,
            sidebarSection: 'chat',
            sidebarModule: 'chat',
          };
        }

        return null;
      })
      .filter(Boolean);
  } catch (e) {
    return [];
  }
};

const UserConsoleLayout = ({ children }) => {
  const [chatItems, setChatItems] = useState(() => loadChatNavItems());
  const { loading: sidebarLoading, isModuleVisible } = useSidebar();

  useEffect(() => {
    const handleStorage = (event) => {
      if (!event || event.key === 'chats') {
        setChatItems(loadChatNavItems());
      }
    };

    window.addEventListener('storage', handleStorage);
    window.addEventListener('focus', handleStorage);
    return () => {
      window.removeEventListener('storage', handleStorage);
      window.removeEventListener('focus', handleStorage);
    };
  }, []);

  const navGroups = useMemo(() => {
    if (chatItems.length === 0) {
      return userConsoleNavGroups;
    }

    return userConsoleNavGroups.map((group) => {
      if (group.key !== 'developer') {
        return group;
      }

      return {
        ...group,
        items: [...group.items, ...chatItems],
      };
    });
  }, [chatItems]);

  const visibleNavGroups = useMemo(() => {
    if (sidebarLoading) {
      return navGroups;
    }

    return navGroups
      .map((group) => ({
        ...group,
        items: (group.items || []).filter((item) => {
          if (!item.sidebarSection || !item.sidebarModule) {
            return true;
          }
          return isModuleVisible(item.sidebarSection, item.sidebarModule);
        }),
      }))
      .filter((group) => group.items.length > 0);
  }, [isModuleVisible, navGroups, sidebarLoading]);

  return (
    <ConsoleShell
      variant='user'
      title='CodeToken AI 控制台'
      subtitle='开发者工作区'
      baseLabel='控制台'
      routeMeta={{
        '/console/personal': {
          groupLabel: '账户',
          itemLabel: '个人设置',
        },
      }}
      navGroups={visibleNavGroups}
    >
      {children}
    </ConsoleShell>
  );
};

export default UserConsoleLayout;
