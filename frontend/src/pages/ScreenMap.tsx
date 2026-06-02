import { useEffect, useMemo, useRef } from "react";
import type { TFunction } from "i18next";
import L from "leaflet";
import "leaflet/dist/leaflet.css";
import { Info } from "lucide-react";
import { useTranslation } from "react-i18next";

import centerPointIcon from "../assets/images/centerPoint.svg";
import i18n from "../i18n";
import type {
  ScreenDeviceLocationResponse,
  ScreenPositionPoint,
  ScreenPositionTarget,
  ScreenPositionTrackPoint,
  WhitelistItem,
} from "../types";
import { cx } from "../utils/classnames";
import { createDrawControlButtonGroup } from "../utils/leafletControls";
import { installLeafletCoordConverter } from "../utils/leafletCoordConverter";
import { isSerialWhitelisted } from "../utils/whitelist";
import {
  REFERENCE_DEFAULT_MAP_LAYER,
  REFERENCE_LEGACY_MAP_LAYER_STORAGE_KEY,
  REFERENCE_MAP_CENTER,
  REFERENCE_MAP_LAYER_STORAGE_KEY,
  REFERENCE_MAP_ZOOM,
  referenceDefaultMapLayerForLocale,
  referenceMapLayers,
  referenceMarkerIcons,
  type ReferenceMapLayer,
} from "./screenData";

installLeafletCoordConverter();

const markerPane = "screenMarkers";
const trajectoryPane = "screenTrajectories";
const selectedPane = "screenSelectedMarkers";
const deviceIconSize: [number, number] = [198 / 5, 256 / 5];
const targetIconSize: [number, number] = [160 / 5, 256 / 5];
const droneTrajectoryColor = "#26c9ff";
const pilotTrajectoryColor = "#f4c95d";

type RealMapData = {
  deviceLocation: ScreenDeviceLocationResponse | null;
  deviceHeadingDeg?: number;
  positions: ScreenPositionTarget[];
  whitelist?: WhitelistItem[];
};

type PositionMapProps = {
  selectedId: string;
  positions: ScreenPositionTarget[];
  whitelist?: WhitelistItem[];
  deviceLocation: ScreenDeviceLocationResponse | null;
  deviceHeadingDeg?: number;
  visibleMapLayers: ReferenceMapLayer[];
  onSelectPosition: (target: ScreenPositionTarget) => void;
  onMapReady?: (map: L.Map | null) => void;
  className?: string;
};

type ScreenMapProps = PositionMapProps & {
  t: TFunction;
};

const noopMapReady = () => undefined;

function getOfflineTileBase() {
  if (typeof window === "undefined") {
    return "";
  }
  const configuredBase = import.meta.env.VITE_BASE_PATH?.trim();
  if (configuredBase) {
    return configuredBase.replace(/\/+$/, "");
  }
  return "";
}

function parseStoredMapLayer(raw: string | null): ReferenceMapLayer | null {
  if (!raw) {
    return null;
  }
  if (referenceMapLayers.includes(raw as ReferenceMapLayer)) {
    return raw as ReferenceMapLayer;
  }

  try {
    const parsed = JSON.parse(raw) as unknown;
    if (typeof parsed === "string" && referenceMapLayers.includes(parsed as ReferenceMapLayer)) {
      return parsed as ReferenceMapLayer;
    }
    if (parsed && typeof parsed === "object" && "mapLayer" in parsed) {
      const layer = (parsed as { mapLayer?: unknown }).mapLayer;
      if (typeof layer === "string" && referenceMapLayers.includes(layer as ReferenceMapLayer)) {
        return layer as ReferenceMapLayer;
      }
    }
  } catch {
    // Ignore malformed storage values.
  }

  return null;
}

function getStoredMapLayer(): ReferenceMapLayer | null {
  if (typeof window === "undefined") {
    return null;
  }

  for (const key of [REFERENCE_MAP_LAYER_STORAGE_KEY, REFERENCE_LEGACY_MAP_LAYER_STORAGE_KEY]) {
    try {
      const layer = parseStoredMapLayer(window.localStorage.getItem(key));
      if (layer) {
        return layer;
      }
    } catch {
      // Ignore storage errors and continue to the next key.
    }
  }

  return null;
}

