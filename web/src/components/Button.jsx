import React from 'react';
import './Button.css';

export default function Button({ children, variant = 'primary', type = 'button', onClick, className = '', disabled = false, ...props }) {
    return (
        <button
            type={type}
            className={`yq-button yq-button-${variant} ${className}`}
            onClick={onClick}
            disabled={disabled}
            {...props}
        >
            {children}
        </button>
    );
}
