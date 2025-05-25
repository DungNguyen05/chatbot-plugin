import React from 'react';
import {Modal} from 'react-bootstrap';
import {FormattedMessage} from 'react-intl';

import RollCallInterface from './roll_call_interface';

interface RollCallModalProps {
    show: boolean;
    onHide: () => void;
}

const RollCallModal: React.FC<RollCallModalProps> = ({show, onHide}) => {
    return (
        <Modal
            show={show}
            onHide={onHide}
            centered={true}
            backdrop='static'
            keyboard={false}
            size="lg"
        >
            <Modal.Header closeButton={true}>
                <Modal.Title>
                    <FormattedMessage defaultMessage='Roll Call'/>
                </Modal.Title>
            </Modal.Header>
            <Modal.Body style={{padding: 0}}>
                <RollCallInterface onClose={onHide}/>
            </Modal.Body>
        </Modal>
    );
};

export default RollCallModal;