import React, { useEffect, useMemo, useState } from 'react';

import { DataSourceApi, getDefaultTimeRange, LoadingState, PanelData, SelectableValue, TimeRange } from '@grafana/data';
import {
  EditorRow,
  // LabelFilters, this is broken in @grafana/experimental so we need to use the one from prometheus
  OperationExplainedBox,
  OperationList,
  OperationListExplained,
  OperationsEditorRow,
  QueryBuilderHints,
  RawQuery,
  QueryBuilderLabelFilter,
  QueryBuilderOperation,
} from '@grafana/experimental';
import { config } from '@grafana/runtime';
import { LabelFilters } from 'app/plugins/datasource/prometheus/querybuilder/shared/LabelFilters';

import { testIds } from '../../components/LokiQueryEditor';
import { LokiDatasource } from '../../datasource';
import { escapeLabelValueInSelector } from '../../languageUtils';
import logqlGrammar from '../../syntax';
import { LokiQuery } from '../../types';
import { lokiQueryModeller } from '../LokiQueryModeller';
import { isConflictingFilter } from '../operationUtils';
import { buildVisualQueryFromString } from '../parsing';
import { LokiOperationId, LokiVisualQuery } from '../types';

import { EXPLAIN_LABEL_FILTER_CONTENT } from './LokiQueryBuilderExplained';
import { NestedQueryList } from './NestedQueryList';

export interface Props {
  query: LokiVisualQuery;
  datasource: LokiDatasource;
  showExplain: boolean;
  timeRange?: TimeRange;
  onChange: (update: LokiVisualQuery) => void;
  onRunQuery: () => void;
}
export const LokiQueryBuilder = React.memo<Props>(
  ({ datasource, query, onChange, onRunQuery, showExplain, timeRange }) => {
    const [sampleData, setSampleData] = useState<PanelData>();
    const [highlightedOp, setHighlightedOp] = useState<QueryBuilderOperation | undefined>(undefined);

    const onChangeLabels = (labels: QueryBuilderLabelFilter[]) => {
      onChange({ ...query, labels });
    };

    const withTemplateVariableOptions = async (optionsPromise: Promise<string[]>): Promise<SelectableValue[]> => {
      const options = await optionsPromise;
      return [...datasource.getVariables(), ...options].map((value) => ({ label: value, value }));
    };

    const onGetLabelNames = async (forLabel: Partial<QueryBuilderLabelFilter>): Promise<string[]> => {
      const labelsToConsider = query.labels.filter((x) => x !== forLabel);

      if (labelsToConsider.length === 0) {
        return await datasource.languageProvider.fetchLabels({ timeRange });
      }

      const expr = lokiQueryModeller.renderLabels(labelsToConsider);
      const series = await datasource.languageProvider.fetchSeriesLabels(expr, { timeRange });
      const labelsNamesToConsider = labelsToConsider.map((l) => l.label);

      const labelNames = Object.keys(series)
        // Filter out label names that are already selected
        .filter((name) => !labelsNamesToConsider.includes(name))
        .sort();

      return labelNames;
    };

    const onGetLabelValues = async (forLabel: Partial<QueryBuilderLabelFilter>) => {
      if (!forLabel.label) {
        return [];
      }

      let values;
      const labelsToConsider = query.labels.filter((x) => x !== forLabel);
      if (labelsToConsider.length === 0) {
        values = await datasource.languageProvider.fetchLabelValues(forLabel.label, { timeRange });
      } else {
        const expr = lokiQueryModeller.renderLabels(labelsToConsider);
        const result = await datasource.languageProvider.fetchSeriesLabels(expr, { timeRange });
        values = result[datasource.interpolateString(forLabel.label)];
      }

      return values ? values.map((v) => escapeLabelValueInSelector(v, forLabel.op)) : []; // Escape values in return
    };

    const labelFilterRequired: boolean = useMemo(() => {
      const { labels, operations: op } = query;
      if (!labels.length && op.length) {
        // Filter is required when operations are present (empty line contains operation is exception)
        if (op.length === 1 && op[0].id === LokiOperationId.LineContains && op[0].params[0] === '') {
          return false;
        }
        return true;
      }
      return false;
    }, [query]);

    useEffect(() => {
      const onGetSampleData = async () => {
        const lokiQuery = { expr: lokiQueryModeller.renderQuery(query), refId: 'data-samples' };
        const range = timeRange ?? getDefaultTimeRange();
        const series = await datasource.getDataSamples(lokiQuery, range);
        const sampleData = { series, state: LoadingState.Done, timeRange: range };
        setSampleData(sampleData);
      };

      if (config.featureToggles.lokiQueryHints) {
        onGetSampleData().catch(console.error);
      }
    }, [datasource, query, timeRange]);

    const lang = { grammar: logqlGrammar, name: 'logql' };
    return (
      <div data-testid={testIds.editor}>
        <EditorRow>
          <LabelFilters
            onGetLabelNames={(forLabel: Partial<QueryBuilderLabelFilter>) =>
              withTemplateVariableOptions(onGetLabelNames(forLabel))
            }
            onGetLabelValues={(forLabel: Partial<QueryBuilderLabelFilter>) =>
              withTemplateVariableOptions(onGetLabelValues(forLabel))
            }
            labelsFilters={query.labels}
            onChange={onChangeLabels}
            labelFilterRequired={labelFilterRequired}
          />
        </EditorRow>
        {showExplain && (
          <OperationExplainedBox
            stepNumber={1}
            title={<RawQuery query={`${lokiQueryModeller.renderLabels(query.labels)}`} language={lang} />}
          >
            {EXPLAIN_LABEL_FILTER_CONTENT}
          </OperationExplainedBox>
        )}
        <OperationsEditorRow>
          <OperationList
            queryModeller={lokiQueryModeller}
            query={query}
            onChange={onChange}
            onRunQuery={onRunQuery}
            datasource={datasource as DataSourceApi}
            highlightedOp={highlightedOp}
            isConflictingOperation={(operation: QueryBuilderOperation, otherOperations: QueryBuilderOperation[]) =>
              operation.id === LokiOperationId.LabelFilter && isConflictingFilter(operation, otherOperations)
            }
          />
          <QueryBuilderHints<LokiVisualQuery, LokiQuery>
            datasource={datasource}
            query={query}
            onChange={onChange}
            data={sampleData}
            queryModeller={lokiQueryModeller}
            buildVisualQueryFromString={buildVisualQueryFromString}
            buildDataQueryFromQueryString={(queryString) => ({ expr: queryString, refId: 'hints' })}
            buildQueryStringFromDataQuery={(query) => query.expr}
          />
        </OperationsEditorRow>
        {showExplain && (
          <OperationListExplained<LokiVisualQuery>
            stepNumber={2}
            queryModeller={lokiQueryModeller}
            query={query}
            language={lang}
            onMouseEnter={(op) => {
              setHighlightedOp(op);
            }}
            onMouseLeave={() => {
              setHighlightedOp(undefined);
            }}
          />
        )}
        {query.binaryQueries && query.binaryQueries.length > 0 && (
          <NestedQueryList
            query={query}
            datasource={datasource}
            onChange={onChange}
            onRunQuery={onRunQuery}
            showExplain={showExplain}
          />
        )}
      </div>
    );
  }
);

LokiQueryBuilder.displayName = 'LokiQueryBuilder';
