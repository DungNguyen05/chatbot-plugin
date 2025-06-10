// Copyright (c) 2023-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

import {useState, useEffect, useCallback} from 'react';
import {useIntl} from 'react-intl';

import {doReindexPosts, getReindexStatus, cancelReindex} from '../../../client';

import {JobStatusType, StatusMessageType} from './types';

export const useJobStatus = () => {
    const intl = useIntl();
    const [jobStatus, setJobStatus] = useState<JobStatusType | null>(null);
    const [statusMessage, setStatusMessage] = useState<StatusMessageType>({});
    const [polling, setPolling] = useState(false);
    const [showReindexConfirmation, setShowReindexConfirmation] = useState(false);
    const [pendingReindexParams, setPendingReindexParams] = useState<{lastKPosts?: number, fullReindex?: boolean}>({});

    // Function to fetch job status
    const fetchJobStatus = useCallback(async () => {
        try {
            const status = await getReindexStatus();
            setJobStatus(status);

            // Handle different status conditions
            if (status.status === 'completed') {
                const jobTypeMessage = status.full_reindex ? 
                    intl.formatMessage({defaultMessage: 'Full reindex completed successfully.'}) :
                    intl.formatMessage(
                        {defaultMessage: 'Reindex of last {count} posts completed successfully.'},
                        {count: status.last_k_posts?.toLocaleString() || '0'}
                    );
                
                setStatusMessage({
                    success: true,
                    message: jobTypeMessage,
                });
                setPolling(false);
            } else if (status.status === 'failed') {
                setStatusMessage({
                    success: false,
                    message: intl.formatMessage(
                        {defaultMessage: 'Failed to reindex posts: {error}'},
                        {error: status.error || intl.formatMessage({defaultMessage: 'Unknown error'})},
                    ),
                });
                setPolling(false);
            } else if (status.status === 'canceled') {
                setStatusMessage({
                    success: false,
                    message: intl.formatMessage({defaultMessage: 'Reindexing was canceled.'}),
                });
                setPolling(false);
            }
        } catch (error) {
            // 404 is expected when no job has run yet, don't show an error
            if (error && typeof error === 'object' && 'status_code' in error && error.status_code !== 404) {
                setStatusMessage({
                    success: false,
                    message: intl.formatMessage({defaultMessage: 'Failed to get reindexing status.'}),
                });
            }
            setPolling(false);
        }
    }, [intl]);

    // Polling effect for job status
    useEffect(() => {
        if (polling) {
            const interval = setInterval(() => {
                fetchJobStatus();
            }, 2000); // Poll every 2 seconds

            return () => clearInterval(interval);
        }

        // Return a noop function
        return function noop() { /* No cleanup needed */ };
    }, [polling, fetchJobStatus]);

    // Check status on component mount
    useEffect(() => {
        fetchJobStatus();
    }, [fetchJobStatus]);

    const handleReindexClick = (lastKPosts?: number, fullReindex?: boolean) => {
        setPendingReindexParams({lastKPosts, fullReindex});
        
        // Show confirmation for full reindex or large partial reindex
        if (fullReindex || (lastKPosts && lastKPosts > 10000)) {
            setShowReindexConfirmation(true);
        } else {
            // For small partial reindex, proceed directly
            executeReindex(lastKPosts, fullReindex);
        }
    };

    const executeReindex = async (lastKPosts?: number, fullReindex?: boolean) => {
        setStatusMessage({});

        try {
            const response = await doReindexPosts(lastKPosts, fullReindex);
            setJobStatus(response);
            setPolling(true);
            
            const startMessage = fullReindex ? 
                intl.formatMessage({defaultMessage: 'Full reindex started successfully.'}) :
                intl.formatMessage(
                    {defaultMessage: 'Reindex of last {count} posts started successfully.'},
                    {count: lastKPosts?.toLocaleString() || '0'}
                );
            
            setStatusMessage({
                success: true,
                message: startMessage,
            });
        } catch (error) {
            setStatusMessage({
                success: false,
                message: intl.formatMessage({defaultMessage: 'Failed to start reindexing. Please try again.'}),
            });
        }
    };

    const handleConfirmReindex = async () => {
        setShowReindexConfirmation(false);
        await executeReindex(pendingReindexParams.lastKPosts, pendingReindexParams.fullReindex);
        setPendingReindexParams({});
    };

    const handleCancelReindex = () => {
        setShowReindexConfirmation(false);
        setPendingReindexParams({});
    };

    const handleCancelJob = async () => {
        try {
            const response = await cancelReindex();
            setJobStatus(response);
            setStatusMessage({
                success: false,
                message: intl.formatMessage({defaultMessage: 'Reindexing job canceled.'}),
            });
            setPolling(false);
        } catch (error) {
            setStatusMessage({
                success: false,
                message: intl.formatMessage({defaultMessage: 'Failed to cancel reindexing job.'}),
            });
        }
    };

    return {
        jobStatus,
        statusMessage,
        polling,
        showReindexConfirmation,
        pendingReindexParams,
        handleReindexClick,
        handleConfirmReindex,
        handleCancelReindex,
        handleCancelJob,
    };
};