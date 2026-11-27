<script setup lang="ts">
// Site search with Pagefind's default UI. The index is made after
// `vitepress build` (mise run docs:build), so it does not exist in dev.
import { useRoute, withBase } from "vitepress";
import { onBeforeUnmount, onMounted, ref, watch } from "vue";

declare global {
  interface Window {
    PagefindUI?: new (options: Record<string, unknown>) => unknown;
  }
}

const dialog = ref<HTMLDialogElement>();
const status = ref<"idle" | "loading" | "ready" | "missing">("idle");
const route = useRoute();

function loadUI(): Promise<void> {
  if (window.PagefindUI) return Promise.resolve();
  const css = document.createElement("link");
  css.rel = "stylesheet";
  css.href = withBase("/pagefind/pagefind-ui.css");
  document.head.append(css);
  return new Promise((resolve, reject) => {
    const script = document.createElement("script");
    script.src = withBase("/pagefind/pagefind-ui.js");
    script.onload = () => resolve();
    script.onerror = () => reject(new Error("pagefind-ui.js not found"));
    document.head.append(script);
  });
}

async function open() {
  dialog.value?.showModal();
  if (status.value !== "idle") return;
  status.value = "loading";
  try {
    await loadUI();
  } catch {
    status.value = "missing";
    return;
  }
  if (!window.PagefindUI) {
    status.value = "missing";
    return;
  }
  new window.PagefindUI({
    element: "#pagefind-search",
    showSubResults: true,
    showImages: false,
  });
  status.value = "ready";
  focusInput();
}

function focusInput() {
  dialog.value?.querySelector<HTMLInputElement>("input")?.focus();
}

function close() {
  dialog.value?.close();
}

function onBackdrop(e: MouseEvent) {
  if (e.target === dialog.value) close();
}

function onKey(e: KeyboardEvent) {
  if (e.key === "k" && (e.metaKey || e.ctrlKey)) {
    e.preventDefault();
    open();
  }
}

watch(() => route.path, close);
onMounted(() => window.addEventListener("keydown", onKey));
onBeforeUnmount(() => window.removeEventListener("keydown", onKey));
</script>

<template>
  <div class="pw-search">
    <button type="button" class="pw-search-button" aria-haspopup="dialog" @click="open">
      <span class="vpi-search" aria-hidden="true"></span>
      <span class="pw-search-label">検索</span>
    </button>
    <dialog
      ref="dialog"
      class="pw-search-dialog"
      aria-label="ドキュメントを検索"
      @click="onBackdrop"
    >
      <div class="pw-search-head">
        <h2 class="pw-search-title">ドキュメントを検索</h2>
        <button type="button" class="pw-search-close" @click="close">閉じる</button>
      </div>
      <p v-if="status === 'loading'">検索の準備をしています…</p>
      <p v-if="status === 'missing'">
        検索の索引がありません。<code>mise run docs:build</code> のあと
        <code>mise run docs:preview</code> で開くと検索できます。
      </p>
      <div id="pagefind-search"></div>
    </dialog>
  </div>
</template>

<style scoped>
.pw-search {
  display: flex;
  align-items: center;
  flex-grow: 1;
  padding-left: 24px;
}

.pw-search-button {
  display: flex;
  align-items: center;
  gap: 8px;
  height: 40px;
  padding: 0 12px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 8px;
  background: var(--vp-c-bg-alt);
  color: var(--vp-c-text-2);
  font-size: 14px;
}

.pw-search-button:hover {
  border-color: var(--vp-c-brand-1);
}

.pw-search-button:focus-visible,
.pw-search-close:focus-visible {
  outline: 2px solid var(--vp-c-brand-1);
  outline-offset: 2px;
}

.pw-search-dialog {
  width: min(720px, calc(100vw - 32px));
  max-height: calc(100vh - 96px);
  margin: 64px auto auto;
  padding: 16px 20px 24px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 12px;
  background: var(--vp-c-bg);
  color: var(--vp-c-text-1);
  --pagefind-ui-primary: var(--vp-c-brand-1);
  --pagefind-ui-text: var(--vp-c-text-1);
  --pagefind-ui-background: var(--vp-c-bg);
  --pagefind-ui-border: var(--vp-c-divider);
  --pagefind-ui-tag: var(--vp-c-bg-soft);
  --pagefind-ui-font: var(--vp-font-family-base);
}

.pw-search-dialog::backdrop {
  background: rgb(0 0 0 / 50%);
}

.pw-search-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 12px;
}

.pw-search-title {
  font-size: 16px;
  font-weight: 600;
}

.pw-search-close {
  padding: 4px 12px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 6px;
  font-size: 14px;
}

@media (max-width: 767px) {
  .pw-search {
    flex-grow: 0;
    padding-left: 0;
  }

  .pw-search-label {
    position: absolute;
    width: 1px;
    height: 1px;
    overflow: hidden;
    clip: rect(0 0 0 0);
  }
}
</style>
