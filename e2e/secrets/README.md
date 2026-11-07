# E2E secrets

SOPS files for E2E accounts that live outside `deploy/`. Everything under
`deploy/` is copied to the production host on every deploy, so test accounts
stay here. The repo-root `.sops.yaml` rule for `e2e/secrets/*.sops.yaml` uses
the same age recipient as the deploy secrets, and CI decrypts with
`SOPS_AGE_KEY_<ENV>`. No seat decrypts these files.

## production-member.sops.yaml

The FunnelBarn member test account in the production organization, created
by iambarn's "Ensure FunnelBarn e2e member" job (iambarn#449). Keys:
`e2e_member.email` and `e2e_member.password`. The file is the job's artifact,
committed as it arrives (already encrypted to this repo's recipient).

Before the real iambarn run, its dry run logs the organization id the slug
resolves to. Run `oidc-org-match.yml` with that id; the real run goes only on
`match=yes`. A member of another organization is refused at sign-in too,
which would make the smoke below pass without testing the role check.

`member-signin-refusal.yml` (manual) decrypts the account in CI and runs
`e2e/member/member-signin-refusal.spec.ts` against production: the OIDC
callback must answer 403 and start no session.
