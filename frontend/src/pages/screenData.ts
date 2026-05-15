import detectionDeviceIconOfflineUrl from "../assets/images/detectionDeviceIconOffline.svg";
import detectionDeviceIconOnlineUrl from "../assets/images/detectionDeviceIconOnline.svg";
import remoteControlBlackFlyIconUrl from "../assets/images/remoteControlBlackFlyIcon.svg";
import remoteControlIconUrl from "../assets/images/remoteControlIcon.svg";
import selectedRemoteControlBlackFlyIconUrl from "../assets/images/selectedRemoteControlBlackFlyIcon.svg";
import selectedRemoteControlIconUrl from "../assets/images/selectedRemoteControlIcon.svg";
import selectedUavBlackFlyIconUrl from "../assets/images/selectedUavBlackFlyIcon.svg";
import selectedUavIconUrl from "../assets/images/selectedUavIcon.svg";
import uavBlackFlyIconUrl from "../assets/images/uavBlackFlyIcon.svg";
import uavIconUrl from "../assets/images/uavIcon.svg";

export type ScreenAlertKind = "detection" | "position" | "fpv";

export type ScreenAlertStatus = "tracking" | "warning" | "lost";

export type ReferenceMapLayer =
  | "leaflet.map.gaodeMap"
  | "leaflet.map.gaodeSatellite"
  | "leaflet.map.googleMap"
  | "leaflet.map.googleSatellite"
  | "leaflet.map.offlineMap";

export interface ScreenDevice {
  id: string;
  nameKey: string;
  sn: string;
  type: "detector" | "jammer";
  lat: number;
  lng: number;
  online: boolean;
}

export interface ScreenAlert {
  id: string;
  kind: ScreenAlertKind;
  nameKey: string;
  sn: string;
  frequencyMHz: number;
  orientation: number;
  rssi: number;
  lat: number;
  lng: number;
  rcLat: number;
  rcLng: number;
  time: string;
  status: ScreenAlertStatus;
  inWhiteList: boolean;
}

export const REFERENCE_MAP_CENTER: [number, number] = [39.909181, 116.397472];
export const REFERENCE_MAP_ZOOM = 13;
export const REFERENCE_MAP_LAYER_STORAGE_KEY = "mapLayer";
export const REFERENCE_DEFAULT_MAP_LAYER: ReferenceMapLayer = "leaflet.map.gaodeSatellite";

export const referenceMapLayers: ReferenceMapLayer[] = [
  "leaflet.map.gaodeMap",
  "leaflet.map.gaodeSatellite",
  "leaflet.map.googleMap",
  "leaflet.map.googleSatellite",
  "leaflet.map.offlineMap",
];

export const screenDevices: ScreenDevice[] = [
  {
    id: "dev-north",
    nameKey: "devices.northDetector",
    sn: "DR600-N-01",
    type: "detector",
    lat: 39.9262,
    lng: 116.3919,
    online: true,
  },
  {
    id: "dev-east",
    nameKey: "devices.eastJammer",
    sn: "DR600-E-02",
    type: "jammer",
    lat: 39.9138,
    lng: 116.4298,
    online: true,
  },
  {
    id: "dev-south",
    nameKey: "devices.southDetector",
    sn: "DR600-S-03",
    type: "detector",
    lat: 39.8858,
    lng: 116.3904,
    online: false,
  },
];

export const screenAlerts: ScreenAlert[] = [
  {
    id: "alert-001",
    kind: "detection",
    nameKey: "alerts.mini4Pro",
    sn: "UAV-8F12",
    frequencyMHz: 5745,
    orientation: 43,
    rssi: -58,
    lat: 39.9172,
    lng: 116.4129,
    rcLat: 39.9212,
    rcLng: 116.4028,
    time: "17:18:32",
    status: "tracking",
    inWhiteList: true,
  },
  {
    id: "alert-002",
    kind: "position",
    nameKey: "alerts.remoteIdTarget",
    sn: "RID-2A91",
    frequencyMHz: 2440,
    orientation: 282,
    rssi: -66,
    lat: 39.9008,
    lng: 116.3748,
    rcLat: 39.906,
    rcLng: 116.3855,
    time: "17:17:46",
    status: "warning",
    inWhiteList: false,
  },
  {
    id: "alert-003",
    kind: "fpv",
    nameKey: "alerts.fpvSignal",
    sn: "FPV-77C0",
    frequencyMHz: 5865,
    orientation: 118,
    rssi: -71,
    lat: 39.8915,
    lng: 116.4219,
    rcLat: 39.8845,
    rcLng: 116.4109,
    time: "17:16:19",
    status: "tracking",
    inWhiteList: false,
  },
  {
    id: "alert-004",
    kind: "detection",
    nameKey: "alerts.unknownUav",
    sn: "UAV-RAW",
    frequencyMHz: 2472,
    orientation: 331,
    rssi: -79,
    lat: 39.9345,
    lng: 116.3656,
    rcLat: 39.924,
    rcLng: 116.3688,
    time: "17:15:04",
    status: "lost",
    inWhiteList: false,
  },
];

export const referenceMarkerIcons = {
  remote: remoteControlIconUrl,
  selectedRemote: selectedRemoteControlIconUrl,
  remoteBlackFly: remoteControlBlackFlyIconUrl,
  selectedRemoteBlackFly: selectedRemoteControlBlackFlyIconUrl,
  uav: uavIconUrl,
  selectedUav: selectedUavIconUrl,
  uavBlackFly: uavBlackFlyIconUrl,
  selectedUavBlackFly: selectedUavBlackFlyIconUrl,
  detectionOnline: detectionDeviceIconOnlineUrl,
  detectionOffline: detectionDeviceIconOfflineUrl,
} as const;