function persistMapLayer(layer: ReferenceMapLayer) {
  if (typeof window === "undefined") {
    return;
  }

  try {
    window.localStorage.setItem(REFERENCE_MAP_LAYER_STORAGE_KEY, JSON.stringify({ mapLayer: layer }));
    window.localStorage.removeItem(REFERENCE_LEGACY_MAP_LAYER_STORAGE_KEY);
  } catch {
    // Ignore storage errors.
  }
}

function getAvailableMapLayers(visibleMapLayers: ReferenceMapLayer[]) {
  const visibleSet = new Set(visibleMapLayers);
  const layers = referenceMapLayers.filter((key) => visibleSet.has(key));
  return layers.length ? layers : referenceMapLayers;
}

function resolveActiveMapLayer(
  storedLayer: ReferenceMapLayer | null,
  availableMapLayers: ReferenceMapLayer[],
  defaultMapLayer: ReferenceMapLayer,
) {
  if (storedLayer && availableMapLayers.includes(storedLayer)) {
    return storedLayer;
  }
  if (availableMapLayers.includes(defaultMapLayer)) {
    return defaultMapLayer;
  }
  return availableMapLayers[0] ?? REFERENCE_DEFAULT_MAP_LAYER;
}

function buildBaseLayers(): Record<ReferenceMapLayer, L.TileLayer> {
  return {
    "leaflet.map.gaodeMap": L.tileLayer(
      "https://webrd04.is.autonavi.com/appmaptile?lang=zh_cn&size=1&scale=1&style=7&x={x}&y={y}&z={z}",
      {
        coordFunction: "gps84ToGcj02",
      },
    ),
    "leaflet.map.gaodeSatellite": L.tileLayer("https://webst01.is.autonavi.com/appmaptile?style=6&x={x}&y={y}&z={z}", {
      coordFunction: "gps84ToGcj02",
      minZoom: 3,
      maxZoom: 16,
    }),
    "leaflet.map.googleMap": L.tileLayer("https://mt1.google.com/vt/lyrs=m&x={x}&y={y}&z={z}", {
      coordFunction: "gps84ToGcj02",
      maxZoom: 22,
    }),
    "leaflet.map.googleSatellite": L.tileLayer("https://mt1.google.com/vt/lyrs=s&x={x}&y={y}&z={z}", {
      maxZoom: 21,
    }),
    "leaflet.map.offlineMap": L.tileLayer(`${getOfflineTileBase()}/map/dt/{z}/{x}/{y}.jpg`),
  };
}

function createIcon(iconUrl: string, size: [number, number], className?: string) {
  return L.icon({
    iconUrl,
    iconSize: size,
    iconAnchor: [size[0] / 2, size[1]],
    className,
  });
}

function escapeHtml(value: string) {
  return value
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#039;");
}

function formatMapTime(value?: string) {
  if (!value) {
    return "-";
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return "-";
  }
  return date.toLocaleTimeString(i18n.language.startsWith("zh") ? "zh-CN" : "en-US", { hour12: false });
}

function formatCoordinate(point: ScreenPositionPoint) {
  return `${point.latitude.toFixed(6)}, ${point.longitude.toFixed(6)}`;
}

function formatMapFrequency(value?: number) {
  if (typeof value !== "number" || !Number.isFinite(value)) {
    return "-";
  }
  return `${Math.round(value)}MHz`;
}

function formatMapRSSI(value?: number) {
  if (typeof value !== "number" || !Number.isFinite(value)) {
    return "-";
  }
  return `${Math.round(value)}dBm`;
}

function formatMapOptionalNumber(value: number | undefined, unit: string, digits: number) {
  if (typeof value !== "number" || !Number.isFinite(value)) {
    return "-";
  }
  return `${value.toFixed(digits)}${unit}`;
}

function validHeading(value?: number): value is number {
  return typeof value === "number" && Number.isFinite(value);
}

