import React, { useCallback, useMemo, useRef, useState } from 'react';

import { CartesianCoords2D, DashboardCursorSync, DataFrame, FieldType, PanelProps } from '@grafana/data';
import { config } from '@grafana/runtime';
import {
  Portal,
  TooltipDisplayMode,
  TooltipPlugin2,
  UPlotConfigBuilder,
  usePanelContext,
  useTheme2,
  VizTooltipContainer,
  ZoomPlugin,
} from '@grafana/ui';
import { addTooltipSupport, HoverEvent } from '@grafana/ui/src/components/uPlot/config/addTooltipSupport';
import { TimeRange2, TooltipHoverMode } from '@grafana/ui/src/components/uPlot/plugins/TooltipPlugin2';
import { CloseButton } from 'app/core/components/CloseButton/CloseButton';
import { TimelineChart } from 'app/core/components/TimelineChart/TimelineChart';
import {
  prepareTimelineFields,
  prepareTimelineLegendItems,
  TimelineMode,
} from 'app/core/components/TimelineChart/utils';

import { AnnotationsPlugin } from '../timeseries/plugins/AnnotationsPlugin';
import { AnnotationsPlugin2 } from '../timeseries/plugins/AnnotationsPlugin2';
import { OutsideRangePlugin } from '../timeseries/plugins/OutsideRangePlugin';
import { getTimezones } from '../timeseries/utils';

import { StatusHistoryTooltip } from './StatusHistoryTooltip';
import { StatusHistoryTooltip2 } from './StatusHistoryTooltip2';
import { Options } from './panelcfg.gen';

const TOOLTIP_OFFSET = 10;

interface TimelinePanelProps extends PanelProps<Options> {}

/**
 * @alpha
 */
