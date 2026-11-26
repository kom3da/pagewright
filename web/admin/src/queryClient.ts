import { MutationCache, QueryCache, QueryClient } from "@tanstack/react-query";
import { ApiError } from "./api/client";

/**
 * Creates the QueryClient. `onApiError` is called for every failed query or mutation so
 * the app can react to 401 (go to login) and 403 no_site (go to site selection) in one place.
 */
export function createQueryClient(onApiError: (e: ApiError) => void = () => {}) {
  const handle = (e: unknown) => {
    if (e instanceof ApiError) onApiError(e);
  };
  return new QueryClient({
    queryCache: new QueryCache({ onError: handle }),
    mutationCache: new MutationCache({ onError: handle }),
    defaultOptions: {
      queries: {
        // Client errors (4xx) will not fix themselves by retrying.
        retry: (count, e) =>
          !(e instanceof ApiError && e.status >= 400 && e.status < 500) && count < 2,
        refetchOnWindowFocus: false,
      },
      mutations: { retry: false },
    },
  });
}
