export function MCPViewSwitch({
  activeView,
  onChange,
}: {
  activeView: 'servers' | 'profiles';
  onChange: (view: 'servers' | 'profiles') => void;
}) {
  return (
    <div className="ai-resource-view-switch" role="tablist" aria-label="MCP view">
      <button
        type="button"
        className="ai-resource-view-switch__item"
        onClick={() => onChange('servers')}
        role="tab"
        aria-selected={activeView === 'servers'}
      >
        Servers
      </button>
      <button
        type="button"
        className="ai-resource-view-switch__item"
        onClick={() => onChange('profiles')}
        role="tab"
        aria-selected={activeView === 'profiles'}
      >
        Profiles
      </button>
    </div>
  );
}
