import React from 'react';
import './Input.css';

export default function Input({ label, error, className = '', ...props }) {
    return (
        <div className={`yq-input-wrapper ${className}`}>
            {label && <label className="yq-input-label">{label}</label>}
            <input
                className={`yq-input ${error ? 'yq-input-error' : ''}`}
                {...props}
            />
            {error && <span className="yq-input-error-msg">{error}</span>}
        </div>
    );
}
