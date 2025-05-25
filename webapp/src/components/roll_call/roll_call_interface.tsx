import React, {useState} from 'react';
import styled from 'styled-components';
import {FormattedMessage, useIntl} from 'react-intl';

import {doCheckIn, doCheckOut, doAbsent} from '../../client';

const Container = styled.div`
    display: flex;
    flex-direction: column;
    padding: 24px;
    gap: 16px;
    max-width: 500px;
    margin: 0 auto;
    background: white;
    border-radius: 8px;
`;

const Title = styled.h2`
    font-size: 20px;
    font-weight: 600;
    margin-bottom: 8px;
    text-align: center;
    color: var(--center-channel-color);
`;

const ButtonRow = styled.div`
    display: flex;
    gap: 12px;
    justify-content: center;
    flex-wrap: wrap;
`;

const ActionButton = styled.button`
    min-width: 120px;
    padding: 12px 16px;
    border: none;
    border-radius: 4px;
    font-weight: 600;
    font-size: 14px;
    cursor: pointer;
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 8px;
    transition: all 0.2s ease;
    
    &:disabled {
        opacity: 0.6;
        cursor: not-allowed;
    }
`;

const CheckInButton = styled(ActionButton)`
    background-color: #28a745;
    color: white;
    
    &:hover:not(:disabled) {
        background-color: #218838;
        transform: translateY(-1px);
    }
`;

const CheckOutButton = styled(ActionButton)`
    background-color: #007bff;
    color: white;
    
    &:hover:not(:disabled) {
        background-color: #0056b3;
        transform: translateY(-1px);
    }
`;

const AbsentButton = styled(ActionButton)`
    background-color: #dc3545;
    color: white;
    
    &:hover:not(:disabled) {
        background-color: #c82333;
        transform: translateY(-1px);
    }
`;

const AbsentModal = styled.div<{show: boolean}>`
    display: ${props => props.show ? 'flex' : 'none'};
    position: fixed;
    top: 0;
    left: 0;
    width: 100%;
    height: 100%;
    background-color: rgba(0, 0, 0, 0.5);
    justify-content: center;
    align-items: center;
    z-index: 1000;
`;

const ModalContent = styled.div`
    background: white;
    padding: 24px;
    border-radius: 8px;
    min-width: 400px;
    max-width: 90%;
    box-shadow: 0 4px 12px rgba(0, 0, 0, 0.3);
`;

const ModalTitle = styled.h3`
    margin-bottom: 16px;
    font-size: 18px;
    font-weight: 600;
    color: var(--center-channel-color);
`;

const ReasonInput = styled.textarea`
    width: 100%;
    min-height: 80px;
    padding: 8px 12px;
    border: 1px solid rgba(var(--center-channel-color-rgb), 0.16);
    border-radius: 4px;
    resize: vertical;
    font-family: inherit;
    margin-bottom: 16px;
    font-size: 14px;
    
    &:focus {
        outline: none;
        border-color: var(--button-bg);
        box-shadow: 0 0 0 2px rgba(var(--button-bg-rgb), 0.25);
    }
`;

const ModalActions = styled.div`
    display: flex;
    gap: 8px;
    justify-content: flex-end;
`;

const SecondaryButton = styled.button`
    padding: 8px 16px;
    border: 1px solid rgba(var(--center-channel-color-rgb), 0.24);
    border-radius: 4px;
    background: white;
    color: var(--center-channel-color);
    font-weight: 600;
    cursor: pointer;
    
    &:hover {
        background-color: rgba(var(--center-channel-color-rgb), 0.04);
    }
`;

const PrimaryButton = styled.button`
    padding: 8px 16px;
    border: none;
    border-radius: 4px;
    background-color: #dc3545;
    color: white;
    font-weight: 600;
    cursor: pointer;
    
    &:hover:not(:disabled) {
        background-color: #c82333;
    }
    
    &:disabled {
        opacity: 0.6;
        cursor: not-allowed;
    }
`;

const StatusMessage = styled.div<{type: 'success' | 'error'}>`
    padding: 12px;
    border-radius: 4px;
    margin-bottom: 16px;
    background-color: ${props => props.type === 'success' ? '#d4edda' : '#f8d7da'};
    color: ${props => props.type === 'success' ? '#155724' : '#721c24'};
    border: 1px solid ${props => props.type === 'success' ? '#c3e6cb' : '#f5c6cb'};
    font-size: 14px;
`;

const LoadingSpinner = styled.div`
    display: inline-block;
    width: 16px;
    height: 16px;
    border: 2px solid transparent;
    border-top: 2px solid currentColor;
    border-radius: 50%;
    animation: spin 1s linear infinite;
    
    @keyframes spin {
        0% { transform: rotate(0deg); }
        100% { transform: rotate(360deg); }
    }
`;

interface RollCallInterfaceProps {
    onClose?: () => void;
}

