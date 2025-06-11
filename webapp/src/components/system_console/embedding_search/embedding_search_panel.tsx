// Copyright (c) 2023-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

import React from 'react';
import {useIntl, FormattedMessage} from 'react-intl';
import styled from 'styled-components';

import {useIsBasicsLicensed} from '@/license';

import {Pill} from '../../pill';
import EnterpriseChip from '../enterprise_chip';
import Panel from '../panel';
import {ItemList, SelectionItem, SelectionItemOption} from '../item';
import {IntItem} from '../number_items';

import {EmbeddingSearchConfig} from './types';
import {OpenAIProviderConfig, OpenAICompatibleProviderConfig} from './provider_configs';
import {ChunkingOptionsConfig} from './chunking_options';
import {ReindexSection} from './reindex_section';
import {ReindexConfirmation} from './reindex_confirmation';
import {useJobStatus} from './use_job_status';

const Horizontal = styled.div`
    display: flex;
    flex-direction: row;
    align-items: center;
    gap: 8px;
`;

const DatabaseWarning = styled.div`
    padding: 12px;
    margin-bottom: 16px;
    background-color: #fff3cd;
    border: 1px solid #ffeaa7;
    border-radius: 4px;
    color: #856404;
    font-size: 14px;
    line-height: 1.4;
`;

const WarningIcon = styled.span`
    font-weight: bold;
    margin-right: 8px;
`;

interface Props {
    value: EmbeddingSearchConfig;
    onChange: (config: EmbeddingSearchConfig) => void;
    // Add database type prop to determine if PostgreSQL is being used
    databaseType?: string;
}

