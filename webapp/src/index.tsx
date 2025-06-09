// webapp/src/index.tsx - Enhanced version with proper Roll Call integration and Floating Button
import React from 'react';
import {Store, Action} from 'redux';
import styled from 'styled-components';
import {FormattedMessage} from 'react-intl';
import ReactDOM from 'react-dom';

import {GlobalState} from '@mattermost/types/store';

//@ts-ignore it exists
import aiIcon from '../../assets/bot_icon.png';
//@ts-ignore it exists
import rollCallIcon from '../../assets/roll_call_icon.png';

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

// Import Roll Call component
import RollCallInterface from './components/roll_call/roll_call_interface';

type WebappStore = Store<GlobalState, Action<Record<string, unknown>>>

const StreamingPostWebsocketEvent = 'custom_mattermost-ai_postupdate';

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

// Enhanced modal overlay styles
const ModalOverlay = styled.div`
    position: fixed;
    top: 0;
    left: 0;
    width: 100%;
    height: 100%;
    background-color: rgba(0, 0, 0, 0.64);
    backdrop-filter: blur(8px);
    display: flex;
    justify-content: center;
    align-items: center;
    z-index: var(--z-index-modal, 9999);
    animation: overlayFadeIn 0.15s ease-out;
    
    @keyframes overlayFadeIn {
        from { opacity: 0; }
        to { opacity: 1; }
    }
`;

