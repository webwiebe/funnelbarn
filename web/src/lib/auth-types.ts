// Auth response shapes shared with api.ts. Kept in their own module so
// api.ts stays under its line-count cap (mirrors iambarn-config.ts).

export interface LogoutResponse {
  status?: string
  /** For OIDC sessions: the IdP end-session URL the browser must follow so
   *  the central IAMBarn session ends too (server-driven logout). */
  logout_url?: string
}
