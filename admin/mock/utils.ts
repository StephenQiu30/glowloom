export const defaultUser = {
  name: 'Serati Ma',
  avatar:
    'https://gw.alipayobjects.com/zos/antfincdn/XAosXuNZyF/BiazfanxmamNRoxxVxka.png',
  userid: '00000001',
  email: 'antdesign@alipay.com',
  signature: '海纳百川，有容乃大',
  title: '交互专家',
  group: 'Lanverse 管理团队',
  tags: [
    { key: '0', label: '专注设计' },
    { key: '1', label: '海纳百川' },
  ],
  notifyCount: 0,
  unreadCount: 0,
  country: 'China',
  geographic: {
    province: { label: '浙江省', key: '330000' },
    city: { label: '杭州市', key: '330100' },
  },
  address: '西湖区工专路 77 号',
  phone: '0752-268888888',
};

export const waitTime = (time: number = 100): Promise<boolean> =>
  new Promise((resolve) => {
    setTimeout(() => resolve(true), time);
  });
