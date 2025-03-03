import { css, cx } from '@emotion/css';
import React from 'react';

import { GrafanaTheme2 } from '@grafana/data';
import { Button, ModalsController, CollapsableSection, HorizontalGroup, useStyles2 } from '@grafana/ui';
import { DiffGroup } from 'app/features/dashboard-scene/settings/version-history/DiffGroup';
import { DiffViewer } from 'app/features/dashboard-scene/settings/version-history/DiffViewer';
import { jsonDiff } from 'app/features/dashboard-scene/settings/version-history/utils';

import { DecoratedRevisionModel } from '../DashboardSettings/VersionsSettings';

import { RevertDashboardModal } from './RevertDashboardModal';

type DiffViewProps = {
  isNewLatest: boolean;
  newInfo: DecoratedRevisionModel;
  baseInfo: DecoratedRevisionModel;
  diffData: { lhs: string; rhs: string };
};

export const VersionHistoryComparison = ({ baseInfo, newInfo, diffData, isNewLatest }: DiffViewProps) => {
  const diff = jsonDiff(diffData.lhs, diffData.rhs);
  const styles = useStyles2(getStyles);

  return (
    <div>
      <div className={styles.spacer}>
        <HorizontalGroup justify="space-between" align="center">
          <div>
            <p className={styles.versionInfo}>
              <strong>Version {newInfo.version}</strong> updated by {newInfo.createdBy} {newInfo.ageString} -{' '}
              {newInfo.message}
            </p>
            <p className={cx(styles.versionInfo, styles.noMarginBottom)}>
              <strong>Version {baseInfo.version}</strong> updated by {baseInfo.createdBy} {baseInfo.ageString} -{' '}
              {baseInfo.message}
            </p>
          </div>
          {isNewLatest && (
            <ModalsController>
              {({ showModal, hideModal }) => (
                <Button
                  variant="destructive"
                  icon="history"
                  onClick={() => {
                    showModal(RevertDashboardModal, {
                      version: baseInfo.version,
                      hideModal,
                    });
                  }}
                >
                  Restore to version {baseInfo.version}
                </Button>
              )}
            </ModalsController>
          )}
        </HorizontalGroup>
      </div>
      <div className={styles.spacer}>
        {Object.entries(diff).map(([key, diffs]) => (
          <DiffGroup diffs={diffs} key={key} title={key} />
        ))}
      </div>
      <CollapsableSection isOpen={false} label="View JSON Diff">
        <DiffViewer oldValue={JSON.stringify(diffData.lhs, null, 2)} newValue={JSON.stringify(diffData.rhs, null, 2)} />
      </CollapsableSection>
    </div>
  );
};

const getStyles = (theme: GrafanaTheme2) => ({
  spacer: css({
    marginBottom: theme.spacing(4),
  }),
  versionInfo: css({
    color: theme.colors.text.secondary,
    fontSize: theme.typography.bodySmall.fontSize,
  }),
  noMarginBottom: css({
    marginBottom: 0,
  }),
});
