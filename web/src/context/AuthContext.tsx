import React, { createContext, useContext, useState, useEffect, useCallback } from 'react';
import { apiFetch } from '../api/client';

export interface UserSession {
  user_id: string;
  email: string;
  role: 'ADMIN' | 'BENUTZER' | 'VERTRIEB' | 'BACKOFFICE';
  first_name?: string;
  last_name?: string;
  /**
   * The real 2FA state read from the users table by GET /api/v1/me.
   *
   * It is `boolean | null` on purpose: `null` means the server could not read the
   * user row, which is "unknown", not "disabled". The UI must never present an
   * unknown state as "2FA is off", because an admin who believes 2FA is off may
   * leave it unprotected.
   */
  totp_enabled?: boolean | null;
  /**
   * Set to true for initial bootstrap admin credentials until the password is changed.
   * null means unknown, never treated as "no change needed".
   */
  must_change_password?: boolean | null;
}

export interface LoginResult {
  totpRequired?: boolean;
  message?: string;
}

interface LoginApiResponse {
  token?: string;
  refresh_token?: string;
  totp_required?: boolean;
  message?: string;
  user?: UserSession;
}

interface AuthContextType {
  user: UserSession | null;
  loading: boolean;
  login: (email: string, pass: string, totpCode?: string) => Promise<LoginResult>;
  logout: () => void;
  /**
   * Re-reads the session from GET /api/v1/me. The settings screen calls it after
   * a successful 2FA change so the displayed state comes from the server rather
   * than from an optimistic local toggle.
   */
  refreshUser: () => Promise<UserSession | null>;
}

const AuthContext = createContext<AuthContextType>({
  user: null,
  loading: true,
  login: async () => ({}),
  logout: () => {},
  refreshUser: async () => null,
});

export const AuthProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const [user, setUser] = useState<UserSession | null>(null);
  const [loading, setLoading] = useState<boolean>(true);

  const refreshUser = useCallback(async (): Promise<UserSession | null> => {
    try {
      const session = await apiFetch<UserSession>('/api/v1/me');
      setUser(session);
      return session;
    } catch {
      // A failed refresh is reported as "no session", never as a partial one.
      setUser(null);
      return null;
    }
  }, []);

  useEffect(() => {
    // Check active session on mount via HttpOnly cookie
    refreshUser().finally(() => setLoading(false));

    const handleAuthLogout = () => {
      setUser(null);
    };
    window.addEventListener('auth:logout', handleAuthLogout);
    return () => window.removeEventListener('auth:logout', handleAuthLogout);
  }, [refreshUser]);

  const login = async (email: string, pass: string, totpCode?: string): Promise<LoginResult> => {
    const res = await apiFetch<LoginApiResponse>('/api/v1/auth/login', {
      method: 'POST',
      body: JSON.stringify({
        email: email.trim().toLowerCase(),
        password: pass,
        totp_code: totpCode,
      }),
    });

    if (res && res.totp_required) {
      return {
        totpRequired: true,
        message: res.message || 'Bitte geben Sie Ihren 6-stelligen 2FA Authenticator-Code ein.',
      };
    }

    if (res && res.user) {
      setUser(res.user);
      // The login response does not carry the stored TOTP flag, so re-read the
      // session instead of leaving the settings screen on a local default.
      await refreshUser();
      return { totpRequired: false };
    }

    throw new Error('Ungültige Anmeldeantwort vom Server erhalten');
  };

  const logout = () => {
    apiFetch('/api/v1/auth/logout', { method: 'POST' }).catch(() => {});
    setUser(null);
  };

  return (
    <AuthContext.Provider value={{ user, loading, login, logout, refreshUser }}>
      {children}
    </AuthContext.Provider>
  );
};

export const useAuth = () => useContext(AuthContext);
