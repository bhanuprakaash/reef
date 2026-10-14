// The Reef desktop app's preload script exposes this on window.reef. Plain
// Chrome pointed at reef serve has no preload script, so it is undefined
// there.
export type ReefBridge = {
  setBrowserBounds: (rect: { x: number; y: number; width: number; height: number }) => void;
  navigateBrowser: (url: string) => void;
  onBrowserNavigated: (callback: (url: string) => void) => () => void;
};

declare global {
  interface Window {
    reef?: ReefBridge;
  }
}
