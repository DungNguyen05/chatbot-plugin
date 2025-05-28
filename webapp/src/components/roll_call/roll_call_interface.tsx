import React, {useState} from 'react';
import styled, { keyframes } from 'styled-components';

import {doCheckIn, doCheckOut, doAbsent} from '../../client';

// Keyframes for animations
const slideInUp = keyframes`
    from {
        opacity: 0;
        transform: translateY(20px);
    }
    to {
        opacity: 1;
        transform: translateY(0);
    }
`;

const pulseGlow = keyframes`
    0%, 100% {
        box-shadow: 0 0 5px rgba(76, 175, 80, 0.3);
    }
    50% {
        box-shadow: 0 0 20px rgba(76, 175, 80, 0.6), 0 0 30px rgba(76, 175, 80, 0.4);
    }
`;

const shimmer = keyframes`
    0% {
        background-position: -200px 0;
    }
    100% {
        background-position: calc(200px + 100%) 0;
    }
`;

const Container = styled.div<{show: boolean}>`
    display: ${props => props.show ? 'flex' : 'none'};
    flex-direction: column;
    padding: 40px;
    gap: 28px;
    max-width: 550px;
    width: 100%;
    margin: 0 auto;
    background: linear-gradient(135deg, 
        var(--center-channel-bg) 0%, 
        rgba(var(--center-channel-color-rgb), 0.02) 100%);
    border-radius: 16px;
    color: var(--center-channel-color);
    position: relative;
    animation: ${slideInUp} 0.3s ease-out;
    box-shadow: 
        0 20px 60px rgba(0, 0, 0, 0.1),
        0 8px 24px rgba(0, 0, 0, 0.06),
        inset 0 1px 0 rgba(255, 255, 255, 0.1);
    border: 1px solid rgba(var(--center-channel-color-rgb), 0.08);
    backdrop-filter: blur(20px);
`;

const HeaderSection = styled.div`
    text-align: center;
    position: relative;
`;

const Title = styled.h2`
    font-size: 28px;
    font-weight: 700;
    margin-bottom: 12px;
    background: linear-gradient(135deg, var(--center-channel-color), rgba(var(--center-channel-color-rgb), 0.7));
    -webkit-background-clip: text;
    -webkit-text-fill-color: transparent;
    background-clip: text;
    font-family: var(--font-family);
    letter-spacing: -0.02em;
    line-height: 1.2;
`;

const Subtitle = styled.p`
    font-size: 16px;
    color: rgba(var(--center-channel-color-rgb), 0.65);
    margin: 0 0 8px 0;
    font-weight: 400;
    line-height: 1.4;
`;

const DateBadge = styled.div`
    display: inline-flex;
    align-items: center;
    gap: 8px;
    padding: 8px 16px;
    background: rgba(var(--center-channel-color-rgb), 0.06);
    border-radius: 20px;
    font-size: 14px;
    font-weight: 500;
    color: rgba(var(--center-channel-color-rgb), 0.8);
    border: 1px solid rgba(var(--center-channel-color-rgb), 0.1);
    margin-top: 12px;
`;

const ButtonGrid = styled.div`
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 20px;
    margin-bottom: 12px;
    
    @media (max-width: 480px) {
        grid-template-columns: 1fr;
        gap: 16px;
    }
`;

const ActionButton = styled.button`
    padding: 18px 24px;
    border: none;
    border-radius: 12px;
    font-weight: 600;
    font-size: 15px;
    cursor: pointer;
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 10px;
    transition: all 0.2s cubic-bezier(0.4, 0, 0.2, 1);
    min-height: 56px;
    background: rgba(var(--center-channel-color-rgb), 0.04);
    color: rgba(var(--center-channel-color-rgb), 0.8);
    position: relative;
    overflow: hidden;
    border: 1px solid rgba(var(--center-channel-color-rgb), 0.12);
    
    &::before {
        content: '';
        position: absolute;
        top: 0;
        left: -100%;
        width: 100%;
        height: 100%;
        background: linear-gradient(
            90deg,
            transparent,
            rgba(255, 255, 255, 0.1),
            transparent
        );
        transition: left 0.5s;
    }
    
    &:disabled {
        opacity: 0.4;
        cursor: not-allowed;
        transform: none !important;
    }
    
    &:not(:disabled):hover {
        transform: translateY(-2px);
        box-shadow: 0 8px 25px rgba(0, 0, 0, 0.15);
        border-color: rgba(var(--center-channel-color-rgb), 0.2);
        
        &::before {
            left: 100%;
        }
    }
    
    &:not(:disabled):active {
        transform: translateY(-1px);
        transition: all 0.1s ease-out;
    }
`;

