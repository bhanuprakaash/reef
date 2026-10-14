// The Reef desktop app's preload script exposes this on window.reef. Plain
// Chrome pointed at reef serve has no preload script, so it is undefined
// there.
export type ReefBridge = {
  setBrowserBounds: (rect: { x: number; y: number; width: number; height: number }) => void;
  navigateBrowser: (url: string) => Promise<void>;
  setTakeOver: (value: boolean) => Promise<boolean>;
  onBrowserNavigated: (callback: (url: string) => void) => () => void;
  openTerminal: (cwd: string, cols: number, rows: number) => Promise<string>;
  writeTerminal: (cwd: string, data: string) => void;
  resizeTerminal: (cwd: string, cols: number, rows: number) => void;
  onTerminalData: (cwd: string, callback: (data: string) => void) => () => void;
};

declare global {
  interface Window {
    reef?: ReefBridge;
  }
}
