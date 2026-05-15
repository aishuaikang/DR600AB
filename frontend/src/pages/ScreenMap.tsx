import { useEffect, useMemo, useRef } from "react";
import type { TFunction } from "i18next";
import L from "leaflet";
import "leaflet/dist/leaflet.css";

import centerPointIcon from "../assets/images/centerPoint.svg";
import compassIcon from "../assets/images/compass.png";
import i18n from "../i18n";
import { createDrawControlButtonGroup } from "../utils/leafletControls";
import { installLeafletCoordConverter } from "../utils/leafletCoordConverter";
import {
  REFERENCE_DEFAULT_MAP_LAYER,
  REFERENCE_MAP_CENTER,
  REFERENCE_MAP_LAYER_STORAGE_KEY,
  REFERENCE_MAP_ZOOM,
  referenceMapLayers,
  referenceMarkerIcons,
  screenAlerts,
  screenDevices,
  type ReferenceMapLayer,
  type ScreenAlert,
} from "./screenData";

installLeafletCoordConverter();

const markerPane = "screenMarkers";
const selectedPane = "screenSelectedMarkers";

function getOfflineTileBase() {
  if (typeof window === "undefined") {
    return "";
  }
  return import.meta.env.DEV ? import.meta.env.VITE_BASE_PATH || "" : `http://${window.location.hostname}:8099`;
}

function getStoredMapLayer(): ReferenceMapLayer {
  if (typeof window === "undefined") {
    return REFERENCE_DEFAULT_MAP_LAYER;
  }

  try {
    const stored = window.localStorage.getItem(REFERENCE_MAP_LAYER_STORAGE_KEY);
    if (stored && referenceMapLayers.includes(stored as ReferenceMapLayer)) {
      return stored as ReferenceMapLayer;
    }
    if (stored) {
      const parsed = JSON.parse(stored) as { mapLayer?: string };
      if (parsed.mapLayer && referenceMapLayers.includes(parsed.mapLayer as ReferenceMapLayer)) {
        return parsed.mapLayer as ReferenceMapLayer;
      }
    }
  } catch {
    // Ignore storage errors and use the reference default.
  }

  return REFERENCE_DEFAULT_MAP_LAYER;
}

function persistMapLayer(layer: ReferenceMapLayer) {
  try {
    window.localStorage.setItem(REFERENCE_MAP_LAYER_STORAGE_KEY, JSON.stringify({ mapLayer: layer }));
  } catch {
    // Ignore storage errors.
  }
}

