import React, { useEffect } from 'react';
import Button from './Button';
import './Modal.css';

export default function Modal({ isOpen, onClose, title, children, footer, width = 500 }) {
    useEffect(() => {
        document.body.style.overflow = isOpen ? 'hidden' : 'auto';
        return () => {
            document.body.style.overflow = 'auto';
        };
    }, [isOpen]);

    if (!isOpen) return null;

    return (
        <div className="yq-modal-overlay" onClick={onClose}>
            <div
                className="yq-modal-content"
                style={{ width }}
                onClick={(event) => event.stopPropagation()}
            >
                <div className="yq-modal-header">
                    <h3 className="yq-modal-title">{title}</h3>
                    <button
                        type="button"
                        className="yq-modal-close"
                        aria-label="Close modal"
                        onClick={onClose}
                    >
                        x
                    </button>
                </div>
                <div className="yq-modal-body">{children}</div>
                {footer !== false && (
                    <div className="yq-modal-footer">
                        {footer || (
                            <Button variant="secondary" onClick={onClose}>Close</Button>
                        )}
                    </div>
                )}
            </div>
        </div>
    );
}


