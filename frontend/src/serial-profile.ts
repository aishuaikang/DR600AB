export const FIXED_SERIAL_PROFILE = {
  baudRate: 115200,
  dataBits: 8,
  stopBits: 1,
  parity: "none",
  readTimeoutMs: 1000,
} as const;

export const COMPASS_SERIAL_PROFILE = {
  ...FIXED_SERIAL_PROFILE,
  baudRate: 9600,
} as const;

export const DETECTION_DEFAULT_RX_BAUD_RATE = 115200;
export const DETECTION_DEFAULT_TX_BAUD_RATE = 460800;
export const DETECTION_DEFAULT_BAUD_RATE = DETECTION_DEFAULT_RX_BAUD_RATE;

export const SERIAL_BAUD_RATE_LIMITS = {
  min: 1200,
  max: 3000000,
} as const;

export function normalizeSerialBaudRate(value: number | undefined, fallback = DETECTION_DEFAULT_RX_BAUD_RATE) {
  const baudRate = Math.trunc(Number(value));
  if (!Number.isFinite(baudRate)) {
    return fallback;
  }
  if (baudRate < SERIAL_BAUD_RATE_LIMITS.min || baudRate > SERIAL_BAUD_RATE_LIMITS.max) {
    return fallback;
  }
  return baudRate;
}