function getAvailableMapLayers() {
  if (i18n.language.startsWith("zh")) {
    return referenceMapLayers;
  }
  return referenceMapLayers.filter((key) =>
    ["leaflet.map.googleMap", "leaflet.map.googleSatellite", "leaflet.map.offlineMap"].includes(key),
  );
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

function fitScreenBounds(map: L.Map) {
  const points = [
    ...screenDevices.map((item) => L.latLng(item.lat, item.lng)),
    ...screenAlerts.flatMap((item) => [L.latLng(item.lat, item.lng), L.latLng(item.rcLat, item.rcLng)]),
  ];

  if (!points.length) {
    map.setView(REFERENCE_MAP_CENTER, REFERENCE_MAP_ZOOM);
    return;
  }

  map.fitBounds(L.latLngBounds(points), {
    paddingTopLeft: L.point(650, 150),
    paddingBottomRight: L.point(550, 150),
    maxZoom: 14,
  });
}

function renderStaticLayers(map: L.Map, t: TFunction) {
  const deviceGroup = L.layerGroup().addTo(map);

  screenDevices.forEach((device) => {
    L.marker([device.lat, device.lng], {
      icon: createIcon(device.online ? referenceMarkerIcons.detectionOnline : referenceMarkerIcons.detectionOffline, [198 / 5, 256 / 5]),
      pane: markerPane,
      riseOnHover: true,
      alt: device.sn,
    })
      .bindTooltip(`${t(device.nameKey, { ns: "screen" })}<br>${device.sn}`, {
        direction: "top",
        offset: [0, -(256 / 5)],
        className: "module-location-tooltip",
        opacity: 0.9,
      })
      .addTo(deviceGroup);
  });

  return deviceGroup;
}

function renderAlertLayers(map: L.Map, selectedId: string, onSelectAlert: (alert: ScreenAlert) => void, t: TFunction) {
  const group = L.layerGroup().addTo(map);

  screenAlerts.forEach((alert) => {
    const isSelected = selectedId === alert.id;
    const uavIconUrl = alert.inWhiteList
      ? isSelected
        ? referenceMarkerIcons.selectedUav
        : referenceMarkerIcons.uav
      : isSelected
        ? referenceMarkerIcons.selectedUavBlackFly
        : referenceMarkerIcons.uavBlackFly;
    const remoteIconUrl = alert.inWhiteList
      ? isSelected
        ? referenceMarkerIcons.selectedRemote
        : referenceMarkerIcons.remote
      : isSelected
        ? referenceMarkerIcons.selectedRemoteBlackFly
        : referenceMarkerIcons.remoteBlackFly;

    L.marker([alert.lat, alert.lng], {
      icon: createIcon(uavIconUrl, [198 / 5, 256 / 5], isSelected ? "screen-reference-marker-selected" : undefined),
      pane: isSelected ? selectedPane : markerPane,
      riseOnHover: true,
      alt: alert.sn,
    })
      .on("click", () => onSelectAlert(alert))
      .bindTooltip(`${t(alert.nameKey, { ns: "screen" })}<br>${alert.frequencyMHz}MHz / ${alert.rssi}dBm`, {
        direction: "top",
        offset: [0, -(256 / 5)],
        opacity: 0.9,
      })
      .addTo(group);

    L.marker([alert.rcLat, alert.rcLng], {
      icon: createIcon(remoteIconUrl, [160 / 5, 256 / 5]),
      pane: markerPane,
      riseOnHover: true,
      alt: `${alert.sn}-remote`,
    })
      .on("click", () => onSelectAlert(alert))
      .bindTooltip(`${t("remoteController", { ns: "screen" })}<br>${alert.sn}`, {
        direction: "top",
        offset: [0, -(256 / 5)],
        opacity: 0.9,
      })
      .addTo(group);
  });

  return group;
}

export function ScreenMap({
  t,
  selectedId,
  onSelectAlert,
  onMapReady,
}: {
  t: TFunction;
  selectedId: string;
  onSelectAlert: (alert: ScreenAlert) => void;
  onMapReady: (map: L.Map | null) => void;
}) {
  const containerRef = useRef<HTMLDivElement | null>(null);
  const mapRef = useRef<L.Map | null>(null);
  const alertLayerRef = useRef<L.LayerGroup | null>(null);
  const onSelectAlertRef = useRef(onSelectAlert);
  const selectedIdRef = useRef(selectedId);

  useEffect(() => {
    onSelectAlertRef.current = onSelectAlert;
  }, [onSelectAlert]);

  useEffect(() => {
    selectedIdRef.current = selectedId;
  }, [selectedId]);

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
    map.createPane(selectedPane);
    const markerPaneElement = map.getPane(markerPane);
    const selectedPaneElement = map.getPane(selectedPane);
    if (markerPaneElement) {
      markerPaneElement.style.zIndex = "610";
    }
    if (selectedPaneElement) {
      selectedPaneElement.style.zIndex = "660";
    }

    const availableMapLayers = getAvailableMapLayers();
    const baseLayers = buildBaseLayers();
    const storedLayer = getStoredMapLayer();
    const activeLayer = availableMapLayers.includes(storedLayer) ? storedLayer : "leaflet.map.googleSatellite";
    baseLayers[activeLayer].addTo(map);

    const customButtons = createDrawControlButtonGroup([
      {
        title: t("screenPage.compass", { ns: "screen" }),
        contentType: "image",
        text: compassIcon,
        className: "compass-button",
        onClick: () => {},
      },
      {
        title: t("screenPage.center", { ns: "screen" }),
        contentType: "image",
        text: centerPointIcon,
        className: "center-point-button",
        onClick: () => fitScreenBounds(map),
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
    renderStaticLayers(map, t);
    alertLayerRef.current = renderAlertLayers(map, selectedIdRef.current, (alert) => onSelectAlertRef.current(alert), t);

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
      if (mapRef.current === map) {
        fitScreenBounds(map);
      }
    }, 0);
    onMapReady(map);

    return () => {
      window.clearTimeout(fitTimer);
      map.remove();
      mapRef.current = null;
      alertLayerRef.current = null;
      onMapReady(null);
    };
  }, [onMapReady, t]);

  useEffect(() => {
    const map = mapRef.current;
    if (!map) {
      return;
    }

    if (alertLayerRef.current) {
      map.removeLayer(alertLayerRef.current);
    }
    alertLayerRef.current = renderAlertLayers(map, selectedId, (alert) => onSelectAlertRef.current(alert), t);

    const selectedAlert = screenAlerts.find((alert) => alert.id === selectedId);
    if (selectedAlert) {
      map.setView([selectedAlert.lat, selectedAlert.lng], Math.max(map.getZoom(), 14), { animate: false });
    }
  }, [selectedId, t]);

  return <div id="lmap" ref={containerRef} className="screen-map dark" />;
}
