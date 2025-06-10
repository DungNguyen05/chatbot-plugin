// Copyright (c) 2023-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

import React, {useState} from 'react';
import {FormattedMessage} from 'react-intl';
import styled from 'styled-components';

import {PrimaryButton, SecondaryButton} from '../../assets/buttons';

import {HelpText, ItemLabel} from '../item';
import {IntItem} from '../number_items';

import {JobStatusType, StatusMessageType} from './types';

const ButtonContainer = styled.div`
    margin-top: 24px;
    padding-top: 24px;
    border-top: 1px solid rgba(var(--center-channel-color-rgb), 0.08);
    grid-column: 1 / -1;
`;

const ActionContainer = styled.div`
    display: grid;
    grid-template-columns: minmax(auto, 275px) 1fr;
    grid-column-gap: 16px;
`;

const SuccessHelpText = styled(HelpText)`
    margin-top: 8px;
    color: var(--online-indicator);
`;

const ErrorHelpText = styled(HelpText)`
    margin-top: 8px;
    color: var(--error-text);
`;

const ProgressContainer = styled.div`
    margin-top: 8px;
    width: 100%;
    background-color: rgba(var(--center-channel-color-rgb), 0.08);
    border-radius: 4px;
    height: 8px;
    overflow: hidden;
`;

const ProgressBar = styled.div<{progress: number}>`
    height: 100%;
    width: ${(props) => props.progress}%;
    background-color: var(--button-bg);
    transition: width 0.3s ease-in-out;
`;

const ProgressText = styled(HelpText)`
    margin-top: 8px;
    margin-bottom: 12px;
    font-size: 12px;
`;

const ButtonGroup = styled.div`
    display: flex;
    gap: 8px;
`;

const ReindexOptionsContainer = styled.div`
    margin-bottom: 16px;
    padding: 16px;
    border: 1px solid rgba(var(--center-channel-color-rgb), 0.16);
    border-radius: 4px;
    background-color: rgba(var(--center-channel-color-rgb), 0.04);
`;

const RadioGroup = styled.div`
    margin-bottom: 16px;
`;

const RadioOption = styled.label`
    display: flex;
    align-items: center;
    margin-bottom: 8px;
    cursor: pointer;
    
    input[type="radio"] {
        margin-right: 8px;
    }
`;

const StatusBadge = styled.span<{type: 'full' | 'partial'}>`
    display: inline-block;
    padding: 2px 8px;
    margin-left: 8px;
    border-radius: 12px;
    font-size: 11px;
    font-weight: 600;
    text-transform: uppercase;
    color: white;
    background-color: ${props => props.type === 'full' ? 'var(--error-text)' : 'var(--online-indicator)'};
`;

const JobTypeContainer = styled.div`
    display: flex;
    align-items: center;
    margin-bottom: 8px;
`;

const RadioLabel = styled.span`
    font-weight: 500;
    margin-right: 8px;
`;

const RadioDescription = styled.div`
    font-size: 12px;
    color: rgba(var(--center-channel-color-rgb), 0.72);
    margin-left: 20px;
    margin-bottom: 8px;
`;

const PartialReindexOptions = styled.div`
    margin-left: 20px;
    margin-top: 8px;
    padding: 12px;
    background-color: rgba(var(--center-channel-color-rgb), 0.04);
    border-radius: 4px;
    border-left: 3px solid var(--button-bg);
`;

interface ReindexSectionProps {
    jobStatus: JobStatusType | null;
    statusMessage: StatusMessageType;
    onReindexClick: (lastKPosts?: number, fullReindex?: boolean) => void;
    onCancelJob: () => void;
}

