<script>
  import { invoke } from "@tauri-apps/api/core";
  import { onMount } from "svelte";
  import { appState, PLATFORMS } from "$lib/stores.svelte.js";
  import { monogram } from "$lib/identity.js";

  /** @type {HTMLElement | undefined} */
  let contentEl = $state();
  /** @type {HTMLInputElement | undefined} */
  let nameInput = $state();

  let adding = $state(false);
  let newName = $state("");
  let pendingDeleteId = $state(/** @type {string | null} */ (null));
  let busy = $state(false);

  /** @param {unknown} err */
  function showError(err) {
    appState.error = err instanceof Error ? err.message : String(err);
  }

  async function loadProfiles() {
    try {
      appState.profiles = await invoke("list_profiles");
    } catch (err) {
      showError(err);
    }
  }

  async function reportSize() {
    if (!contentEl) return;
    const rect = contentEl.getBoundingClientRect();
    try {
      await invoke("resize_content_area", {
        width: rect.width,
        height: rect.height,
      });
    } catch (err) {
      showError(err);
    }
  }

  /**
   * @param {string} profileId
   * @param {string} platform
   */
  async function switchTab(profileId, platform) {
    appState.activeProfileId = profileId;
    appState.activeTab = platform;
    appState.error = "";
    try {
      await invoke("switch_to_tab", { profileId, platform });
    } catch (err) {
      showError(err);
    }
  }

  /** @param {string} profileId */
  async function selectProfile(profileId) {
    pendingDeleteId = null;
    await switchTab(profileId, appState.activeTab);
  }

  function startAdd() {
    adding = true;
    newName = "";
    appState.error = "";
    queueMicrotask(() => nameInput?.focus());
  }

  function cancelAdd() {
    adding = false;
    newName = "";
  }

  async function submitAdd() {
    const trimmed = newName.trim();
    if (!trimmed) {
      appState.error = "Name the profile first.";
      nameInput?.focus();
      return;
    }
    if (busy) return;

    busy = true;
    appState.error = "";
    try {
      const profile = await invoke("create_profile", { name: trimmed });
      appState.profiles = [...appState.profiles, profile];
      adding = false;
      newName = "";
      await switchTab(profile.id, "threads");
    } catch (err) {
      showError(err);
    } finally {
      busy = false;
    }
  }

  /** @param {SubmitEvent} event */
  function onAddSubmit(event) {
    event.preventDefault();
    submitAdd();
  }

  /**
   * @param {MouseEvent} event
   * @param {{ id: string, name: string }} profile
   */
  function requestDelete(event, profile) {
    event.stopPropagation();
    pendingDeleteId = profile.id;
    appState.error = "";
  }

  /**
   * @param {MouseEvent} event
   * @param {{ id: string, name: string }} profile
   */
  async function confirmDelete(event, profile) {
    event.stopPropagation();
    if (busy) return;

    busy = true;
    appState.error = "";
    try {
      await invoke("delete_profile", { id: profile.id });
      appState.profiles = appState.profiles.filter((p) => p.id !== profile.id);
      pendingDeleteId = null;
      if (appState.activeProfileId === profile.id) {
        const next = appState.profiles[0];
        if (next) {
          await selectProfile(next.id);
        } else {
          appState.activeProfileId = null;
        }
      }
    } catch (err) {
      showError(err);
    } finally {
      busy = false;
    }
  }

  /** @param {MouseEvent} event */
  function cancelDelete(event) {
    event.stopPropagation();
    pendingDeleteId = null;
  }

  /** @param {KeyboardEvent} event */
  function onKey(event) {
    if (event.key !== "Escape") return;
    if (adding) cancelAdd();
    if (pendingDeleteId) pendingDeleteId = null;
  }

  onMount(() => {
    loadProfiles();
    window.addEventListener("keydown", onKey);

    const observer = new ResizeObserver(() => {
      reportSize();
    });

    if (contentEl) {
      observer.observe(contentEl);
      reportSize();
    }

    return () => {
      observer.disconnect();
      window.removeEventListener("keydown", onKey);
    };
  });
</script>

