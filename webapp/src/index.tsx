// Copyright (c) 2023-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

import React from 'react';
import {Store, Action} from 'redux';
import styled from 'styled-components';
import {FormattedMessage} from 'react-intl';

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

// Import Roll Call components - add error handling
let RollCallModal: any = null;
try {
    RollCallModal = require('./components/roll_call/roll_call_modal').default;
    console.log('✅ RollCallModal imported successfully');
} catch (error) {
    console.error('❌ Failed to import RollCallModal:', error);
}

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

// Roll Call Icon for the channel header
const RollCallIcon = styled.i`
    font-size: 16px;
    color: var(--center-channel-color);
    cursor: pointer;
    
    &:hover {
        color: var(--button-bg);
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

export default class Plugin {
    postEventListener: PostEventListener = new PostEventListener();
    private rollCallModalContainer: HTMLDivElement | null = null;

    // Helper function to open Roll Call modal
    private openRollCallModal = () => {
        console.log('🔄 Opening Roll Call modal...');
        
        // Check if RollCallModal is available
        if (!RollCallModal) {
            console.error('❌ RollCallModal not available');
            alert('Roll Call interface is not available. Please check the console for errors.');
            return;
        }

        // Clean up any existing modal
        this.closeRollCallModal();

        try {
            // Create modal container
            const modalContainer = document.createElement('div');
            modalContainer.id = 'rollcall-modal-container';
            modalContainer.style.position = 'fixed';
            modalContainer.style.top = '0';
            modalContainer.style.left = '0';
            modalContainer.style.width = '100%';
            modalContainer.style.height = '100%';
            modalContainer.style.zIndex = '9999';
            modalContainer.style.backgroundColor = 'rgba(0, 0, 0, 0.5)';
            document.body.appendChild(modalContainer);

            console.log('📦 Modal container created');

            // Create and render modal
            const modalElement = React.createElement(RollCallModal, {
                show: true,
                onHide: this.closeRollCallModal
            });

            console.log('⚛️ Modal element created');

            // Try to render with ReactDOM
            import('react-dom').then((ReactDOM) => {
                console.log('📚 ReactDOM imported');
                
                try {
                    if ((ReactDOM as any).createRoot) {
                        // React 18
                        console.log('🚀 Using React 18 createRoot');
                        const root = (ReactDOM as any).createRoot(modalContainer);
                        root.render(modalElement);
                    } else {
                        // React 17 and below
                        console.log('🔧 Using React 17 render');
                        ReactDOM.render(modalElement, modalContainer);
                    }
                    
                    this.rollCallModalContainer = modalContainer;
                    console.log('✅ Modal rendered successfully');
                    
                } catch (renderError) {
                    console.error('❌ Failed to render modal:', renderError);
                    this.closeRollCallModal();
                    alert('Failed to open Roll Call interface. Check console for details.');
                }
            }).catch((error) => {
                console.error('❌ Failed to load ReactDOM:', error);
                this.closeRollCallModal();
                alert('Failed to load ReactDOM. Check console for details.');
            });
            
        } catch (error) {
            console.error('❌ Error in openRollCallModal:', error);
            alert('Failed to open Roll Call interface. Check console for details.');
        }
    };

    // Helper function to close Roll Call modal
    private closeRollCallModal = () => {
        console.log('🔄 Closing Roll Call modal...');
        if (this.rollCallModalContainer && this.rollCallModalContainer.parentNode) {
            this.rollCallModalContainer.parentNode.removeChild(this.rollCallModalContainer);
            this.rollCallModalContainer = null;
            console.log('✅ Modal closed successfully');
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
            )
            ;
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
            registry.registerPostDropdownMenuAction(<><span className='icon'><IconThreadSummarization/></span><FormattedMessage defaultMessage='Summarize Thread'/></>, (postId: string) => {
                const state = store.getState();
                const team = state.entities.teams.teams[state.entities.teams.currentTeamId];
                window.WebappUtils.browserHistory.push('/' + team.name + '/messages/@ai');
                doThreadAnalysis(postId, 'summarize_thread', '');
                if (rhs) {
                    store.dispatch(rhs.showRHSPlugin);
                }
            });
            registry.registerPostDropdownMenuAction(<><span className='icon'><IconReactForMe/></span><FormattedMessage defaultMessage='React for me'/></>, doReaction);
            
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
            registry.registerChannelHeaderButtonAction(<IconAIContainer src={aiIcon}/>, () => {
                store.dispatch(rhs.toggleRHSPlugin);
            },
            'Copilot',
            'Copilot',
            );
        }

        // Register Roll Call channel header button
        console.log('📋 Registering Roll Call channel header button...');
        registry.registerChannelHeaderButtonAction(
            <RollCallIcon className="fa fa-calendar-check-o" />,
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

    // Cleanup function
    public uninitialize() {
        this.closeRollCallModal();
    }
}

declare global {
    interface Window {
        registerPlugin(pluginId: string, plugin: Plugin): void
        WebappUtils: any
    }
}

window.registerPlugin(manifest.id, new Plugin());