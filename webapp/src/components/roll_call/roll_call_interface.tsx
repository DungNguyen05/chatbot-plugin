import React, {useState} from 'react';
import styled from 'styled-components';

import {doCheckIn, doCheckOut, doAbsent} from '../../client';

const Container = styled.div<{show: boolean}>`
    display: ${props => props.show ? 'flex' : 'none'};
    flex-direction: column;
    padding: 32px;
    gap: 24px;
    max-width: 500px;
    margin: 0 auto;
    background: var(--center-channel-bg);
    border-radius: 8px;
    color: var(--center-channel-color);
`;

const Title = styled.h2`
    font-size: 22px;
    font-weight: 600;
    margin-bottom: 8px;
    text-align: center;
    color: var(--center-channel-color);
    font-family: var(--font-family);
    letter-spacing: -0.025em;
`;

const Subtitle = styled.p`
    font-size: 14px;
    color: rgba(var(--center-channel-color-rgb), 0.72);
    text-align: center;
    margin: 0 0 16px 0;
    font-weight: 400;
`;

const ButtonGrid = styled.div`
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 16px;
    margin-bottom: 8px;
    
    @media (max-width: 480px) {
        grid-template-columns: 1fr;
    }
`;

const ActionButton = styled.button`
    padding: 12px 20px;
    border: none;
    border-radius: 4px;
    font-weight: 600;
    font-size: 14px;
    cursor: pointer;
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 8px;
    transition: all 0.15s ease-out;
    min-height: 40px;
    background: rgba(var(--center-channel-color-rgb), 0.08);
    color: rgba(var(--center-channel-color-rgb), 0.72);
    position: relative;
    overflow: hidden;
    
    &:disabled {
        opacity: 0.32;
        cursor: not-allowed;
        transform: none !important;
    }
    
    &:not(:disabled):hover {
        background: rgba(var(--center-channel-color-rgb), 0.12);
        color: rgba(var(--center-channel-color-rgb), 0.80);
        transform: translateY(-1px);
        box-shadow: 0 4px 12px rgba(0, 0, 0, 0.1);
    }
    
    &:not(:disabled):active {
        transform: translateY(0);
        transition: all 0.1s ease-out;
    }
`;

const CheckInButton = styled(ActionButton)`
    background: var(--online-indicator);
    color: var(--button-color);
    
    &:hover:not(:disabled) {
        background: rgba(var(--online-indicator-rgb), 0.88);
        color: var(--button-color);
    }
    
    &:active:not(:disabled) {
        background: rgba(var(--online-indicator-rgb), 0.92);
    }
`;

const CheckOutButton = styled(ActionButton)`
    background: var(--button-bg);
    color: var(--button-color);
    
    &:hover:not(:disabled) {
        background: rgba(var(--button-bg-rgb), 0.88);
        color: var(--button-color);
    }
    
    &:active:not(:disabled) {
        background: rgba(var(--button-bg-rgb), 0.92);
    }
`;

const AbsentButton = styled(ActionButton)`
    background: transparent;
    color: var(--error-text);
    border: 2px solid var(--error-text);
    grid-column: 1 / -1;
    
    &:hover:not(:disabled) {
        background: var(--error-text);
        color: var(--button-color);
    }
`;

const AbsentModal = styled.div<{show: boolean}>`
    display: ${props => props.show ? 'flex' : 'none'};
    width: 100%;
    height: 100%;
    background-color: rgba(0, 0, 0, 0.64);
    backdrop-filter: blur(8px);
    justify-content: center;
    align-items: center;
    z-index: 1000;
`;

const ModalContent = styled.div`
    background: var(--center-channel-bg);
    padding: 32px;
    border-radius: 8px;
    min-width: 400px;
    max-width: 90%;
    box-shadow: var(--elevation-8);
    transform: scale(1);
    animation: modalAppear 0.2s ease-out;
    
    @keyframes modalAppear {
        from {
            transform: scale(0.95);
            opacity: 0;
        }
        to {
            transform: scale(1);
            opacity: 1;
        }
    }
`;

