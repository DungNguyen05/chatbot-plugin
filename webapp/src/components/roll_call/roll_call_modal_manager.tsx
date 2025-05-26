// webapp/src/components/roll_call/roll_call_modal_manager.tsx
import React from 'react';
import {createPortal} from 'react-dom';

import RollCallInterface from './roll_call_interface';

interface RollCallModalManagerProps {
    isOpen: boolean;
    onClose: () => void;
}

const RollCallModalManager: React.FC<RollCallModalManagerProps> = ({isOpen, onClose}) => {
    if (!isOpen) {
        return null;
    }

    const modalRoot = document.getElementById('root') || document.body;

    return createPortal(
        <div
            style={{
                position: 'fixed',
                top: 0,
                left: 0,
                width: '100%',
                height: '100%',
                backgroundColor: 'rgba(0, 0, 0, 0.5)',
                display: 'flex',
                justifyContent: 'center',
                alignItems: 'center',
                zIndex: 9999,
            }}
            onClick={(e) => {
                if (e.target === e.currentTarget) {
                    onClose();
                }
            }}
        >
            <div
                style={{
                    background: 'white',
                    borderRadius: '8px',
                    maxWidth: '90%',
                    maxHeight: '90%',
                    overflow: 'auto',
                    boxShadow: '0 4px 12px rgba(0, 0, 0, 0.3)',
                }}
            >
                <div
                    style={{
                        display: 'flex',
                        justifyContent: 'space-between',
                        alignItems: 'center',
                        padding: '16px 24px',
                        borderBottom: '1px solid rgba(var(--center-channel-color-rgb), 0.16)',
                    }}
                >
                    <h2
                        style={{
                            margin: 0,
                            fontSize: '20px',
                            fontWeight: 600,
                            color: 'var(--center-channel-color)',
                        }}
                    >
                        Roll Call
                    </h2>
                    <button
                        onClick={onClose}
                        style={{
                            background: 'none',
                            border: 'none',
                            fontSize: '24px',
                            cursor: 'pointer',
                            color: 'var(--center-channel-color)',
                            padding: 0,
                            width: '32px',
                            height: '32px',
                            display: 'flex',
                            alignItems: 'center',
                            justifyContent: 'center',
                            borderRadius: '4px',
                        }}
                        onMouseEnter={(e) => {
                            e.currentTarget.style.backgroundColor = 'rgba(var(--center-channel-color-rgb), 0.08)';
                        }}
                        onMouseLeave={(e) => {
                            e.currentTarget.style.backgroundColor = 'transparent';
                        }}
                    >
                        ×
                    </button>
                </div>
                <RollCallInterface onClose={onClose} />
            </div>
        </div>,
        modalRoot
    );
};

export default RollCallModalManager;