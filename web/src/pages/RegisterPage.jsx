import React, { useRef, useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import useAuth from '../store/useAuth';
import Input from '../components/Input';
import Button from '../components/Button';
import './Auth.css';

export default function RegisterPage() {
    const [formData, setFormData] = useState({
        username: '',
        email: '',
        password: '',
        confirm_password: '',
    });
    const [file, setFile] = useState(null);
    const [preview, setPreview] = useState('');
    const [error, setError] = useState('');
    const [loading, setLoading] = useState(false);
    const fileInputRef = useRef(null);
    const { register } = useAuth();
    const navigate = useNavigate();

    const onChange = (event) => {
        setFormData({ ...formData, [event.target.name]: event.target.value });
    };

    const onFileChange = (event) => {
        const selected = event.target.files?.[0];
        if (!selected) return;
        setFile(selected);
        const reader = new FileReader();
        reader.onloadend = () => setPreview(String(reader.result || ''));
        reader.readAsDataURL(selected);
    };

    const onSubmit = async (event) => {
        event.preventDefault();
        if (formData.password !== formData.confirm_password) {
            setError('\u4e24\u6b21\u8f93\u5165\u7684\u5bc6\u7801\u4e0d\u4e00\u81f4');
            return;
        }
        if (!file) {
            setError('\u8bf7\u4e0a\u4f20\u5934\u50cf');
            return;
        }

        setLoading(true);
        setError('');
        try {
            const data = new FormData();
            Object.keys(formData).forEach((key) => data.append(key, formData[key]));
            data.append('file', file);
            await register(data);
            navigate('/login');
        } catch (err) {
            setError(err.message || '\u6ce8\u518c\u5931\u8d25');
        } finally {
            setLoading(false);
        }
    };

    return (
        <div className="yq-auth-page">
            <div className="yq-auth-card">
                <h1 className="yq-auth-title">{'\u521b\u5efa\u8d26\u53f7'}</h1>
                <p className="yq-auth-subtitle">{'\u4eca\u5929\u5c31\u5f00\u59cb\u89c4\u5212\u4efb\u52a1'}</p>

                {error && <div className="yq-auth-error">{error}</div>}

                <form onSubmit={onSubmit} className="yq-auth-form">
                    <div className="yq-avatar-upload" onClick={() => fileInputRef.current?.click()}>
                        {preview ? (
                            <img src={preview} alt="Avatar Preview" className="yq-avatar-preview" />
                        ) : (
                            <div className="yq-avatar-placeholder">{'+ \u4e0a\u4f20\u5934\u50cf'}</div>
                        )}
                        <input
                            type="file"
                            ref={fileInputRef}
                            onChange={onFileChange}
                            accept="image/*"
                            style={{ display: 'none' }}
                        />
                    </div>

                    <Input label={'\u90ae\u7bb1'} name="email" value={formData.email} onChange={onChange} required />
                    <Input label={'\u7528\u6237\u540d'} name="username" value={formData.username} onChange={onChange} required minLength={2} />
                    <Input label={'\u5bc6\u7801'} type="password" name="password" value={formData.password} onChange={onChange} required minLength={8} />
                    <Input
                        label={'\u786e\u8ba4\u5bc6\u7801'}
                        type="password"
                        name="confirm_password"
                        value={formData.confirm_password}
                        onChange={onChange}
                        required
                        minLength={8}
                    />

                    <Button type="submit" className="yq-auth-submit" disabled={loading}>
                        {loading ? '\u63d0\u4ea4\u4e2d...' : '\u6ce8\u518c'}
                    </Button>
                </form>

                <div className="yq-auth-footer">
                    {'\u5df2\u7ecf\u6709\u8d26\u53f7\u4e86\uff1f'} <Link to="/login" className="yq-link">{'\u767b\u5f55'}</Link>
                </div>
            </div>
        </div>
    );
}
