import { render, screen, getAllByRole, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import React from 'react';
import { getSelectParent } from 'test/helpers/selectOptionInTest';

import { dateTime } from '@grafana/data';

import { createLokiDatasource } from '../../__mocks__/datasource';
import { LokiOperationId, LokiVisualQuery } from '../types';

import { LokiQueryBuilder } from './LokiQueryBuilder';
import { EXPLAIN_LABEL_FILTER_CONTENT } from './LokiQueryBuilderExplained';

const MISSING_LABEL_FILTER_ERROR_MESSAGE = 'Select at least 1 label filter (label and value)';
const defaultQuery: LokiVisualQuery = {
  labels: [{ op: '=', label: 'baz', value: 'bar' }],
  operations: [],
};

const mockTimeRange = {
  from: dateTime(1546372800000),
  to: dateTime(1546380000000),
  raw: {
    from: dateTime(1546372800000),
    to: dateTime(1546380000000),
  },
};

const createDefaultProps = () => {
  const datasource = createLokiDatasource();

  const props = {
    datasource,
    onRunQuery: () => {},
    onChange: () => {},
    showExplain: false,
    timeRange: mockTimeRange,
  };

  return props;
};

describe('LokiQueryBuilder', () => {
  it('tries to load labels when no labels are selected', async () => {
    const props = createDefaultProps();
    props.datasource.getDataSamples = jest.fn().mockResolvedValue([]);
    props.datasource.languageProvider.fetchSeriesLabels = jest.fn().mockReturnValue({ job: ['a'], instance: ['b'] });

    render(<LokiQueryBuilder {...props} query={defaultQuery} />);
    await userEvent.click(screen.getByLabelText('Add'));
    const labels = screen.getByText(/Label filters/);
    const selects = getAllByRole(getSelectParent(labels)!, 'combobox');
    await userEvent.click(selects[3]);
    expect(props.datasource.languageProvider.fetchSeriesLabels).toBeCalledWith('{baz="bar"}', {
      timeRange: mockTimeRange,
    });
    await waitFor(() => expect(screen.getByText('job')).toBeInTheDocument());
  });

  it('does refetch label values with the correct timerange', async () => {
    const props = createDefaultProps();
    props.datasource.getDataSamples = jest.fn().mockResolvedValue([]);
    props.datasource.languageProvider.fetchSeriesLabels = jest
      .fn()
      .mockReturnValue({ job: ['a'], instance: ['b'], baz: ['bar'] });

    render(<LokiQueryBuilder {...props} query={defaultQuery} />);
    await userEvent.click(screen.getByLabelText('Add'));
    const labels = screen.getByText(/Label filters/);
    const selects = getAllByRole(getSelectParent(labels)!, 'combobox');
    await userEvent.click(selects[3]);
    await waitFor(() => expect(screen.getByText('job')).toBeInTheDocument());
    await userEvent.click(screen.getByText('job'));
    await userEvent.click(selects[5]);
    expect(props.datasource.languageProvider.fetchSeriesLabels).toHaveBeenNthCalledWith(2, '{baz="bar"}', {
      timeRange: mockTimeRange,
    });
  });

  it('does not show already existing label names as option in label filter', async () => {
    const props = createDefaultProps();
    props.datasource.getDataSamples = jest.fn().mockResolvedValue([]);
    props.datasource.languageProvider.fetchSeriesLabels = jest
      .fn()
      .mockReturnValue({ job: ['a'], instance: ['b'], baz: ['bar'] });

    render(<LokiQueryBuilder {...props} query={defaultQuery} />);
    await userEvent.click(screen.getByLabelText('Add'));
    const labels = screen.getByText(/Label filters/);
    const selects = getAllByRole(getSelectParent(labels)!, 'combobox');
    await userEvent.click(selects[3]);
    await waitFor(() => expect(screen.getByText('job')).toBeInTheDocument());
    await waitFor(() => expect(screen.getByText('instance')).toBeInTheDocument());
    await waitFor(() => expect(screen.getAllByText('baz')).toHaveLength(1));
  });

  it('shows error for query with operations and no stream selector', async () => {
    const query = { labels: [], operations: [{ id: LokiOperationId.Logfmt, params: [] }] };
    render(<LokiQueryBuilder {...createDefaultProps()} query={query} />);

    expect(await screen.findByText(MISSING_LABEL_FILTER_ERROR_MESSAGE)).toBeInTheDocument();
  });

  it('shows no error for query with empty __line_contains operation and no stream selector', async () => {
    const query = { labels: [], operations: [{ id: LokiOperationId.LineContains, params: [''] }] };
    render(<LokiQueryBuilder {...createDefaultProps()} query={query} />);

    await waitFor(() => {
      expect(screen.queryByText(MISSING_LABEL_FILTER_ERROR_MESSAGE)).not.toBeInTheDocument();
    });
  });
  it('shows explain section when showExplain is true', async () => {
    const query = {
      labels: [{ label: 'foo', op: '=', value: 'bar' }],
      operations: [{ id: LokiOperationId.LineContains, params: ['error'] }],
    };
    const props = createDefaultProps();
    props.showExplain = true;
    props.datasource.getDataSamples = jest.fn().mockResolvedValue([]);

    render(<LokiQueryBuilder {...props} query={query} />);
    expect(await screen.findByText(EXPLAIN_LABEL_FILTER_CONTENT)).toBeInTheDocument();
  });

  it('does not shows explain section when showExplain is false', async () => {
    const query = {
      labels: [{ label: 'foo', op: '=', value: 'bar' }],
      operations: [{ id: LokiOperationId.LineContains, params: ['error'] }],
    };
    const props = createDefaultProps();
    props.datasource.getDataSamples = jest.fn().mockResolvedValue([]);

    render(<LokiQueryBuilder {...props} query={query} />);
    await waitFor(() => {
      expect(screen.queryByText(EXPLAIN_LABEL_FILTER_CONTENT)).not.toBeInTheDocument();
    });
  });
});
