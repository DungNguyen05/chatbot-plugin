import React, {useState} from 'react';
import styled from 'styled-components';

import {doCheckIn, doCheckOut, doAbsent} from '../../client';

const Container = styled.div`
    display: flex;
    flex-direction: column;
    padding: 32px;
    gap: 24px;
    max-width: 500px;
    margin: 0 auto;
    background: white;
    border-radius: 12px;
`;

const Title = styled.h2`
    font-size: 24px;
    font-weight: 700;
    margin-bottom: 8px;
    text-align: center;
    color: #1a1a1a;
    letter-spacing: -0.025em;
`;

const Subtitle = styled.p`
    font-size: 14px;
    color: #6b7280;
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
    padding: 16px 20px;
    border: none;
    border-radius: 8px;
    font-weight: 600;
    font-size: 15px;
    cursor: pointer;
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 8px;
    transition: all 0.2s cubic-bezier(0.4, 0, 0.2, 1);
    min-height: 60px;
    position: relative;
    overflow: hidden;
    
    &:disabled {
        opacity: 0.6;
        cursor: not-allowed;
        transform: none !important;
    }
    
    &:not(:disabled):hover {
        transform: translateY(-2px);
        box-shadow: 0 8px 25px rgba(0, 0, 0, 0.15);
    }
    
    &:not(:disabled):active {
        transform: translateY(0);
        transition: all 0.1s cubic-bezier(0.4, 0, 0.2, 1);
    }
`;

const CheckInButton = styled(ActionButton)`
    background: linear-gradient(135deg, #10b981 0%, #059669 100%);
    color: white;
    
    &:hover:not(:disabled) {
        background: linear-gradient(135deg, #059669 0%, #047857 100%);
    }
`;

const CheckOutButton = styled(ActionButton)`
    background: linear-gradient(135deg, #3b82f6 0%, #2563eb 100%);
    color: white;
    
    &:hover:not(:disabled) {
        background: linear-gradient(135deg, #2563eb 0%, #1d4ed8 100%);
    }
`;

const AbsentButton = styled(ActionButton)`
    background: transparent;
    color: #dc2626;
    border: 2px solid #dc2626;
    grid-column: 1 / -1;
    
    &:hover:not(:disabled) {
        background: #dc2626;
        color: white;
    }
`;

const AbsentModal = styled.div<{show: boolean}>`
    display: ${props => props.show ? 'flex' : 'none'};
    position: fixed;
    top: 0;
    left: 0;
    width: 100%;
    height: 100%;
    background-color: rgba(0, 0, 0, 0.6);
    backdrop-filter: blur(4px);
    justify-content: center;
    align-items: center;
    z-index: 1000;
`;

const ModalContent = styled.div`
    background: white;
    padding: 32px;
    border-radius: 16px;
    min-width: 400px;
    max-width: 90%;
    box-shadow: 0 25px 50px rgba(0, 0, 0, 0.25);
    transform: scale(1);
    animation: modalAppear 0.2s cubic-bezier(0.4, 0, 0.2, 1);
    
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
    font-weight: 700;
    color: #1a1a1a;
    letter-spacing: -0.025em;
`;

const ReasonInput = styled.textarea`
    width: 100%;
    min-height: 100px;
    padding: 12px 16px;
    border: 2px solid #e5e7eb;
    border-radius: 8px;
    resize: vertical;
    font-family: inherit;
    margin-bottom: 20px;
    font-size: 14px;
    transition: border-color 0.2s cubic-bezier(0.4, 0, 0.2, 1);
    
    &:focus {
        outline: none;
        border-color: #3b82f6;
        box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.1);
    }
    
    &::placeholder {
        color: #9ca3af;
    }
`;

const ModalActions = styled.div`
    display: flex;
    gap: 12px;
    justify-content: flex-end;
`;

const SecondaryButton = styled.button`
    padding: 10px 20px;
    border: 2px solid #e5e7eb;
    border-radius: 8px;
    background: white;
    color: #374151;
    font-weight: 600;
    cursor: pointer;
    transition: all 0.2s cubic-bezier(0.4, 0, 0.2, 1);
    
    &:hover {
        border-color: #d1d5db;
        background-color: #f9fafb;
    }
`;

const PrimaryButton = styled.button`
    padding: 10px 20px;
    border: none;
    border-radius: 8px;
    background: linear-gradient(135deg, #dc2626 0%, #b91c1c 100%);
    color: white;
    font-weight: 600;
    cursor: pointer;
    transition: all 0.2s cubic-bezier(0.4, 0, 0.2, 1);
    
    &:hover:not(:disabled) {
        background: linear-gradient(135deg, #b91c1c 0%, #991b1b 100%);
        transform: translateY(-1px);
    }
    
    &:disabled {
        opacity: 0.6;
        cursor: not-allowed;
        transform: none !important;
    }
`;

const StatusMessage = styled.div<{type: 'success' | 'error'}>`
    padding: 16px 20px;
    border-radius: 8px;
    margin-bottom: 20px;
    background-color: ${props => props.type === 'success' ? '#ecfdf5' : '#fef2f2'};
    color: ${props => props.type === 'success' ? '#065f46' : '#991b1b'};
    border: 1px solid ${props => props.type === 'success' ? '#a7f3d0' : '#fca5a5'};
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
}

const RollCallInterface: React.FC<RollCallInterfaceProps> = ({onClose}) => {
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
                message: response.message || 'Successfully checked in!'
            });
            setTimeout(() => {
                onClose?.();
            }, 2000);
        } catch (error: any) {
            const errorMessage = error?.message || 'Failed to check in. Please try again.';
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
                message: response.message || 'Successfully checked out!'
            });
            setTimeout(() => {
                onClose?.();
            }, 2000);
        } catch (error: any) {
            const errorMessage = error?.message || 'Failed to check out. Please try again.';
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
                message: 'Please provide a reason for absence.'
            });
            return;
        }

        setLoading(true);
        setStatusMessage(null);
        try {
            const response = await doAbsent(absentReason.trim());
            setStatusMessage({
                type: 'success',
                message: response.message || 'Absence recorded successfully!'
            });
            setShowAbsentModal(false);
            setAbsentReason('');
            setTimeout(() => {
                onClose?.();
            }, 2000);
        } catch (error: any) {
            const errorMessage = error?.message || 'Failed to record absence. Please try again.';
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
                <div>
                    <Title>Roll Call</Title>
                    <Subtitle>Record your attendance for today</Subtitle>
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
                    >
                        {loading ? <LoadingSpinner /> : null}
                        Check In
                    </CheckInButton>
                    
                    <CheckOutButton 
                        onClick={handleCheckOut}
                        disabled={loading}
                    >
                        {loading ? <LoadingSpinner /> : null}
                        Check Out
                    </CheckOutButton>
                    
                    <AbsentButton 
                        onClick={() => setShowAbsentModal(true)}
                        disabled={loading}
                    >
                        Report Absence
                    </AbsentButton>
                </ButtonGrid>
            </Container>

            <AbsentModal show={showAbsentModal}>
                <ModalContent>
                    <ModalTitle>Report Absence</ModalTitle>
                    
                    <ReasonInput
                        placeholder="Please provide a reason for your absence..."
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
                            Cancel
                        </SecondaryButton>
                        
                        <PrimaryButton 
                            onClick={handleAbsentSubmit}
                            disabled={loading || !absentReason.trim()}
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