const RollCallInterface: React.FC<RollCallInterfaceProps> = ({onClose}) => {
    const intl = useIntl();
    const [showAbsentModal, setShowAbsentModal] = useState(false);
    const [absentReason, setAbsentReason] = useState('');
    const [loading, setLoading] = useState(false);
    const [statusMessage, setStatusMessage] = useState<{type: 'success' | 'error', message: string} | null>(null);

    const handleCheckIn = async () => {
        setLoading(true);
        setStatusMessage(null);
        try {
            const response = await doCheckIn();
            setStatusMessage({
                type: 'success',
                message: response.message || intl.formatMessage({defaultMessage: 'Successfully checked in!'})
            });
            setTimeout(() => {
                onClose?.();
            }, 2000);
        } catch (error: any) {
            const errorMessage = error?.message || intl.formatMessage({defaultMessage: 'Failed to check in. Please try again.'});
            setStatusMessage({
                type: 'error',
                message: errorMessage
            });
        } finally {
            setLoading(false);
        }
    };

    const handleCheckOut = async () => {
        setLoading(true);
        setStatusMessage(null);
        try {
            const response = await doCheckOut();
            setStatusMessage({
                type: 'success',
                message: response.message || intl.formatMessage({defaultMessage: 'Successfully checked out!'})
            });
            setTimeout(() => {
                onClose?.();
            }, 2000);
        } catch (error: any) {
            const errorMessage = error?.message || intl.formatMessage({defaultMessage: 'Failed to check out. Please try again.'});
            setStatusMessage({
                type: 'error',
                message: errorMessage
            });
        } finally {
            setLoading(false);
        }
    };

    const handleAbsentSubmit = async () => {
        if (!absentReason.trim()) {
            setStatusMessage({
                type: 'error',
                message: intl.formatMessage({defaultMessage: 'Please provide a reason for absence.'})
            });
            return;
        }

        setLoading(true);
        setStatusMessage(null);
        try {
            const response = await doAbsent(absentReason.trim());
            setStatusMessage({
                type: 'success',
                message: response.message || intl.formatMessage({defaultMessage: 'Absence recorded successfully!'})
            });
            setShowAbsentModal(false);
            setAbsentReason('');
            setTimeout(() => {
                onClose?.();
            }, 2000);
        } catch (error: any) {
            const errorMessage = error?.message || intl.formatMessage({defaultMessage: 'Failed to record absence. Please try again.'});
            setStatusMessage({
                type: 'error',
                message: errorMessage
            });
        } finally {
            setLoading(false);
        }
    };

    return (
        <>
            <Container>
                <Title>
                    <FormattedMessage defaultMessage="Roll Call"/>
                </Title>
                
                {statusMessage && (
                    <StatusMessage type={statusMessage.type}>
                        {statusMessage.message}
                    </StatusMessage>
                )}

                <ButtonRow>
                    <CheckInButton 
                        onClick={handleCheckIn}
                        disabled={loading}
                    >
                        {loading ? <LoadingSpinner /> : '✓'}
                        <FormattedMessage defaultMessage="Check In"/>
                    </CheckInButton>
                    
                    <CheckOutButton 
                        onClick={handleCheckOut}
                        disabled={loading}
                    >
                        {loading ? <LoadingSpinner /> : '⏰'}
                        <FormattedMessage defaultMessage="Check Out"/>
                    </CheckOutButton>
                    
                    <AbsentButton 
                        onClick={() => setShowAbsentModal(true)}
                        disabled={loading}
                    >
                        {loading ? <LoadingSpinner /> : '❌'}
                        <FormattedMessage defaultMessage="Mark Absent"/>
                    </AbsentButton>
                </ButtonRow>
            </Container>

            <AbsentModal show={showAbsentModal}>
                <ModalContent>
                    <ModalTitle>
                        <FormattedMessage defaultMessage="Mark as Absent"/>
                    </ModalTitle>
                    
                    <ReasonInput
                        placeholder={intl.formatMessage({defaultMessage: 'Please provide a reason for your absence...'})}
                        value={absentReason}
                        onChange={(e) => setAbsentReason(e.target.value)}
                        maxLength={500}
                    />
                    
                    <ModalActions>
                        <SecondaryButton 
                            onClick={() => {
                                setShowAbsentModal(false);
                                setAbsentReason('');
                            }}
                        >
                            <FormattedMessage defaultMessage="Cancel"/>
                        </SecondaryButton>
                        
                        <PrimaryButton 
                            onClick={handleAbsentSubmit}
                            disabled={loading || !absentReason.trim()}
                        >
                            {loading ? <LoadingSpinner /> : null}
                            <FormattedMessage defaultMessage="Submit"/>
                        </PrimaryButton>
                    </ModalActions>
                </ModalContent>
            </AbsentModal>
        </>
    );
};

export default RollCallInterface;