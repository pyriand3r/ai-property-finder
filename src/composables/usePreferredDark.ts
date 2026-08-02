import { onBeforeUnmount, onMounted, shallowRef } from 'vue';

/** Tracks the OS-level `prefers-color-scheme` media query reactively. */
export function usePreferredDark() {
  const query = window.matchMedia('(prefers-color-scheme: dark)');
  const prefersDark = shallowRef(query.matches);

  function onChange(event: MediaQueryListEvent) {
    prefersDark.value = event.matches;
  }

  onMounted(() => query.addEventListener('change', onChange));
  onBeforeUnmount(() => query.removeEventListener('change', onChange));

  return prefersDark;
}
