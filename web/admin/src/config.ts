/** Origin of the public (static) site, used to link to published pages. */
export const publicSiteOrigin: string = (
  import.meta.env.VITE_PUBLIC_SITE_ORIGIN || "http://localhost:8081"
).replace(/\/$/, "");

export const publicUrl = (path: string) => `${publicSiteOrigin}${path}`;

export const appName = "Pagewright 管理画面";
