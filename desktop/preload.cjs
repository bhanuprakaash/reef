// A sandboxed preload script runs as CommonJS even in an ES module project, so
// this file stays .cjs and uses require.
const { contextBridge, ipcRenderer } = require('electron');

// window.reef lets the renderer position the browser pane's native view and
// drive it from the address bar. A plain browser pointed at reef serve has no
// preload script, so window.reef is undefined there.
contextBridge.exposeInMainWorld('reef', {
  capture: () => ipcRenderer.invoke('capture:open'),
  chooseFolder: () => ipcRenderer.invoke('folder:choose'),
  setTheme: (theme) => ipcRenderer.send('theme:set', theme),
  setBrowserBounds: (rect) => ipcRenderer.send('browser:bounds', rect),
  navigateBrowser: (url) => ipcRenderer.invoke('browser:navigate', url),
  browserBack: () => ipcRenderer.send('browser:back'),
  browserForward: () => ipcRenderer.send('browser:forward'),
  browserReload: () => ipcRenderer.send('browser:reload'),
  setTakeOver: (value) => ipcRenderer.invoke('browser:take-over', value),
  onBrowserNavigated: (callback) => {
    const listener = (_event, state) => callback(state);
    ipcRenderer.on('browser:navigated', listener);
    return () => ipcRenderer.removeListener('browser:navigated', listener);
  },
  openTerminal: (cwd, cols, rows) => ipcRenderer.invoke('terminal:open', cwd, cols, rows),
  writeTerminal: (cwd, data) => ipcRenderer.send('terminal:write', cwd, data),
  resizeTerminal: (cwd, cols, rows) => ipcRenderer.send('terminal:resize', cwd, cols, rows),
  onTerminalData: (cwd, callback) => {
    const listener = (_event, key, data) => {
      if (key === cwd) callback(data);
    };
    ipcRenderer.on('terminal:data', listener);
    return () => ipcRenderer.removeListener('terminal:data', listener);
  },
});