export const StatusHistoryPanel = ({
  data,
  timeRange,
  timeZone,
  options,
  width,
  height,
  onChangeTimeRange,
}: TimelinePanelProps) => {
  const theme = useTheme2();

  const oldConfig = useRef<UPlotConfigBuilder | undefined>(undefined);
  const isToolTipOpen = useRef<boolean>(false);

  const [hover, setHover] = useState<HoverEvent | undefined>(undefined);
  const [coords, setCoords] = useState<{ viewport: CartesianCoords2D; canvas: CartesianCoords2D } | null>(null);
  const [focusedSeriesIdx, setFocusedSeriesIdx] = useState<number | null>(null);
  const [focusedPointIdx, setFocusedPointIdx] = useState<number | null>(null);
  const [isActive, setIsActive] = useState<boolean>(false);
  const [shouldDisplayCloseButton, setShouldDisplayCloseButton] = useState<boolean>(false);
  // temp range set for adding new annotation set by TooltipPlugin2, consumed by AnnotationPlugin2
  const [newAnnotationRange, setNewAnnotationRange] = useState<TimeRange2 | null>(null);
  const { sync, canAddAnnotations } = usePanelContext();

  const enableAnnotationCreation = Boolean(canAddAnnotations && canAddAnnotations());

  const onCloseToolTip = () => {
    isToolTipOpen.current = false;
    setCoords(null);
    setShouldDisplayCloseButton(false);
  };

  const onUPlotClick = () => {
    isToolTipOpen.current = !isToolTipOpen.current;

    // Linking into useState required to re-render tooltip
    setShouldDisplayCloseButton(isToolTipOpen.current);
  };

  const { frames, warn } = useMemo(
    () => prepareTimelineFields(data.series, false, timeRange, theme),
    [data.series, timeRange, theme]
  );

  const legendItems = useMemo(
    () => prepareTimelineLegendItems(frames, options.legend, theme),
    [frames, options.legend, theme]
  );

  const renderCustomTooltip = useCallback(
    (alignedData: DataFrame, seriesIdx: number | null, datapointIdx: number | null) => {
      const data = frames ?? [];

      // Count value fields in the state-timeline-ready frame
      const valueFieldsCount = data.reduce(
        (acc, frame) => acc + frame.fields.filter((field) => field.type !== FieldType.time).length,
        0
      );

      // Not caring about multi mode in StatusHistory
      if (seriesIdx === null || datapointIdx === null) {
        return null;
      }

      /**
       * There could be a case when the tooltip shows a data from one of a multiple query and the other query finishes first
       * from refreshing. This causes data to be out of sync. alignedData - 1 because Time field doesn't count.
       * Render nothing in this case to prevent error.
       * See https://github.com/grafana/support-escalations/issues/932
       */
      if (
        (!alignedData.meta?.transformations?.length && alignedData.fields.length - 1 !== valueFieldsCount) ||
        !alignedData.fields[seriesIdx]
      ) {
        return null;
      }

      return (
        <>
          {shouldDisplayCloseButton && (
            <div
              style={{
                width: '100%',
                display: 'flex',
                justifyContent: 'flex-end',
              }}
            >
              <CloseButton
                onClick={onCloseToolTip}
                style={{
                  position: 'relative',
                  top: 'auto',
                  right: 'auto',
                  marginRight: 0,
                }}
              />
            </div>
          )}
          <StatusHistoryTooltip
            data={data}
            alignedData={alignedData}
            seriesIdx={seriesIdx}
            datapointIdx={datapointIdx}
            timeZone={timeZone}
          />
        </>
      );
    },
    [timeZone, frames, shouldDisplayCloseButton]
  );

  const renderTooltip = (alignedFrame: DataFrame) => {
    if (options.tooltip.mode === TooltipDisplayMode.None) {
      return null;
    }

    if (focusedPointIdx === null || (!isActive && sync && sync() === DashboardCursorSync.Crosshair)) {
      return null;
    }

    return (
      <Portal>
        {hover && coords && focusedSeriesIdx && (
          <VizTooltipContainer
            position={{ x: coords.viewport.x, y: coords.viewport.y }}
            offset={{ x: TOOLTIP_OFFSET, y: TOOLTIP_OFFSET }}
            allowPointerEvents={isToolTipOpen.current}
          >
            {renderCustomTooltip(alignedFrame, focusedSeriesIdx, focusedPointIdx)}
          </VizTooltipContainer>
        )}
      </Portal>
    );
  };

  const timezones = useMemo(() => getTimezones(options.timezone, timeZone), [options.timezone, timeZone]);

  if (!frames || warn) {
    return (
      <div className="panel-empty">
        <p>{warn ?? 'No data found in response'}</p>
      </div>
    );
  }

  // Status grid requires some space between values
  if (frames[0].length > width / 2) {
    return (
      <div className="panel-empty">
        <p>
          Too many points to visualize properly. <br />
          Update the query to return fewer points. <br />({frames[0].length} points received)
        </p>
      </div>
    );
  }

  const showNewVizTooltips =
    config.featureToggles.newVizTooltips && (sync == null || sync() !== DashboardCursorSync.Tooltip);

  return (
    <TimelineChart
      theme={theme}
      frames={frames}
      structureRev={data.structureRev}
      timeRange={timeRange}
      timeZone={timezones}
      width={width}
      height={height}
      legendItems={legendItems}
      {...options}
      mode={TimelineMode.Samples}
    >
      {(builder, alignedFrame) => {
        if (oldConfig.current !== builder && !showNewVizTooltips) {
          oldConfig.current = addTooltipSupport({
            config: builder,
            onUPlotClick,
            setFocusedSeriesIdx,
            setFocusedPointIdx,
            setCoords,
            setHover,
            isToolTipOpen,
            isActive,
            setIsActive,
          });
        }

        return (
          <>
            {showNewVizTooltips ? (
              <>
                {options.tooltip.mode !== TooltipDisplayMode.None && (
                  <TooltipPlugin2
                    config={builder}
                    hoverMode={TooltipHoverMode.xyOne}
                    queryZoom={onChangeTimeRange}
                    render={(u, dataIdxs, seriesIdx, isPinned, dismiss, timeRange2, viaSync) => {
                      if (viaSync) {
                        return null;
                      }

                      if (enableAnnotationCreation && timeRange2 != null) {
                        setNewAnnotationRange(timeRange2);
                        dismiss();
                        return;
                      }

                      const annotate = () => {
                        let xVal = u.posToVal(u.cursor.left!, 'x');

                        setNewAnnotationRange({ from: xVal, to: xVal });
                        dismiss();
                      };

                      return (
                        <StatusHistoryTooltip2
                          data={frames ?? []}
                          dataIdxs={dataIdxs}
                          alignedData={alignedFrame}
                          seriesIdx={seriesIdx}
                          timeZone={timeZone}
                          mode={options.tooltip.mode}
                          sortOrder={options.tooltip.sort}
                          isPinned={isPinned}
                          annotate={enableAnnotationCreation ? annotate : undefined}
                        />
                      );
                    }}
                    maxWidth={options.tooltip.maxWidth}
                    maxHeight={options.tooltip.maxHeight}
                  />
                )}
                <AnnotationsPlugin2
                  annotations={data.annotations ?? []}
                  config={builder}
                  timeZone={timeZone}
                  newRange={newAnnotationRange}
                  setNewRange={setNewAnnotationRange}
                  canvasRegionRendering={false}
                />
              </>
            ) : (
              <>
                <ZoomPlugin config={builder} onZoom={onChangeTimeRange} />
                {renderTooltip(alignedFrame)}
                <OutsideRangePlugin config={builder} onChangeTimeRange={onChangeTimeRange} />
                {data.annotations && (
                  <AnnotationsPlugin
                    annotations={data.annotations}
                    config={builder}
                    timeZone={timeZone}
                    disableCanvasRendering={true}
                  />
                )}
              </>
            )}
          </>
        );
      }}
    </TimelineChart>
  );
};