const ModalTitle = styled.h3`
    margin-bottom: 20px;
    font-size: 20px;
    font-weight: 600;
    color: var(--center-channel-color);
    font-family: var(--font-family);
    letter-spacing: -0.025em;
`;

const ReasonInput = styled.textarea`
    width: 100%;
    min-height: 100px;
    padding: 10px 16px;
    border: 1px solid rgba(var(--center-channel-color-rgb), 0.16);
    border-radius: 4px;
    resize: vertical;
    font-family: inherit;
    margin-bottom: 20px;
    font-size: 14px;
    background: var(--center-channel-bg);
    color: var(--center-channel-color);
    transition: border-color 0.15s ease-out;
    
    &:focus {
        outline: none;
        border-color: var(--button-bg);
        box-shadow: inset 0 1px 1px rgba(0, 0, 0, 0.075), 0 0 8px rgba(var(--button-bg-rgb), 0.3);
    }
    
    &::placeholder {
        color: rgba(var(--center-channel-color-rgb), 0.5);
    }
`;

const ModalActions = styled.div`
    display: flex;
    gap: 12px;
    justify-content: flex-end;
`;

const SecondaryButton = styled.button`
    padding: 10px 20px;
    border: 1px solid rgba(var(--center-channel-color-rgb), 0.24);
    border-radius: 4px;
    background: var(--center-channel-bg);
    color: var(--center-channel-color);
    font-weight: 600;
    cursor: pointer;
    transition: all 0.15s ease-out;
    
    &:hover {
        border-color: rgba(var(--center-channel-color-rgb), 0.32);
        background: rgba(var(--center-channel-color-rgb), 0.08);
    }
`;

const PrimaryButton = styled.button`
    padding: 10px 20px;
    border: none;
    border-radius: 4px;
    background: var(--error-text);
    color: var(--button-color);
    font-weight: 600;
    cursor: pointer;
    transition: all 0.15s ease-out;
    
    &:hover:not(:disabled) {
        background: rgba(var(--error-text-color-rgb), 0.88);
        transform: translateY(-1px);
    }
    
    &:disabled {
        opacity: 0.32;
        cursor: not-allowed;
        transform: none !important;
    }
`;

const StatusMessage = styled.div<{type: 'success' | 'error'}>`
    padding: 16px 20px;
    border-radius: 4px;
    margin-bottom: 20px;
    background-color: ${props => props.type === 'success' ? 'rgba(var(--online-indicator-rgb), 0.12)' : 'rgba(var(--error-text-color-rgb), 0.12)'};
    color: ${props => props.type === 'success' ? 'var(--online-indicator)' : 'var(--error-text)'};
    border: 1px solid ${props => props.type === 'success' ? 'rgba(var(--online-indicator-rgb), 0.24)' : 'rgba(var(--error-text-color-rgb), 0.24)'};
    font-size: 14px;
    font-weight: 500;
`;

const LoadingSpinner = styled.div`
    display: inline-block;
    width: 16px;
    height: 16px;
    border: 2px solid transparent;
    border-top: 2px solid currentColor;
    border-radius: 50%;
    animation: spin 0.8s linear infinite;
    
    @keyframes spin {
        0% { transform: rotate(0deg); }
        100% { transform: rotate(360deg); }
    }
`;

interface RollCallInterfaceProps {
    onClose?: () => void;
    setIsShowHeader?: (show: boolean) => void;
}

