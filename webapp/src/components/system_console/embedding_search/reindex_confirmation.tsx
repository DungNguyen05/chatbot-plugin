// Copyright (c) 2023-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

import React from 'react';
import {FormattedMessage} from 'react-intl';

import ConfirmationDialog from '../../confirmation_dialog';

interface ReindexConfirmationProps {
    show: boolean;
    onConfirm: () => void;
    onCancel: () => void;
}

export const ReindexConfirmation = ({show, onConfirm, onCancel}: ReindexConfirmationProps) => {
    if (!show) {
        return null;
    }

    return (
        <ConfirmationDialog
            title={<FormattedMessage defaultMessage='Confirm Reindexing'/>}
            message={
                <>
                    <p>
                        <FormattedMessage defaultMessage='Are you sure you want to start the reindexing process?'/>
                    </p>
                    <p>
                        <FormattedMessage defaultMessage='This process will:'/>
                    </p>
                    <ul>
                        <li><FormattedMessage defaultMessage='Index the selected posts in the database'/></li>
                        <li><FormattedMessage defaultMessage='Take time depending on the number of posts selected'/></li>
                        <li><FormattedMessage defaultMessage='Increase database load during the reindexing process'/></li>
                    </ul>
                    <p>
                        <strong>
                            <FormattedMessage defaultMessage='For partial reindexing: Only the most recent posts will be processed and existing embeddings for those posts will be updated.'/>
                        </strong>
                    </p>
                    <p>
                        <strong>
                            <FormattedMessage defaultMessage='For full reindexing: The entire search index will be cleared and rebuilt from scratch.'/>
                        </strong>
                    </p>
                </>
            }
            confirmButtonText={<FormattedMessage defaultMessage='Start Reindexing'/>}
            onConfirm={onConfirm}
            onCancel={onCancel}
        />
    );
};