function normalizeMapHeading(value: number) {
  return ((value % 360) + 360) % 360;
}

function formatDeviceHeading(value?: number) {
  if (!validHeading(value)) {
    return "-";
  }
  return `${Math.round(normalizeMapHeading(value))}°`;
}

type MapLegendItem = {
  id: string;
  label: string;
  kind: "marker" | "line";
  iconUrl?: string;
  iconClassName?: string;
  color?: string;
};

function buildMapLegendItems(t: TFunction) {
  return [
    {
      id: "device",
      label: t("deviceLocation", { ns: "screen" }),
      kind: "marker" as const,
      iconUrl: referenceMarkerIcons.detectionOnline,
    },
    {
      id: "drone-whitelist",
      label: t("whitelistDrone", { ns: "screen" }),
      kind: "marker" as const,
      iconUrl: referenceMarkerIcons.uav,
      iconClassName: "screen-legend-panel__icon--whitelist",
    },
    {
      id: "drone-unwhitelisted",
      label: t("unwhitelistedDrone", { ns: "screen" }),
      kind: "marker" as const,
      iconUrl: referenceMarkerIcons.uavBlackFly,
      iconClassName: "screen-legend-panel__icon--alert",
    },
    {
      id: "pilot-whitelist",
      label: t("whitelistPilot", { ns: "screen" }),
      kind: "marker" as const,
      iconUrl: referenceMarkerIcons.remote,
      iconClassName: "screen-legend-panel__icon--whitelist",
    },
    {
      id: "pilot-unwhitelisted",
      label: t("unwhitelistedPilot", { ns: "screen" }),
      kind: "marker" as const,
      iconUrl: referenceMarkerIcons.remoteBlackFly,
      iconClassName: "screen-legend-panel__icon--alert",
    },
    {
      id: "drone-trajectory",
      label: t("trajectory", { ns: "screen" }),
      kind: "line" as const,
      color: droneTrajectoryColor,
    },
    {
      id: "pilot-trajectory",
      label: t("pilotTrajectory", { ns: "screen" }),
      kind: "line" as const,
      color: pilotTrajectoryColor,
    },
  ] satisfies MapLegendItem[];
}

export function ScreenMapLegend({ t }: { t: TFunction }) {
  const items = useMemo(() => buildMapLegendItems(t), [t]);

  return (
    <details className="screen-legend-toggle">
      <summary className="screen-legend-trigger" aria-label={t("mapLegend", { ns: "screen" })} title={t("mapLegend", { ns: "screen" })}>
        <Info size={13} strokeWidth={2.4} aria-hidden="true" />
        <span className="sr-only">{t("mapLegend", { ns: "screen" })}</span>
      </summary>
      <div className="screen-legend-panel" role="note" aria-label={t("mapLegend", { ns: "screen" })}>
        <strong className="screen-legend-panel__title">{t("mapLegend", { ns: "screen" })}</strong>
        <div className="screen-legend-panel__items">
          {items.map((item) => (
            <div key={item.id} className={cx("screen-legend-panel__item", item.kind === "line" && "screen-legend-panel__item--line")}>
              {item.kind === "marker" ? (
                <img className={cx("screen-legend-panel__icon", item.iconClassName)} src={item.iconUrl} alt="" aria-hidden="true" />
              ) : (
                <span className="screen-legend-panel__line" aria-hidden="true" style={{ backgroundColor: item.color }} />
              )}
              <span>{item.label}</span>
            </div>
          ))}
        </div>
      </div>
    </details>
  );
}

function validMapPoint(point?: ScreenPositionPoint | null): point is ScreenPositionPoint {
  return Boolean(
    point &&
      Number.isFinite(point.latitude) &&
      Number.isFinite(point.longitude) &&
      point.latitude >= -90 &&
      point.latitude <= 90 &&
      point.longitude >= -180 &&
      point.longitude <= 180 &&
      !(point.latitude === 0 && point.longitude === 0),
  );
}

