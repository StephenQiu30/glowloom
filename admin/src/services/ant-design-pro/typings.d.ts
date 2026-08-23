declare namespace API {
  type RoleCode = 'admin' | 'user' | 'ban';

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
