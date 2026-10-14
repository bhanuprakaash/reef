// A sandboxed preload script runs as CommonJS even in an ES module project, so
// this file stays .cjs and uses require.
const { contextBridge, ipcRenderer } = require('electron');

// window.reef lets the renderer position the browser pane's native view and
// drive it from the address bar. A plain browser pointed at reef serve has no
// preload script, so window.reef is undefined there.
contextBridge.exposeInMainWorld('reef', {
  setBrowserBounds: (rect) => ipcRenderer.send('browser:bounds', rect),
  navigateBrowser: (url) => ipcRenderer.invoke('browser:navigate', url),
  onBrowserNavigated: (callback) => {
    const listener = (_event, url) => callback(url);
    ipcRenderer.on('browser:navigated', listener);
    return () => ipcRenderer.removeListener('browser:navigated', listener);
  },
});