function validMapTrackPoint(point?: ScreenPositionTrackPoint | null): point is ScreenPositionTrackPoint {
  return Boolean(point && validMapPoint(point));
}

function toLatLng(point: ScreenPositionPoint) {
  return L.latLng(point.latitude, point.longitude);
}

function toTrackLatLngs(points?: ScreenPositionTrackPoint[]) {
  if (!points?.length) {
    return [];
  }
  return points.filter(validMapTrackPoint).map(toLatLng);
}

function collectRealMapPoints(deviceLocation: ScreenDeviceLocationResponse | null, positions: ScreenPositionTarget[]) {
  const points: L.LatLng[] = [];
  if (deviceLocation?.valid && validMapPoint(deviceLocation.point)) {
    points.push(toLatLng(deviceLocation.point));
  }

  positions.forEach((target) => {
    if (validMapPoint(target.drone)) {
      points.push(toLatLng(target.drone));
    }
    if (validMapPoint(target.pilot)) {
      points.push(toLatLng(target.pilot));
    }
    points.push(...toTrackLatLngs(target.droneTrajectory));
    points.push(...toTrackLatLngs(target.pilotTrajectory));
  });
  return points;
}

function fitBoundsPadding(map: L.Map) {
  const size = map.getSize();
  return {
    paddingTopLeft: L.point(Math.min(110, Math.max(32, size.x * 0.1)), Math.min(120, Math.max(36, size.y * 0.16))),
    paddingBottomRight: L.point(Math.min(520, Math.max(48, size.x * 0.28)), Math.min(120, Math.max(36, size.y * 0.16))),
  };
}

function fitRealScreenBounds(
  map: L.Map,
  deviceLocation: ScreenDeviceLocationResponse | null,
  positions: ScreenPositionTarget[],
) {
  const points = collectRealMapPoints(deviceLocation, positions);
  if (!points.length) {
    map.setView(REFERENCE_MAP_CENTER, REFERENCE_MAP_ZOOM);
    return;
  }
  if (points.length === 1) {
    map.setView(points[0], Math.max(map.getZoom(), 14), { animate: false });
    return;
  }

  map.fitBounds(L.latLngBounds(points), {
    ...fitBoundsPadding(map),
    maxZoom: 14,
  });
}

function createDeviceHeadingIcon(heading: number) {
  const normalized = normalizeMapHeading(heading);
  return L.divIcon({
    className: "screen-map-device-heading-marker",
    iconSize: [126, 126],
    iconAnchor: [63, 88],
    html: [
      `<svg class="screen-map-device-heading-marker__sector" style="--device-heading: ${normalized.toFixed(1)}deg" viewBox="0 0 126 126" aria-hidden="true" focusable="false">`,
      `<defs>`,
      `<radialGradient id="screenDeviceHeadingFanFill" cx="63" cy="88" r="76" gradientUnits="userSpaceOnUse">`,
      `<stop offset="0%" stop-color="currentColor" stop-opacity="0"></stop>`,
      `<stop offset="32%" stop-color="currentColor" stop-opacity="0.32"></stop>`,
      `<stop offset="72%" stop-color="currentColor" stop-opacity="0.26"></stop>`,
      `<stop offset="100%" stop-color="currentColor" stop-opacity="0.07"></stop>`,
      `</radialGradient>`,
      `<linearGradient id="screenDeviceHeadingFanArc" x1="42" y1="16" x2="84" y2="16" gradientUnits="userSpaceOnUse">`,
      `<stop offset="0%" stop-color="currentColor" stop-opacity="0.22"></stop>`,
      `<stop offset="50%" stop-color="currentColor" stop-opacity="1"></stop>`,
      `<stop offset="100%" stop-color="currentColor" stop-opacity="0.22"></stop>`,
      `</linearGradient>`,
      `<linearGradient id="screenDeviceHeadingFanBeam" x1="63" y1="70" x2="63" y2="15" gradientUnits="userSpaceOnUse">`,
      `<stop offset="0%" stop-color="currentColor" stop-opacity="0"></stop>`,
      `<stop offset="48%" stop-color="currentColor" stop-opacity="0.72"></stop>`,
      `<stop offset="100%" stop-color="currentColor" stop-opacity="0.18"></stop>`,
      `</linearGradient>`,
      `<mask id="screenDeviceHeadingFanMask" maskUnits="userSpaceOnUse">`,
      `<rect width="126" height="126" fill="white"></rect>`,
      `<circle cx="63" cy="88" r="18" fill="black"></circle>`,
      `</mask>`,
      `</defs>`,
      `<path class="screen-map-device-heading-marker__fan" fill="url(#screenDeviceHeadingFanFill)" d="M63 88 L42 16 A74 74 0 0 1 84 16 Z" mask="url(#screenDeviceHeadingFanMask)"></path>`,
      `<path class="screen-map-device-heading-marker__fan-beam" stroke="url(#screenDeviceHeadingFanBeam)" d="M63 70 L63 17"></path>`,
      `<path class="screen-map-device-heading-marker__fan-arc" stroke="url(#screenDeviceHeadingFanArc)" d="M42 16 A74 74 0 0 1 84 16"></path>`,
      `</svg>`,
    ].join(""),
  });
}

