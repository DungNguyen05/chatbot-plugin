import React, {useState, useEffect} from 'react';
import styled, { keyframes } from 'styled-components';

import {doCheckIn, doCheckOut, doAbsent} from '../../client';

// Keyframes for animations
const slideUp = keyframes`
    from {
        opacity: 0;
        transform: translateY(30px);
    }
    to {
        opacity: 1;
        transform: translateY(0);
    }
`;

const slideDown = keyframes`
    from {
        opacity: 0;
        transform: translateY(-10px);
        max-height: 0;
    }
    to {
        opacity: 1;
        transform: translateY(0);
        max-height: 200px;
    }
`;

const Container = styled.div<{show: boolean}>`
    display: ${props => props.show ? 'flex' : 'none'};
    flex-direction: column;
    padding: 0;
    gap: 0;
    max-width: 560px; /* Keep original modal width */
    min-width: 530px;
    width: 100%;
    margin: 0 auto;
    background: white;
    border-radius: 16px;
    color: #1f2937;
    position: relative;
    animation: ${slideUp} 0.3s cubic-bezier(0.4, 0, 0.2, 1);
    box-shadow: 
        0 25px 50px -12px rgba(0, 0, 0, 0.25),
        0 0 0 1px rgba(0, 0, 0, 0.05);
    border: 1px solid #e5e7eb;
    overflow: hidden;
    min-height: 600px; /* Keep original modal height */
    max-height: none;
    height: auto; /* Allow natural height calculation */
    
    @media (max-width: 768px) {
        max-width: 95vw; /* Slightly larger on mobile */
        margin: 20px auto;
        border-radius: 12px;
        min-height: 624px; /* Keep original mobile height */
        max-height: none;
    }
`;

const ModalHeader = styled.div`
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 22px 22px 18px 22px; /* 60% of original padding */
    border-bottom: 1px solid #f3f4f6;
    background: #fafafa;
    flex-shrink: 0; /* Prevent header from shrinking */
`;

const Title = styled.h2`
    font-size: 18px; /* 60% of 30px */
    font-weight: 600;
    margin: 0;
    color: #0f172a;
    font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
`;

const CloseButton = styled.button`
    padding: 7px; /* 60% of 12px */
    background: transparent;
    border: none;
    border-radius: 8px;
    cursor: pointer;
    color: #6b7280;
    transition: all 0.2s ease;
    display: flex;
    align-items: center;
    justify-content: center;
    
    &:hover {
        background: #f3f4f6;
        color: #374151;
    }
    
    &:active {
        transform: scale(0.95);
    }
`;

const ModalContent = styled.div`
    padding: 18px 22px 22px 22px; /* 60% of original padding */
    flex: 1;
    display: flex;
    flex-direction: column;
    overflow-y: auto;
    min-height: 0; /* Allow flex shrinking */
`;

const TimeDisplay = styled.div`
    text-align: center;
    padding: 14px; /* 60% of 24px */
    background: #f1f5f9;
    border-radius: 12px;
    margin-bottom: 18px; /* 60% of 30px */
    border: 1px solid #f3f4f6;
`;

const TimeLabel = styled.div`
    font-size: 16px; /* 60% of 21px */
    color: #49576b;
    margin-bottom: 7px; /* 60% of 12px */
    font-weight: 500;
`;

const CurrentTime = styled.div`
    font-family: 'Roboto', -apple-system, BlinkMacSystemFont, sans-serif;
    letter-spacing: 0.5px; // Added for better spacing
    font-size: 25px; /* 60% of 33px */
    font-weight: 700;
    color: #1f2937;
    margin-bottom: 4px; /* 60% of 6px */
    margin-top: -6px;
`;

const CurrentDate = styled.div`
    font-size: 15px; /* 60% of 21px */
    color: #49576b;
    font-weight: 500;
`;

const SectionLabel = styled.label`
    display: block;
    font-size: 14px; /* 60% of 21px */
    font-weight: 600;
    color: #374151;
    margin-bottom: 11px; /* 60% of 18px */
`;

