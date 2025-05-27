// webapp/src/index.tsx - Modified to move Roll Call to sidebar bottom
import React from 'react';
import {Store, Action} from 'redux';
import styled from 'styled-components';
import {FormattedMessage} from 'react-intl';
import ReactDOM from 'react-dom';

import {GlobalState} from '@mattermost/types/store';

//@ts-ignore it exists
import aiIcon from '../../assets/bot_icon.png';

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

// Enhanced Roll Call sidebar button styling
const RollCallSidebarButton = styled.div`
    display: flex;
    align-items: center;
    justify-content: center;
    width: 40px;
    height: 40px;
    margin: 8px auto;
    background: linear-gradient(135deg, #4CAF50, #45a049);
    border-radius: 8px;
    cursor: pointer;
    transition: all 0.2s ease;
    box-shadow: 0 2px 8px rgba(76, 175, 80, 0.3);
    position: relative;
    
    &:hover {
        background: linear-gradient(135deg, #45a049, #4CAF50);
        transform: translateY(-2px);
        box-shadow: 0 4px 12px rgba(76, 175, 80, 0.4);
    }
    
    &:active {
        transform: translateY(0);
        transition: all 0.1s ease;
    }
    
    &::before {
        content: '';
        position: absolute;
        top: -2px;
        left: -2px;
        right: -2px;
        bottom: -2px;
        background: linear-gradient(135deg, #4CAF50, #45a049);
        border-radius: 10px;
        z-index: -1;
        opacity: 0;
        transition: opacity 0.2s ease;
    }
    
    &:hover::before {
        opacity: 0.3;
    }
`;

const RollCallIcon = styled.i`
    font-size: 18px;
    color: white;
    text-shadow: 0 1px 2px rgba(0, 0, 0, 0.2);
`;

// Alternative using FontAwesome or similar icon
const RollCallIconSVG = styled.div`
    width: 20px;
    height: 20px;
    color: white;
    
    svg {
        width: 100%;
        height: 100%;
        fill: currentColor;
        filter: drop-shadow(0 1px 2px rgba(0, 0, 0, 0.2));
    }
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

// Roll Call Sidebar Component
const RollCallSidebarComponent: React.FC<{openModal: () => void}> = ({ openModal }) => {
    return (
        <RollCallSidebarButton 
            onClick={openModal}
            title="Roll Call - Check In/Out"
            aria-label="Open Roll Call interface"
        >
            <RollCallIcon className="fa fa-calendar-check-o" />
            {/* Alternative SVG icon if FontAwesome is not available */}
            {/* <RollCallIconSVG>
                <svg viewBox="0 0 24 24">
                    <path d="M19,3H18V1H16V3H8V1H6V3H5A2,2 0 0,0 3,5V19A2,2 0 0,0 5,21H19A2,2 0 0,0 21,19V5A2,2 0 0,0 19,3M19,19H5V8H19V19Z" />
                    <path d="M10.5,12.5L12,14L15.5,10.5L14.09,9.09L12,11.17L10.91,10.09L10.5,12.5Z" />
                </svg>
            </RollCallIconSVG> */}
        </RollCallSidebarButton>
    );
};

export default class Plugin {
    postEventListener: PostEventListener = new PostEventListener();
    private rollCallModalElement: HTMLDivElement | null = null;
    private rollCallModalRoot: any = null;

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

            // Use React 18 createRoot if available, otherwise fall back to render
            if (ReactDOM.createRoot) {
                console.log('🚀 Using React 18 createRoot');
                this.rollCallModalRoot = ReactDOM.createRoot(this.rollCallModalElement);
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
            
            // Add Roll Call to post dropdown menu
            registry.registerPostDropdownMenuAction(
                <>
                    <span className='icon'>📋</span>
                    <FormattedMessage defaultMessage='Roll Call'/>
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

        // REMOVED: Roll Call channel header button registration
        // This was: registry.registerChannelHeaderButtonAction(...)

        // NEW: Register Roll Call as a fixed bottom component
        console.log('📋 Registering Roll Call sidebar component...');
        
        // Always use fixed positioning at the bottom of the sidebar
        registry.registerGlobalComponent(() => (
            <div style={{
                position: 'fixed',
                left: '16px',           // Align with sidebar content
                bottom: '80px',         // Above the bottom app bar if it exists
                zIndex: 1000,
                pointerEvents: 'auto'   // Ensure it's clickable
            }}>
                <RollCallSidebarComponent openModal={this.openRollCallModal} />
            </div>
        ));

        // Register main menu action for Roll Call (keep this as backup)
        if (registry.registerMainMenuAction) {
            console.log('📋 Registering Roll Call main menu action...');
            registry.registerMainMenuAction(
                'Roll Call',
                () => {
                    console.log('📋 Roll Call main menu clicked!');
                    this.openRollCallModal();
                },
                null
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

        console.log('✅ Plugin initialized successfully');
    }

    // Enhanced cleanup function
    public uninitialize() {
        console.log('🔄 Plugin uninitializing...');
        this.closeRollCallModal();
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