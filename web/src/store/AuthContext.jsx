import React, { useState, useEffect } from 'react';
import { getProfile, login as apiLogin, register as apiRegister, logout as apiLogout } from '../api/user';
import { AuthContext } from './auth-context';

export const AuthProvider = ({ children }) => {
    const [user, setUser] = useState(null);
    const [loading, setLoading] = useState(true);

    const checkAuth = async () => {
        const token = localStorage.getItem('access_token');
        if (!token) {
            setLoading(false);
            return;
        }

        try {
            const userData = await getProfile();
            setUser(userData);
        } catch (err) {
            console.error("Auth check failed:", err);
            localStorage.removeItem('access_token');
            setUser(null);
        } finally {
            setLoading(false);
        }
    };

    useEffect(() => {
        checkAuth();
    }, []);

    const login = async (username, password) => {
        const res = await apiLogin(username, password);
        localStorage.setItem('access_token', res.access_token);
        await checkAuth();
    };

    const register = async (formData) => {
        await apiRegister(formData);
        // Registration doesn't automatically login in our backend, need to redirect to login
    };

    const logout = async () => {
        try {
            if (localStorage.getItem('access_token')) {
                await apiLogout();
            }
        } catch (err) {
            console.error(err);
        } finally {
            localStorage.removeItem('access_token');
            setUser(null);
        }
    };

    const updateUserToken = async (tokenStr, newUserObj) => {
        if (tokenStr) {
            localStorage.setItem('access_token', tokenStr);
        }
        if (newUserObj) {
            setUser(newUserObj);
        } else {
            await checkAuth();
        }
    };

    return (
        <AuthContext.Provider value={{ user, loading, login, register, logout, checkAuth, updateUserToken }}>
            {children}
        </AuthContext.Provider>
    );
};