const CheckInButton = styled(ActionButton)`
    background: linear-gradient(135deg, #4CAF50, #45a049);
    color: white;
    border: 1px solid #45a049;
    
    &:hover:not(:disabled) {
        background: linear-gradient(135deg, #45a049, #3d8b40);
        box-shadow: 0 8px 25px rgba(76, 175, 80, 0.3);
        animation: ${pulseGlow} 2s infinite;
    }
    
    &:active:not(:disabled) {
        background: linear-gradient(135deg, #3d8b40, #45a049);
    }
`;

const CheckOutButton = styled(ActionButton)`
    background: linear-gradient(135deg, #2196F3, #1976D2);
    color: white;
    border: 1px solid #1976D2;
    
    &:hover:not(:disabled) {
        background: linear-gradient(135deg, #1976D2, #1565C0);
        box-shadow: 0 8px 25px rgba(33, 150, 243, 0.3);
    }
    
    &:active:not(:disabled) {
        background: linear-gradient(135deg, #1565C0, #1976D2);
    }
`;

const AbsentButton = styled(ActionButton)`
    background: transparent;
    color: #f44336;
    border: 2px solid #f44336;
    grid-column: 1 / -1;
    position: relative;
    
    &::after {
        content: '';
        position: absolute;
        top: 0;
        left: 0;
        right: 0;
        bottom: 0;
        background: linear-gradient(135deg, #f44336, #d32f2f);
        opacity: 0;
        transition: opacity 0.2s ease;
        border-radius: 10px;
        z-index: -1;
    }
    
    &:hover:not(:disabled) {
        color: white;
        border-color: #d32f2f;
        box-shadow: 0 8px 25px rgba(244, 67, 54, 0.3);
        
        &::after {
            opacity: 1;
        }
    }
`;

const AbsentModal = styled.div<{show: boolean}>`
    display: ${props => props.show ? 'flex' : 'none'};
    width: 100%;
    height: 100%;
    background-color: rgba(0, 0, 0, 0.7);
    backdrop-filter: blur(12px);
    justify-content: center;
    align-items: center;
    z-index: 1000;
    animation: ${slideInUp} 0.2s ease-out;
`;

const ModalContent = styled.div`
    background: var(--center-channel-bg);
    padding: 40px;
    border-radius: 16px;
    min-width: 450px;
    max-width: 90%;
    box-shadow: 
        0 25px 60px rgba(0, 0, 0, 0.2),
        0 8px 24px rgba(0, 0, 0, 0.1);
    transform: scale(1);
    animation: ${slideInUp} 0.3s ease-out;
    border: 1px solid rgba(var(--center-channel-color-rgb), 0.12);
    position: relative;
`;

const ModalTitle = styled.h3`
    margin-bottom: 24px;
    font-size: 24px;
    font-weight: 600;
    color: var(--center-channel-color);
    font-family: var(--font-family);
    letter-spacing: -0.02em;
    text-align: center;
`;

const ReasonInput = styled.textarea`
    width: 100%;
    min-height: 120px;
    padding: 16px 20px;
    border: 2px solid rgba(var(--center-channel-color-rgb), 0.12);
    border-radius: 12px;
    resize: vertical;
    font-family: inherit;
    margin-bottom: 24px;
    font-size: 15px;
    line-height: 1.5;
    background: var(--center-channel-bg);
    color: var(--center-channel-color);
    transition: all 0.2s ease;
    
    &:focus {
        outline: none;
        border-color: #f44336;
        box-shadow: 0 0 0 3px rgba(244, 67, 54, 0.1);
        background: rgba(var(--center-channel-color-rgb), 0.02);
    }
    
    &::placeholder {
        color: rgba(var(--center-channel-color-rgb), 0.5);
    }
`;

const ModalActions = styled.div`
    display: flex;
    gap: 16px;
    justify-content: flex-end;
`;

const SecondaryButton = styled.button`
    padding: 12px 24px;
    border: 2px solid rgba(var(--center-channel-color-rgb), 0.2);
    border-radius: 8px;
    background: var(--center-channel-bg);
    color: var(--center-channel-color);
    font-weight: 600;
    cursor: pointer;
    transition: all 0.2s ease;
    font-size: 14px;
    
    &:hover {
        border-color: rgba(var(--center-channel-color-rgb), 0.3);
        background: rgba(var(--center-channel-color-rgb), 0.04);
        transform: translateY(-1px);
    }
`;

