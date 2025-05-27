// webapp/src/index.tsx - Enhanced Roll Call with better button and modal
import React from 'react';
import {Store, Action} from 'redux';
import styled, { keyframes } from 'styled-components';
import {FormattedMessage} from 'react-intl';
import ReactDOM from 'react-dom';

import {GlobalState} from '@mattermost/types/store';

//@ts-ignore it exists
import aiIcon from '../../assets/bot_icon.png';
//@ts-ignore it exists
import clipboardIcon from '../../assets/clipboard_icon.png';

import manifest from '@/manifest';

import {LLMBotPost} from './components/llmbot_post';
import PostMenu from './components/post_menu';
import IconThreadSummarization from './components/assets/icon_thread_summarization';
import IconReactForMe from './components/assets/icon_react_for_me';
import RHS from './components/rhs/rhs';
import Config from './components/system_console/config';
import {doReaction, doRunSearch, doThreadAnalysis, getAIDirectChannel} from './client';
import {setOpenRHSAction} from './redux_actions';
import PostEventListener from './websocket';
import {BotsHandler, setupRedux} from './redux';
import UnreadsSummarize from './components/unreads_summarize';
import {PostbackPost} from './components/postback_post';
import {isRHSCompatable} from './mm_webapp';
import SearchButton from './components/search_button';
import {doSelectPost} from './hooks';
import {handleAskChannelCommand, handleSummarizeChannelCommand} from './commands';
import SearchHints from './components/search_hints';

// Import enhanced Roll Call component
import RollCallInterface from './components/roll_call/roll_call_interface';

type WebappStore = Store<GlobalState, Action<Record<string, unknown>>>

const StreamingPostWebsocketEvent = 'custom_mattermost-ai_postupdate';



// Enhanced animations
const float = keyframes`
    0%, 100% {
        transform: translateY(0px);
    }
    50% {
        transform: translateY(-3px);
    }
`;

const pulseRing = keyframes`
    0% {
        transform: scale(0.8);
        opacity: 1;
    }
    100% {
        transform: scale(1.4);
        opacity: 0;
    }
`;

const shimmerEffect = keyframes`
    0% {
        background-position: -200px 0;
    }
    100% {
        background-position: calc(200px + 100%) 0;
    }
`;

const IconAIContainer = styled.img`
	border-radius: 50%;
    width: 24px;
    height: 24px;
`;

const RHSTitleContainer = styled.span`
    display: flex;
	gap: 8px;
    align-items: center;
	margin-left: 8px;
`;

// Enhanced Roll Call sidebar button with modern design
const RollCallSidebarButton = styled.div`
    display: flex;
    align-items: center;
    justify-content: center;
    width: 56px;
    height: 56px;
    margin: 12px auto;
    background: linear-gradient(135deg, #4CAF50 0%, #45a049 50%, #3d8b40 100%);
    border-radius: 16px;
    cursor: pointer;
    transition: all 0.3s cubic-bezier(0.4, 0, 0.2, 1);
    box-shadow: 
        0 4px 15px rgba(76, 175, 80, 0.3),
        0 2px 8px rgba(0, 0, 0, 0.1),
        inset 0 1px 0 rgba(255, 255, 255, 0.2);
    position: relative;
    overflow: hidden;
    
    &::before {
        content: '';
        position: absolute;
        top: -50%;
        left: -50%;
        width: 200%;
        height: 200%;
        background: linear-gradient(
            45deg,
            transparent,
            rgba(255, 255, 255, 0.1),
            transparent
        );
        transform: rotate(45deg);
        transition: all 0.6s;
        opacity: 0;
    }
    
    &::after {
        content: '';
        position: absolute;
        top: 50%;
        left: 50%;
        width: 0;
        height: 0;
        background: rgba(255, 255, 255, 0.2);
        border-radius: 50%;
        transform: translate(-50%, -50%);
        transition: all 0.6s ease;
    }
    
    &:hover {
        background: linear-gradient(135deg, #45a049 0%, #3d8b40 50%, #2e7d32 100%);
        transform: translateY(-4px) scale(1.05);
        box-shadow: 
            0 8px 25px rgba(76, 175, 80, 0.4),
            0 4px 15px rgba(0, 0, 0, 0.15);
        animation: ${float} 2s ease-in-out infinite;
        
        &::before {
            opacity: 1;
            transform: rotate(45deg) translate(50%, 50%);
        }
        
        &::after {
            width: 100%;
            height: 100%;
            opacity: 0;
        }
    }
    
    &:active {
        transform: translateY(-2px) scale(1.02);
        transition: all 0.1s ease;
    }
    
    /* Pulse effect on hover */
    &:hover .pulse-ring {
        animation: ${pulseRing} 1.5s infinite;
    }
`;

