export function pipelineRunsNavPath(pathname: string) {
  if (pathname.startsWith('/pipelineruns/recent')) return '/pipelineruns/recent';
  if (pathname.startsWith('/pipelineruns/events')) return '/pipelineruns/events';
  return '/pipelineruns/main';
}
