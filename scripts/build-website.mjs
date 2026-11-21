import { readFile, writeFile } from 'node:fs/promises';

async function loadMochiPicRelease() {
  const response = await fetch('https://api.github.com/repos/luzhenhua/mochipic/releases/latest', {
    headers: { Accept: 'application/vnd.github+json' },
    signal: AbortSignal.timeout(5000),
  });
  if (!response.ok) throw new Error(`GitHub returned HTTP ${response.status}`);

  const { tag_name: tag, assets } = await response.json();
  if (typeof tag !== 'string' || !/^v\d+\.\d+\.\d+$/.test(tag) || !Array.isArray(assets)) {
    throw new Error('GitHub returned unexpected release data');
  }

  const releaseBase = 'https://github.com/luzhenhua/mochipic/releases';
  const installPackages = {};
  const prefix = `MochiPic-${tag.slice(1)}-arm64-`;
  for (const { name, browser_download_url: url } of assets) {
    if (typeof name !== 'string' || !name.startsWith(prefix) || !name.endsWith('.dmg')) continue;
    const language = name.slice(prefix.length, -4);
    if (!/^[a-z]{2}(?:-[A-Za-z]{2,4})?$/.test(language)) continue;
    if (url === `${releaseBase}/download/${tag}/${name}`) installPackages[language] = url;
  }

  return { tag, url: `${releaseBase}/tag/${tag}`, installPackages };
}

// Apply the release integration to the bundled HTML export at build time.
const entry = new URL('../website/index.html', import.meta.url);
let html = await readFile(entry, 'utf8');
function replaceOnce(original, updated) {
  const first = html.indexOf(original);
  if (first === -1 || first !== html.lastIndexOf(original)) {
    throw new Error(`Expected one match in the landing page: ${original}`);
  }
  html = html.slice(0, first) + updated + html.slice(first + original.length);
}

replaceOnce('<script>"use strict";', `<script>${loadMochiPicRelease.toString()}</script><script>"use strict";`);
replaceOnce('zS=`${tg}/releases/tag/v0.0.14`', 'zS=`${tg}/releases/latest`');
replaceOnce('function fT({lang:e,setLang:a,url:t})', 'function fT({lang:e,setLang:a,url:t,directDownload})');
replaceOnce(String.raw`icon:(0,M.jsx)(si,{size:14}),children:"\u4E0B\u8F7D"`, String.raw`icon:(0,M.jsx)(si,{size:14}),children:directDownload?"\u4E0B\u8F7D":"查看发布"`);
replaceOnce('function dT({url:e})', 'function dT({url:e,release,directDownload})');
replaceOnce(String.raw`className:"version",href:zS,children:[(0,M.jsx)("span",{children:"v0.0.14"})," \u73B0\u5DF2\u53D1\u5E03 ",`, 'className:"version",href:release?.url??zS,children:[(0,M.jsx)("span",{children:release?`${release.tag} 现已发布`:"查看最新发布"})," ",');
replaceOnce(String.raw`children:"\u4E0B\u8F7D Mac \u7248"`, String.raw`children:directDownload?"\u4E0B\u8F7D Mac \u7248":"前往 GitHub 下载"`);
replaceOnce('function mT(){', 'function mT({releaseUrl}){');
replaceOnce('href:zS,children:["GitHub Releases "', 'href:releaseUrl,children:["GitHub Releases "');
replaceOnce(
  't=`${tg}/releases/download/v0.0.14/MochiPic-0.0.14-arm64-${e}.dmg`;return',
  '[release,setRelease]=(0,Po.useState)(null),releaseUrl=release?.url??zS,downloadUrl=release?.installPackages[e],t=downloadUrl??releaseUrl;' +
    '(0,Po.useEffect)(()=>{let active=true;loadMochiPicRelease().then(value=>{if(active)setRelease(value)}).catch(()=>{});return()=>{active=false}},[]);return',
);
replaceOnce('fT,{lang:e,setLang:a,url:t}', 'fT,{lang:e,setLang:a,url:t,directDownload:!!downloadUrl}');
replaceOnce('dT,{url:t}', 'dT,{url:t,release,directDownload:!!downloadUrl}');
replaceOnce('mT,{}', 'mT,{releaseUrl}');

await writeFile(entry, html);