const AttendanceGrid = styled.div`
    display: grid;
    grid-template-columns: 1fr;
    gap: 9px; /* 60% of 15px */
    margin-bottom: 14px; /* 60% of 24px */
    flex: 1;
`;

const AttendanceOption = styled.button<{selected: boolean, variant: 'success' | 'primary' | 'warning'}>`
    padding: 13px; /* 60% of 21px */
    border: 2px solid ${props => {
        if (props.selected) {
            return props.variant === 'success' ? '#079669' : 
                   props.variant === 'primary' ? '#1d40b0' : '#d97708';
        }
        return '#e5e7eb';
    }};
    border-radius: 12px;
    background: ${props => {
        if (props.selected) {
            return props.variant === 'success' ? 'rgba(16, 185, 129, 0.05)' : 
                   props.variant === 'primary' ? 'rgba(59, 130, 246, 0.05)' : 'rgba(245, 158, 11, 0.05)';
        }
        return 'white';
    }};
    cursor: pointer;
    transition: all 0.2s ease;
    text-align: left;
    display: flex;
    align-items: center;
    gap: 11px; /* 60% of 18px */
    font-family: inherit;
    width: 100%;
    
    &:hover {
        border-color: ${props => {
            return props.variant === 'success' ? '#079669' : 
                   props.variant === 'primary' ? '#1d40b0' : '#d97708';
        }};
        background: ${props => {
            if (props.selected) return; // Keep current background if selected
            return props.variant === 'success' ? 'rgba(16, 185, 129, 0.05)' : 
                   props.variant === 'primary' ? 'rgba(59, 130, 246, 0.05)' : 'rgba(245, 158, 11, 0.05)';
        }};
    }
    
    &:disabled {
        opacity: 0.5;
        cursor: not-allowed;
    }
`;

const OptionIcon = styled.div<{variant: 'success' | 'primary' | 'warning', selected: boolean}>`
    width: 32px; /* 60% of 54px */
    height: 32px; /* 60% of 54px */
    border-radius: 8px;
    background: ${props => {
        if (props.selected) {
            return props.variant === 'success' ? '#079669' : 
                   props.variant === 'primary' ? '#1d40b0' : '#d97708';
        }
        return props.variant === 'success' ? 'rgba(16, 185, 129, 0.05)' : 
               props.variant === 'primary' ? 'rgba(59, 130, 246, 0.05)' : 'rgba(245, 158, 11, 0.05)';
    }};
    color: ${props => {
        if (props.selected) return 'white';
        return props.variant === 'success' ? '#079669' : 
               props.variant === 'primary' ? '#1d40b0' : '#d97708';
    }};
    display: flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
    
    svg {
        width: 16px; /* 60% of 27px */
        height: 16px; /* 60% of 27px */
    }
`;

const OptionContent = styled.div`
    flex: 1;
`;

const OptionTitle = styled.div`
    font-size: 14px; /* 60% of 23px */
    font-weight: 600;
    color: #1f2937;
    margin-bottom: 2px; /* 60% of 3px */
`;

const OptionDescription = styled.div`
    font-size: 13px; /* 60% of 21px */
    color: #6b7280;
    opacity: 0.8;
`;

const AbsenceReasonSection = styled.div<{show: boolean}>`
    display: ${props => props.show ? 'block' : 'none'};
    animation: ${slideDown} 0.25s ease-out;
    margin-bottom: 14px; /* 60% of 24px */
`;

const InputLabel = styled.label`
    display: block;
    font-size: 14px; /* 60% of 21px */
    font-weight: 600;
    color: #374151;
    margin-bottom: 7px; /* 60% of 12px */
`;

const TextInput = styled.input`
    width: 100%;
    padding: 11px 14px; /* 60% of 18px 24px */
    border: 2px solid #e5e7eb;
    border-radius: 8px;
    font-size: 13px; /* 60% of 21px */
    font-family: inherit;
    color: #1f2937;
    background: white;
    transition: all 0.2s ease;
    box-sizing: border-box;
    
    &:focus {
        outline: none;
        border-color: rgb(229, 231, 235);
        box-shadow: 0 0 0 3px rgb(229, 231, 235);
    }
    
    &::placeholder {
        color: #9ca3af;
    }
`;

