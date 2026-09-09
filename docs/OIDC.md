# Single sign-on (OIDC)

valheim-server-ui can authenticate users against one OpenID Connect provider in
addition to (or instead of) local accounts. This page covers how the login works,
every setting, how groups become roles, provider-specific setup, and
troubleshooting. Configuration happens in the UI under
**Settings → Authentication → Single sign-on (OIDC)** (admin only).

## How it works

1. The login page shows a **Continue with <provider name>** button when OIDC is
   enabled. It sends the browser to `/api/v1/auth/oidc/login`.
2. The manager starts an **authorization code flow with PKCE** (S256), a random
   `state` and a `nonce`, kept in a short-lived HttpOnly cookie (10 minutes).
3. The provider authenticates the user and redirects back to
   `https://<base_url>/api/v1/auth/oidc/callback`.
4. The manager exchanges the code, **verifies the ID token** (signature via the
   provider's JWKS, issuer, audience = client id, expiry, nonce), then fetches the
   provider's **UserInfo** endpoint and merges any claims the ID token lacks
   (groups, email, name are often only there). ID-token claims win on conflict;
   a UserInfo failure is not fatal.
5. The identity `(issuer URL, sub)` is looked up. If it is unknown but a local
   account has the **same email** and the provider marks it verified, the SSO
   identity is linked to that account (see "Merging with local accounts").
   Otherwise the first login creates a local user (if auto-create is on).
   Later logins reuse the linked account, even if the username or email changes
   at the provider.
6. Groups from the configured claim are mapped to a role (see below). A normal
   session cookie is issued and the browser lands on the page it started from.

Local login and OIDC coexist. A user created by OIDC has no password
(`has_password: false`) and cannot use the local form unless an admin sets one.

## Merging with local accounts

An existing local account and an SSO login for the same person become one
account automatically:

- On the first SSO login of an identity, the manager looks for a local user whose
  email equals the token's `email` claim (case-insensitive).
- The match is used only when the provider vouches for the address:
  `email_verified` is `true`, or the provider does not send that claim at all
  (Entra ID, for instance). An explicit `email_verified: false` never links; a
  separate account is created instead, so an unverified address registered at
  the IdP cannot take over a local account.
- The link keeps everything on the local account: username, password (local
  login keeps working), role and history. With **Sync roles** on, the role is
  re-evaluated from the groups on that login like any other SSO login.
- The audit log records the merge as `auth.oidc.link`.
- From then on the password is owned by the identity provider: the account's
  own "change password" and the admin "set password" action are refused for any
  SSO-linked account (the existing local password keeps working until the
  provider is the only way in). Host-side break-glass remains
  `valheim-ui admin reset-password`.

If you use several accounts with the same email on purpose, give the local one a
different address before enabling SSO.

## Prerequisites

- `base_url` in `/etc/valheim-ui/config.yaml` must be the exact public URL the
  browser uses (scheme, host, port). The redirect URI is derived from it and
  shown read-only in the settings form. Restart the service after changing it.
- HTTPS in front of the manager (reverse proxy). Providers generally refuse
  plain-http redirect URIs except on localhost.
- Keep at least one **local admin** until SSO is proven. Break-glass on the host:
  `sudo -u valheim /usr/local/bin/valheim-ui admin reset-password --username <admin>`.

## Settings reference

| Setting | Meaning | Default |
|---------|---------|---------|
| Enabled | Show the SSO button and accept OIDC logins. | off |
| Provider name | Label on the login button ("Continue with …"). | `SSO` |
| Issuer URL | The OpenID issuer. Discovery is loaded from `<issuer>/.well-known/openid-configuration`. Must match the `iss` claim exactly (trailing slash included or not, as the provider issues it). | |
| Client ID / Client secret | Confidential client credentials. The secret is write-only: the form shows it blank; leaving it blank on save keeps the stored one. | |
| Scopes | Requested scopes. Add whatever your provider needs to release groups. | `openid profile email groups` |
| Groups claim | Name of the claim holding the user's groups (array of strings; a single string is accepted). | `groups` |
| Role mapping | Group name → role (`viewer`, `operator`, `admin`). Exact, case-sensitive match. When a user is in several mapped groups, the **highest** role wins. | empty |
| Default role | Role for users with no mapped group: `viewer`, `operator`, `admin`, or `deny` (login refused). | `viewer` |
| Auto-create users | Create a local account on first login. When off, only identities already linked to an existing user may log in. | on |
| Sync roles | Re-evaluate the mapping on every login and overwrite the user's role. Turn off if you want to manage roles in the UI after the first login. | on |
| Local login enabled | The username/password form. Can be switched off once SSO works; the CLI remains. | on |

Username for a new account: `preferred_username`, else the part of `email`
before `@`, lower-cased, restricted to `a-z 0-9 . _ -`, de-duplicated with a
numeric suffix. Display name comes from `name`, email from `email`.

Roles: `viewer` sees everything with secrets masked, `operator` runs and
configures instances, `admin` additionally manages users, settings and can
create or delete instances.

Use **Test connection** in the form before saving: it performs discovery on the
issuer and reports the authorization endpoint or the error.

## Provider recipes

Register a **confidential** client with:

- Redirect URI: `https://<base_url>/api/v1/auth/oidc/callback`
- Grant: authorization code, PKCE allowed (S256)
- Scopes: `openid profile email groups` (or your provider's group scope)
- Token endpoint auth: client secret (basic or post; both are handled)

### Authelia

```yaml
identity_providers:
  oidc:
    clients:
      - client_id: valheim-ui
        client_name: Valheim Server UI
        client_secret: '$pbkdf2-sha512$...'      # authelia crypto hash generate pbkdf2
        public: false
        authorization_policy: two_factor
        redirect_uris:
          - https://valheim.example.com/api/v1/auth/oidc/callback
        scopes: [openid, profile, email, groups]
        grant_types: [authorization_code]
        response_types: [code]
        token_endpoint_auth_method: client_secret_basic
```

Authelia puts `groups` in the UserInfo response; the manager reads it from there.
Map Authelia group names (as in your users database or LDAP) to roles.

### Keycloak

1. Clients → Create: type OpenID Connect, client id `valheim-ui`, **Client
   authentication on**, Standard flow on, redirect URI as above.
2. Client scopes → the client's dedicated scope → Add mapper → *By configuration*
   → **Group Membership**: name `groups`, token claim name `groups`, *Full group
   path* **off**, add to ID token and userinfo.
3. Map the group names (without leading `/`) in Role mapping.

### Authentik

1. Create a Provider: OAuth2/OpenID, Authorization Code, client type
   Confidential, redirect URI as above, signing key selected.
2. Scopes: include the built-in `openid`, `email`, `profile` and add a Scope
   mapping that exposes groups, e.g. name `groups`, scope name `groups`,
   expression `return {"groups": [g.name for g in request.user.ak_groups.all()]}`.
3. Create an Application bound to the provider. Issuer URL is
   `https://auth.example.com/application/o/<application-slug>/`.

### Pocket ID

Create an OIDC client with the redirect URI above. Pocket ID exposes `groups` in
the `groups` scope; add that scope in the client and keep the default claim name.

### Google

Google has no groups claim for ordinary accounts. Leave Role mapping empty, set
**Default role** to the role every allowed person should get, and set
**Auto-create users** off after the first logins so only accounts you have
already linked can sign in, or restrict the client to your Workspace domain.
Issuer: `https://accounts.google.com`. Scopes: `openid profile email`.

### Microsoft Entra ID

Issuer: `https://login.microsoftonline.com/<tenant-id>/v2.0`. Add the optional
`groups` claim to ID/access tokens in the app registration (token configuration).
Entra emits group **object IDs**, not names, so map the IDs in Role mapping.

## Recommended rollout

1. Create the OIDC client at the provider, fill in the form, press **Test
   connection**, save with Enabled on.
2. In a private window, click **Continue with …** and confirm the created user
   and its role under **Users**.
3. Set **Default role** to `deny` if only mapped groups may log in.
4. Only then consider switching **Local login enabled** off.

## Troubleshooting

| Symptom | Cause and fix |
|---------|---------------|
| Button missing on the login page | OIDC not enabled, or the settings failed validation (issuer, client id required). |
| Provider error "redirect_uri mismatch" | The URI registered at the provider differs from `<base_url>/api/v1/auth/oidc/callback`. Check `base_url` (scheme, host, port) and restart the service. |
| Back on `/login?error=oidc_error` | Token exchange or ID token verification failed. Common: wrong client secret, clock skew, issuer URL not identical to the token's `iss` (trailing slash), provider signing algorithm not RS256/ES256. Check `journalctl -u valheim-ui` for "oidc login rejected". |
| `/login?error=forbidden` "no role mapped" | Default role is `deny` and the user is in none of the mapped groups, or the groups claim is named differently. Verify the claim name and that the provider releases groups (in the ID token or UserInfo). |
| `/login?error=forbidden` "account does not exist" | Auto-create is off and this identity was never linked. Turn auto-create on for the first login or create the user and log in once with auto-create on. |
| `/login?error=account_disabled` | The local user is disabled under Users. |
| User gets `viewer` although in an admin group | Group name mismatch (case, full path like `/admins`, or object IDs at Entra). Compare with what the provider sends; adjust the mapping or the mapper. |
| Roles do not update after changing groups | Sync roles is off, or the user was edited manually. Turn Sync roles on; the next login applies the mapping. |
| "cross-origin request rejected" after login | `base_url` does not match the host the browser uses. |
| Locked out of everything | `sudo -u valheim /usr/local/bin/valheim-ui admin reset-password --username <admin>` on the host, then log in locally (the CLI works even when local login is disabled in settings only if you re-enable it; use `admin create-user --role admin` and then edit settings via the UI). |

## Security notes

- Client secret is stored in the manager database (`/var/lib/valheim/manager.db`,
  readable only by the `valheim` user) and never returned by the API.
- State, nonce and PKCE verifier are bound to the browser through an HttpOnly,
  SameSite=Lax cookie; a callback without a matching cookie is rejected.
- Only the ID token is trusted for identity; UserInfo supplements claims and is
  ignored when its subject differs.
- Every login and role change is recorded in the audit log.
