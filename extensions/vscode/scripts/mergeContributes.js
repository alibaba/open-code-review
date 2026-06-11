const OCR_PREFIX = 'ocr.';
const OCR_VIEW_CONTAINER = 'ocr-container';

function isOcrCommand(c) { return c.command && c.command.startsWith(OCR_PREFIX); }

function mergeContributes(aone, ocr) {
  const result = JSON.parse(JSON.stringify(aone || {}));

  // commands：移除所有旧 ocr.* 再并入新的
  const aoneCommands = (result.commands || []).filter((c) => !isOcrCommand(c));
  result.commands = [...aoneCommands, ...(ocr.commands || [])];

  // views：替换 ocr-container 这一组
  result.views = result.views || {};
  if (ocr.views && ocr.views[OCR_VIEW_CONTAINER]) {
    result.views[OCR_VIEW_CONTAINER] = ocr.views[OCR_VIEW_CONTAINER];
  }

  // viewsContainers.activitybar：移除旧 ocr-container 再并入
  if (ocr.viewsContainers) {
    result.viewsContainers = result.viewsContainers || {};
    const aoneBar = (result.viewsContainers.activitybar || []).filter((v) => v.id !== OCR_VIEW_CONTAINER);
    const ocrBar = (ocr.viewsContainers.activitybar || []);
    result.viewsContainers.activitybar = [...aoneBar, ...ocrBar];
  }

  // menus.comments/commentThread/title：移除旧 ocr.* 命令项再并入
  if (ocr.menus && ocr.menus['comments/commentThread/title']) {
    result.menus = result.menus || {};
    const key = 'comments/commentThread/title';
    const aoneMenu = (result.menus[key] || []).filter((m) => !isOcrCommand(m));
    result.menus[key] = [...aoneMenu, ...ocr.menus[key]];
  }

  return result;
}

module.exports = { mergeContributes };