const PulseRing = styled.div`
    position: absolute;
    top: -4px;
    left: -4px;
    right: -4px;
    bottom: -4px;
    border: 2px solid rgba(76, 175, 80, 0.5);
    border-radius: 20px;
    opacity: 0;
`;

const RollCallIcon = styled.div`
    font-size: 24px;
    color: white;
    text-shadow: 0 2px 4px rgba(0, 0, 0, 0.3);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 2;
    position: relative;
    
    /* If using emoji */
    &.emoji {
        font-size: 28px;
    }
    
    /* If using FontAwesome */
    &.fa {
        font-size: 22px;
    }
    
    /* NEW: If using image icon */
    &.image {
        img {
            width: 24px;
            height: 24px;
            /* Remove the filter to preserve the original image details */
            opacity: 0.95;
            /* Optional: Add a subtle white glow to make it stand out on green background */
            filter: drop-shadow(0 1px 2px rgba(0, 0, 0, 0.3));
        }
    }
`;

// Enhanced badge for notifications
const NotificationBadge = styled.div`
    position: absolute;
    top: -6px;
    right: -6px;
    width: 20px;
    height: 20px;
    background: linear-gradient(135deg, #FF5722, #F44336);
    border-radius: 50%;
    display: flex;
    align-items: center;
    justify-content: center;
    font-size: 11px;
    font-weight: 700;
    color: white;
    box-shadow: 0 2px 8px rgba(255, 87, 34, 0.4);
    z-index: 3;
    animation: ${pulseRing} 2s infinite;
    border: 2px solid var(--sidebar-bg, #f8f9fa);
`;

// Enhanced modal overlay with glassmorphism
const ModalOverlay = styled.div`
    position: fixed;
    top: 0;
    left: 0;
    width: 100%;
    height: 100%;
    background: rgba(0, 0, 0, 0.4);
    backdrop-filter: blur(12px);
    -webkit-backdrop-filter: blur(12px);
    display: flex;
    justify-content: center;
    align-items: center;
    z-index: var(--z-index-modal, 9999);
    animation: overlayFadeIn 0.3s cubic-bezier(0.4, 0, 0.2, 1);
    
    @keyframes overlayFadeIn {
        from { 
            opacity: 0;
            backdrop-filter: blur(0px);
        }
        to { 
            opacity: 1;
            backdrop-filter: blur(12px);
        }
    }
`;

const ModalContainer = styled.div`
    background: var(--center-channel-bg);
    border-radius: 20px;
    max-width: 95%;
    max-height: 95%;
    overflow: auto;
    box-shadow: 
        0 25px 50px rgba(0, 0, 0, 0.25),
        0 10px 30px rgba(0, 0, 0, 0.15),
        inset 0 1px 0 rgba(255, 255, 255, 0.1);
    transform: scale(1);
    animation: modalAppear 0.4s cubic-bezier(0.34, 1.56, 0.64, 1);
    border: 1px solid rgba(var(--center-channel-color-rgb), 0.08);
    position: relative;
    
    @keyframes modalAppear {
        0% {
            transform: scale(0.8) translateY(20px);
            opacity: 0;
        }
        100% {
            transform: scale(1) translateY(0);
            opacity: 1;
        }
    }
    
    /* Scrollbar styling */
    &::-webkit-scrollbar {
        width: 6px;
    }
    
    &::-webkit-scrollbar-track {
        background: rgba(var(--center-channel-color-rgb), 0.05);
        border-radius: 3px;
    }
    
    &::-webkit-scrollbar-thumb {
        background: rgba(var(--center-channel-color-rgb), 0.2);
        border-radius: 3px;
        
        &:hover {
            background: rgba(var(--center-channel-color-rgb), 0.3);
        }
    }
`;

