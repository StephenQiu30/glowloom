declare module '*.css';
declare module '*.less';
declare module '*.scss';
declare module '*.sass';
declare module '*.svg';
declare module '*.png';
declare module '*.jpg';
declare module '*.jpeg';
declare module '*.gif';
declare module '*.bmp';
declare module '*.tiff';
declare module '*.md' {
  const content: string;
  export default content;
}
declare const __APP_VERSION__: string;
declare const __UMI_VERSION__: string;
declare const __UTOO_VERSION__: string;

declare namespace API {
  type CurrentUser = {
    id?: string;
    name?: string;
    avatar?: string;
    userid?: string;
    email?: string;
    signature?: string;
    title?: string;
    group?: string;
    tags?: { key?: string; label?: string }[];
    notifyCount?: number;
    unreadCount?: number;
    country?: string;
    access?: RoleCode;
    role?: RoleCode;
    workspaceId?: string;
    workspaceName?: string;
    membershipId?: string;
    geographic?: {
      province?: { label?: string; key?: string };
      city?: { label?: string; key?: string };
    };
    address?: string;
    phone?: string;
  };

  type LoginParams = {
    email?: string;
    password?: string;
    autoLogin?: boolean;
  };
}