function deviceTooltipContent(deviceLocation: ScreenDeviceLocationResponse, heading: number | undefined, t: TFunction) {
  const point = deviceLocation.point;
  const sourceLabel = deviceLocation.source === "manual"
    ? t("deviceLocationManual", { ns: "screen" })
    : t("deviceLocationGps", { ns: "screen" });
  return [
    `<strong>${escapeHtml(t("deviceLocation", { ns: "screen" }))}</strong>`,
    escapeHtml(sourceLabel),
    point ? escapeHtml(formatCoordinate(point)) : "-",
    `${escapeHtml(t("deviceHeading", { ns: "screen" }))}: ${escapeHtml(formatDeviceHeading(heading))}`,
    `${escapeHtml(t("time", { ns: "screen" }))}: ${escapeHtml(formatMapTime(deviceLocation.updatedAt))}`,
  ].join("<br>");
}

function positionTooltipContent(target: ScreenPositionTarget, kind: "drone" | "pilot", point: ScreenPositionPoint, t: TFunction) {
  const title = target.model || t("unknownTarget", { ns: "screen" });
  const kindLabel = kind === "drone" ? t("positionDrone", { ns: "screen" }) : t("positionPilot", { ns: "screen" });
  return [
    `<strong>${escapeHtml(kindLabel)}</strong>`,
    escapeHtml(title),
    escapeHtml(target.serial || target.id),
    escapeHtml(formatCoordinate(point)),
    `${escapeHtml(t("frequency", { ns: "screen" }))}: ${escapeHtml(formatMapFrequency(target.frequency))}`,
    `${escapeHtml(t("signalStrength", { ns: "screen" }))}: ${escapeHtml(formatMapRSSI(target.rssi))}`,
  ].join("<br>");
}

function trajectoryTooltipContent(
  target: ScreenPositionTarget,
  kind: "drone" | "pilot",
  point: ScreenPositionTrackPoint,
  t: TFunction,
) {
  const title = target.model || t("unknownTarget", { ns: "screen" });
  const kindLabel = kind === "drone" ? t("positionDrone", { ns: "screen" }) : t("positionPilot", { ns: "screen" });
  return [
    `<strong>${escapeHtml(kindLabel)}</strong>`,
    escapeHtml(title),
    escapeHtml(target.serial || target.id),
    escapeHtml(formatCoordinate(point)),
    `${escapeHtml(t("speed", { ns: "screen" }))}: ${escapeHtml(formatMapOptionalNumber(point.speed, "m/s", 1))}`,
    `${escapeHtml(t("height", { ns: "screen" }))}: ${escapeHtml(formatMapOptionalNumber(point.height, "m", 0))}`,
    `${escapeHtml(t("time", { ns: "screen" }))}: ${escapeHtml(formatMapTime(point.time))}`,
  ].join("<br>");
}

