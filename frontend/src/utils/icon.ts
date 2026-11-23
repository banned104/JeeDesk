import type { Item } from '../types/config';

/**
 * Resolves an Item's icon path into a URL served by Wails AssetServer middleware,
 * or returns an empty string when the inline SVG fallback should be rendered.
 */
export function resolveItemIconUrl(icon: string, cacheBuster: number | string = 0): string {
  const trimmed = (icon || '').trim();
  if (!trimmed) {
    return '';
  }

  const normalized = trimmed.replace(/\\/g, '/');
  if (normalized.startsWith('icons/') || normalized.startsWith('/icons/')) {
    const filename = normalized.replace(/^\/?icons\//, '');
    if (!filename) {
      return '';
    }
    return `/icons/${filename}?v=${cacheBuster}`;
  }

  return `/custom-icon?path=${encodeURIComponent(trimmed)}&v=${cacheBuster}`;
}

/**
 * Determines whether a target path looks like a folder (no file extension)
 * for selecting the appropriate inline SVG fallback icon.
 */
export function isFolderLikePath(targetPath: string): boolean {
  const trimmed = (targetPath || '').trim();
  if (!trimmed) {
    return false;
  }
  if (trimmed.endsWith('\\') || trimmed.endsWith('/')) {
    return true;
  }
  const parts = trimmed.split(/[\\/]/);
  const baseName = parts[parts.length - 1] || '';
  if (!baseName) {
    return true;
  }
  const dotIdx = baseName.lastIndexOf('.');
  return dotIdx <= 0;
}

/**
 * Formats a concise multi-line tooltip string for an item card.
 */
export function formatItemTooltip(item: Item): string {
  const lines = [item.name];
  if (item.path) {
    lines.push(item.path);
  }
  if (item.args) {
    lines.push(`参数: ${item.args}`);
  }
  return lines.join('\n');
}

/**
 * Extracts a clean display name from a file or directory path.
 */
export function inferNameFromPath(targetPath: string): string {
  const cleaned = targetPath.trim().replace(/[\\/]+$/, '');
  if (!cleaned) {
    return '';
  }
  const parts = cleaned.split(/[\\/]/);
  const base = parts[parts.length - 1] || '';
  const dotIdx = base.lastIndexOf('.');
  if (dotIdx > 0) {
    return base.slice(0, dotIdx);
  }
  return base;
}
