import { useCallback, useEffect, useState } from "react";

import { normalizePage } from "../app/navigation";
import type { Page } from "../app/types";

export function useHashPage(): [Page, (page: Page) => void] {
  const [page, setPage] = useState<Page>(() =>
    typeof window === "undefined" ? "did_encrypted" : normalizePage(window.location.hash),
  );

  useEffect(() => {
    const onHashChange = () => setPage(normalizePage(window.location.hash));
    window.addEventListener("hashchange", onHashChange);
    if (!window.location.hash) {
      window.location.hash = "#/did_encrypted";
    }
    return () => window.removeEventListener("hashchange", onHashChange);
  }, []);

  const navigate = useCallback((next: Page) => {
    window.location.hash = `#/${next}`;
  }, []);

  return [page, navigate];
}
