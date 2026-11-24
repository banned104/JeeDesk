import { ref } from 'vue';
import type { Language } from './types/config';

/**
 * UI language. Every component calls `t('中文')` for user-visible text; translations are registered
 * next to the component that needs them with `registerTranslations`, so a new string and its
 * English text live in the same file. A missing translation falls back to the Chinese source text
 * instead of showing a key, which keeps a partial translation readable.
 */
export const language = ref<Language>('zh');

const translations: Record<string, Record<string, string>> = { en: {} };

/** registerTranslations adds English strings for the given Chinese source texts. */
export function registerTranslations(table: Record<string, string>): void {
  Object.assign(translations.en, table);
}

/** setLanguage switches the UI language and mirrors it onto the document element. */
export function setLanguage(lang: Language): void {
  language.value = lang === 'en' ? 'en' : 'zh';
  if (typeof document !== 'undefined') {
    document.documentElement.lang = language.value === 'en' ? 'en' : 'zh-CN';
  }
}

/**
 * t translates a Chinese source string. `vars` replaces `{name}` placeholders, so a sentence can be
 * translated as a whole instead of being concatenated in the template.
 */
export function t(source: string, vars?: Record<string, string | number>): string {
  const table = translations[language.value];
  let text = table?.[source] ?? source;
  if (vars) {
    for (const [key, value] of Object.entries(vars)) {
      text = text.split(`{${key}}`).join(String(value));
    }
  }
  return text;
}

registerTranslations({
  '搜索项目…': 'Search items…',
  设置: 'Settings',
  新建分组: 'New group',
  新建项目: 'New item',
  从开始菜单导入: 'Import from Start Menu',
  最近使用: 'Recent',
  清空最近使用: 'Clear recent',
  分组: 'Groups',
  没有匹配的项目: 'No matching items',
  还没有任何项目: 'No items yet',
  '拖入文件、文件夹或快捷方式即可添加': 'Drop a file, folder or shortcut to add it',
  松开即可添加: 'Release to add',
  '上次没有正常退出，日志与现场见 logs 目录': 'JeeDesk did not exit cleanly last time. See the logs folder.',
  '共 {count} 个项目': '{count} items',
  '已选中 {name}': 'Selected {name}',
  隐藏: 'Hide',
  取消: 'Cancel',
  保存: 'Save',
  删除: 'Delete',
  编辑: 'Edit',
  打开位置: 'Open location',
  以管理员身份运行: 'Run as administrator',
});
