let pageGestureGuardsInstalled = false;

const nonPassiveOptions: AddEventListenerOptions = { passive: false };

const preventDefault = (event: Event) => {
  if (event.cancelable) {
    event.preventDefault();
  }
};

export function installPageGestureGuards() {
  if (pageGestureGuardsInstalled) {
    return;
  }

  pageGestureGuardsInstalled = true;

  let lastTouchEnd = 0;

  document.addEventListener("gesturestart", preventDefault, nonPassiveOptions);
  document.addEventListener("gesturechange", preventDefault, nonPassiveOptions);
  document.addEventListener("gestureend", preventDefault, nonPassiveOptions);

  document.addEventListener(
    "touchmove",
    (event) => {
      if (event.touches.length > 1) {
        preventDefault(event);
      }
    },
    nonPassiveOptions,
  );

  document.addEventListener(
    "touchend",
    (event) => {
      const now = Date.now();

      if (now - lastTouchEnd <= 300) {
        preventDefault(event);
      }

      lastTouchEnd = now;
    },
    nonPassiveOptions,
  );

  document.addEventListener(
    "wheel",
    (event) => {
      if (event.ctrlKey) {
        preventDefault(event);
      }
    },
    nonPassiveOptions,
  );
}