function renderDeviceLayer(
  map: L.Map,
  deviceLocation: ScreenDeviceLocationResponse | null,
  deviceHeadingDeg: number | undefined,
  t: TFunction,
) {
  const group = L.layerGroup().addTo(map);
  if (!deviceLocation?.valid || !validMapPoint(deviceLocation.point)) {
    return group;
  }

  const latLng: L.LatLngExpression = [deviceLocation.point.latitude, deviceLocation.point.longitude];
  if (validHeading(deviceHeadingDeg)) {
    L.marker(latLng, {
      icon: createDeviceHeadingIcon(deviceHeadingDeg),
      pane: markerPane,
      interactive: false,
      keyboard: false,
      zIndexOffset: -30,
    }).addTo(group);
  }

  const className = deviceLocation.source === "manual"
    ? "screen-map-device-marker screen-map-device-marker--manual"
    : "screen-map-device-marker";
  L.marker(latLng, {
    icon: createIcon(referenceMarkerIcons.detectionOnline, deviceIconSize, className),
    pane: markerPane,
    riseOnHover: true,
    alt: t("deviceLocation", { ns: "screen" }),
  })
    .bindTooltip(deviceTooltipContent(deviceLocation, deviceHeadingDeg, t), {
      direction: "top",
      offset: [0, -deviceIconSize[1]],
      className: "module-location-tooltip screen-map-tooltip",
      opacity: 0.92,
    })
    .addTo(group);

  return group;
}

function renderTrajectoryLayer(
  group: L.LayerGroup,
  target: ScreenPositionTarget,
  kind: "drone" | "pilot",
  selected: boolean,
  onSelectPosition: (target: ScreenPositionTarget) => void,
  t: TFunction,
) {
  const trajectory = kind === "drone" ? target.droneTrajectory : target.pilotTrajectory;
  const points = toTrackLatLngs(trajectory);
  if (points.length < 2) {
    return;
  }

  const color = kind === "drone" ? droneTrajectoryColor : pilotTrajectoryColor;
  L.polyline(points, {
    color,
    weight: selected ? 4 : 2.5,
    opacity: selected ? 0.92 : 0.55,
    pane: trajectoryPane,
    className: selected ? "screen-map-trajectory screen-map-trajectory--selected" : "screen-map-trajectory",
  })
    .on("click", () => onSelectPosition(target))
    .addTo(group);

  const lastPoint = trajectory?.filter(validMapTrackPoint).at(-1);
  if (!lastPoint) {
    return;
  }

  L.circleMarker([lastPoint.latitude, lastPoint.longitude], {
    radius: selected ? 4.5 : 3,
    color,
    weight: 1,
    fillColor: color,
    fillOpacity: selected ? 0.9 : 0.58,
    opacity: selected ? 0.95 : 0.65,
    pane: trajectoryPane,
    className: "screen-map-trajectory-point",
  })
    .on("click", () => onSelectPosition(target))
    .bindTooltip(trajectoryTooltipContent(target, kind, lastPoint, t), {
      direction: "top",
      className: "screen-map-tooltip",
      opacity: 0.92,
    })
    .addTo(group);
}

