const fs = require('node:fs');
const path = require('node:path');

const docsRoot = path.join(__dirname, 'src/content/docs/en');

function englishDocFiles() {
  return fs.readdirSync(docsRoot, { recursive: true })
    .filter(file => file.endsWith('.md')).map(file => path.join(docsRoot, file));
}

function readEnglishDocIdentifiers() {
  const identifiers = new Set();
  for (const file of englishDocFiles()) {
    const markdown = fs.readFileSync(file, 'utf8');
    const title = markdown.match(/^title: (.+)$/m)?.[1] ?? '';
    for (const [word] of title.matchAll(/\b[A-Z][a-zA-Z0-9]*\b/g)) {
      if (/[Ii]/.test(word)) identifiers.add(word.toLowerCase());
    }
    for (const [, word] of markdown.matchAll(/`([a-z][a-z0-9]*)`/g)) {
      if (word.includes('i')) identifiers.add(word);
    }
    for (const [word] of markdown.matchAll(/[\p{L}\p{M}\p{N}_]+/gu)) {
      if (/^[a-zA-Z0-9_]+$/.test(word) && /[Ii]/.test(word) &&
          (word.includes('_') || /[a-z][A-Z]/.test(word) || /^[A-Z]{2,}$/.test(word))) {
        identifiers.add(word.toLowerCase());
      }
    }
  }
  return [...identifiers].sort();
}

module.exports = { docsRoot, englishDocFiles, readEnglishDocIdentifiers };
