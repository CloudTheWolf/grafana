import { getPanelPlugin } from '@grafana/data/test/__mocks__/pluginMocks';
import { PanelModel } from 'app/features/dashboard/state';

import { getAngularPanelMigrationHandler } from './angularMigration';

describe('getAngularPanelMigrationHandler', () => {
  describe('Given an old angular panel', () => {
    it('Should call migration handler', () => {
      const onPanelTypeChanged = (panel: PanelModel, prevPluginId: string, prevOptions: Record<string, any>) => {
        panel.fieldConfig = { defaults: { unit: 'bytes' }, overrides: [] };
        return { name: prevOptions.angular.oldOptionProp };
      };

      const reactPlugin = getPanelPlugin({ id: 'geomap' }).setPanelChangeHandler(onPanelTypeChanged as any);

      const oldModel = new PanelModel({
        autoMigrateFrom: 'grafana-worldmap-panel',
        oldOptionProp: 'old name',
        type: 'geomap',
      });

      const mutatedModel = {
        id: 1,
        type: 'geomap',
        options: {},
        fieldConfig: { defaults: {}, overrides: [] },
      };

      getAngularPanelMigrationHandler(oldModel)(mutatedModel, reactPlugin);

      expect(mutatedModel.options).toEqual({ name: 'old name' });
      expect(mutatedModel.fieldConfig).toEqual({ defaults: { unit: 'bytes' }, overrides: [] });
    });
  });
});