function renderPositionLayers(
  map: L.Map,
  positions: ScreenPositionTarget[],
  selectedId: string,
  whitelist: WhitelistItem[] | undefined,
  onSelectPosition: (target: ScreenPositionTarget) => void,
  t: TFunction,
) {
  const group = L.layerGroup().addTo(map);

  positions.forEach((target) => {
    const selected = selectedId === target.id;
    const whitelisted = isSerialWhitelisted(target.serial, whitelist);
    const markerClassName = cx(
      selected && "screen-reference-marker-selected",
      !whitelisted && "screen-reference-marker-alert",
    );
    const remoteIcon = whitelisted
      ? selected ? referenceMarkerIcons.selectedRemote : referenceMarkerIcons.remote
      : selected ? referenceMarkerIcons.selectedRemoteBlackFly : referenceMarkerIcons.remoteBlackFly;
    const uavIcon = whitelisted
      ? selected ? referenceMarkerIcons.selectedUav : referenceMarkerIcons.uav
      : selected ? referenceMarkerIcons.selectedUavBlackFly : referenceMarkerIcons.uavBlackFly;
    renderTrajectoryLayer(group, target, "pilot", selected, onSelectPosition, t);
    renderTrajectoryLayer(group, target, "drone", selected, onSelectPosition, t);

    if (validMapPoint(target.pilot)) {
      L.marker([target.pilot.latitude, target.pilot.longitude], {
        icon: createIcon(remoteIcon, targetIconSize, markerClassName),
        pane: selected ? selectedPane : markerPane,
        riseOnHover: true,
        alt: `${target.serial || target.id}-pilot`,
      })
        .on("click", () => onSelectPosition(target))
        .bindTooltip(positionTooltipContent(target, "pilot", target.pilot, t), {
          direction: "top",
          offset: [0, -targetIconSize[1]],
          className: "screen-map-tooltip",
          opacity: 0.92,
        })
        .addTo(group);
    }

    if (validMapPoint(target.drone)) {
      L.marker([target.drone.latitude, target.drone.longitude], {
        icon: createIcon(uavIcon, targetIconSize, markerClassName),
        pane: selected ? selectedPane : markerPane,
        riseOnHover: true,
        alt: `${target.serial || target.id}-drone`,
      })
        .on("click", () => onSelectPosition(target))
        .bindTooltip(positionTooltipContent(target, "drone", target.drone, t), {
          direction: "top",
          offset: [0, -targetIconSize[1]],
          className: "screen-map-tooltip",
          opacity: 0.92,
        })
        .addTo(group);
    }
  });

  return group;
}