const RHSTitle = () => {
    return (
        <RHSTitleContainer>
            <IconAIContainer src={aiIcon}/>
            {'Copilot'}
        </RHSTitleContainer>
    );
};

// Enhanced React component for the modal with better animations and UX
const RollCallModal: React.FC<{
    onClose: () => void;
}> = ({ onClose }) => {
    const handleOverlayClick = React.useCallback((e: React.MouseEvent) => {
        if (e.target === e.currentTarget) {
            onClose();
        }
    }, [onClose]);

    const handleKeyDown = React.useCallback((e: React.KeyboardEvent) => {
        if (e.key === 'Escape') {
            onClose();
        }
    }, [onClose]);

    React.useEffect(() => {
        // Prevent body scroll when modal is open
        const originalOverflow = document.body.style.overflow;
        document.body.style.overflow = 'hidden';
        
        // Focus management
        const activeElement = document.activeElement as HTMLElement;
        
        return () => {
            document.body.style.overflow = originalOverflow;
            // Restore focus to the element that opened the modal
            if (activeElement && typeof activeElement.focus === 'function') {
                activeElement.focus();
            }
        };
    }, []);

    return (
        <ModalOverlay 
            onClick={handleOverlayClick}
            onKeyDown={handleKeyDown}
            tabIndex={-1}
            role="dialog"
            aria-modal="true"
            aria-labelledby="rollcall-modal-title"
        >
            <ModalContainer>
                <RollCallInterface onClose={onClose} />
            </ModalContainer>
        </ModalOverlay>
    );
};

// Enhanced Roll Call Sidebar Component with better accessibility and animations
const RollCallSidebarComponent: React.FC<{
    openModal: () => void;
    hasNotification?: boolean;
}> = ({ openModal, hasNotification = false }) => {
    const [isPressed, setIsPressed] = React.useState(false);

    const handleMouseDown = () => setIsPressed(true);
    const handleMouseUp = () => setIsPressed(false);
    const handleMouseLeave = () => setIsPressed(false);

    return (
        <RollCallSidebarButton 
            onClick={openModal}
            onMouseDown={handleMouseDown}
            onMouseUp={handleMouseUp}
            onMouseLeave={handleMouseLeave}
            title="Roll Call - Check In/Out & Attendance Tracking"
            aria-label="Open Roll Call interface for attendance tracking"
            role="button"
            tabIndex={0}
            onKeyDown={(e) => {
                if (e.key === 'Enter' || e.key === ' ') {
                    e.preventDefault();
                    openModal();
                }
            }}
            style={{
                transform: isPressed ? 'translateY(-2px) scale(1.02)' : undefined
            }}
        >
            <PulseRing className="pulse-ring" />
            
            {/* You can choose between emoji or FontAwesome icon */}
            <RollCallIcon className="image">
                <img src={clipboardIcon} alt="Roll Call" />
            </RollCallIcon>
            {/* Alternative FontAwesome icon (uncomment if preferred) */}
            {/* <RollCallIcon className="fa fa-calendar-check-o" /> */}
            
            {hasNotification && (
                <NotificationBadge title="Pending attendance action">
                    !
                </NotificationBadge>
            )}
        </RollCallSidebarButton>
    );
};

// Enhanced tooltip component for better UX
const Tooltip = styled.div<{ show: boolean }>`
    position: absolute;
    left: 70px;
    top: 50%;
    transform: translateY(-50%);
    background: var(--center-channel-bg);
    color: var(--center-channel-color);
    padding: 8px 12px;
    border-radius: 8px;
    font-size: 12px;
    font-weight: 500;
    white-space: nowrap;
    box-shadow: 0 4px 15px rgba(0, 0, 0, 0.15);
    border: 1px solid rgba(var(--center-channel-color-rgb), 0.1);
    opacity: ${props => props.show ? 1 : 0};
    visibility: ${props => props.show ? 'visible' : 'hidden'};
    transition: all 0.2s ease;
    z-index: 1000;
    pointer-events: none;
    
    &::before {
        content: '';
        position: absolute;
        left: -4px;
        top: 50%;
        transform: translateY(-50%);
        width: 0;
        height: 0;
        border-top: 4px solid transparent;
        border-bottom: 4px solid transparent;
        border-right: 4px solid var(--center-channel-bg);
    }
`;

