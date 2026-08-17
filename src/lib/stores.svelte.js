/** @typedef {{ id: string, name: string, created_at: string }} Profile */

export const PLATFORMS = [
  { id: "threads", label: "Threads", hint: "threads.net" },
  { id: "x", label: "X", hint: "x.com" },
  { id: "instagram", label: "Instagram", hint: "instagram.com" },
];

export const appState = $state({
  /** @type {Profile[]} */
  profiles: [],
  /** @type {string | null} */
  activeProfileId: null,
  activeTab: "threads",
  error: "",
});
