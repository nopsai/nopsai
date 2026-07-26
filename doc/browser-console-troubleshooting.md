# Browser Console Troubleshooting

This runbook covers browser DevTools messages that appear while using the
NopsAI UI but originate outside the NopsAI application bundle.

## Symptom

You may see console output similar to:

```text
contentscript.js:14083 MaxListenersExceededWarning: Possible EventEmitter memory leak detected. 11 close listeners added.
contentscript.js:14083 MaxListenersExceededWarning: Possible EventEmitter memory leak detected. 11 end listeners added.
contentscript.js:14083 ObjectMultiplex - orphaned data for stream "app-init-liveness"
contentscript.js:14083 ObjectMultiplex - orphaned data for stream "background-liveness"
```

## Likely Cause

When the stack frame is `contentscript.js` and the stream names mention
`ObjectMultiplex`, `app-init-liveness`, or `background-liveness`, the warning is
from an injected browser extension content script, commonly a wallet/provider
extension. The NopsAI UI does not import wallet, web3, `ObjectMultiplex`, or
Node `EventEmitter` browser code.

Do not raise listener limits in NopsAI UI code for this symptom. That would hide
extension diagnostics and would not address the source of the listeners.

## Checks

1. Inspect the console stack or source URL.
   - NopsAI UI code should resolve to the NopsAI origin, Vite `/src/...` paths,
     or built `/assets/...` files.
   - Extension code usually resolves to `chrome-extension://...`,
     `moz-extension://...`, or a generic `contentscript.js` frame.
2. Reproduce in a clean browser profile, an incognito window with extensions
   disabled, or Playwright/Chromium without installed extensions.
3. Reload the same route after disabling wallet/provider extensions for the
   NopsAI origin.
4. If warnings remain and the stack points to a NopsAI source file, capture the
   route, action, authenticated role, console stack, and network trace before
   filing a product bug.

## Resolution

- For extension-owned warnings, disable the extension on the NopsAI origin,
  update the extension, or use a dedicated browser profile for NopsAI
  administration.
- For product-owned warnings, investigate the source file named by the stack and
  add or fix listener cleanup in the owning UI module.

## Platform Impact

- AAA is unaffected. These warnings do not grant or deny NopsAI access.
- GitOps is unaffected. No NopsAI configuration or config repository state is
  changed.
- MCP is unaffected unless separate `/v1/mcp` errors appear in NopsAI API logs,
  System Logs, or browser network responses.
- Monitoring is unaffected because extension content-script warnings are not
  emitted by NopsAI services or metrics endpoints.