const ModalContainer = styled.div`
    background: var(--center-channel-bg);
    border-radius: 8px;
    max-width: 90%;
    max-height: 90%;
    overflow: auto;
    box-shadow: var(--elevation-8, 0 25px 50px rgba(0, 0, 0, 0.25));
    transform: scale(1);
    animation: modalAppear 0.2s ease-out;
    border: 1px solid rgba(var(--center-channel-color-rgb), 0.16);
    
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

// Floating Roll Call Button Component
const FloatingRollCallButton = styled.button`
    position: fixed;
    bottom: 70px;
    right: 5px;
    width: 30px;
    height: 30px;
    border-radius: 20%;
    background: linear-gradient(135deg, #4CAF50, #45a049);
    border: none;
    cursor: pointer;
    box-shadow: 
        0 8px 25px rgba(76, 175, 80, 0.3),
        0 4px 12px rgba(0, 0, 0, 0.15);
    z-index: 1000;
    transition: all 0.3s cubic-bezier(0.4, 0, 0.2, 1);
    display: flex;
    align-items: center;
    justify-content: center;
    color: white;
    font-size: 24px;
    transform: scale(1);
    
    &:hover {
        transform: scale(1.1);
        box-shadow: 
            0 12px 35px rgba(76, 175, 80, 0.4),
            0 6px 16px rgba(0, 0, 0, 0.2);
        background: linear-gradient(135deg, #45a049, #3d8b40);
    }
    
    &:active {
        transform: scale(0.95);
        transition: all 0.1s ease-out;
    }
    
    // Responsive design - tablet
    @media (max-width: 768px) {
        top: 10px;
        right: 19%;
        width: 33px;
        height: 33px;
        font-size: 22px;
    }
    
    // Even smaller on mobile
    @media (max-width: 480px) {
        right: 38%;
        top: 10px;
        width: 33px;
        height: 33px;
        font-size: 20px;
    }
`;

const FloatingButtonIcon = styled.img`
    width: 120%; // Flexible size based on button size
    height: 120%; // Flexible size based on button size
    object-fit: contain; // Maintain aspect ratio
    transition: all 0.3s cubic-bezier(0.4, 0, 0.2, 1); // Smooth transitions
`;

const RHSTitle = () => {
    return (
        <RHSTitleContainer>
            <IconAIContainer src={aiIcon}/>
            {'Copilot'}
        </RHSTitleContainer>
    );
};

// Enhanced React component for the modal with better error handling
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
        document.body.style.overflow = 'hidden';
        
        return () => {
            document.body.style.overflow = 'auto';
        };
    }, []);

    return (
        <ModalOverlay 
            onClick={handleOverlayClick}
            onKeyDown={handleKeyDown}
            tabIndex={-1}
            role="dialog"
            aria-modal="true"
        >
            <ModalContainer>
                <RollCallInterface onClose={onClose} />
            </ModalContainer>
        </ModalOverlay>
    );
};

// Floating Roll Call Button React Component
const FloatingRollCallButtonComponent: React.FC<{
    onClick: () => void;
}> = ({ onClick }) => {
    return (
        <FloatingRollCallButton
            onClick={onClick}
            title="Open Roll Call"
            aria-label="Open Roll Call interface"
        >
            <FloatingButtonIcon src={rollCallIcon} alt="Roll Call" />
        </FloatingRollCallButton>
    );
};

export default class Plugin {
    postEventListener: PostEventListener = new PostEventListener();
    private rollCallModalElement: HTMLDivElement | null = null;
    private rollCallModalRoot: any = null;
    private floatingButtonElement: HTMLElement | null = null;

    // Enhanced modal creation with better error handling and cleanup
    private openRollCallModal = () => {
        console.log('🔄 Opening Roll Call modal...');
        
        try {
            // Close any existing modal first
            this.closeRollCallModal();

            // Create modal container
            this.rollCallModalElement = document.createElement('div');
            this.rollCallModalElement.id = 'rollcall-modal-root';
            this.rollCallModalElement.setAttribute('data-testid', 'rollcall-modal');
            document.body.appendChild(this.rollCallModalElement);

            // Type-safe React 18 check
            const ReactDOMWithCreateRoot = ReactDOM as any;
            if (ReactDOMWithCreateRoot.createRoot && typeof ReactDOMWithCreateRoot.createRoot === 'function') {
                console.log('🚀 Using React 18 createRoot');
                this.rollCallModalRoot = ReactDOMWithCreateRoot.createRoot(this.rollCallModalElement);
                this.rollCallModalRoot.render(
                    <RollCallModal onClose={this.closeRollCallModal} />
                );
            } else {
                console.log('🔧 Using React 17 render');
                ReactDOM.render(
                    <RollCallModal onClose={this.closeRollCallModal} />, 
                    this.rollCallModalElement
                );
                this.rollCallModalRoot = this.rollCallModalElement;
            }
            
            console.log('✅ Modal opened successfully with React component');
            
        } catch (error) {
            console.error('❌ Error in openRollCallModal:', error);
            this.closeRollCallModal();
            
            // Show user-friendly error message
            const errorMessage = error instanceof Error ? error.message : 'Unknown error occurred';
            alert('Failed to open Roll Call interface: ' + errorMessage);
        }
    };

    private closeRollCallModal = () => {
        console.log('🔄 Closing Roll Call modal...');
        
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
                this.rollCallModalElement.parentNode.removeChild(this.rollCallModalElement);
                this.rollCallModalElement = null;
            }
            
            // Restore body scroll
            document.body.style.overflow = 'auto';
            
            console.log('✅ Modal closed successfully');
        } catch (error) {
            console.error('❌ Error closing modal:', error);
            
            // Force cleanup
            const existingModal = document.getElementById('rollcall-modal-root');
            if (existingModal && existingModal.parentNode) {
                try {
                    existingModal.parentNode.removeChild(existingModal);
                } catch (cleanupError) {
                    console.error('❌ Error in force cleanup:', cleanupError);
                }
            }
            
            this.rollCallModalElement = null;
            this.rollCallModalRoot = null;
            document.body.style.overflow = 'auto';
        }
    };

    // Helper function to check if we're on the main chat interface
    private isMainInterface = (): boolean => {
        const currentPath = window.location.pathname;
        
        // Hide on authentication pages
        if (currentPath.includes('/login') || 
            currentPath.includes('/signup') || 
            currentPath.includes('/reset_password') || 
            currentPath.includes('/should_verify_email') || 
            currentPath.includes('/do_verify_email') || 
            currentPath.includes('/claim') || 
            currentPath === '/' && !document.querySelector('[data-testid="team-sidebar"]')) {
            return false;
        }
        
        // Show on team channels and direct messages, hide on system console and other admin pages
        return !currentPath.includes('/admin_console') && 
               !currentPath.includes('/system_console') && 
               !currentPath.includes('/settings') &&
               !currentPath.includes('/integrations') &&
               !currentPath.includes('/user_settings');
    };

    // Create floating button method with interface detection
    private createFloatingButton = () => {
        // Check if we should show the button on current interface
        if (!this.isMainInterface()) {
            console.log('📋 Not on main interface, skipping floating button creation');
            return;
        }

        // Remove any existing floating button
        const existingButton = document.getElementById('rollcall-floating-button');
        if (existingButton) {
            existingButton.remove();
        }

        // Create new button container
        const buttonContainer = document.createElement('div');
        buttonContainer.id = 'rollcall-floating-button';
        document.body.appendChild(buttonContainer);

        // Render the floating button
        try {
            // Type-safe React 18 check
            const ReactDOMWithCreateRoot = ReactDOM as any;
            if (ReactDOMWithCreateRoot.createRoot && typeof ReactDOMWithCreateRoot.createRoot === 'function') {
                const root = ReactDOMWithCreateRoot.createRoot(buttonContainer);
                root.render(
                    <FloatingRollCallButtonComponent onClick={this.openRollCallModal} />
                );
            } else {
                ReactDOM.render(
                    <FloatingRollCallButtonComponent onClick={this.openRollCallModal} />,
                    buttonContainer
                );
            }
            
            // Store reference for cleanup
            this.floatingButtonElement = buttonContainer;
            console.log('✅ Floating Roll Call button created successfully');
        } catch (error) {
            console.error('❌ Error creating floating button:', error);
        }
    };

    // Enhanced cleanup function
    private cleanupFloatingButton = () => {
        if (this.floatingButtonElement && this.floatingButtonElement.parentNode) {
            this.floatingButtonElement.parentNode.removeChild(this.floatingButtonElement);
            this.floatingButtonElement = null;
        }
    };

    // Method to handle route changes
    private handleRouteChange = () => {
        // Small delay to ensure DOM is ready
        setTimeout(() => {
            if (this.isMainInterface()) {
                // Create button if we're on main interface and it doesn't exist
                if (!document.getElementById('rollcall-floating-button')) {
                    this.createFloatingButton();
                }
            } else {
                // Remove button if we're not on main interface
                this.cleanupFloatingButton();
            }
        }, 100);
    };

    // eslint-disable-next-line @typescript-eslint/no-unused-vars, @typescript-eslint/no-empty-function
    public async initialize(registry: any, store: WebappStore) {
        console.log('🎯 Plugin initializing...');
        
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
            
            registry.registerPostDropdownMenuAction(
                <>
                    <span className='icon'>
                        <img src={rollCallIcon} alt="Roll Call" style={{width: '16px', height: '16px'}} />
                    </span>
                    <FormattedMessage id="rollcall.title" defaultMessage="Roll Call"/>
                </>, 
                () => {
                    console.log('📋 Roll Call clicked from post dropdown');
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

        if (registry.registerNewMessagesSeparatorActionComponent) {
            registry.registerNewMessagesSeparatorActionComponent(UnreadsSummarize);
        }

        // Register slash commands
        if (rhs) {
            registry.registerSlashCommandWillBePostedHook((message: string, args: any) => {
                if (message.startsWith('/ask-channel')) {
                    const query = message.replace('/ask-channel', '').trim();
                    return handleAskChannelCommand(query, args, store, rhs);
                } else if (message.startsWith('/summarize-channel')) {
                    const commandParams = message.replace('/summarize-channel', '').trim();
                    return handleSummarizeChannelCommand(commandParams, args, store, rhs);
                } else if (message.startsWith('/rollcall') || message.trim() === '/rollcall') {
                    // Open Roll Call modal when /rollcall command is used
                    console.log('📋 /rollcall command used!');
                    this.openRollCallModal();
                    return Promise.resolve({});
                }
                return {message, args};
            });
        }

        if (registry.registerSearchComponents) {
            // The SearchButton and SearchHints components will check if search is enabled
            registry.registerSearchComponents({
                buttonComponent: SearchButton,
                suggestionsComponent: () => null,
                hintsComponent: SearchHints,
                action: async (searchTerms: string) => {
                    // Get the active bot from the state
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

        // Create floating Roll Call button only on main interface
        console.log('📋 Creating floating Roll Call button...');
        this.createFloatingButton();

        // Listen for route changes to show/hide button appropriately
        if (window.WebappUtils && window.WebappUtils.browserHistory) {
            window.WebappUtils.browserHistory.listen(this.handleRouteChange);
        }

        // Also listen for popstate events (back/forward browser buttons)
        window.addEventListener('popstate', this.handleRouteChange);

        console.log('✅ Plugin initialized successfully');
    }

    // Enhanced cleanup function
    public uninitialize() {
        console.log('🔄 Plugin uninitializing...');
        this.closeRollCallModal();
        this.cleanupFloatingButton();
        
        // Remove event listeners
        window.removeEventListener('popstate', this.handleRouteChange);
        
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