const EmbeddingSearchPanel = ({value, onChange, databaseType = 'postgres'}: Props) => {
    const intl = useIntl();
    const isBasicsLicensed = useIsBasicsLicensed();
    const isPostgreSQL = databaseType === 'postgres';

    const {
        jobStatus,
        statusMessage,
        showReindexConfirmation,
        pendingReindexParams,
        handleReindexClick,
        handleConfirmReindex,
        handleCancelReindex,
        handleCancelJob,
    } = useJobStatus();

    if (!isBasicsLicensed) {
        return (
            <Panel
                title={
                    <Horizontal>
                        <FormattedMessage defaultMessage='Embedding Search'/>
                        <Pill><FormattedMessage defaultMessage='EXPERIMENTAL'/></Pill>
                    </Horizontal>
                }
                subtitle={''}
            >
                <EnterpriseChip
                    text={intl.formatMessage({defaultMessage: 'Embeddings search is available on Enterprise plans'})}
                    subtext={intl.formatMessage({defaultMessage: 'Embeddings search is available on Enterprise plans'})}
                />
            </Panel>
        );
    }

    const renderDatabaseWarning = () => {
        if (isPostgreSQL) {
            return null;
        }

        return (
            <DatabaseWarning>
                <WarningIcon>⚠️</WarningIcon>
                <strong>Database Compatibility Notice:</strong> Embedding search requires PostgreSQL with the pgvector extension. 
                Your current database ({databaseType === 'mysql' ? 'MySQL' : databaseType}) does not support vector operations. 
                To use embedding search features, please migrate to PostgreSQL and install the pgvector extension.
            </DatabaseWarning>
        );
    };

    const isSearchDisabled = !isPostgreSQL;

    return (
        <Panel
            title={
                <Horizontal>
                    <FormattedMessage defaultMessage='Embedding Search'/>
                    <Pill><FormattedMessage defaultMessage='EXPERIMENTAL'/></Pill>
                </Horizontal>
            }
            subtitle={intl.formatMessage({defaultMessage: 'Configure embedding search settings. Note: The current implementation is experimental and subject to breaking changes. This includes having to reindex all posts.'})}
        >
            {renderDatabaseWarning()}
            
            <ItemList>
                <SelectionItem
                    label={intl.formatMessage({defaultMessage: 'Type'})}
                    value={isSearchDisabled ? '' : value.type}
                    onChange={(e) => {
                        if (isSearchDisabled) return;
                        
                        const newType = e.target.value;
                        if (newType === '') {
                            onChange({
                                type: '',
                                vectorStore: {type: '', parameters: {}},
                                embeddingProvider: {type: '', parameters: {}},
                                parameters: {},
                                dimensions: 0,
                                chunkingOptions: {
                                    chunkSize: 1000,
                                    chunkOverlap: 200,
                                    minChunkSize: 0.75,
                                    chunkingStrategy: 'sentences',
                                },
                            });
                        } else if (value.type === '') {
                            // Set defaults when enabling
                            onChange({
                                type: newType,
                                vectorStore: {type: 'pgvector', parameters: {}},
                                embeddingProvider: {type: 'openai', parameters: {embeddingModel: '', apiKey: ''}},
                                parameters: {},
                                dimensions: 0,
                                chunkingOptions: {
                                    chunkSize: 1000,
                                    chunkOverlap: 200,
                                    minChunkSize: 0.75,
                                    chunkingStrategy: 'sentences',
                                },
                            });
                        } else {
                            onChange({...value, type: newType});
                        }
                    }}
                >
                    <SelectionItemOption value=''>{'Disabled'}</SelectionItemOption>
                    <SelectionItemOption value='composite' disabled={isSearchDisabled}>
                        {'Composite'}
                        {isSearchDisabled && ' (PostgreSQL Required)'}
                    </SelectionItemOption>
                </SelectionItem>

                {!isSearchDisabled && value.type && value.type !== '' &&
                <SelectionItem
                    label={intl.formatMessage({defaultMessage: 'Vector Store Type'})}
                    value={value.vectorStore.type}
                    onChange={(e) => onChange({
                        ...value,
                        vectorStore: {...value.vectorStore, type: e.target.value},
                    })}
                >
                    <SelectionItemOption value='pgvector'>{'PostgreSQL pgvector'}</SelectionItemOption>
                </SelectionItem>
                }

                {!isSearchDisabled && value.type && value.type !== '' &&
                <SelectionItem
                    label={intl.formatMessage({defaultMessage: 'Embedding Provider Type'})}
                    value={value.embeddingProvider.type}
                    onChange={(e) => {
                        const newType = e.target.value;
                        let newParameters = {};
                        if (newType === 'openai-compatible') {
                            newParameters = {embeddingModel: '', apiKey: '', apiURL: ''};
                        } else if (newType === 'openai') {
                            newParameters = {embeddingModel: '', apiKey: ''};
                        }
                        onChange({
                            ...value,
                            embeddingProvider: {
                                type: newType,
                                parameters: newParameters,
                            },
                        });
                    }}
                >
                    <SelectionItemOption value='openai'>{'OpenAI'}</SelectionItemOption>
                    <SelectionItemOption value='openai-compatible'>{'OpenAI-compatible API'}</SelectionItemOption>
                </SelectionItem>
                }

                {!isSearchDisabled && value.type && value.type !== '' && value.embeddingProvider.type === 'openai' && (
                    <OpenAIProviderConfig
                        value={value.embeddingProvider}
                        onChange={(config) => onChange({...value, embeddingProvider: config})}
                    />
                )}

                {!isSearchDisabled && value.type && value.type !== '' && value.embeddingProvider.type === 'openai-compatible' && (
                    <OpenAICompatibleProviderConfig
                        value={value.embeddingProvider}
                        onChange={(config) => onChange({...value, embeddingProvider: config})}
                    />
                )}

                {!isSearchDisabled && value.type === 'composite' && (
                    <>
                        <IntItem
                            label={intl.formatMessage({defaultMessage: 'Dimensions'})}
                            placeholder='1024'
                            value={value?.dimensions}
                            onChange={(dimensionsValue) => {
                                onChange({
                                    ...value,
                                    dimensions: dimensionsValue,
                                });
                            }}
                            min={0}
                            helptext={intl.formatMessage({defaultMessage: 'The number of dimensions for the vector embeddings. Common values are 768, 1024, or 1536 depending on the model.'})}
                        />

                        <ChunkingOptionsConfig
                            value={value}
                            onChange={onChange}
                        />
                    </>
                )}

                {!isSearchDisabled && value.type && value.type !== '' && (
                    <ReindexSection
                        jobStatus={jobStatus}
                        statusMessage={statusMessage}
                        onReindexClick={handleReindexClick}
                        onCancelJob={handleCancelJob}
                    />
                )}
            </ItemList>

            <ReindexConfirmation
                show={showReindexConfirmation}
                isFullReindex={pendingReindexParams.fullReindex ?? false}
                lastKPosts={pendingReindexParams.lastKPosts}
                onConfirm={handleConfirmReindex}
                onCancel={handleCancelReindex}
            />
        </Panel>
    );
};

export default EmbeddingSearchPanel;