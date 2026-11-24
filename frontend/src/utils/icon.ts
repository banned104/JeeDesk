import type { Item } from '../types/config';

// File types whose shell-extracted icon is plain or unhelpful; these always
// render a bundled SVG instead. Artwork from vscode-icons (MIT).
const bundledIconByExt: Record<string, string> = {
  bat: '/defaults/file_type_bat.svg',
  cmd: '/defaults/file_type_bat.svg',
  ps1: '/defaults/file_type_powershell.svg',
};

/** Bundled icons shown when no icon could be resolved for the target. */
export const bundledFolderIconUrl = '/defaults/default_folder.svg';
export const bundledFileIconUrl = '/defaults/default_file.svg';

/**
 * Returns a bundled default icon URL when the target's extension is covered,
 * or an empty string when normal icon resolution should apply.
 */
export function bundledDefaultIconUrl(targetPath: string): string {
  const trimmed = (targetPath || '').trim();
  if (!trimmed) {
    return '';
  }
  const parts = trimmed.split(/[\\/]/);
  const baseName = parts[parts.length - 1] || '';
  const dotIdx = baseName.lastIndexOf('.');
  if (dotIdx <= 0) {
    return '';
  }
  return bundledIconByExt[baseName.slice(dotIdx + 1).toLowerCase()] || '';
}

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
  // ".gitignore" 等点开头的文件没有扩展名意义上的主名，按文件处理
  if (dotIdx === 0) {
    return false;
  }
  return dotIdx <= 0;
}

/**
 * Formats a concise multi-line tooltip string for an item card.
 *
 * `argsLabel` is the label placed in front of the launch arguments, colon included. Callers pass
 * an already translated label (e.g. `t('参数:')`) so the tooltip follows the UI language; the
 * Chinese default keeps existing callers working unchanged.
 */
export function formatItemTooltip(item: Item, argsLabel = '参数:'): string {
  const lines = [item.name];
  if (item.path) {
    lines.push(item.path);
  }
  if (item.args) {
    lines.push(`${argsLabel} ${item.args}`);
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