const RollCallInterface: React.FC<RollCallInterfaceProps> = ({onClose, setIsShowHeader}) => {
    const [showAbsentModal, setShowAbsentModal] = useState(false);
    const [absentReason, setAbsentReason] = useState('');
    const [loading, setLoading] = useState(false);
    const [statusMessage, setStatusMessage] = useState<{type: 'success' | 'error', message: string} | null>(null);
    const [requestTimeout, setRequestTimeout] = useState<NodeJS.Timeout | null>(null);

    const clearRequestTimeout = () => {
        if (requestTimeout) {
            clearTimeout(requestTimeout);
            setRequestTimeout(null);
        }
    };

    const handleApiCall = async (apiCall: () => Promise<any>, successMessage: string) => {
        setLoading(true);
        setStatusMessage(null);
        
        const timeout = setTimeout(() => {
            setStatusMessage({
                type: 'error',
                message: 'Request timed out. Please try again.'
            });
            setLoading(false);
        }, 30000);
        
        setRequestTimeout(timeout);
        
        try {
            const response = await apiCall();
            clearTimeout(timeout);
            setStatusMessage({
                type: 'success',
                message: response?.message || successMessage
            });
            setTimeout(() => {
                onClose?.();
            }, 2000);
        } catch (error: any) {
            clearTimeout(timeout);
            const errorMessage = error?.message || 'An error occurred. Please try again.';
            setStatusMessage({
                type: 'error',
                message: errorMessage
            });
        } finally {
            setLoading(false);
            setRequestTimeout(null);
        }
    };

    const handleCheckIn = () => handleApiCall(doCheckIn, 'Successfully checked in!');
    const handleCheckOut = () => handleApiCall(doCheckOut, 'Successfully checked out!');

    const handleAbsentClick = () => {
        setShowAbsentModal(true);
        setIsShowHeader?.(false);
    };

    const handleAbsentCancel = () => {
        setShowAbsentModal(false);
        setAbsentReason('');
        setStatusMessage(null);
        setIsShowHeader?.(true);
        clearRequestTimeout();
    };

    const handleAbsentSubmit = async () => {
        if (!absentReason.trim()) {
            setStatusMessage({
                type: 'error',
                message: 'Please provide a reason for absence.'
            });
            return;
        }

        await handleApiCall(
            () => doAbsent(absentReason.trim()),
            'Absence recorded successfully!'
        );
        
        setShowAbsentModal(false);
        setAbsentReason('');
        setIsShowHeader?.(true);
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
                <div>
                    <Title id="rollcall-title">Roll Call</Title>
                    <Subtitle id="rollcall-description">Record your attendance for today</Subtitle>
                </div>
                
                {statusMessage && (
                    <StatusMessage type={statusMessage.type}>
                        {statusMessage.message}
                    </StatusMessage>
                )}

                <ButtonGrid>
                    <CheckInButton 
                        onClick={handleCheckIn}
                        disabled={loading}
                        aria-label="Check in for work today"
                    >
                        {loading ? <LoadingSpinner /> : null}
                        Check In
                    </CheckInButton>
                    
                    <CheckOutButton 
                        onClick={handleCheckOut}
                        disabled={loading}
                        aria-label="Check out from work today"
                    >
                        {loading ? <LoadingSpinner /> : null}
                        Check Out
                    </CheckOutButton>
                    
                    <AbsentButton 
                        onClick={handleAbsentClick}
                        disabled={loading}
                        aria-label="Report absence with reason"
                    >
                        Report Absence
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
                    <ModalTitle id="absent-modal-title">Report Absence</ModalTitle>
                    
                    {statusMessage && (
                        <StatusMessage type={statusMessage.type}>
                            {statusMessage.message}
                        </StatusMessage>
                    )}
                    
                    <ReasonInput
                        placeholder="Please provide a reason for your absence..."
                        value={absentReason}
                        onChange={(e) => setAbsentReason(e.target.value)}
                        maxLength={500}
                        aria-label="Absence reason"
                        autoFocus
                    />
                    
                    <ModalActions>
                        <SecondaryButton 
                            onClick={handleAbsentCancel}
                            disabled={loading}
                        >
                            Cancel
                        </SecondaryButton>
                        
                        <PrimaryButton 
                            onClick={handleAbsentSubmit}
                            disabled={loading || !absentReason.trim()}
                            aria-label="Submit absence report"
                        >
                            {loading ? <LoadingSpinner /> : null}
                            Submit
                        </PrimaryButton>
                    </ModalActions>
                </ModalContent>
            </AbsentModal>
        </>
    );
};

export default RollCallInterface;