const InputDescription = styled.div`
    font-size: 11px; /* 60% of 18px */
    color: #6b7280;
    margin-top: 5px; /* 60% of 9px */
`;

const ErrorMessage = styled.div`
    display: flex;
    align-items: center;
    gap: 7px; /* 60% of 12px */
    padding: 11px 14px; /* 60% of 18px 24px */
    background: rgba(239, 68, 68, 0.05);
    border: 1px solid rgba(239, 68, 68, 0.2);
    border-radius: 8px;
    margin-bottom: 14px; /* 60% of 24px */
    
    svg {
        width: 14px; /* 60% of 24px */
        height: 14px; /* 60% of 24px */
        color: #ef4444;
        flex-shrink: 0;
    }
    
    span {
        font-size: 13px; /* 60% of 21px */
        color: #dc2626;
    }
`;

const ModalActions = styled.div`
    display: flex;
    align-items: center;
    justify-content: flex-end;
    gap: 11px; /* 60% of 18px */
    padding-top: 14px; /* 60% of 24px */
    border-top: 1px solid #f3f4f6;
    margin-top: auto;
    flex-shrink: 0; /* Prevent actions from shrinking */
    
    @media (max-width: 480px) {
        flex-direction: column;
        gap: 7px; /* 60% of 12px */
        
        button {
            width: 100%;
        }
    }
`;

