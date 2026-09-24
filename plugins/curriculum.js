// Exposes which curriculum chapters exist on disk, so the roadmap can show
// written chapters as links and planned ones as "coming soon".
const fs = require('fs');
const path = require('path');
const curriculum = require('../src/data/curriculum.json');

function chapterFile(siteDir, partId, chapterId) {
  for (const ext of ['.mdx', '.md']) {
    const p = path.join(siteDir, 'docs', partId, chapterId + ext);
    if (fs.existsSync(p)) return p;
  }
  return null;
}

module.exports = function curriculumPlugin(context) {
  return {
    name: 'curriculum-plugin',
    async contentLoaded({actions}) {
      const available = [];
      for (const part of curriculum.parts) {
        for (const ch of part.chapters) {
          if (chapterFile(context.siteDir, part.id, ch.id)) available.push(`${part.id}/${ch.id}`);
        }
      }
      actions.setGlobalData({available});
    },
  };
};
module.exports.chapterFile = chapterFile;
