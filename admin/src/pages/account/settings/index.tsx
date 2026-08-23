import { GridContent } from '@ant-design/pro-components';
import React from 'react';
import BaseView from './components/base';
import useStyles from './style.style';

const Settings: React.FC = () => {
  const { styles } = useStyles();
  return (
    <GridContent>
      <div className={styles.main}>
        <div className={styles.right}>
          <div className={styles.title}>基本设置</div>
          <BaseView />
        </div>
      </div>
    </GridContent>
  );
};
export default Settings;
