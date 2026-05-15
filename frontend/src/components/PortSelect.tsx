import type { ReactNode } from "react";

import type { PortInfo } from "../types";

function SelectField({
  label,
  value,
  disabled,
  children,
  onChange,
}: {
  label: string;
  value: string;
  disabled?: boolean;
  children: ReactNode;
  onChange: (value: string) => void;
}) {
  return (
    <label className="grid min-w-0 gap-1.5">
      <span className="text-xs font-medium text-base-content/60">{label}</span>
      <select
        className="select select-sm select-primary w-full bg-base-100"
        value={value}
        onChange={(event) => onChange(event.target.value)}
        disabled={disabled}
      >
        {children}
      </select>
    </label>
  );
}

export function PortSelect({
  label,
  placeholder,
  value,
  ports,
  activeText,
  onChange,
}: {
  label: string;
  placeholder: string;
  value: string;
  ports: PortInfo[];
  activeText: string;
  onChange: (value: string) => void;
}) {
  const hasCurrent = Boolean(value) && !ports.some((port) => port.name === value);

  return (
    <SelectField label={label} value={value} disabled={ports.length === 0 && !hasCurrent} onChange={onChange}>
      <option value="">{placeholder}</option>
      {hasCurrent ? <option value={value}>{value}</option> : null}
      {ports.map((port) => (
        <option key={port.name} value={port.name}>
          {port.active ? `${port.name} (${activeText})` : port.name}
        </option>
      ))}
    </SelectField>
  );
}
