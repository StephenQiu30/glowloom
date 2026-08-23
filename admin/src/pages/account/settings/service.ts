import { queryCurrentIdentity } from '@/services/identity';
import { getCityOptions, provinceOptions } from '@/utils/chinaDivision';
import type { CurrentUser, GeographicItemType } from './data';

export async function queryCurrent(): Promise<{ data: CurrentUser }> {
  const response = await queryCurrentIdentity();
  const identity = response.data;
  return {
    data: {
      name: identity.name || identity.email || identity.userid || '',
      avatar: '',
      userid: identity.userid || '',
      notice: [],
      email: identity.email || '',
      signature: '',
      title: identity.role || '',
      group: identity.workspaceName || '',
      tags: [],
      notifyCount: 0,
      unreadCount: 0,
      country: 'China',
      geographic: {
        province: { label: '', key: '' },
        city: { label: '', key: '' },
      },
      address: '',
      phone: '',
    },
  };
}

export async function queryProvince(): Promise<GeographicItemType[]> {
  return provinceOptions;
}

export async function queryCity(
  province: string,
): Promise<GeographicItemType[]> {
  return getCityOptions(province);
}
