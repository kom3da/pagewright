import { useQuery } from "@tanstack/react-query";
import { siteQuery } from "../api/queries";
import { devPublicSiteOrigin } from "../config";

/**
 * Builds links to the public site from the server's `publicBaseUrl` (GET /api/site). Until
 * that is loaded, or when the server sends none, the build-time fallback is used.
 */
export function usePublicUrl() {
  const { data } = useQuery(siteQuery);
  const base = (data?.publicBaseUrl || devPublicSiteOrigin).replace(/\/$/, "");
  return (path = "/") => `${base}${path}`;
}
