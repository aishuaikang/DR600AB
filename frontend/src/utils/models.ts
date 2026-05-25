export interface ModelLike {
  displayModel?: string;
  model?: string;
}

export function resolveDisplayModel(value?: ModelLike | null) {
  const displayModel = value?.displayModel?.trim();
  if (displayModel) {
    return displayModel;
  }
  const model = value?.model?.trim();
  return model || "";
}
