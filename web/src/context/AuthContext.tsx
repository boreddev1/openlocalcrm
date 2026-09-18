import React, { createContext, useContext, useState, useEffect } from 'react';
import { apiFetch } from '../api/client';

export interface UserSession {
  user_id: string;
  email: string;
  role: 'ADMIN' | 'BENUTZER';
  first_name?: string;
  last_name?: string;
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
}

const AuthContext = createContext<AuthContextType>({
  user: null,
  loading: true,
  login: async () => ({}),
  logout: () => {},
});

export const AuthProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const [user, setUser] = useState<UserSession | null>(null);
  const [loading, setLoading] = useState<boolean>(true);

  useEffect(() => {
    // Check active session on mount if token is present
    const token = localStorage.getItem('auth_token');
    if (!token) {
      setUser(null);
      setLoading(false);
      return;
    }

    apiFetch<UserSession>('/api/v1/me')
      .then((session) => setUser(session))
      .catch(() => {
        localStorage.removeItem('auth_token');
        setUser(null);
      })
      .finally(() => setLoading(false));

    const handleAuthLogout = () => {
      setUser(null);
    };
    window.addEventListener('auth:logout', handleAuthLogout);
    return () => window.removeEventListener('auth:logout', handleAuthLogout);
  }, []);

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

    if (res && res.token && res.user) {
      localStorage.setItem('auth_token', res.token);
      setUser(res.user);
      return { totpRequired: false };
    }

    throw new Error('Ungültige Anmeldeantwort vom Server erhalten');
  };

  const logout = () => {
    apiFetch('/api/v1/auth/logout', { method: 'POST' }).catch(() => {});
    localStorage.removeItem('auth_token');
    setUser(null);
  };

  return (
    <AuthContext.Provider value={{ user, loading, login, logout }}>{children}</AuthContext.Provider>
  );
};

export const useAuth = () => useContext(AuthContext);