const TooltipWrapper = styled.div`
    position: relative;
    display: inline-block;
`;

// Enhanced sidebar component with tooltip
const EnhancedRollCallSidebarComponent: React.FC<{
    openModal: () => void;
    hasNotification?: boolean;
}> = ({ openModal, hasNotification = false }) => {
    const [showTooltip, setShowTooltip] = React.useState(false);
    const [isPressed, setIsPressed] = React.useState(false);

    const handleMouseEnter = () => setShowTooltip(true);
    const handleMouseLeave = () => {
        setShowTooltip(false);
        setIsPressed(false);
    };
    const handleMouseDown = () => setIsPressed(true);
    const handleMouseUp = () => setIsPressed(false);

    return (
        <TooltipWrapper>
            <RollCallSidebarButton 
                onClick={openModal}
                onMouseEnter={handleMouseEnter}
                onMouseLeave={handleMouseLeave}
                onMouseDown={handleMouseDown}
                onMouseUp={handleMouseUp}
                title="Roll Call - Check In/Out & Attendance Tracking"
                aria-label="Open Roll Call interface for attendance tracking"
                role="button"
                tabIndex={0}
                onKeyDown={(e) => {
                    if (e.key === 'Enter' || e.key === ' ') {
                        e.preventDefault();
                        openModal();
                    }
                }}
                style={{
                    transform: isPressed ? 'translateY(-2px) scale(1.02)' : undefined
                }}
            >
                <PulseRing className="pulse-ring" />
                <RollCallIcon className="image">
                    <img src={clipboardIcon} alt="Roll Call" />
                </RollCallIcon>
                
                {hasNotification && (
                    <NotificationBadge title="Pending attendance action">
                        !
                    </NotificationBadge>
                )}
            </RollCallSidebarButton>
            
            <Tooltip show={showTooltip}>
                Roll Call - Attendance Tracking
            </Tooltip>
        </TooltipWrapper>
    );
};

export default class Plugin {
    postEventListener: PostEventListener = new PostEventListener();
    private rollCallModalElement: HTMLDivElement | null = null;
    private rollCallModalRoot: any = null;

    // Enhanced modal creation with better error handling, accessibility, and animations
    private openRollCallModal = () => {
        console.log('🔄 Opening enhanced Roll Call modal...');
        
        try {
            // Close any existing modal first
            this.closeRollCallModal();

            // Create modal container with better attributes
            this.rollCallModalElement = document.createElement('div');
            this.rollCallModalElement.id = 'rollcall-modal-root';
            this.rollCallModalElement.setAttribute('data-testid', 'rollcall-modal');
            this.rollCallModalElement.setAttribute('aria-hidden', 'false');
            this.rollCallModalElement.style.isolation = 'isolate'; // Create new stacking context
            document.body.appendChild(this.rollCallModalElement);

            // Use React 18 createRoot if available, otherwise fall back to render
            if (ReactDOM.createRoot) {
                console.log('🚀 Using React 18 createRoot for enhanced modal');
                this.rollCallModalRoot = ReactDOM.createRoot(this.rollCallModalElement);
                this.rollCallModalRoot.render(
                    <RollCallModal onClose={this.closeRollCallModal} />
                );
            } else {
                console.log('🔧 Using React 17 render for enhanced modal');
                ReactDOM.render(
                    <RollCallModal onClose={this.closeRollCallModal} />, 
                    this.rollCallModalElement
                );
                this.rollCallModalRoot = this.rollCallModalElement;
            }
            
            console.log('✅ Enhanced modal opened successfully');
            
            // Analytics/tracking (optional)
            if (typeof window !== 'undefined' && (window as any).analytics) {
                (window as any).analytics.track('Roll Call Modal Opened');
            }
            
        } catch (error) {
            console.error('❌ Error in enhanced openRollCallModal:', error);
            this.closeRollCallModal();
            
            // Show user-friendly error message with better styling
            const errorMessage = error instanceof Error ? error.message : 'Unknown error occurred';
            this.showErrorNotification('Failed to open Roll Call interface: ' + errorMessage);
        }
    };

