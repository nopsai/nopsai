import MCPPanel from '../features/system/MCPPanel';

export default function MCPPage({ canManage }: { canManage: boolean }) {
  return (
    <div data-page="mcp" className="active p-6 space-y-6">
      <MCPPanel canManage={canManage} />
    </div>
  );
}