<div class="shell">
  <aside class="sidebar">
    <header class="mast">
      <span class="wordmark">Goggles</span>
    </header>

    <div class="roll">
      {#if appState.profiles.length === 0 && !adding}
        <p class="roll-empty">No profiles yet</p>
      {/if}

      <ul class="profiles">
        {#each appState.profiles as profile (profile.id)}
          {@const active = profile.id === appState.activeProfileId}
          <li class="pair" class:on={active}>
            <div class="pair-row">
              <button
                type="button"
                class="pair-hit"
                aria-current={active ? "true" : undefined}
                onclick={() => selectProfile(profile.id)}
              >
                <span class="badge">{monogram(profile.name)}</span>
                <span class="pair-name">{profile.name}</span>
              </button>
              {#if active && pendingDeleteId !== profile.id}
                <button
                  type="button"
                  class="icon-btn"
                  onclick={(event) => requestDelete(event, profile)}
                  aria-label="Remove {profile.name}"
                >
                  Remove
                </button>
              {/if}
            </div>

            {#if pendingDeleteId === profile.id}
              <div class="confirm">
                <p>Remove {profile.name} and its logins?</p>
                <div class="actions">
                  <button
                    type="button"
                    class="btn danger"
                    onclick={(event) => confirmDelete(event, profile)}
                    disabled={busy}
                  >
                    Remove
                  </button>
                  <button type="button" class="btn ghost" onclick={cancelDelete}>
                    Keep
                  </button>
                </div>
              </div>
            {/if}

            {#if active}
              <nav class="sights" aria-label="Platforms">
                {#each PLATFORMS as platform (platform.id)}
                  {@const current = appState.activeTab === platform.id}
                  <button
                    type="button"
                    class="sight"
                    class:lit={current}
                    aria-current={current ? "page" : undefined}
                    onclick={() => switchTab(profile.id, platform.id)}
                  >
                    <span class="sight-label">{platform.label}</span>
                    <span class="sight-hint">{platform.hint}</span>
                  </button>
                {/each}
              </nav>
            {/if}
          </li>
        {/each}
      </ul>
    </div>

    <footer class="dock">
      {#if adding}
        <form class="add-form" onsubmit={onAddSubmit}>
          <input
            bind:this={nameInput}
            bind:value={newName}
            class="name-input"
            type="text"
            placeholder="Work, home, client…"
            maxlength="40"
            disabled={busy}
            aria-label="Profile name"
          />
          <div class="actions">
            <button type="submit" class="btn solid" disabled={busy}>
              {busy ? "Creating…" : "Add profile"}
            </button>
            <button type="button" class="btn ghost" onclick={cancelAdd} disabled={busy}>
              Cancel
            </button>
          </div>
        </form>
      {:else}
        <button type="button" class="btn solid wide" onclick={startAdd}>
          Add a profile
        </button>
      {/if}

      {#if appState.error}
        <p class="error" role="alert">{appState.error}</p>
      {/if}
    </footer>
  </aside>

  <section class="content" bind:this={contentEl}>
    {#if !appState.activeProfileId}
      <div class="stage">
        <p class="kicker">No profile selected</p>
        <h2>Add a profile to start</h2>
        <p class="lede">
          Threads, X, and Instagram open here. Each profile keeps its own logins.
        </p>
        <button type="button" class="btn solid" onclick={startAdd}>
          Add a profile
        </button>
      </div>
    {/if}
  </section>
</div>

<style>
  .shell {
    display: flex;
    height: 100vh;
    width: 100vw;
  }

  .sidebar {
    width: var(--sidebar-width);
    flex: 0 0 var(--sidebar-width);
    background: var(--sidebar);
    border-right: 1px solid var(--line);
    display: flex;
    flex-direction: column;
    min-height: 0;
  }

  .mast {
    padding: 42px 16px 14px;
  }

  .wordmark {
    font-size: 13px;
    font-weight: 600;
    letter-spacing: 0.01em;
    color: var(--text-strong);
  }

  .roll {
    flex: 1;
    min-height: 0;
    overflow: auto;
    padding: 4px 8px 12px;
  }

  .roll-empty {
    margin: 8px;
    color: var(--muted);
    font-size: 12px;
  }

  .profiles {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 2px;
  }

  .pair {
    border-radius: var(--radius);
    padding: 2px;
  }

  .pair.on {
    background: var(--active);
  }

  .pair-row {
    display: flex;
    align-items: center;
    gap: 2px;
  }

  .pair-hit {
    flex: 1;
    min-width: 0;
    display: flex;
    align-items: center;
    gap: 8px;
    border: 0;
    background: transparent;
    text-align: left;
    cursor: pointer;
    padding: 6px 8px;
    border-radius: var(--radius);
  }

  .pair-hit:hover {
    background: var(--hover);
  }

  .pair.on .pair-hit:hover {
    background: transparent;
  }

  .badge {
    width: 22px;
    height: 22px;
    flex: 0 0 22px;
    border-radius: var(--radius);
    display: grid;
    place-items: center;
    font-size: 11px;
    font-weight: 600;
    color: var(--text-strong);
    background: var(--badge);
    border: 1px solid var(--line);
  }

  .pair-name {
    font-weight: 500;
    font-size: 13px;
    color: var(--text-strong);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .icon-btn {
    border: 0;
    background: transparent;
    color: var(--muted);
    font-size: 11px;
    padding: 6px 8px;
    cursor: pointer;
    border-radius: var(--radius);
  }

  .icon-btn:hover {
    color: var(--danger);
    background: var(--danger-bg);
  }

  .sights {
    display: flex;
    flex-direction: column;
    padding: 2px 4px 6px 30px;
    gap: 1px;
  }

  .sight {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
    border: 0;
    background: transparent;
    text-align: left;
    cursor: pointer;
    padding: 6px 8px;
    border-radius: var(--radius);
    color: var(--muted);
  }

  .sight:hover {
    background: var(--hover);
    color: var(--text);
  }

  .sight.lit {
    color: var(--text-strong);
    background: var(--hover);
  }

  .sight-label {
    font-size: 13px;
    font-weight: 500;
  }

  .sight-hint {
    font-size: 11px;
    color: var(--muted);
  }

  .confirm {
    margin: 4px 6px 8px;
    padding: 10px;
    background: var(--danger-bg);
    border-radius: var(--radius);
  }

  .confirm p {
    margin: 0 0 8px;
    color: var(--text-strong);
    font-size: 12px;
  }

  .dock {
    padding: 12px;
    border-top: 1px solid var(--line);
    background: var(--sidebar);
  }

  .add-form {
    display: flex;
    flex-direction: column;
    gap: 8px;
  }

  .name-input {
    width: 100%;
    border: 1px solid var(--line);
    border-radius: var(--radius);
    background: var(--input);
    padding: 8px 10px;
    outline: none;
  }

  .name-input::placeholder {
    color: var(--muted);
  }

  .actions {
    display: flex;
    gap: 6px;
  }

  .btn {
    border: 0;
    border-radius: var(--radius);
    padding: 8px 10px;
    cursor: pointer;
    font-weight: 500;
    font-size: 12px;
  }

  .btn.wide {
    width: 100%;
  }

  .btn.solid {
    background: var(--solid-bg);
    color: var(--solid-text);
  }

  .btn.ghost {
    background: transparent;
    color: var(--muted);
    border: 1px solid var(--line);
  }

  .btn.ghost:hover {
    color: var(--text);
  }

  .btn.danger {
    background: var(--danger);
    color: #fff;
  }

  .btn:disabled {
    opacity: 0.5;
    cursor: default;
  }

  .error {
    margin: 10px 2px 0;
    color: var(--danger);
    font-size: 12px;
    white-space: pre-wrap;
  }

  .content {
    flex: 1;
    min-width: 0;
    background: var(--bg);
    position: relative;
  }

  .stage {
    height: 100%;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 8px;
    padding: 48px 32px;
    text-align: center;
  }

  .kicker {
    margin: 0;
    font-size: 11px;
    letter-spacing: 0.08em;
    text-transform: uppercase;
    color: var(--muted);
  }

  .stage h2 {
    margin: 0;
    font-size: 20px;
    font-weight: 600;
    color: var(--text-strong);
  }

  .lede {
    margin: 0 0 10px;
    max-width: 36ch;
    color: var(--muted);
    font-size: 13px;
    line-height: 1.55;
  }
</style>