export const ReindexSection = ({
    jobStatus,
    statusMessage,
    onReindexClick,
    onCancelJob,
}: ReindexSectionProps) => {
    const [reindexType, setReindexType] = useState<'full' | 'partial'>('partial');
    const [lastKPosts, setLastKPosts] = useState<number>(1000);

    // Check if job is running
    const isReindexing = jobStatus?.status === 'running';

    const handleReindexClick = () => {
        if (reindexType === 'full') {
            onReindexClick(0, true);
        } else {
            onReindexClick(lastKPosts, false);
        }
    };

    const getJobTypeDisplay = () => {
        if (!jobStatus) return null;
        
        if (jobStatus.full_reindex) {
            return <StatusBadge type="full">Full Reindex</StatusBadge>;
        } else if (jobStatus.last_k_posts) {
            return <StatusBadge type="partial">Last {jobStatus.last_k_posts.toLocaleString()} Posts</StatusBadge>;
        }
        
        return null;
    };

    const formatProgressMessage = () => {
        if (!jobStatus) return '';
        
        const processed = jobStatus.processed_rows.toLocaleString();
        const total = jobStatus.total_rows.toLocaleString();
        const percent = jobStatus.total_rows ? Math.floor((jobStatus.processed_rows / jobStatus.total_rows) * 100) : 0;
        
        const baseMessage = `Processing: ${processed} of ${total} posts (${percent}%)`;
        
        if (jobStatus.full_reindex) {
            return `${baseMessage} - Full Reindex`;
        } else if (jobStatus.last_k_posts) {
            return `${baseMessage} - Last ${jobStatus.last_k_posts.toLocaleString()} Posts`;
        }
        
        return baseMessage;
    };

    return (
        <ButtonContainer>
            <ActionContainer>
                <ItemLabel>
                    <FormattedMessage defaultMessage='Reindex Posts'/>
                </ItemLabel>
                <div>
                    {/* Show current job type if running */}
                    {isReindexing && (
                        <JobTypeContainer>
                            <span>Running:</span>
                            {getJobTypeDisplay()}
                        </JobTypeContainer>
                    )}

                    {/* Show reindex options only when not running */}
                    {!isReindexing && (
                        <ReindexOptionsContainer>
                            <RadioGroup>
                                <div>
                                    <RadioOption>
                                        <input
                                            type="radio"
                                            name="reindexType"
                                            value="partial"
                                            checked={reindexType === 'partial'}
                                            onChange={() => setReindexType('partial')}
                                        />
                                        <RadioLabel>
                                            <FormattedMessage defaultMessage='Reindex Recent Posts Only'/>
                                        </RadioLabel>
                                        <StatusBadge type="partial">Recommended</StatusBadge>
                                    </RadioOption>
                                    <RadioDescription>
                                        <FormattedMessage defaultMessage='Faster option that reindexes only the most recent posts. Good for incremental updates and when you want to index new content without rebuilding everything.'/>
                                    </RadioDescription>
                                </div>
                                
                                <div>
                                    <RadioOption>
                                        <input
                                            type="radio"
                                            name="reindexType"
                                            value="full"
                                            checked={reindexType === 'full'}
                                            onChange={() => setReindexType('full')}
                                        />
                                        <RadioLabel>
                                            <FormattedMessage defaultMessage='Full Reindex (All Posts)'/>
                                        </RadioLabel>
                                        <StatusBadge type="full">Slower</StatusBadge>
                                    </RadioOption>
                                    <RadioDescription>
                                        <FormattedMessage defaultMessage='Complete rebuild of the entire search index. This will clear all existing embeddings and recreate the vector storage. Use when changing dimensions or doing a fresh start.'/>
                                    </RadioDescription>
                                </div>
                            </RadioGroup>

                            {reindexType === 'partial' && (
                                <PartialReindexOptions>
                                    <IntItem
                                        label="Number of Recent Posts"
                                        placeholder="1000"
                                        value={lastKPosts}
                                        onChange={setLastKPosts}
                                        min={1}
                                        max={100000}
                                        helptext="Enter the number of most recent posts to reindex. Recommended values: 1000 for daily updates, 10000 for weekly updates, 50000+ for major updates."
                                    />
                                    <HelpText style={{marginTop: '8px', fontSize: '11px'}}>
                                        <FormattedMessage 
                                            defaultMessage='Estimated time: ~{time} minutes for {count} posts'
                                            values={{
                                                time: Math.ceil(lastKPosts / 1000 * 2), // Rough estimate: 2 minutes per 1000 posts
                                                count: lastKPosts.toLocaleString()
                                            }}
                                        />
                                    </HelpText>
                                </PartialReindexOptions>
                            )}

                            {reindexType === 'full' && (
                                <div style={{marginLeft: '20px', marginTop: '8px'}}>
                                    <HelpText style={{fontSize: '11px', color: 'var(--error-text)'}}>
                                        <FormattedMessage defaultMessage='⚠️ Warning: Full reindex will recreate the vector storage and may take several hours for large installations.'/>
                                    </HelpText>
                                </div>
                            )}
                        </ReindexOptionsContainer>
                    )}

                    {/* Show different UI based on job status */}
                    {isReindexing ? (
                        <>
                            <ButtonGroup>
                                <SecondaryButton onClick={onCancelJob}>
                                    <FormattedMessage defaultMessage='Cancel Reindexing'/>
                                </SecondaryButton>
                            </ButtonGroup>

                            {jobStatus && (
                                <>
                                    <ProgressText>
                                        {formatProgressMessage()}
                                    </ProgressText>
                                    <ProgressContainer>
                                        <ProgressBar
                                            progress={jobStatus.total_rows ? Math.min((jobStatus.processed_rows / jobStatus.total_rows) * 100, 100) : 0}
                                        />
                                    </ProgressContainer>
                                </>
                            )}
                        </>
                    ) : (
                        <PrimaryButton onClick={handleReindexClick}>
                            {reindexType === 'full' ? (
                                <FormattedMessage defaultMessage='Start Full Reindex'/>
                            ) : (
                                <FormattedMessage 
                                    defaultMessage='Reindex Last {count} Posts'
                                    values={{count: lastKPosts.toLocaleString()}}
                                />
                            )}
                        </PrimaryButton>
                    )}

                    {statusMessage.message && (
                        statusMessage.success ? (
                            <SuccessHelpText>
                                {statusMessage.message}
                            </SuccessHelpText>
                        ) : (
                            <ErrorHelpText>
                                {statusMessage.message}
                            </ErrorHelpText>
                        )
                    )}

                    <HelpText>
                        <FormattedMessage defaultMessage='Choose between reindexing recent posts only (faster, good for incremental updates) or all posts (slower, rebuilds entire index). Partial reindex only processes the most recent posts and is much more efficient for regular maintenance.'/>
                    </HelpText>
                </div>
            </ActionContainer>
        </ButtonContainer>
    );
};