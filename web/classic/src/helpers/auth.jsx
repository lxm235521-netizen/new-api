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

import React from 'react';
import { Navigate } from 'react-router-dom';
import { history } from './history';
import { useUserPermissions } from '../hooks/common/useUserPermissions';
import Loading from '../components/common/ui/Loading';

export function authHeader() {
  // return authorization header with jwt token
  let user = JSON.parse(localStorage.getItem('user'));

  if (user && user.token) {
    return { Authorization: 'Bearer ' + user.token };
  } else {
    return {};
  }
}

export const AuthRedirect = ({ children }) => {
  const user = localStorage.getItem('user');

  if (user) {
    return <Navigate to='/console' replace />;
  }

  return children;
};

function PrivateRoute({ children }) {
  if (!localStorage.getItem('user')) {
    return <Navigate to='/login' state={{ from: history.location }} />;
  }
  return children;
}

export function AdminPermissionRoute({ permission, children }) {
  const raw = localStorage.getItem('user');
  const { loading, error, hasAdminPermission } = useUserPermissions();

  if (!raw) {
    return <Navigate to='/login' state={{ from: history.location }} />;
  }
  if (loading) return <Loading />;
  if (error) return <Navigate to='/forbidden' replace />;
  return hasAdminPermission(permission) ? children : <Navigate to='/forbidden' replace />;
}

export function AdminRoute({ children }) {
  const raw = localStorage.getItem('user');
  if (!raw) {
    return <Navigate to='/login' state={{ from: history.location }} />;
  }
  try {
    const user = JSON.parse(raw);
    if (user && typeof user.role === 'number' && user.role >= 10) {
      return children;
    }
  } catch (e) {
    // ignore
  }
  return <Navigate to='/forbidden' replace />;
}

export function AuditRoute({ children }) {
  const raw = localStorage.getItem('user');
  const { loading, hasAdminPermission, canManageRedemptions, role } = useUserPermissions();

  if (!raw) {
    return <Navigate to='/login' state={{ from: history.location }} />;
  }
  if (loading) return null;
  return hasAdminPermission('audit') || (role < 10 && canManageRedemptions) ? children : <Navigate to='/forbidden' replace />;
}

export { PrivateRoute };
