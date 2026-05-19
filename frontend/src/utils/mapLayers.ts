import {
  REFERENCE_DEFAULT_MAP_LAYER,
  referenceMapLayers,
  type ReferenceMapLayer,
} from "../pages/screenData";

const VISIBLE_MAP_LAYERS_KEY = "dr600ab.visible-map-layers";

export function normalizeVisibleMapLayers(
  options: ReferenceMapLayer[] = referenceMapLayers,
  visible: string[] = [],
) {
  const optionSet = new Set(options);
  const next = visible.filter((item): item is ReferenceMapLayer => optionSet.has(item as ReferenceMapLayer));
  const unique = Array.from(new Set(next));

  if (unique.length > 0) {
    return unique;
  }

  if (options.includes(REFERENCE_DEFAULT_MAP_LAYER)) {
    return options;
  }

  return referenceMapLayers;
}

export function getStoredVisibleMapLayers() {
  if (typeof window === "undefined") {
    return [];
  }

  try {
    const raw = window.localStorage.getItem(VISIBLE_MAP_LAYERS_KEY);
    if (!raw) {
      return [];
    }
    const parsed = JSON.parse(raw) as unknown;
    return Array.isArray(parsed) ? parsed.filter((item): item is string => typeof item === "string") : [];
  } catch {
    return [];
  }
}

export function persistVisibleMapLayers(layers: ReferenceMapLayer[]) {
  if (typeof window === "undefined") {
    return;
  }

  try {
    window.localStorage.setItem(VISIBLE_MAP_LAYERS_KEY, JSON.stringify(Array.from(new Set(layers))));
  } catch {
    // 忽略本地存储错误。
  }
}
