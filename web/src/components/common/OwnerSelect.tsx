import React from 'react';
import { useQuery } from '@tanstack/react-query';
import { apiFetch } from '../../api/client';
import { useAuth, UserSession } from '../../context/AuthContext';

export function canModifyRecord(
  user: UserSession | null,
  ownerId: string | null | undefined,
): boolean {
  if (!user) return false;
  if (user.role === 'ADMIN' || user.role === 'VERTRIEB' || user.role === 'BACKOFFICE') {
    return true;
  }
  if (!ownerId) {
    return false;
  }
  return ownerId === user.user_id;
}

export function canReassignRecord(
  user: UserSession | null,
  currentOwnerId: string | null | undefined,
): boolean {
  if (!user) return false;
  if (user.role === 'ADMIN') {
    return true;
  }
  if (!currentOwnerId) {
    return user.role === 'VERTRIEB' || user.role === 'BACKOFFICE';
  }
  return currentOwnerId === user.user_id;
}

interface OwnerSelectProps {
  value: string | null | undefined;
  onChange?: (ownerId: string) => void;
  disabled?: boolean;
  label?: string;
}

export const OwnerSelect: React.FC<OwnerSelectProps> = ({
  value,
  onChange,
  disabled = false,
  label = 'Zuständiger Eigentümer',
}) => {
  const { user } = useAuth();
  const canAdminUsers = user?.role === 'ADMIN';
  const canReassign = canReassignRecord(user, value);

  const { data: users = [] } = useQuery<any[]>({
    queryKey: ['users-list-owner-select'],
    queryFn: () => apiFetch('/api/v1/users'),
    enabled: Boolean(canAdminUsers),
  });

  const selectedUser = users.find((u) => u.id === value);
  const displayName = selectedUser
    ? `${selectedUser.first_name || ''} ${selectedUser.last_name || ''} (${selectedUser.email})`.trim()
    : value
      ? value
      : 'Nicht zugewiesen (System)';

  if (!canAdminUsers || !canReassign || disabled) {
    return (
      <div className="space-y-1">
        {label && <label className="block text-xs font-semibold text-slate-400">{label}</label>}
        <div className="px-3 py-2 bg-slate-900 border border-slate-800 rounded-xl text-xs text-slate-300">
          {displayName}
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-1">
      {label && <label className="block text-xs font-semibold text-slate-400">{label}</label>}
      <select
        value={value || ''}
        onChange={(e) => onChange?.(e.target.value)}
        className="w-full px-3 py-2 bg-slate-900 border border-slate-800 rounded-xl text-xs text-slate-200 focus:outline-none focus:border-emerald-500"
      >
        <option value="">-- Nicht zugewiesen --</option>
        {users.map((u) => (
          <option key={u.id} value={u.id}>
            {u.first_name || u.last_name
              ? `${u.first_name || ''} ${u.last_name || ''} (${u.email})`
              : u.email}
          </option>
        ))}
      </select>
    </div>
  );
};
