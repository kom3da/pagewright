/**
 * Fallback origin of the public (static) site, for the mock and for development. The real
 * one comes from the server (`publicBaseUrl` of GET /api/site): see `usePublicUrl`.
 */
export const devPublicSiteOrigin: string = (
  import.meta.env.VITE_PUBLIC_SITE_ORIGIN || "http://localhost:8081"
).replace(/\/$/, "");

export const appName = "Pagewright 管理画面";
