import fs from "node:fs";
import path from "node:path";

const rootDir = path.resolve(import.meta.dirname, "..");
const baseDictPath = path.join(rootDir, "third_party/rime-luna-pinyin/rime-ice-base.dict.yaml");
const charDictPath = path.join(rootDir, "third_party/rime-luna-pinyin/rime-ice-8105.dict.yaml");
const outputPath = path.join(rootDir, "src/assets/ime/pinyinRimeDictionary.json");

const projectTerms = [
  ["wurenji", "无人机", 9_000_000],
  ["wurenjizhinengguankongxitong", "无人机智能管控系统", 9_000_000],
  ["zhineng", "智能", 8_000_000],
  ["guankong", "管控", 8_000_000],
  ["zhinengguankong", "智能管控", 8_000_000],
  ["daping", "大屏", 8_000_000],
  ["guanlihoutai", "管理后台", 8_000_000],
  ["kaifazhe", "开发者", 8_000_000],
  ["dongtaima", "动态码", 8_000_000],
  ["xunijianpan", "虚拟键盘", 8_000_000],
  ["jiance", "检测", 7_000_000],
  ["jianceqi", "检测器", 7_000_000],
  ["jiancezhan", "检测站", 7_000_000],
  ["shebei", "设备", 7_000_000],
  ["shezhi", "设置", 7_000_000],
  ["ditu", "地图", 7_000_000],
  ["liebiao", "列表", 7_000_000],
  ["zaixian", "在线", 7_000_000],
  ["lixian", "离线", 7_000_000],
  ["yuyan", "语言", 7_000_000],
  ["biaoti", "标题", 7_000_000],
  ["zidingyi", "自定义", 7_000_000],
  ["xitong", "系统", 7_000_000],
  ["zhongwen", "中文", 7_000_000],
];

function parseRimeDict(filePath, { maxWordLength, minWeight }) {
  const lines = fs.readFileSync(filePath, "utf8").split(/\r?\n/);
  const rows = [];
  let bodyStarted = false;

  for (const line of lines) {
    const trimmed = line.trim();
    if (trimmed === "...") {
      bodyStarted = true;
      continue;
    }
    if (!bodyStarted || !trimmed || trimmed.startsWith("#")) {
      continue;
    }

    const [rawWord, rawPinyin, rawWeight] = line.split("\t");
    const word = rawWord?.trim();
    const pinyin = rawPinyin?.trim().replace(/\s+/g, "");
    const weight = Number(rawWeight || 0);
    const wordLength = Array.from(word || "").length;

    if (!word || !pinyin || !/^[a-z]+$/.test(pinyin)) {
      continue;
    }
    if (!/^[\u4e00-\u9fa5]+$/.test(word)) {
      continue;
    }
    if (wordLength > maxWordLength || weight < minWeight) {
      continue;
    }

    rows.push([pinyin, word, weight]);
  }

  return rows;
}

function addCandidate(map, pinyin, word, weight) {
  const values = map.get(pinyin) ?? new Map();
  const previous = values.get(word) ?? 0;
  values.set(word, Math.max(previous, weight));
  map.set(pinyin, values);
}

const candidates = new Map();

for (const [pinyin, word, weight] of parseRimeDict(charDictPath, { maxWordLength: 1, minWeight: 0 })) {
  addCandidate(candidates, pinyin, word, weight);
}

for (const [pinyin, word, weight] of parseRimeDict(baseDictPath, { maxWordLength: 5, minWeight: 8_000 })) {
  addCandidate(candidates, pinyin, word, weight);
}

for (const [pinyin, word, weight] of projectTerms) {
  addCandidate(candidates, pinyin, word, weight);
}

const output = Object.fromEntries(
  Array.from(candidates.entries())
    .sort(([left], [right]) => left.localeCompare(right))
    .map(([pinyin, values]) => [
      pinyin,
      Array.from(values.entries())
        .sort(([, leftWeight], [, rightWeight]) => rightWeight - leftWeight)
        .slice(0, 8)
        .map(([word]) => word),
    ]),
);

fs.mkdirSync(path.dirname(outputPath), { recursive: true });
fs.writeFileSync(
  outputPath,
  `${JSON.stringify(output)}\n`,
  "utf8",
);

const entryCount = Object.keys(output).length;
const candidateCount = Object.values(output).reduce((sum, values) => sum + values.length, 0);
console.log(`Wrote ${entryCount} pinyin entries and ${candidateCount} candidates to ${path.relative(rootDir, outputPath)}`);
