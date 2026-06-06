import React from 'react';
import ChannelsTable from '../../../components/table/channels';
import '../../../pages/Channel/channel.css';

const AdminChannels = () => {
  return (
    <div className='ct-channel-page ct-admin-channel-page'>
      <ChannelsTable variant='admin' />
    </div>
  );
};

export default AdminChannels;
