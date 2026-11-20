import { displayTrafficBytes } from '@/lib/clash/traffic';
import { memo, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { Popover, Progress } from 'antd';

import InfinityIcon from '@/components/ui/InfinityIcon';
import { useTheme } from '@/hooks/useTheme';
import { computeTrafficDisplay } from '@/lib/clients/traffic-display';
import { SizeFormatter } from '@/utils';
import './ClientTrafficCell.css';

export interface ClientTrafficCellProps {
  up?: number;
  down?: number;
  total?: number;
  enabled?: boolean;
  trafficDiff?: number;
  compact?: boolean;
  serverView?: boolean;
}

// Every prop is a primitive and the component is pure, so the memo bails out
// whenever a client's counters did not move — which is most of them on most
// pushes. Each skipped instance is one antd Popover (rc-trigger), one Progress,
// a useTranslation subscription and a theme context read, times up to 200 rows.
const ClientTrafficCell = memo(function ClientTrafficCell({
  up = 0,
  down = 0,
  total = 0,
  enabled = true,
  trafficDiff = 0,
  compact = false,
  serverView = false,
}: ClientTrafficCellProps) {
  const { t } = useTranslation();
  const { isDark } = useTheme();

  const display = useMemo(
    () => computeTrafficDisplay({ up, down, total, enabled, trafficDiff }, isDark),
    [up, down, total, enabled, trafficDiff, isDark],
  );

  const shownUsed = displayTrafficBytes(display.used, serverView);
  const shownLimit = displayTrafficBytes(total, serverView);
  const popover = (
    <table className="client-traffic-popover">
      <tbody>
        <tr>
          <td>↑</td>
          <td>{SizeFormatter.sizeFormat(up)}</td>
          <td>↓</td>
          <td>{SizeFormatter.sizeFormat(down)}</td>
        </tr>
        <tr><td colSpan={2}>服务器统计</td><td colSpan={2}>{SizeFormatter.sizeFormat(displayTrafficBytes(display.used, true))}</td></tr>
        {!display.isUnlimited && (
          <tr>
            <td colSpan={2}>{t('remained')}</td>
            <td colSpan={2}>{SizeFormatter.sizeFormat(displayTrafficBytes(display.remaining, serverView))}</td>
          </tr>
        )}
      </tbody>
    </table>
  );

  const rootClass = [
    'client-traffic-cell',
    compact ? 'is-compact' : '',
    display.isUnlimited ? 'is-unlimited' : '',
  ]
    .filter(Boolean)
    .join(' ');

  return (
    <Popover content={popover} trigger={['hover', 'click']} placement="top">
      <div className={rootClass}>
        <span className="client-traffic-cell-used">{SizeFormatter.sizeFormat(shownUsed)}</span>
        <Progress
          className="client-traffic-cell-bar"
          aria-label={`${SizeFormatter.sizeFormat(shownUsed)} / ${display.isUnlimited ? t('subscription.unlimited') : SizeFormatter.sizeFormat(shownLimit)}`}
          percent={display.percent}
          showInfo={false}
          strokeColor={display.strokeColor}
          status={display.status}
          size={compact ? 'small' : 'medium'}
        />
        <span className="client-traffic-cell-limit">
          {display.isUnlimited ? (
            <span
              className="client-traffic-cell-infinity"
              role="img"
              aria-label={t('subscription.unlimited')}
            >
              <InfinityIcon />
            </span>
          ) : (
            SizeFormatter.sizeFormat(shownLimit)
          )}
        </span>
      </div>
    </Popover>
  );
});

export default ClientTrafficCell;
