/*
Copyright (C) 2025 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

import React, { useRef } from 'react';
import { Form, Button } from '@douyinfe/semi-ui';
import { RotateCcw, Search } from 'lucide-react';

const TokensFilters = ({
  formInitValues,
  setFormApi,
  searchTokens,
  loading,
  searching,
  t,
}) => {
  // Handle form reset and immediate search
  const formApiRef = useRef(null);

  const handleReset = () => {
    if (!formApiRef.current) return;
    formApiRef.current.reset();
    setTimeout(() => {
      searchTokens();
    }, 100);
  };

  return (
    <Form
      initValues={formInitValues}
      getFormApi={(api) => {
        setFormApi(api);
        formApiRef.current = api;
      }}
      onSubmit={() => searchTokens(1)}
      allowEmpty={true}
      autoComplete='off'
      layout='horizontal'
      trigger='change'
      stopValidateWithError={false}
      className='ct-token-filter-form'
    >
      <div className='ct-token-filters'>
        <div className='ct-token-filter-field'>
          <Form.Input
            field='searchKeyword'
            prefix={<Search size={15} />}
            placeholder={t('搜索关键字')}
            showClear
            pure
            size='small'
          />
        </div>

        <div className='ct-token-filter-field'>
          <Form.Input
            field='searchToken'
            prefix={<Search size={15} />}
            placeholder={t('密钥')}
            showClear
            pure
            size='small'
          />
        </div>

        <div className='ct-token-filter-buttons'>
          <Button
            type='primary'
            theme='light'
            icon={<Search size={15} />}
            htmlType='submit'
            loading={loading || searching}
            className='ct-token-filter-submit'
            size='small'
          >
            {t('查询')}
          </Button>

          <Button
            type='tertiary'
            theme='borderless'
            icon={<RotateCcw size={15} />}
            onClick={handleReset}
            className='ct-token-filter-reset'
            size='small'
          >
            {t('重置')}
          </Button>
        </div>
      </div>
    </Form>
  );
};

export default TokensFilters;
