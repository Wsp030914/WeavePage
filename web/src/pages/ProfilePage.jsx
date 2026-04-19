import React, { useEffect, useRef, useState } from 'react';
import useAuth from '../store/useAuth';
import { updateProfile } from '../api/user';
import Input from '../components/Input';
import Button from '../components/Button';
import Avatar from '../components/Avatar';
import './ProfilePage.css';

export default function ProfilePage() {
    const { user, updateUserToken } = useAuth();
    const [isEditing, setIsEditing] = useState(false);
    const [formData, setFormData] = useState({
        username: '',
        email: '',
        password: '',
        confirm_password: '',
    });
    const [file, setFile] = useState(null);
    const [preview, setPreview] = useState('');
    const [error, setError] = useState('');
    const [saving, setSaving] = useState(false);
    const fileInputRef = useRef(null);

    useEffect(() => {
        if (!user) return;
        setFormData((prev) => ({
            ...prev,
            username: user.username || '',
            email: user.email || '',
            password: '',
            confirm_password: '',
        }));
        setPreview(user.avatar_url || '');
        setFile(null);
    }, [user, isEditing]);

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
        setError('');

        if (formData.password && formData.password !== formData.confirm_password) {
            setError('Passwords do not match.');
            return;
        }

        setSaving(true);
        try {
            const data = new FormData();
            data.append('username', formData.username);
            data.append('email', formData.email);
            if (formData.password) data.append('password', formData.password);
            if (file) data.append('file', file);

            const response = await updateProfile(data);
            if (response && response.access_token) {
                await updateUserToken(response.access_token, null);
            } else {
                await updateUserToken(null, null);
            }
            setIsEditing(false);
        } catch (err) {
            setError(err.message || 'Failed to update profile');
        } finally {
            setSaving(false);
        }
    };

    if (!user) return null;

    return (
        <div className="yq-page-container yq-profile-page">
            <div className="yq-page-header">
                <h1>Profile</h1>
                {!isEditing && <Button onClick={() => setIsEditing(true)}>Edit Profile</Button>}
            </div>

            <div className="yq-profile-card">
                {error && <div className="yq-error-alert">{error}</div>}

                {!isEditing ? (
                    <div className="yq-profile-view">
                        <Avatar src={user.avatar_url} alt={user.username} size={80} className="yq-profile-avatar-large" />
                        <div className="yq-profile-info-grid">
                            <div className="yq-info-item">
                                <span className="yq-info-label">Username</span>
                                <span className="yq-info-value">{user.username}</span>
                            </div>
                            <div className="yq-info-item">
                                <span className="yq-info-label">Email</span>
                                <span className="yq-info-value">{user.email}</span>
                            </div>
                        </div>
                    </div>
                ) : (
                    <form className="yq-profile-form" onSubmit={onSubmit}>
                        <div
                            className="yq-avatar-upload"
                            onClick={() => fileInputRef.current?.click()}
                            style={{ margin: '0 0 24px 0' }}
                        >
                            {preview ? (
                                <img src={preview} alt="Avatar Preview" className="yq-avatar-preview" />
                            ) : (
                                <div className="yq-avatar-placeholder">Upload Avatar</div>
                            )}
                            <input
                                type="file"
                                ref={fileInputRef}
                                onChange={onFileChange}
                                accept="image/*"
                                style={{ display: 'none' }}
                            />
                        </div>
                        <div className="yq-avatar-upload-hint">Click image to upload a new avatar</div>

                        <div className="yq-form-row">
                            <Input
                                label="Username"
                                value={formData.username}
                                onChange={(event) => setFormData({ ...formData, username: event.target.value })}
                                required
                            />
                            <Input
                                label="Email"
                                type="email"
                                value={formData.email}
                                onChange={(event) => setFormData({ ...formData, email: event.target.value })}
                                required
                            />
                        </div>

                        <div className="yq-form-divider">Change Password (optional)</div>

                        <div className="yq-form-row">
                            <Input
                                label="New Password"
                                type="password"
                                value={formData.password}
                                onChange={(event) => setFormData({ ...formData, password: event.target.value })}
                                minLength={8}
                            />
                            <Input
                                label="Confirm New Password"
                                type="password"
                                value={formData.confirm_password}
                                onChange={(event) => setFormData({ ...formData, confirm_password: event.target.value })}
                                minLength={8}
                            />
                        </div>

                        <div className="yq-profile-form-actions">
                            <Button variant="secondary" type="button" onClick={() => setIsEditing(false)}>
                                Cancel
                            </Button>
                            <Button type="submit" disabled={saving}>
                                {saving ? 'Saving...' : 'Save Changes'}
                            </Button>
                        </div>
                    </form>
                )}
            </div>
        </div>
    );
}
