<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import {
  bundledDefaultIconUrl,
  bundledFileIconUrl,
  bundledFolderIconUrl,
  isFolderLikePath,
  resolveItemIconUrl,
} from '../utils/icon';

const props = withDefaults(
  defineProps<{
    icon: string;
    path: string;
    name?: string;
    cacheBuster?: number;
    sizeClass?: string;
  }>(),
  {
    name: '',
    cacheBuster: 0,
    sizeClass: 'w-12 h-12',
  }
);

const loadError = ref(false);
const fallbackError = ref(false);

const iconUrl = computed(() => {
  const icon = (props.icon || '').trim();
  const targetPath = (props.path || '').trim();
  // No dedicated custom icon set for this item: bundled types (e.g. .bat)
  // render their stored SVG instead of the plain shell-extracted icon.
  const sameAsTarget =
    targetPath !== '' && icon.replace(/\\/g, '/').toLowerCase() === targetPath.replace(/\\/g, '/').toLowerCase();
  if (!icon || sameAsTarget) {
    const bundled = bundledDefaultIconUrl(targetPath);
    if (bundled) {
      return bundled;
    }
  }
  return resolveItemIconUrl(icon, props.cacheBuster);
});
const isFolder = computed(() => isFolderLikePath(props.path));
const fallbackUrl = computed(() => (isFolder.value ? bundledFolderIconUrl : bundledFileIconUrl));

watch(
  () => [props.icon, props.path, props.cacheBuster],
  () => {
    loadError.value = false;
    fallbackError.value = false;
  }
);
</script>

<template>
  <div
    :class="[
      sizeClass,
      'relative flex items-center justify-center rounded-xl overflow-hidden shrink-0 transition-transform duration-150',
    ]"
  >
    <img
      v-if="iconUrl && !loadError"
      :src="iconUrl"
      :alt="name"
      class="w-full h-full object-contain rounded-xl drop-shadow-sm"
      draggable="false"
      @error="loadError = true"
    />

    <!-- Bundled default icon (folder / plain file) when extraction failed or none set -->
    <img
      v-else-if="!fallbackError"
      :src="fallbackUrl"
      :alt="name"
      class="w-full h-full object-contain rounded-xl drop-shadow-sm"
      draggable="false"
      @error="fallbackError = true"
    />

    <!-- Fallback Folder SVG -->
    <div
      v-else-if="isFolder"
      class="w-full h-full rounded-xl bg-amber-400/15 border border-amber-300/20 flex items-center justify-center text-amber-300"
    >
      <svg class="w-7 h-7" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75">
        <path
          stroke-linecap="round"
          stroke-linejoin="round"
          d="M3 7a2 2 0 012-2h4.172a2 2 0 011.414.586l1.828 1.828A2 2 0 0013.828 8H19a2 2 0 012 2v7a2 2 0 01-2 2H5a2 2 0 01-2-2V7z"
        />
      </svg>
    </div>

    <!-- Fallback Application SVG -->
    <div
      v-else
      class="w-full h-full rounded-xl bg-sky-400/15 border border-sky-300/20 flex items-center justify-center text-sky-300"
    >
      <svg class="w-7 h-7" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75">
        <rect x="3.5" y="4.5" width="17" height="15" rx="3" />
        <path stroke-linecap="round" d="M3.5 9.5h17" />
        <path stroke-linecap="round" stroke-linejoin="round" d="M9.5 14l2 2 3-3.5" />
      </svg>
    </div>
  </div>
</template>
