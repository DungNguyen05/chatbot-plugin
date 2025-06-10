// Copyright (c) 2023-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

import React from 'react';
import {FormattedMessage} from 'react-intl';

import ConfirmationDialog from '../../confirmation_dialog';

interface ReindexConfirmationProps {
    show: boolean;
    isFullReindex: boolean;
    lastKPosts?: number;
    onConfirm: () => void;
    onCancel: () => void;
}

export const ReindexConfirmation = ({show, isFullReindex, lastKPosts, onConfirm, onCancel}: ReindexConfirmationProps) => {
    if (!show) {
        return null;
    }

    return (
        <ConfirmationDialog
            title={<FormattedMessage defaultMessage='Confirm Search Index Creation'/>}
            message={
                <>
                    <p>
                        <FormattedMessage defaultMessage='Are you sure you want to create a fresh search index?'/>
                    </p>
                    <p style={{color: 'var(--error-text)', fontWeight: 'bold', padding: '8px', backgroundColor: 'rgba(var(--error-text-color), 0.08)', borderRadius: '4px', border: '1px solid rgba(var(--error-text-color), 0.16)'}}>
                        ⚠️ <FormattedMessage defaultMessage='WARNING: All existing search data will be permanently deleted and cannot be recovered.'/>
                    </p>
                    <p>
                        <FormattedMessage defaultMessage='This process will:'/>
                    </p>
                    <ul>
                        <li><FormattedMessage defaultMessage='Delete all existing search embeddings'/></li>
                        <li><FormattedMessage defaultMessage='Recreate the vector storage with new dimensions'/></li>
                        <li>
                            {isFullReindex ? (
                                <FormattedMessage defaultMessage='Index all posts in the database'/>
                            ) : (
                                <FormattedMessage 
                                    defaultMessage='Index only the {count} most recent posts'
                                    values={{count: lastKPosts?.toLocaleString() || '0'}}
                                />
                            )}
                        </li>
                        <li><FormattedMessage defaultMessage='Take time depending on the number of posts selected'/></li>
                        <li><FormattedMessage defaultMessage='Increase database load during the processing'/></li>
                    </ul>
                    <p>
                        <strong>
                            {isFullReindex ? (
                                <FormattedMessage defaultMessage='Full indexing: Creates a fresh search index using all posts in the database.'/>
                            ) : (
                                <FormattedMessage 
                                    defaultMessage='Partial indexing: Creates a fresh search index using only the {count} most recent posts.'
                                    values={{count: lastKPosts?.toLocaleString() || '0'}}
                                />
                            )}
                        </strong>
                    </p>
                    {!isFullReindex && (
                        <p style={{fontSize: '12px', color: 'rgba(var(--center-channel-color-rgb), 0.72)'}}>
                            <FormattedMessage defaultMessage='Note: Only recent posts will be searchable. Older posts will not be included in search results.'/>
                        </p>
                    )}
                </>
            }
            confirmButtonText={
                isFullReindex ? (
                    <FormattedMessage defaultMessage='Create Full Index'/>
                ) : (
                    <FormattedMessage 
                        defaultMessage='Create Index with {count} Posts'
                        values={{count: lastKPosts?.toLocaleString() || '0'}}
                    />
                )
            }
            onConfirm={onConfirm}
            onCancel={onCancel}
        />
    );
};