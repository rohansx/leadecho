import { defineConfig } from "wxt";

export default defineConfig({
  modules: ["@wxt-dev/module-react"],
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
      "https://x.com/*",
      "https://twitter.com/*",
      "https://news.ycombinator.com/*",
    ],
    action: {
      default_popup: "popup/index.html",
      default_title: "LeadEcho",
    },
    side_panel: {
      default_path: "sidepanel/index.html",
    },
  },
});
