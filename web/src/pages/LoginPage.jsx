import React, { useState } from 'react';
import { useNavigate, Link } from 'react-router-dom';
import useAuth from '../store/useAuth';
import Input from '../components/Input';
import Button from '../components/Button';
import './Auth.css';

export default function LoginPage() {
    const [username, setUsername] = useState('');
    const [password, setPassword] = useState('');
    const [error, setError] = useState('');
    const [loading, setLoading] = useState(false);
    const { login } = useAuth();
    const navigate = useNavigate();

    const handleSubmit = async (e) => {
        e.preventDefault();
        if (!username || !password) {
            setError('\u8bf7\u8f93\u5165\u7528\u6237\u540d\u548c\u5bc6\u7801');
            return;
        }

        setLoading(true);
        setError('');
        try {
            await login(username, password);
            navigate('/');
        } catch (err) {
            setError(err.message || 'Login failed');
        } finally {
            setLoading(false);
        }
    };

    return (
        <div className="yq-auth-page">
            <div className="yq-auth-card">
                <h1 className="yq-auth-title">{'\u6b22\u8fce\u56de\u6765'}</h1>
                <p className="yq-auth-subtitle">{'\u767b\u5f55\u5230\u60a8\u7684\u5de5\u4f5c\u533a'}</p>

                {error && <div className="yq-auth-error">{error}</div>}

                <form onSubmit={handleSubmit} className="yq-auth-form">
                    <Input
                        label={'\u7528\u6237\u540d\u6216\u90ae\u7bb1'}
                        value={username}
                        onChange={(e) => setUsername(e.target.value)}
                        autoComplete="username"
                    />
                    <Input
                        label={'\u5bc6\u7801'}
                        type="password"
                        value={password}
                        onChange={(e) => setPassword(e.target.value)}
                        autoComplete="current-password"
                    />
                    <Button type="submit" className="yq-auth-submit" disabled={loading}>
                        {loading ? '\u767b\u5f55\u4e2d...' : '\u767b\u5f55'}
                    </Button>
                </form>

                <div className="yq-auth-footer">
                    {'\u8fd8\u6ca1\u6709\u8d26\u53f7\uff1f'} <Link to="/register" className="yq-link">{'\u6ce8\u518c'}</Link>
                </div>
            </div>
        </div>
    );
}

