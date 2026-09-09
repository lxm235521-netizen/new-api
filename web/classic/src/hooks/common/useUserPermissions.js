/*
Copyright (C) 2025 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { useState, useEffect, useCallback } from 'react';
import { API } from '../../helpers/api';

let permissionsCache = null;
let permissionsRequest = null;

const fetchPermissions = async () => {
  if (permissionsCache) return permissionsCache;
  if (!permissionsRequest) {
    permissionsRequest = API.get('/api/user/self')
      .then((res) => {
        if (!res.data.success) {
          throw new Error(res.data.message || '获取权限失败');
        }
        const userData = res.data.data;
        permissionsCache = {
          permissions: {
            ...(userData.permissions || {}),
            admin_permissions: userData.admin_permissions || {},
          },
          role: userData.role,
          canManageRedemptions: userData.can_manage_redemptions === true,
        };
        return permissionsCache;
      })
      .finally(() => {
        permissionsRequest = null;
      });
  }
  return permissionsRequest;
};

export const invalidateUserPermissions = () => {
  permissionsCache = null;
};

export const useUserPermissions = () => {
  const [state, setState] = useState({
    permissions: permissionsCache?.permissions || null,
    role: permissionsCache?.role || null,
    canManageRedemptions: permissionsCache?.canManageRedemptions || false,
    loading: !permissionsCache,
    error: null,
  });

  const loadPermissions = useCallback(async () => {
    setState((current) => ({ ...current, loading: true, error: null }));
    try {
      const data = await fetchPermissions();
      setState({ ...data, loading: false, error: null });
      return data;
    } catch (error) {
      setState((current) => ({
        ...current,
        loading: false,
        error: error.message || '网络错误，请重试',
      }));
      throw error;
    }
  }, []);

  useEffect(() => {
    if (permissionsCache) {
      setState({ ...permissionsCache, loading: false, error: null });
      return;
    }
    loadPermissions().catch(() => {});
  }, [loadPermissions]);

  const hasAdminPermission = (permission) =>
    state.permissions?.admin_permissions?.[permission] === true;

  const isRoot = () => state.role >= 100;

  const hasSidebarSettingsPermission = () =>
    state.permissions?.sidebar_settings === true;

  const isSidebarSectionAllowed = (sectionKey) => {
    if (!state.permissions?.sidebar_modules) return true;
    const sectionPerms = state.permissions.sidebar_modules[sectionKey];
    return sectionPerms !== false;
  };

  const isSidebarModuleAllowed = (sectionKey, moduleKey) => {
    if (!state.permissions?.sidebar_modules) return true;
    const sectionPerms = state.permissions.sidebar_modules[sectionKey];
    if (sectionPerms === false) return false;
    if (sectionPerms && sectionPerms[moduleKey] === false) return false;
    return true;
  };

  const getAllowedSidebarSections = () => {
    if (!state.permissions?.sidebar_modules) return [];
    return Object.keys(state.permissions.sidebar_modules).filter((sectionKey) =>
      isSidebarSectionAllowed(sectionKey),
    );
  };

  const getAllowedSidebarModules = (sectionKey) => {
    if (!state.permissions?.sidebar_modules) return [];
    const sectionPerms = state.permissions.sidebar_modules[sectionKey];
    if (sectionPerms === false) return [];
    if (!sectionPerms || typeof sectionPerms !== 'object') return [];
    return Object.keys(sectionPerms).filter(
      (moduleKey) =>
        moduleKey !== 'enabled' && sectionPerms[moduleKey] === true,
    );
  };

  return {
    permissions: state.permissions,
    role: state.role,
    loading: state.loading,
    error: state.error,
    loadPermissions,
    hasAdminPermission,
    isRoot,
    canManageRedemptions: state.canManageRedemptions,
    hasSidebarSettingsPermission,
    isSidebarSectionAllowed,
    isSidebarModuleAllowed,
    getAllowedSidebarSections,
    getAllowedSidebarModules,
  };
};

export default useUserPermissions;
