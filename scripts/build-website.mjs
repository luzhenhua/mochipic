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
  const windowsName = `MochiPic-${tag.slice(1)}-windows-x64-setup.exe`;
  let windowsInstaller;
  for (const { name, browser_download_url: url } of assets) {
    if (name === windowsName && url === `${releaseBase}/download/${tag}/${windowsName}`) {
      windowsInstaller = url;
      continue;
    }
    if (typeof name !== 'string' || !name.startsWith(prefix) || !name.endsWith('.dmg')) continue;
    const language = name.slice(prefix.length, -4);
    if (!/^[a-z]{2}(?:-[A-Za-z]{2,4})?$/.test(language)) continue;
    if (url === `${releaseBase}/download/${tag}/${name}`) installPackages[language] = url;
  }

  return { tag, url: `${releaseBase}/tag/${tag}`, installPackages, windowsInstaller };
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
replaceOnce('</style>', `#download{display:flex;flex-wrap:wrap;justify-content:center;align-items:flex-start;gap:14px;scroll-margin-top:76px}.download-option{display:flex;flex-direction:column;align-items:center;gap:8px}.intro .download-option small{margin:0}.download-option a{white-space:nowrap}@media(max-width:420px){#download{gap:12px}.download-option{min-width:150px}}</style>`);
replaceOnce('zS=`${tg}/releases/tag/v0.0.14`', 'zS=`${tg}/releases/latest`');
replaceOnce('function dT({url:e})', 'function dT({macURL,windowsURL,release,releaseUrl})');
replaceOnce(String.raw`className:"version",href:zS,children:[(0,M.jsx)("span",{children:"v0.0.14"})," \u73B0\u5DF2\u53D1\u5E03 ",`, 'className:"version",href:release?.url??zS,children:[(0,M.jsx)("span",{children:release?`${release.tag} 现已发布`:"查看最新发布"})," ",');
replaceOnce(
  String.raw`\u5728 Mac \u4E0A\u6279\u91CF\u88C1\u526A\u3001\u52A0\u6C34\u5370\u3001\u8F6C\u683C\u5F0F\u3002`,
  '在电脑上批量裁剪、加水印、转格式。',
);
replaceOnce(
  String.raw`(0,M.jsx)("div",{className:"cta",children:(0,M.jsx)(ou,{type:"primary",size:"large",href:e,icon:(0,M.jsx)(si,{size:17}),children:"\u4E0B\u8F7D Mac \u7248"})}),(0,M.jsxs)("small",{children:["macOS 13 \u6216\u66F4\u9AD8\u7248\u672C ",(0,M.jsx)("i",{children:"\xB7"})," Apple Silicon"]})`,
  '(0,M.jsx)("div",{className:"cta",id:"download",children:[' +
    '[macURL,"下载 Mac 版","查看最新发布","macOS 13+ · Apple Silicon"],' +
    '...(windowsURL?[[windowsURL,"下载 Windows 版","","Windows 10 / 11 · x64"]]:[])' +
    '].map(([url,label,fallback,requirements])=>(0,M.jsxs)("div",{className:"download-option",children:[' +
    '(0,M.jsx)(ou,{type:"primary",size:"large",href:url??releaseUrl,icon:(0,M.jsx)(si,{size:17}),children:url?label:fallback}),' +
    '(0,M.jsx)("small",{children:requirements})]},label))})',
);
replaceOnce('function mT(){', 'function mT({releaseUrl}){');
replaceOnce('href:zS,children:["GitHub Releases "', 'href:releaseUrl,children:["GitHub Releases "');
replaceOnce(
  't=`${tg}/releases/download/v0.0.14/MochiPic-0.0.14-arm64-${e}.dmg`;return',
  '[release,setRelease]=(0,Po.useState)(null),releaseUrl=release?.url??zS,macURL=release?.installPackages[e],windowsURL=release?.windowsInstaller;' +
    '(0,Po.useEffect)(()=>{let active=true;loadMochiPicRelease().then(value=>{if(active)setRelease(value)}).catch(()=>{});return()=>{active=false}},[]);return',
);
replaceOnce('fT,{lang:e,setLang:a,url:t}', 'fT,{lang:e,setLang:a,url:"#download"}');
replaceOnce('dT,{url:t}', 'dT,{macURL,windowsURL,release,releaseUrl}');
replaceOnce('mT,{}', 'mT,{releaseUrl}');

await writeFile(entry, html);
