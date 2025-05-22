// Copyright (c) 2023-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

import React from 'react';
import {useIntl} from 'react-intl';
import styled from 'styled-components';

import {SelectChannel} from '../select';

import {BooleanItem, ItemList, TextItem} from './item';

const Horizontal = styled.div`
    display: flex;
    flex-direction: row;
    align-items: center;
    gap: 8px;
`;

const SelectWrapper = styled.div`
    margin-top: 8px;
    width: 90%;
`;

export type RollCallConfig = {
    enabled: boolean;
    erpDomain: string;
    erpAPIKey: string;
    erpAPISecret: string;
    notifyChannels: string[];
};


type Props = {
    value: RollCallConfig;
    onChange: (config: RollCallConfig) => void;
};

const RollCallConfigComponent = (props: Props) => {
    const intl = useIntl();

    return (
        <ItemList>
            <BooleanItem
                label={intl.formatMessage({defaultMessage: 'Enable Roll Call'})}
                value={props.value.enabled}
                onChange={(enabled: boolean) => props.onChange({...props.value, enabled})}
                helpText={intl.formatMessage({defaultMessage: 'Enable automatic roll call notifications when users check in/out.'})}
            />
            
            {props.value.enabled && (
                <>
                    <TextItem
                        label={intl.formatMessage({defaultMessage: 'ERP Domain'})}
                        value={props.value.erpDomain || ''}
                        onChange={(e) => props.onChange({...props.value, erpDomain: e.target.value})}
                        helptext={intl.formatMessage({defaultMessage: 'The ERP system domain (e.g., https://example.erp.com)'})}
                        placeholder="https://example.erp.com"
                    />
                    
                    <TextItem
                        label={intl.formatMessage({defaultMessage: 'ERP API Key'})}
                        type="password"
                        value={props.value.erpAPIKey || ''}
                        onChange={(e) => props.onChange({...props.value, erpAPIKey: e.target.value})}
                        helptext={intl.formatMessage({defaultMessage: 'API key for the ERP system'})}
                        placeholder="your_api_key"
                    />

                    <TextItem
                        label={intl.formatMessage({defaultMessage: 'ERP API Secret'})}
                        type="password"
                        value={props.value.erpAPISecret || ''}
                        onChange={(e) => props.onChange({...props.value, erpAPISecret: e.target.value})}
                        helptext={intl.formatMessage({defaultMessage: 'API secret for the ERP system'})}
                        placeholder="your_api_secret"
                    />

                    <div>
                        <div style={{marginBottom: '8px'}}>
                            <strong>{intl.formatMessage({defaultMessage: 'Notification Channels'})}</strong>
                        </div>
                        <SelectWrapper>
                            <SelectChannel
                                channelIDs={props.value.notifyChannels || []}
                                onChangeChannelIDs={(channels: string[]) => 
                                    props.onChange({...props.value, notifyChannels: channels})
                                }
                            />
                            <div style={{
                                fontSize: '12px',
                                fontWeight: 400,
                                lineHeight: '16px',
                                color: 'rgba(var(--center-channel-color-rgb), 0.72)',
                                marginTop: '8px'
                            }}>
                                {intl.formatMessage({defaultMessage: 'Select channels where roll call notifications will be posted'})}
                            </div>
                        </SelectWrapper>
                    </div>
                </>
            )}
        </ItemList>
    );
};

export default RollCallConfigComponent;