    private closeRollCallModal = () => {
        console.log('🔄 Closing enhanced Roll Call modal...');
        
        try {
            if (this.rollCallModalRoot) {
                if (typeof this.rollCallModalRoot.unmount === 'function') {
                    // React 18
                    this.rollCallModalRoot.unmount();
                } else if (this.rollCallModalElement) {
                    // React 17
                    ReactDOM.unmountComponentAtNode(this.rollCallModalElement);
                }
                this.rollCallModalRoot = null;
            }
            
            if (this.rollCallModalElement && this.rollCallModalElement.parentNode) {
                this.rollCallModalElement.setAttribute('aria-hidden', 'true');
                // Add exit animation before removing
                this.rollCallModalElement.style.animation = 'modalExit 0.2s ease-out forwards';
                
                setTimeout(() => {
                    if (this.rollCallModalElement && this.rollCallModalElement.parentNode) {
                        this.rollCallModalElement.parentNode.removeChild(this.rollCallModalElement);
                    }
                    this.rollCallModalElement = null;
                }, 200);
            } else {
                this.rollCallModalElement = null;
            }
            
            // Restore body scroll
            document.body.style.overflow = 'auto';
            
            console.log('✅ Enhanced modal closed successfully');
            
            // Analytics/tracking (optional)
            if (typeof window !== 'undefined' && (window as any).analytics) {
                (window as any).analytics.track('Roll Call Modal Closed');
            }
            
        } catch (error) {
            console.error('❌ Error closing enhanced modal:', error);
            
            // Force cleanup with better error handling
            this.forceCleanupModal();
        }
    };

    private forceCleanupModal = () => {
        try {
            const existingModal = document.getElementById('rollcall-modal-root');
            if (existingModal && existingModal.parentNode) {
                existingModal.parentNode.removeChild(existingModal);
            }
            
            // Clean up any other potential modal remnants
            const modalRemnants = document.querySelectorAll('[data-testid="rollcall-modal"]');
            modalRemnants.forEach(element => {
                if (element.parentNode) {
                    element.parentNode.removeChild(element);
                }
            });
            
        } catch (cleanupError) {
            console.error('❌ Error in force cleanup:', cleanupError);
        } finally {
            this.rollCallModalElement = null;
            this.rollCallModalRoot = null;
            document.body.style.overflow = 'auto';
        }
    };

    private showErrorNotification = (message: string) => {
        // Create a styled error notification instead of basic alert
        const notification = document.createElement('div');
        notification.style.cssText = `
            position: fixed;
            top: 20px;
            right: 20px;
            background: linear-gradient(135deg, #f44336, #d32f2f);
            color: white;
            padding: 16px 20px;
            border-radius: 8px;
            box-shadow: 0 4px 15px rgba(244, 67, 54, 0.3);
            z-index: 10000;
            font-family: var(--font-family, -apple-system, BlinkMacSystemFont, sans-serif);
            font-size: 14px;
            font-weight: 500;
            max-width: 400px;
            animation: slideInRight 0.3s ease-out;
        `;
        notification.textContent = message;
        
        // Add animation styles
        const style = document.createElement('style');
        style.textContent = `
            @keyframes slideInRight {
                from {
                    transform: translateX(100%);
                    opacity: 0;
                }
                to {
                    transform: translateX(0);
                    opacity: 1;
                }
            }
        `;
        document.head.appendChild(style);
        
        document.body.appendChild(notification);
        
        // Auto remove after 5 seconds
        setTimeout(() => {
            if (notification.parentNode) {
                notification.style.animation = 'slideInRight 0.3s ease-out reverse';
                setTimeout(() => {
                    if (notification.parentNode) {
                        notification.parentNode.removeChild(notification);
                    }
                    if (style.parentNode) {
                        style.parentNode.removeChild(style);
                    }
                }, 300);
            }
        }, 5000);
    };

