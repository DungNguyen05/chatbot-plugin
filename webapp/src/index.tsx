// webapp/src/index.tsx - Enhanced version with proper Roll Call integration
import React from 'react';
import {Store, Action} from 'redux';
import styled from 'styled-components';
import {FormattedMessage} from 'react-intl';
import ReactDOM from 'react-dom';

import {GlobalState} from '@mattermost/types/store';

//@ts-ignore it exists
import aiIcon from '../../assets/bot_icon.png';
// ADD THIS: Import your Roll Call icon
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

// MODIFY THIS: Replace the icon styling with image container (square shape)
const RollCallIconContainer = styled.img`
    width: 20px;
    height: 20px;
    cursor: pointer;
    padding: 2px;
    border-radius: 0px; /* Square shape - no rounded corners */
    transition: all 0.15s ease-out;
    opacity: 0.8;
    
    &:hover {
        opacity: 1;
        background: var(--button-bg);
        transform: scale(1.1);
    }
    
    &:active {
        transform: scale(0.95);
    }
`;

const RHSTitleContainer = styled.span`
    display: flex;
	gap: 8px;
    align-items: center;
	margin-left: 8px;
`;

// REMOVE THIS: Old icon styling is no longer needed
// const RollCallIcon = styled.i`
//     font-size: 16px;
//     color: #d0ed95;
//     cursor: pointer;
//     padding: 4px;
//     border-radius: 4px;
//     transition: all 0.15s ease-out;
//     
//     &:hover {
//         color: #94a86c;
//         background: var(--button-bg);
//         transform: scale(1.1);
//     }
//     
//     &:active {
//         transform: scale(0.95);
//     }
// `;

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

const ModalHeader = styled.div`
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 20px 24px;
    border-bottom: 1px solid rgba(var(--center-channel-color-rgb), 0.16);
    background: var(--center-channel-bg);
`;

const ModalTitle = styled.h2`
    margin: 0;
    font-size: 22px;
    font-weight: 600;
    color: var(--center-channel-color);
    font-family: var(--font-family, inherit);
    letter-spacing: -0.025em;
`;

const CloseButton = styled.button`
    background: none;
    border: none;
    font-size: 24px;
    cursor: pointer;
    color: rgba(var(--center-channel-color-rgb), 0.56);
    padding: 4px;
    width: 32px;
    height: 32px;
    display: flex;
    align-items: center;
    justify-content: center;
    border-radius: 4px;
    transition: all 0.15s ease-out;
    
    &:hover {
        background-color: rgba(var(--center-channel-color-rgb), 0.08);
        color: rgba(var(--center-channel-color-rgb), 0.72);
    }
    
    &:active {
        background-color: rgba(var(--center-channel-color-rgb), 0.16);
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
                {/* Header completely removed */}
                <RollCallInterface onClose={onClose} />
            </ModalContainer>
        </ModalOverlay>
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
            
            // MODIFY THIS: Update Roll Call post dropdown to use image
            registry.registerPostDropdownMenuAction(
                <>
                    <span className='icon'>
                        <img src={rollCallIcon} alt="Roll Call" style={{width: '16px', height: '16px'}} />
                    </span>
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

        // MODIFY THIS: Register Roll Call channel header button with image instead of icon
        console.log('📋 Registering Roll Call channel header button...');
        registry.registerChannelHeaderButtonAction(
            <RollCallIconContainer src={rollCallIcon} alt="Roll Call" />,
            () => {
                console.log('📋 Roll Call channel header button clicked!');
                this.openRollCallModal();
            },
            'Roll Call',
            'Open Roll Call interface'
        );

        // Register main menu action for Roll Call
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