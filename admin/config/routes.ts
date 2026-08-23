/**
 * Lanverse 管理端路由。
 *
 * 当前只保留身份入口、管理页、账号设置和系统错误页；业务列表、仪表盘
 * 示例页等在对应后端模块落地后再按需加入。
 */
export default [
  {
    path: '/user',
    layout: false,
    routes: [
      {
        path: '/user/login',
        name: '登录',
        component: './user/login',
      },
      {
        path: '/user',
        redirect: '/user/login',
      },
      {
        path: '/user/register-result',
        name: '注册结果',
        component: './user/register-result',
      },
      {
        path: '/user/register',
        name: '注册',
        component: './user/register',
      },
      {
        path: '/user/*',
        component: './exception/404',
      },
    ],
  },
  {
    path: '/admin',
    name: '管理',
    icon: 'crown',
    access: 'canAdmin',
    component: './Admin',
  },
  {
    path: '/account/settings',
    name: '账号',
    icon: 'user',
    component: './account/settings',
  },
  {
    path: '/account',
    redirect: '/account/settings',
    hideInMenu: true,
  },
  {
    path: '/exception',
    hideInMenu: true,
    routes: [
      {
        path: '/exception/403',
        component: './exception/403',
      },
      {
        path: '/exception/404',
        component: './exception/404',
      },
      {
        path: '/exception/500',
        component: './exception/500',
      },
    ],
  },
  {
    path: '/',
    redirect: '/admin',
  },
  {
    path: '/*',
    component: './exception/404',
  },
];