const PrimaryButton = styled.button`
    padding: 12px 24px;
    border: none;
    border-radius: 8px;
    background: linear-gradient(135deg, #f44336, #d32f2f);
    color: white;
    font-weight: 600;
    cursor: pointer;
    transition: all 0.2s ease;
    font-size: 14px;
    
    &:hover:not(:disabled) {
        background: linear-gradient(135deg, #d32f2f, #c62828);
        transform: translateY(-1px);
        box-shadow: 0 4px 12px rgba(244, 67, 54, 0.3);
    }
    
    &:disabled {
        opacity: 0.4;
        cursor: not-allowed;
        transform: none !important;
    }
`;

const StatusMessage = styled.div<{type: 'success' | 'error'}>`
    padding: 16px 20px;
    border-radius: 12px;
    margin-bottom: 24px;
    background: ${props => props.type === 'success' 
        ? 'linear-gradient(135deg, rgba(76, 175, 80, 0.1), rgba(76, 175, 80, 0.05))' 
        : 'linear-gradient(135deg, rgba(244, 67, 54, 0.1), rgba(244, 67, 54, 0.05))'};
    color: ${props => props.type === 'success' ? '#2e7d32' : '#c62828'};
    border: 1px solid ${props => props.type === 'success' ? 'rgba(76, 175, 80, 0.2)' : 'rgba(244, 67, 54, 0.2)'};
    font-size: 14px;
    font-weight: 500;
    display: flex;
    align-items: center;
    gap: 12px;
    
    &::before {
        content: ${props => props.type === 'success' ? '"✓"' : '"⚠"'};
        font-size: 16px;
        font-weight: bold;
    }
`;

const LoadingSpinner = styled.div`
    display: inline-block;
    width: 18px;
    height: 18px;
    border: 2px solid transparent;
    border-top: 2px solid currentColor;
    border-radius: 50%;
    animation: spin 0.8s linear infinite;
    
    @keyframes spin {
        0% { transform: rotate(0deg); }
        100% { transform: rotate(360deg); }
    }
`;

const CloseButton = styled.button`
    position: absolute;
    top: 20px;
    right: 20px;
    background: rgba(var(--center-channel-color-rgb), 0.05);
    border: 1px solid rgba(var(--center-channel-color-rgb), 0.1);
    font-size: 20px;
    cursor: pointer;
    color: rgba(var(--center-channel-color-rgb), 0.6);
    padding: 8px;
    width: 36px;
    height: 36px;
    display: flex;
    align-items: center;
    justify-content: center;
    border-radius: 8px;
    transition: all 0.2s ease;
    z-index: 10;
    
    &:hover {
        background: rgba(var(--center-channel-color-rgb), 0.1);
        color: rgba(var(--center-channel-color-rgb), 0.8);
        transform: scale(1.05);
    }
    
    &:active {
        transform: scale(0.95);
    }
`;

// Built-in translations
const translations = {
    en: {
        'rollcall.title': 'Roll Call',
        'rollcall.subtitle': 'Track your attendance and manage your work schedule',
        'rollcall.checkin.button': 'Check In',
        'rollcall.checkout.button': 'Check Out',
        'rollcall.absent.button': 'Mark Absent',
        'rollcall.checkin.title': 'Check In to Work',
        'rollcall.checkout.title': 'Check Out from Work',
        'rollcall.absent.title': 'Mark Absent',
        'rollcall.checkin.tooltip': 'Mark your arrival for today',
        'rollcall.checkout.tooltip': 'Mark your departure for today',
        'rollcall.absent.tooltip': 'Report that you\'ll be absent today',
        'rollcall.close': 'Close',
        'rollcall.close.tooltip': 'Close (Esc)',
        'rollcall.cancel': 'Cancel',
        'rollcall.confirm': 'Confirm',
        'rollcall.absent.reason.placeholder': 'Enter reason for absence...',
        'rollcall.absent.reason.label': 'Reason for Absence',
        'rollcall.absent.reason.required': 'Please provide a reason for your absence.',
        'rollcall.checkin.success': 'Welcome! You have successfully checked in.',
        'rollcall.checkout.success': 'Have a great day! You have successfully checked out.',
        'rollcall.absent.success': 'Your absence has been recorded. Take care!',
        'rollcall.error': 'An error occurred. Please try again.',
        'rollcall.timeout': 'Request timed out. Please try again.'
    },
    vi: {
        'rollcall.title': 'Điểm Danh',
        'rollcall.subtitle': 'Theo dõi và quản lý lịch làm việc',
        'rollcall.checkin.button': 'Check In',
        'rollcall.checkout.button': 'Check Out',
        'rollcall.absent.button': 'Báo Vắng',
        'rollcall.checkin.title': 'Check In',
        'rollcall.checkout.title': 'Check Out',
        'rollcall.absent.title': 'Báo Vắng Mặt',
        'rollcall.checkin.tooltip': 'Đánh dấu giờ đến hôm nay',
        'rollcall.checkout.tooltip': 'Đánh dấu giờ về hôm nay',
        'rollcall.absent.tooltip': 'Báo cáo vắng mặt hôm nay',
        'rollcall.close': 'Đóng',
        'rollcall.close.tooltip': 'Đóng (Esc)',
        'rollcall.cancel': 'Hủy',
        'rollcall.confirm': 'Xác Nhận',
        'rollcall.absent.reason.placeholder': 'Nhập lý do vắng mặt...',
        'rollcall.absent.reason.label': 'Lý Do Vắng Mặt',
        'rollcall.absent.reason.required': 'Vui lòng cung cấp lý do vắng mặt.',
        'rollcall.checkin.success': 'Chào mừng! Bạn đã check in thành công.',
        'rollcall.checkout.success': 'Chúc bạn một ngày tốt lành! Bạn đã check out thành công.',
        'rollcall.absent.success': 'Thông tin vắng mặt đã được ghi nhận. Hãy chăm sóc sức khỏe!',
        'rollcall.error': 'Đã xảy ra lỗi. Vui lòng thử lại.',
        'rollcall.timeout': 'Yêu cầu đã hết thời gian chờ. Vui lòng thử lại.'
    }
};

