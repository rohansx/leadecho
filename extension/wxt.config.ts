import { defineConfig } from "wxt";

export default defineConfig({
  modules: ["@wxt-dev/module-react"],
  hooks: {
    "build:manifestGenerated": (_wxt, manifest) => {
      // Entrypoint popup/ still builds, but must not claim the toolbar click —
      // otherwise Chrome never opens the side panel on icon click.
      if (manifest.action) {
        delete manifest.action.default_popup;
      }
    },
  },
  manifest: {
    name: "LeadEcho",
    description: "Passively capture intent signals while you browse.",
    version: "0.2.0",
    icons: {
      16: "icons/icon-16.png",
      32: "icons/icon-32.png",
      48: "icons/icon-48.png",
      128: "icons/icon-128.png",
    },
    homepage_url: "https://github.com/rohansx/leadecho",
    permissions: ["storage", "alarms", "sidePanel", "activeTab", "tabs"],
    host_permissions: [
      "https://www.linkedin.com/*",
      "https://www.reddit.com/*",
      "https://reddit.com/*",
      "https://x.com/*",
      "https://twitter.com/*",
      "https://news.ycombinator.com/*",
    ],
    action: {
      default_title: "LeadEcho",
    },
    side_panel: {
      default_path: "sidepanel/index.html",
    },
  },
});
