import MCPPanel from '../features/system/MCPPanel';

export default function MCPPage({ canManage }: { canManage: boolean }) {
  return (
    <div data-page="mcp" className="active h-full flex flex-col">
      <MCPPanel canManage={canManage} />
    </div>
  );
}