// Text helper function with built-in translations
const getText = (key: string, language: 'en' | 'vi' = 'en', t?: (key: string) => string): string => {
    // If external translation function is provided, use it first
    if (t) {
        try {
            return t(key);
        } catch (error) {
            console.warn(`Translation failed for key: ${key}`, error);
        }
    }
    
    // Use built-in translations
    const langTranslations = translations[language] || translations.en;
    return langTranslations[key] || translations.en[key] || key;
};

interface RollCallInterfaceProps {
    onClose?: () => void;
    t?: (key: string) => string; // Your external i18n translation function (optional)
    language?: 'en' | 'vi'; // Built-in language support
    locale?: string; // For date formatting
}

const RollCallInterface: React.FC<RollCallInterfaceProps> = ({
    onClose, 
    t, 
    language = 'vi',
    locale
}) => {
    const [showAbsentModal, setShowAbsentModal] = useState(false);
    const [absentReason, setAbsentReason] = useState('');
    const [loading, setLoading] = useState(false);
    const [statusMessage, setStatusMessage] = useState<{type: 'success' | 'error', message: string} | null>(null);
    const [requestTimeout, setRequestTimeout] = useState<NodeJS.Timeout | null>(null);

    // Use the language for locale if not explicitly provided
    const dateLocale = locale || (language === 'vi' ? 'vi-VN' : 'en-US');

    const getCurrentDate = () => {
        return new Date().toLocaleDateString(dateLocale, {
            weekday: 'long',
            year: 'numeric',
            month: 'long',
            day: 'numeric'
        });
    };

    const clearRequestTimeout = () => {
        if (requestTimeout) {
            clearTimeout(requestTimeout);
            setRequestTimeout(null);
        }
    };

    const handleApiCall = async (apiCall: () => Promise<any>, successMessageKey: string) => {
        setLoading(true);
        setStatusMessage(null);
        
        const timeout = setTimeout(() => {
            setStatusMessage({
                type: 'error',
                message: getText('rollcall.timeout', language, t)
            });
            setLoading(false);
        }, 30000);
        
        setRequestTimeout(timeout);
        
        try {
            const response = await apiCall();
            clearTimeout(timeout);
            setStatusMessage({
                type: 'success',
                message: response?.message || getText(successMessageKey, language, t)
            });
            setTimeout(() => {
                onClose?.();
            }, 2000);
        } catch (error: any) {
            clearTimeout(timeout);
            const errorMessage = error?.message || getText('rollcall.error', language, t);
            setStatusMessage({
                type: 'error',
                message: errorMessage
            });
        } finally {
            setLoading(false);
            setRequestTimeout(null);
        }
    };

    const handleCheckIn = () => handleApiCall(
        doCheckIn, 
        'rollcall.checkin.success'
    );
    
    const handleCheckOut = () => handleApiCall(
        doCheckOut,
        'rollcall.checkout.success'
    );

    const handleAbsentClick = () => {
        setShowAbsentModal(true);
    };

    const handleAbsentCancel = () => {
        setShowAbsentModal(false);
        setAbsentReason('');
        setStatusMessage(null);
        clearRequestTimeout();
    };

    const handleAbsentSubmit = async () => {
        if (!absentReason.trim()) {
            setStatusMessage({
                type: 'error',
                message: getText('rollcall.absent.reason.required', language, t)
            });
            return;
        }

        await handleApiCall(
            () => doAbsent(absentReason.trim()),
            'rollcall.absent.success'
        );
        
        setShowAbsentModal(false);
        setAbsentReason('');
    };

    const handleKeyDown = (event: React.KeyboardEvent) => {
        if (event.key === 'Escape') {
            if (showAbsentModal) {
                handleAbsentCancel();
            } else {
                onClose?.();
            }
        }
    };

    const handleTextareaKeyDown = (event: React.KeyboardEvent<HTMLTextAreaElement>) => {
        if (event.key === 'Enter' && !event.shiftKey) {
            event.preventDefault();
            if (absentReason.trim() && !loading) {
                handleAbsentSubmit();
            }
        }
    };

    return (
        <>
            <Container 
                show={!showAbsentModal}
                onKeyDown={handleKeyDown}
                tabIndex={-1}
                role="dialog"
                aria-labelledby="rollcall-title"
                aria-describedby="rollcall-description"
            > 
                <CloseButton 
                    onClick={onClose}
                    aria-label={getText('rollcall.close', language, t)}
                    title={getText('rollcall.close.tooltip', language, t)}
                >
                    ✕
                </CloseButton>

                <HeaderSection>
                    <Title id="rollcall-title">
                        📋 {getText('rollcall.title', language, t)}
                    </Title>
                    <Subtitle id="rollcall-description">
                        {getText('rollcall.subtitle', language, t)}
                    </Subtitle>
                    <DateBadge>
                        📅 {getCurrentDate()}
                    </DateBadge>
                </HeaderSection>
                
                {statusMessage && (
                    <StatusMessage type={statusMessage.type}>
                        {statusMessage.message}
                    </StatusMessage>
                )}

                <ButtonGrid>
                    <CheckInButton 
                        onClick={handleCheckIn}
                        disabled={loading}
                        aria-label={getText('rollcall.checkin.title', language, t)}
                        title={getText('rollcall.checkin.tooltip', language, t)}
                    >
                        {loading ? <LoadingSpinner /> : null}
                        {getText('rollcall.checkin.button', language, t)}
                    </CheckInButton>
                    
                    <CheckOutButton 
                        onClick={handleCheckOut}
                        disabled={loading}
                        aria-label={getText('rollcall.checkout.title', language, t)}
                        title={getText('rollcall.checkout.tooltip', language, t)}
                    >
                        {loading ? <LoadingSpinner /> : null}
                        {getText('rollcall.checkout.button', language, t)}
                    </CheckOutButton>
                    
                    <AbsentButton 
                        onClick={handleAbsentClick}
                        disabled={loading}
                        aria-label={getText('rollcall.absent.title', language, t)}
                        title={getText('rollcall.absent.tooltip', language, t)}
                    >
                        {getText('rollcall.absent.button', language, t)}
                    </AbsentButton>
                </ButtonGrid>
            </Container>

            <AbsentModal show={showAbsentModal}>
                <ModalContent
                    onKeyDown={handleKeyDown}
                    tabIndex={-1}
                    role="dialog"
                    aria-labelledby="absent-modal-title"
                >
                    <ModalTitle id="absent-modal-title">
                        📝 {getText('rollcall.absent.title', language, t)}
                    </ModalTitle>
                    
                    {statusMessage && (
                        <StatusMessage type={statusMessage.type}>
                            {statusMessage.message}
                        </StatusMessage>
                    )}
                    
                    <ReasonInput
                        placeholder={getText('rollcall.absent.reason.placeholder', language, t)}
                        value={absentReason}
                        onChange={(e) => setAbsentReason(e.target.value)}
                        onKeyDown={handleTextareaKeyDown}
                        maxLength={500}
                        aria-label={getText('rollcall.absent.reason.label', language, t)}
                        autoFocus
                    />
                    
                    <ModalActions>
                        <SecondaryButton 
                            onClick={handleAbsentCancel}
                            disabled={loading}
                        >
                            {getText('rollcall.cancel', language, t)}
                        </SecondaryButton>
                        
                        <PrimaryButton 
                            onClick={handleAbsentSubmit}
                            disabled={loading || !absentReason.trim()}
                            aria-label={getText('rollcall.confirm', language, t)}
                        >
                            {loading ? <LoadingSpinner /> : null}
                            {getText('rollcall.confirm', language, t)}
                        </PrimaryButton>
                    </ModalActions>
                </ModalContent>
            </AbsentModal>
        </>
    );
};

export default RollCallInterface;