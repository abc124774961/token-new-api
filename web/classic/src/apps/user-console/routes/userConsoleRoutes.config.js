export const userConsoleShellRoutes = [
  '/console',
  '/console/channel-status',
  '/console/token',
  '/console/playground',
  '/console/affiliate',
  '/console/recharge',
  '/console/subscription-plans',
  '/console/personal',
  '/console/log',
  '/console/midjourney',
  '/console/task',
];

export const userConsoleHiddenShellRoutes = ['/console/personal'];

export const isUserConsoleShellRoute = (pathname = '') =>
  userConsoleShellRoutes.includes(pathname) ||
  pathname === '/console/chat' ||
  pathname.startsWith('/console/chat/');
