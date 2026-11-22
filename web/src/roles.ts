import { api } from "./api";
import type { Role } from "./types";

// The role list never changes while the app runs, so it is loaded once at startup.
export let roles: Role[] = [];

export async function loadRoles() {
  roles = await api.roles();
}

export const roleLabel = (key: string) => roles.find((r) => r.key === key)?.label ?? key;
