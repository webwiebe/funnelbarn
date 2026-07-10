// Shape of the `iambarn` slice of /api/v1/client-config, consumed by the hosted
// IAMBarn web components. Kept in its own module so api.ts stays under its
// line-count cap and consumers can import the type without pulling in the API
// client. All fields are optional — the backend omits them when IAMBarn isn't
// configured.
export interface IambarnConfig {
  /** Deep link to IAMBarn's own profile admin (legacy external link). */
  profile_url?: string
  /** IAMBarn issuer base URL — the `server-url` for every hosted component. */
  server_url?: string
  /** Public browser client id for the hosted components. */
  client_id?: string
  /** URL of the IAMBarn web-component bundle to load at runtime. */
  widget_url?: string
  /** Registered post-logout redirect for RP-initiated logout. */
  post_logout_redirect_uri?: string
}
