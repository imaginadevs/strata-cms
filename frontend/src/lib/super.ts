import { useQuery } from "@tanstack/react-query";

import { get } from "@/lib/api";
import { useAuthStore } from "@/store/auth.store";
import type { SuperStatus } from "@/types";

/**
 * useSuperStatus returns the caller's *effective* super-admin status from the
 * backend (claim + allowed-IP gate), which is what tenant-management endpoints
 * actually enforce. Drive super-admin-only UI off `active`, not off the raw
 * `user.is_super_admin` claim — otherwise a tenant admin whose token carries the
 * claim but who isn't on an allowed IP gets offered actions the API then 403s.
 */
export function useSuperStatus() {
  const accessToken = useAuthStore((s) => s.accessToken);
  return useQuery({
    queryKey: ["super-status"],
    queryFn: () => get<SuperStatus>("/super-admin/status"),
    enabled: !!accessToken,
    staleTime: 5 * 60 * 1000,
  });
}
