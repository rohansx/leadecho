import { defineConfig } from "wxt";

export default defineConfig({
  modules: ["@wxt-dev/module-react"],
  manifest: {
    name: "LeadEcho",
    description: "Passively capture intent signals while you browse.",
    version: "0.2.0",
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
