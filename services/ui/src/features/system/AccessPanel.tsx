import { AccessPanelView } from './access/AccessPanelView';
import { useAccessPanelController } from './access/useAccessPanelController';
import type { AccessPanelProps } from './access/useAccessPanelController';

function AccessPanel(props: AccessPanelProps) {
  const controller = useAccessPanelController(props);
  return <AccessPanelView controller={controller} />;
}

export type { AccessPanelProps };
export default AccessPanel;