export function PositionMap({
  selectedId,
  positions,
  whitelist,
  deviceLocation,
  deviceHeadingDeg,
  visibleMapLayers,
  onSelectPosition,
  onMapReady = noopMapReady,
  className,
}: PositionMapProps) {
  const { t, i18n: i18nInstance } = useTranslation();
  const containerRef = useRef<HTMLDivElement | null>(null);
  const mapRef = useRef<L.Map | null>(null);
  const deviceLayerRef = useRef<L.LayerGroup | null>(null);
  const positionLayerRef = useRef<L.LayerGroup | null>(null);
  const onSelectPositionRef = useRef(onSelectPosition);
  const selectedIdRef = useRef(selectedId);
  const realDataRef = useRef<RealMapData>({ deviceLocation, deviceHeadingDeg, positions, whitelist });
  const hasFitRealBoundsRef = useRef(false);
  const visibleMapLayersKey = visibleMapLayers.join("|");
  const defaultMapLayer = referenceDefaultMapLayerForLocale(i18nInstance.language);

  useEffect(() => {
    onSelectPositionRef.current = onSelectPosition;
  }, [onSelectPosition]);

  useEffect(() => {
    selectedIdRef.current = selectedId;
  }, [selectedId]);

  useEffect(() => {
    realDataRef.current = { deviceLocation, deviceHeadingDeg, positions, whitelist };
  }, [deviceHeadingDeg, deviceLocation, positions, whitelist]);

  const layerLabels = useMemo(() => {
    return Object.fromEntries(referenceMapLayers.map((key) => [key, t(key, { ns: "screen" })])) as Record<ReferenceMapLayer, string>;
  }, [t]);
  const layerLabelsRef = useRef(layerLabels);

  useEffect(() => {
    layerLabelsRef.current = layerLabels;
  }, [layerLabels]);

  useEffect(() => {
    const container = containerRef.current;
    if (!container || mapRef.current) {
      return;
    }

    const map = L.map(container, {
      center: REFERENCE_MAP_CENTER,
      zoom: REFERENCE_MAP_ZOOM,
      zoomControl: false,
      attributionControl: false,
    });

    map.createPane(markerPane);
    map.createPane(trajectoryPane);
    map.createPane(selectedPane);
    const markerPaneElement = map.getPane(markerPane);
    const trajectoryPaneElement = map.getPane(trajectoryPane);
    const selectedPaneElement = map.getPane(selectedPane);
    if (trajectoryPaneElement) {
      trajectoryPaneElement.style.zIndex = "580";
    }
    if (markerPaneElement) {
      markerPaneElement.style.zIndex = "610";
    }
    if (selectedPaneElement) {
      selectedPaneElement.style.zIndex = "660";
    }

    const availableMapLayers = getAvailableMapLayers(visibleMapLayers);
    const baseLayers = buildBaseLayers();
    const storedLayer = getStoredMapLayer();
    const activeLayer = resolveActiveMapLayer(storedLayer, availableMapLayers, defaultMapLayer);
    baseLayers[activeLayer].addTo(map);

    const customButtons = createDrawControlButtonGroup([
      {
        title: t("screenPage.center", { ns: "screen" }),
        contentType: "image",
        text: centerPointIcon,
        className: "center-point-button",
        onClick: () => {
          const data = realDataRef.current;
          fitRealScreenBounds(map, data.deviceLocation, data.positions);
        },
      },
    ]);
    map.addControl(customButtons);
    map.addControl(
      new L.Control.Zoom({
        zoomInTitle: t("leaflet.map.ZoomIn", { ns: "screen" }),
        zoomOutTitle: t("leaflet.map.ZoomOut", { ns: "screen" }),
      }),
    );

    mapRef.current = map;
    const data = realDataRef.current;
    deviceLayerRef.current = renderDeviceLayer(map, data.deviceLocation, data.deviceHeadingDeg, t);
    positionLayerRef.current = renderPositionLayers(
      map,
      data.positions,
      selectedIdRef.current,
      data.whitelist,
      (target) => onSelectPositionRef.current(target),
      t,
    );

    const labels = layerLabelsRef.current;
    const layersControl = new L.Control.Layers(
      Object.fromEntries(availableMapLayers.map((key) => [labels[key], baseLayers[key]])),
      {},
      {
        position: "topleft",
      },
    );

    map.addControl(layersControl);
    map.on("baselayerchange", (event: L.LayersControlEvent) => {
      const nextLayer = availableMapLayers.find((key) => layerLabelsRef.current[key] === event.name);
      if (!nextLayer) {
        console.warn(event.name);
        return;
      }
      persistMapLayer(nextLayer);
    });

    const fitTimer = window.setTimeout(() => {
      if (mapRef.current !== map) {
        return;
      }
      map.invalidateSize();
      if (collectRealMapPoints(data.deviceLocation, data.positions).length) {
        fitRealScreenBounds(map, data.deviceLocation, data.positions);
        hasFitRealBoundsRef.current = true;
      }
    }, 0);
    onMapReady(map);

    return () => {
      window.clearTimeout(fitTimer);
      map.remove();
      mapRef.current = null;
      deviceLayerRef.current = null;
      positionLayerRef.current = null;
      hasFitRealBoundsRef.current = false;
      onMapReady(null);
    };
  }, [defaultMapLayer, onMapReady, t, visibleMapLayersKey]);

  useEffect(() => {
    const map = mapRef.current;
    if (!map) {
      return;
    }

    if (deviceLayerRef.current) {
      map.removeLayer(deviceLayerRef.current);
    }
    if (positionLayerRef.current) {
      map.removeLayer(positionLayerRef.current);
    }

    deviceLayerRef.current = renderDeviceLayer(map, deviceLocation, deviceHeadingDeg, t);
    positionLayerRef.current = renderPositionLayers(
      map,
      positions,
      selectedId,
      whitelist,
      (target) => onSelectPositionRef.current(target),
      t,
    );

    if (!hasFitRealBoundsRef.current && collectRealMapPoints(deviceLocation, positions).length) {
      fitRealScreenBounds(map, deviceLocation, positions);
      hasFitRealBoundsRef.current = true;
    }
  }, [deviceHeadingDeg, deviceLocation, positions, selectedId, t, whitelist]);

  return (
    <div className={cx("screen-map-shell", className)}>
      <div ref={containerRef} className="screen-map dark" />
    </div>
  );
}

export function ScreenMap({ t: _t, ...props }: ScreenMapProps) {
  return <PositionMap {...props} />;
}