    // eslint-disable-next-line @typescript-eslint/no-unused-vars, @typescript-eslint/no-empty-function
    public async initialize(registry: any, store: WebappStore) {
        console.log('🎯 Plugin initializing with enhanced Roll Call...');
        
        setupRedux(registry, store);

        registry.registerTranslations((locale: string) => {
            try {
                // eslint-disable-next-line global-require
                return require(`./i18n/${locale}.json`);
            } catch (e) {
                return {};
            }
        });

        let rhs: any = null;
        if (isRHSCompatable()) {
            rhs = registry.registerRightHandSidebarComponent(RHS, RHSTitle);
            setOpenRHSAction(rhs.showRHSPlugin);
        }

        let currentUserId = store.getState().entities.users.currentUserId;
        if (currentUserId) {
            getAIDirectChannel(currentUserId).then((botChannelId) => {
                store.dispatch({type: 'SET_AI_BOT_CHANNEL', botChannelId} as any);
            });
        }

        store.subscribe(() => {
            const state = store.getState();
            if (state && state.entities.users.currentUserId !== currentUserId) {
                currentUserId = state.entities.users.currentUserId;
                if (currentUserId) {
                    getAIDirectChannel(currentUserId).then((botChannelId) => {
                        store.dispatch({type: 'SET_AI_BOT_CHANNEL', botChannelId} as any);
                    });
                } else {
                    store.dispatch({type: 'SET_AI_BOT_CHANNEL', botChannelId: ''} as any);
                }
            }
        });

        registry.registerWebSocketEventHandler(StreamingPostWebsocketEvent, this.postEventListener.handlePostUpdateWebsockets);
        const LLMBotPostWithWebsockets = (props: any) => {
            return (
                <LLMBotPost
                    {...props}
                    websocketRegister={this.postEventListener.registerPostUpdateListener}
                    websocketUnregister={this.postEventListener.unregisterPostUpdateListener}
                />
            );
        };

        registry.registerWebSocketEventHandler('config_changed', () => {
            store.dispatch({
                type: BotsHandler,
                bots: null,
            } as any);
        });

        registry.registerPostTypeComponent('custom_llmbot', LLMBotPostWithWebsockets);
        registry.registerPostTypeComponent('custom_llm_postback', PostbackPost);
        
        if (registry.registerPostActionComponent) {
            registry.registerPostActionComponent(PostMenu);
        } else {
            registry.registerPostDropdownMenuAction(
                <>
                    <span className='icon'><IconThreadSummarization/></span>
                    <FormattedMessage defaultMessage='Summarize Thread'/>
                </>, 
                (postId: string) => {
                    const state = store.getState();
                    const team = state.entities.teams.teams[state.entities.teams.currentTeamId];
                    window.WebappUtils.browserHistory.push('/' + team.name + '/messages/@ai');
                    doThreadAnalysis(postId, 'summarize_thread', '');
                    if (rhs) {
                        store.dispatch(rhs.showRHSPlugin);
                    }
                }
            );
            
            registry.registerPostDropdownMenuAction(
                <>
                    <span className='icon'><IconReactForMe/></span>
                    <FormattedMessage defaultMessage='React for me'/>
                </>, 
                doReaction
            );
            
            // Enhanced Roll Call post dropdown menu action
            registry.registerPostDropdownMenuAction(
                <>
                    <span className='icon' style={{fontSize: '16px'}}>📋</span>
                    <FormattedMessage defaultMessage='Roll Call'/>
                </>, 
                () => {
                    console.log('📋 Enhanced Roll Call clicked from post dropdown');
                    this.openRollCallModal();
                }
            );
        }

        registry.registerAdminConsoleCustomSetting('Config', Config);
        
        // Register AI Copilot channel header button
        if (rhs) {
            registry.registerChannelHeaderButtonAction(
                <IconAIContainer src={aiIcon}/>, 
                () => {
                    store.dispatch(rhs.toggleRHSPlugin);
                },
                'Copilot',
                'Copilot'
            );
        }

        // Enhanced Roll Call sidebar component registration
        console.log('📋 Registering enhanced Roll Call sidebar component...');
        
        // Check if user needs to check in (this could be determined by checking last check-in time)
        const hasNotification = this.shouldShowNotification();
        
        // Register as enhanced fixed positioning component with better styling
        registry.registerGlobalComponent(() => (
            <div style={{
                position: 'fixed',
                left: '16px',
                bottom: '100px',       // Moved up slightly for better visibility
                zIndex: 999,           // High but not conflicting with modals
                pointerEvents: 'auto'
            }}>
                <EnhancedRollCallSidebarComponent 
                    openModal={this.openRollCallModal}
                    hasNotification={hasNotification}
                />
            </div>
        ));

        // Enhanced main menu action for Roll Call
        if (registry.registerMainMenuAction) {
            console.log('📋 Registering enhanced Roll Call main menu action...');
            registry.registerMainMenuAction(
                '📋 Roll Call',
                () => {
                    console.log('📋 Enhanced Roll Call main menu clicked!');
                    this.openRollCallModal();
                },
                null
            );
        }

        if (registry.registerNewMessagesSeparatorActionComponent) {
            registry.registerNewMessagesSeparatorActionComponent(UnreadsSummarize);
        }

        // Enhanced slash commands with better feedback
        if (rhs) {
            registry.registerSlashCommandWillBePostedHook((message: string, args: any) => {
                if (message.startsWith('/ask-channel')) {
                    const query = message.replace('/ask-channel', '').trim();
                    return handleAskChannelCommand(query, args, store, rhs);
                } else if (message.startsWith('/summarize-channel')) {
                    const commandParams = message.replace('/summarize-channel', '').trim();
                    return handleSummarizeChannelCommand(commandParams, args, store, rhs);
                } else if (message.startsWith('/rollcall') || message.trim() === '/rollcall') {
                    console.log('📋 Enhanced /rollcall command used!');
                    this.openRollCallModal();
                    return Promise.resolve({});
                }
                return {message, args};
            });
        }

        if (registry.registerSearchComponents) {
            registry.registerSearchComponents({
                buttonComponent: SearchButton,
                suggestionsComponent: () => null,
                hintsComponent: SearchHints,
                action: async (searchTerms: string) => {
                    const state = store.getState() as any;
                    const bots = state['plugins-' + manifest.id]?.bots || [];
                    const activeBotUsername = localStorage.getItem('defaultBot') || '';
                    const activeBot = bots.find((bot: any) => bot.username === activeBotUsername);

                    const result = await doRunSearch(
                        searchTerms,
                        '',
                        '',
                        activeBot?.username,
                    );
                    doSelectPost(result.postId, result.channelId, store.dispatch);
                    if (rhs) {
                        store.dispatch(rhs.showRHSPlugin);
                    }
                },
            });
        }

        console.log('✅ Plugin initialized successfully with enhanced Roll Call features');
    }

    // Helper method to determine if notification badge should show
    private shouldShowNotification = (): boolean => {
        try {
            const lastCheckIn = localStorage.getItem('rollcall_last_checkin');
            const today = new Date().toDateString();
            
            // Show notification if user hasn't checked in today
            return !lastCheckIn || new Date(lastCheckIn).toDateString() !== today;
        } catch {
            return false;
        }
    };

    // Enhanced cleanup function
    public uninitialize() {
        console.log('🔄 Plugin uninitializing with enhanced cleanup...');
        this.closeRollCallModal();
        
        // Clean up any global styles or event listeners
        const dynamicStyles = document.querySelectorAll('style[data-rollcall]');
        dynamicStyles.forEach(style => {
            if (style.parentNode) {
                style.parentNode.removeChild(style);
            }
        });
        
        console.log('✅ Plugin uninitialized successfully');
    }
}

declare global {
    interface Window {
        registerPlugin(pluginId: string, plugin: Plugin): void
        WebappUtils: any
    }
}

window.registerPlugin(manifest.id, new Plugin());