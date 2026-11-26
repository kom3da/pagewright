import { useSuspenseQuery } from "@tanstack/react-query";
import { meQuery } from "../api/queries";

/** Whether the user is an admin of the selected site (editors cannot change settings). */
export function useIsAdmin() {
  const { data: me } = useSuspenseQuery(meQuery);
  return me.sites.find((s) => s.id === me.currentSiteId)?.role === "admin";
}
