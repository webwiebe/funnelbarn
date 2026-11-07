package auth

import oidcv3 "github.com/coreos/go-oidc/v3/oidc"

// requestedScopes is the scope list sent on every authorize request.
//
// offline_access asks iambarn for a refresh_token alongside the short-lived
// (15m) access_token, so the session can be renewed silently instead of
// forcing a full re-login every 15 minutes. iambarn only grants what is both
// allowed on the client and explicitly requested.
//
// groups and roles carry the claims the sign-in check (Allowed) and the admin
// routes read. iambarn emits them without the scope today, and stops doing so
// once IAM_STRICT_CLAIM_SCOPES is on. iambarn rejects a requested scope the
// client has not registered (offline_access excepted), so the deploy step in
// deploy/iambarn/register-mcp-resource.sh checks REQUIRED_SCOPES, which must
// list every scope here except offline_access.
func requestedScopes() []string {
	return []string{oidcv3.ScopeOpenID, "profile", "email", "offline_access", "groups", "roles"}
}