const Button = styled.button<{variant: 'outline' | 'primary', loading?: boolean}>`
    padding: 11px 18px; /* 60% of 18px 30px */
    border: ${props => props.variant === 'outline' ? '2px solid #e5e7eb' : 'none'};
    border-radius: 8px;
    background: ${props => props.variant === 'outline' ? 'white' : 'linear-gradient(135deg, #6366f1 0%, #4f46e5 100%)'};
    color: ${props => props.variant === 'outline' ? '#6b7280' : 'white'};
    font-size: 13px; /* 60% of 21px */
    font-weight: 600;
    cursor: pointer;
    transition: all 0.2s ease;
    font-family: inherit;
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 7px; /* 60% of 12px */
    min-width: 108px; /* 60% of 180px */
    
    &:hover:not(:disabled) {
        ${props => props.variant === 'outline' ? `
            border-color: #d1d5db;
            background: #f9fafb;
            color: #374151;
        ` : `
            background: linear-gradient(135deg, #4f46e5 0%, #4338ca 100%);
            transform: translateY(-1px);
            box-shadow: 0 8px 16px rgba(99, 102, 241, 0.25);
        `}
    }
    
    &:active:not(:disabled) {
        transform: ${props => props.variant === 'outline' ? 'scale(0.98)' : 'translateY(0)'};
    }
    
    &:disabled {
        opacity: 0.5;
        cursor: not-allowed;
        transform: none !important;
    }
`;

const LoadingSpinner = styled.div`
    width: 14px; /* 60% of 24px */
    height: 14px; /* 60% of 24px */
    border: 2px solid transparent;
    border-top: 2px solid currentColor;
    border-radius: 50%;
    animation: spin 0.8s linear infinite;
    
    @keyframes spin {
        0% { transform: rotate(0deg); }
        100% { transform: rotate(360deg); }
    }
`;

const StatusMessage = styled.div<{type: 'success' | 'error'}>`
    display: flex;
    align-items: center;
    gap: 11px; /* 60% of 18px */
    padding: 13px; /* 60% of 21px */
    border-radius: 8px;
    margin-bottom: 14px; /* 60% of 24px */
    background: ${props => props.type === 'success' 
        ? 'rgba(16, 185, 129, 0.05)' 
        : 'rgba(239, 68, 68, 0.05)'};
    border: 1px solid ${props => props.type === 'success' ? 'rgba(16, 185, 129, 0.2)' : 'rgba(239, 68, 68, 0.2)'};
    
    &::before {
        content: ${props => props.type === 'success' ? '"✓"' : '"!"'};
        font-size: 14px; /* 60% of 24px */
        font-weight: bold;
        width: 18px; /* 60% of 30px */
        height: 18px; /* 60% of 30px */
        border-radius: 50%;
        background: ${props => props.type === 'success' ? '#079669' : '#ef4444'};
        color: white;
        display: flex;
        align-items: center;
        justify-content: center;
        flex-shrink: 0;
    }
    
    span {
        font-size: 13px; /* 60% of 21px */
        font-weight: 500;
        color: ${props => props.type === 'success' ? '#047857' : '#dc2626'};
    }
`;

// Built-in translations
const translations: Record<'en' | 'vi', Record<string, string>> = {
    en: {
        'rollcall.title': 'Mark Attendance',
        'rollcall.subtitle': 'Select Attendance Status',
        'rollcall.time.current': 'Current Time',
        'rollcall.checkin.button': 'Check In',
        'rollcall.checkout.button': 'Check Out',
        'rollcall.absent.button': 'Mark Absent',
        'rollcall.checkin.title': 'Check In',
        'rollcall.checkout.title': 'Check Out',
        'rollcall.absent.title': 'Mark Absent',
        'rollcall.checkin.description': 'Mark your arrival',
        'rollcall.checkout.description': 'Mark your departure',
        'rollcall.absent.description': 'Report absence',
        'rollcall.close': 'Close',
        'rollcall.close.tooltip': 'Close modal',
        'rollcall.checkin.success': 'Welcome! You have successfully checked in.',
        'rollcall.checkout.success': 'Have a great day! You have successfully checked out.',
        'rollcall.absent.success': 'Your absence has been recorded. Take care!',
        'rollcall.absent.reason.label': 'Reason for Absence (Optional)',
        'rollcall.absent.reason.placeholder': 'Enter your reason for being absent (optional)...',
        'rollcall.absent.reason.description': 'characters',
        'rollcall.submit': 'Submit',
        'rollcall.cancel': 'Cancel',
        'rollcall.error': 'An error occurred. Please try again.',
        'rollcall.timeout': 'Request timed out. Please try again.',
        'rollcall.error.select': 'Please select an attendance option',
    },
    vi: {
        'rollcall.title': 'Điểm Danh',
        'rollcall.subtitle': 'Chọn Trạng Thái',
        'rollcall.time.current': 'Thời gian hiện tại',
        'rollcall.checkin.button': 'Check In',
        'rollcall.checkout.button': 'Check Out',
        'rollcall.absent.button': 'Báo Vắng',
        'rollcall.checkin.title': 'Check In',
        'rollcall.checkout.title': 'Check Out',
        'rollcall.absent.title': 'Báo Vắng',
        'rollcall.checkin.description': 'Đánh dấu check in',
        'rollcall.checkout.description': 'Đánh dấu check out',
        'rollcall.absent.description': 'Báo cáo vắng mặt',
        'rollcall.close': 'Đóng',
        'rollcall.close.tooltip': 'Đóng hộp thoại',
        'rollcall.checkin.success': 'Chào mừng! Bạn đã check in thành công.',
        'rollcall.checkout.success': 'Bạn đã check out thành công. Chúc bạn một ngày tốt lành!',
        'rollcall.absent.success': 'Thông tin vắng mặt đã được ghi nhận.',
        'rollcall.absent.reason.label': 'Lý do vắng mặt',
        'rollcall.absent.reason.placeholder': 'Nhập lý do vắng mặt...',
        'rollcall.absent.reason.description': 'ký tự',
        'rollcall.submit': 'Xác Nhận',
        'rollcall.cancel': 'Hủy',
        'rollcall.error': 'Đã xảy ra lỗi. Vui lòng thử lại.',
        'rollcall.timeout': 'Yêu cầu đã hết thời gian chờ. Vui lòng thử lại.',
        'rollcall.error.select': 'Vui lòng chọn một tùy chọn chấm công',
        'rollcall.error.reason.required': 'Vui lòng cung cấp lý do vắng mặt',
        'rollcall.error.reason.length': 'Lý do vắng mặt phải có ít nhất 10 ký tự'
    }
};

// Text helper function with built-in translations
const getText = (key: string, language: 'en' | 'vi' = 'en', t?: (key: string) => string): string => {
    if (t) {
        try {
            return t(key);
        } catch (error) {
            console.warn(`Translation failed for key: ${key}`, error);
        }
    }
    
    const langTranslations = translations[language];
    if (langTranslations && key in langTranslations) {
        return langTranslations[key];
    }
    
    const enTranslations = translations.en;
    if (enTranslations && key in enTranslations) {
        return enTranslations[key];
    }
    
    return key;
};

type AttendanceType = 'checkin' | 'checkout' | 'absent' | null;

interface RollCallInterfaceProps {
    onClose?: () => void;
    t?: (key: string) => string;
    language?: 'en' | 'vi';
    locale?: string;
}

const RollCallInterface: React.FC<RollCallInterfaceProps> = ({
    onClose, 
    t, 
    language = 'vi',
    locale
}) => {
    const [loading, setLoading] = useState(false);
    const [statusMessage, setStatusMessage] = useState<{type: 'success' | 'error', message: string} | null>(null);
    const [requestTimeout, setRequestTimeout] = useState<NodeJS.Timeout | null>(null);
    const [selectedType, setSelectedType] = useState<AttendanceType>(null);
    const [absenceReason, setAbsenceReason] = useState('');
    const [error, setError] = useState('');
    const [currentTime, setCurrentTime] = useState(new Date());

    useEffect(() => {
        const timer = setInterval(() => {
            setCurrentTime(new Date());
        }, 1000);
    
        // Cleanup timer on component unmount
        return () => clearInterval(timer);
    }, []);


    const dateLocale = locale || (language === 'vi' ? 'vi-VN' : 'en-US');

    const getCurrentTime = () => {
        return currentTime.toLocaleTimeString(dateLocale, {
            hour12: false,
            hour: '2-digit',
            minute: '2-digit',
            second: '2-digit'
        });
    };

    const getCurrentDate = () => {
        return currentTime.toLocaleDateString(dateLocale, {
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
        setError('');
        
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
            
            // Check if the response indicates success
            if (response?.success === false) {
                // Handle explicit failure from backend
                const errorMessage = response?.message || response?.error || getText('rollcall.error', language, t);
                setStatusMessage({
                    type: 'error',
                    message: errorMessage
                });
            } else {
                // Handle success
                setStatusMessage({
                    type: 'success',
                    message: response?.message || getText(successMessageKey, language, t)
                });
                setTimeout(() => {
                    onClose?.();
                }, 1500);
            }
        } catch (error: any) {
            clearTimeout(timeout);
            
            // Handle different types of errors
            let errorMessage = getText('rollcall.error', language, t);
            
            if (error?.response) {
                // HTTP error response
                const responseData = error.response.data;
                if (responseData?.message) {
                    errorMessage = responseData.message;
                } else if (responseData?.error) {
                    errorMessage = responseData.error;
                } else {
                    // Handle specific HTTP status codes
                    switch (error.response.status) {
                        case 409:
                            errorMessage = language === 'vi' 
                                ? 'Bạn đã thực hiện thao tác này rồi trong ngày hôm nay.'
                                : 'You have already performed this action today.';
                            break;
                        case 401:
                            errorMessage = language === 'vi'
                                ? 'Bạn không có quyền thực hiện thao tác này.'
                                : 'You are not authorized to perform this action.';
                            break;
                        case 404:
                            errorMessage = language === 'vi'
                                ? 'Không tìm thấy thông tin nhân viên. Vui lòng liên hệ quản trị viên.'
                                : 'Employee information not found. Please contact administrator.';
                            break;
                        case 400:
                            errorMessage = language === 'vi'
                                ? 'Yêu cầu không hợp lệ. Vui lòng thử lại.'
                                : 'Invalid request. Please try again.';
                            break;
                        case 500:
                            errorMessage = language === 'vi'
                                ? 'Lỗi hệ thống. Vui lòng thử lại sau.'
                                : 'System error. Please try again later.';
                            break;
                    }
                }
            } else if (error?.message) {
                // Network or other errors
                if (error.message.includes('timeout')) {
                    errorMessage = getText('rollcall.timeout', language, t);
                } else if (error.message.includes('network') || error.message.includes('fetch')) {
                    errorMessage = language === 'vi'
                        ? 'Lỗi kết nối mạng. Vui lòng kiểm tra kết nối và thử lại.'
                        : 'Network connection error. Please check your connection and try again.';
                } else {
                    errorMessage = error.message;
                }
            }
            
            setStatusMessage({
                type: 'error',
                message: errorMessage
            });
        } finally {
            setLoading(false);
            setRequestTimeout(null);
        }
    };

    const handleSelection = (type: AttendanceType) => {
        setSelectedType(type);
        setStatusMessage(null);
        setError('');
        if (type !== 'absent') {
            setAbsenceReason('');
        }
    };

    const handleCancel = () => {
        setSelectedType(null);
        setAbsenceReason('');
        setError('');
        onClose?.();
    };

    const handleSubmit = (e: React.FormEvent) => {
        e.preventDefault();
        
        if (!selectedType) {
            setError(getText('rollcall.error.select', language, t));
            return;
        }

        if (selectedType === 'absent' && !absenceReason.trim()) {
            setError(getText('rollcall.error.reason.required', language, t));
            return;
        }
    
        switch (selectedType) {
            case 'checkin':
                handleApiCall(doCheckIn, 'rollcall.checkin.success');
                break;
            case 'checkout':
                handleApiCall(doCheckOut, 'rollcall.checkout.success');
                break;
            case 'absent':
                // Send empty string by default, or the trimmed reason if provided
                handleApiCall(
                    () => doAbsent(absenceReason.trim()),
                    'rollcall.absent.success'
                );
                break;
        }
    };

    const handleKeyDown = (event: React.KeyboardEvent) => {
        if (event.key === 'Escape') {
            if (selectedType) {
                handleCancel();
            } else {
                onClose?.();
            }
        }
    };

    return (
        <Container 
            show={true}
            onKeyDown={handleKeyDown}
            tabIndex={-1}
            role="dialog"
            aria-modal="true"
            aria-labelledby="attendance-modal-title"
        > 
            <ModalHeader>
                <Title id="attendance-modal-title">
                    {getText('rollcall.title', language, t)}
                </Title>
                <CloseButton 
                    onClick={onClose}
                    aria-label={getText('rollcall.close', language, t)}
                    title={getText('rollcall.close.tooltip', language, t)}
                >
                    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
                        <path d="M18 6L6 18M6 6L18 18" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"/>
                    </svg>
                </CloseButton>
            </ModalHeader>

            <ModalContent>
                <form onSubmit={handleSubmit}>
                    <TimeDisplay>
                        <TimeLabel>{getText('rollcall.time.current', language, t)}</TimeLabel>
                        <CurrentTime>{getCurrentTime()}</CurrentTime>
                        <CurrentDate>{getCurrentDate()}</CurrentDate>
                    </TimeDisplay>
                    
                    {statusMessage && (
                        <StatusMessage type={statusMessage.type}>
                            <span>{statusMessage.message}</span>
                        </StatusMessage>
                    )}

                    <SectionLabel>
                        {getText('rollcall.subtitle', language, t)}
                    </SectionLabel>
                    
                    <AttendanceGrid>
                        <AttendanceOption
                            type="button"
                            selected={selectedType === 'checkin'}
                            variant="success"
                            onClick={() => handleSelection('checkin')}
                            disabled={loading}
                        >
                            <OptionIcon variant="success" selected={selectedType === 'checkin'}>
                                <svg width="18" height="18" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
                                    <path d="M15 3H19C19.5304 3 20.0391 3.21071 20.4142 3.58579C20.7893 3.96086 21 4.46957 21 5V19C21 19.5304 20.7893 20.0391 20.4142 20.4142C20.0391 20.7893 19.5304 21 19 21H15M10 17L15 12M15 12L10 7M15 12H3" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"/>
                                </svg>
                            </OptionIcon>
                            <OptionContent>
                                <OptionTitle>{getText('rollcall.checkin.title', language, t)}</OptionTitle>
                                <OptionDescription>{getText('rollcall.checkin.description', language, t)}</OptionDescription>
                            </OptionContent>
                        </AttendanceOption>

                        <AttendanceOption
                            type="button"
                            selected={selectedType === 'checkout'}
                            variant="primary"
                            onClick={() => handleSelection('checkout')}
                            disabled={loading}
                        >
                            <OptionIcon variant="primary" selected={selectedType === 'checkout'}>
                                <svg width="18" height="18" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
                                    <path d="M9 21H5C4.46957 21 3.96086 20.7893 3.58579 20.4142C3.21071 20.0391 3 19.5304 3 19V5C3 4.46957 3.21071 3.96086 3.58579 3.58579C3.96086 3.21071 4.46957 3 5 3H9M16 17L21 12M21 12L16 7M21 12H9" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"/>
                                </svg>
                            </OptionIcon>
                            <OptionContent>
                                <OptionTitle>{getText('rollcall.checkout.title', language, t)}</OptionTitle>
                                <OptionDescription>{getText('rollcall.checkout.description', language, t)}</OptionDescription>
                            </OptionContent>
                        </AttendanceOption>

                        <AttendanceOption
                            type="button"
                            selected={selectedType === 'absent'}
                            variant="warning"
                            onClick={() => handleSelection('absent')}
                            disabled={loading}
                        >
                            <OptionIcon variant="warning" selected={selectedType === 'absent'}>
                                <svg width="18" height="18" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
                                    <path d="M16 21V19C16 17.9391 15.5786 16.9217 14.8284 16.1716C14.0783 15.4214 13.0609 15 12 15H5C3.93913 15 2.92172 15.4214 2.17157 16.1716C1.42143 16.9217 1 17.9391 1 19V21M20 8V14M20 18H20.01M12.5 7C12.5 9.20914 10.7091 11 8.5 11C6.29086 11 4.5 9.20914 4.5 7C4.5 4.79086 6.29086 3 8.5 3C10.7091 3 12.5 4.79086 12.5 7Z" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"/>
                                </svg>
                            </OptionIcon>
                            <OptionContent>
                                <OptionTitle>{getText('rollcall.absent.title', language, t)}</OptionTitle>
                                <OptionDescription>{getText('rollcall.absent.description', language, t)}</OptionDescription>
                            </OptionContent>
                        </AttendanceOption>
                    </AttendanceGrid>

                    <AbsenceReasonSection show={selectedType === 'absent'}>
                        <InputLabel htmlFor="absence-reason">
                            {getText('rollcall.absent.reason.label', language, t)}
                        </InputLabel>
                        <TextInput
                            id="absence-reason"
                            type="text"
                            value={absenceReason}
                            onChange={(e) => setAbsenceReason(e.target.value)}
                            placeholder={getText('rollcall.absent.reason.placeholder', language, t)}
                            maxLength={200}
                        />
                        <InputDescription>
                            {absenceReason.length}/200 {getText('rollcall.absent.reason.description', language, t)}
                        </InputDescription>
                    </AbsenceReasonSection>

                    {error && (
                        <ErrorMessage>
                            <svg width="24" height="24" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
                                <circle cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="2"/>
                                <line x1="12" y1="8" x2="12" y2="12" stroke="currentColor" strokeWidth="2"/>
                                <line x1="12" y1="16" x2="12.01" y2="16" stroke="currentColor" strokeWidth="2"/>
                            </svg>
                            <span>{error}</span>
                        </ErrorMessage>
                    )}

                    <ModalActions>
                        <Button
                            type="button"
                            variant="outline"
                            onClick={handleCancel}
                            disabled={loading}
                        >
                            {getText('rollcall.cancel', language, t)}
                        </Button>
                        <Button
                            type="submit"
                            variant="primary"
                            loading={loading}
                            disabled={!selectedType || loading}
                        >
                            {loading && <LoadingSpinner />}
                            {loading ? 'Submitting...' : getText('rollcall.submit', language, t)}
                        </Button>
                    </ModalActions>
                </form>
            </ModalContent>
        </Container>
    );
};

export default RollCallInterface;