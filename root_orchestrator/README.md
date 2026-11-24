# Root-components

By design, the centralized Root Orchestrator contains

- a System Manager for user interaction with the system
- a Database (mongodb) to store data about the participating clusters
- a Scheduler which calculates task-to-cluster affinity
- some Monitoring solution (first prototype shows read-only Grafana Dashboards which collect metrics from Prometheus instances on each cluster orchestrator)

During implementation, some components could be changed/renamed/replaced/merged.

## Usage

Export the environment variables with the public ip/URL where the root orchestrator will be exposed.

```
export SYSTEM_MANAGER_URL=<IP ADDRESS OF THE NODE HOSTING THE ROOT ORCHESTRATOR>
```

Then set the docker-compose.yml with `docker-compose -f docker-compose.yml up --build` to start the root components.

## Shared Library Dependency

The shared python libraries (`resource_abstractor_client` and `oakestra_utils_library`) are **always built from this repo's local `libraries/` folder** — nothing is fetched from a remote git repo. The Docker build wires them in via a named build context declared in `docker-compose.yml` (`additional_contexts: libraries=../libraries`), so `docker compose up --build` just works.

To test local changes to a library, edit the code under `libraries/` and rebuild the image (`docker compose up --build`) — no branch pushing required. This requires Docker BuildKit/Buildx (the default on modern Docker).

## Customize deployment

It's possible to use the docker override functionality to exclude or customize the root orchestrator deployment.

E.g.: Do not deploy the dashboard:

`docker-compose -f docker-compose.yml -f override-no-dashboard.yml up --build`

E.g.: Exclude network component:

`docker-compose -f docker-compose.yml -f override-no-network.yml up --build`

E.g.: Customize network component version

- open and edit `override-custom-serivce-manager.yml` with the correct container image
- run the orchestrator with the override file: `docker-compose -f docker-compose.yml -f override-custom-service-manager.yml up --build`

E.g.: Use local development network component

In case you want to use changes made to the root network component in your deployment,
you can use the `override-local-service-manager.yml` override file.

- copy the `oakestra-net/root-service-manager/service-manager` folder to the `root_orchestrator` directory
- run the orchestrator with the override file: `docker-compose -f docker-compose.yml -f override-local-service-manager.yml up --build`

E.g.: Enable IPv6 for container deployments

Usage: `docker-compose -f docker-compose.yml -f override-ipv6-enabled.yml`

This override sets up a bridged docker network, assigning each container a static IPv4+IPv6 address.
Note that the IP protocol version used for connection establishment using hostname resolution depends on the implementation.
Example: IPv6 server receiving IPv4 request -> source address is in 4-to-6 mapped format (http://mars.tekkom.dk/w/index.php/IPv4-Mapped_IPv6_Address)

E.g.: Disable [observability stack](../root_orchestrator/config/README.md)

Usage: `docker-compose -f docker-compose.yml -f override-no-observe.yml`

E.g.: OIDC login only (federated login)

Users sign in through an OpenID Connect provider (Oakverse, Keycloak, ...) instead of an Oakestra password. Off by default; without this override nothing changes.

```
export OIDC_ISSUER=https://idp.example.org/realms/lab
export OIDC_CLIENT_ID=oakestra-lab
export OIDC_CLIENT_SECRET=<client secret>
export OIDC_REDIRECT_URIS=http://192.168.1.10:10000/api/auth/oidc/callback
export OIDC_ROLES_CLAIM=resource_access.oakestra-lab.roles   # where the roles are in the ID token
docker compose -f docker-compose.yml -f override-federated-login.yml up -d
```

The system manager is a confidential OIDC client (Authorization Code + PKCE S256, state, nonce). It validates the provider's ID token (signature via the provider's JWKS, `iss`, `aud`, `exp`, `nonce`) and issues normal Oakestra tokens with the claims `user`, `roles`, `organization`, `federated: true` and `sid`.

| Variable | Meaning | Default |
|---|---|---|
| `OIDC_ISSUER` | Provider issuer URL; setting it enables OIDC login | — |
| `OIDC_CLIENT_ID`, `OIDC_CLIENT_SECRET` | This cluster's client at the provider | required |
| `OIDC_REDIRECT_URIS` | Comma list of callback URLs registered at the provider; the one whose host matches the request is used, else the first | required |
| `OIDC_SCOPES` | Requested scopes (`offline_access` gives the refresh token used for renewal) | `openid profile email offline_access` |
| `OIDC_USERNAME_CLAIM` | Claim used for the username at first login (fallback `email`, then `sub`) | `preferred_username` |
| `OIDC_ROLES_CLAIM` | Dot path to the roles list in the ID token (or userinfo) | `roles` |
| `OIDC_ROLE_MAP` | Optional `provider:OakestraRole,...` renames | — |
| `OIDC_PROVIDER_NAME` | Label of the login button | `OIDC` |
| `OIDC_ALLOWED_RETURN_HOSTS` | Extra hosts allowed for `return_to` (hosts of `OIDC_REDIRECT_URIS` are always allowed, any port) | — |
| `OIDC_SIGNOUT_FEED_URL` | Optional sign-out feed polled every 60 s | — |
| `OIDC_TOKEN_ENCRYPTION_KEY` | Key for the provider refresh tokens stored in Mongo | derived from `OIDC_CLIENT_SECRET` |
| `FEDERATED_SESSION_MINUTES` | Lifetime of Oakestra access tokens of OIDC sessions | 15 |
| `FEDERATED_OUTAGE_GRACE_HOURS` | How long renewal keeps working while the provider is unreachable | 24 |

- **Endpoints:** `GET /api/auth/methods` (`{internal, oidc, provider_name, user_management}`), `GET /api/auth/oidc/login?return_to=<dashboard URL>` → provider → `GET /api/auth/oidc/callback` → `<return_to>#oidc_code=<one-time code>` (or `#oidc_error=access_denied|user_conflict|provider_error|invalid_request`), `POST /api/auth/oidc/exchange {"code"}` → `{token, refresh_token}`. CLI: `POST /api/auth/oidc/device` and `POST /api/auth/oidc/device/token {"device_code"}` proxy the provider's device flow.
- **Users** are keyed on the ID token's `iss`+`sub`. The username is set once at first login and never changes (applications belong to it); if another provider user already has it, a short suffix is added. A provider user can never take over a local account (`user_conflict`).
- **Roles** come from `OIDC_ROLES_CLAIM` (after `OIDC_ROLE_MAP`); only `Admin`, `Organization_Admin`, `Application_Provider`, `Infrastructure_Provider` survive. No role = no access. Every login and renewal *replaces* the user's roles in the root organization.
- **Renewal** (`POST /api/auth/refresh`) uses the provider refresh token (stored AES-GCM encrypted) and re-reads the roles. The provider refusing (or no roles left) → 401. Provider unreachable → the current roles are kept for up to `FEDERATED_OUTAGE_GRACE_HOURS` since the provider last confirmed the user, then 401.
- **Locked management:** with `INTERNAL_LOGIN=false` password login, registration, password changes/resets and every user/role/organization write return 403 (`Users and roles are managed by your identity provider`); reads keep working.
- **Sign-out:** `POST /api/users/<username>/signout` and `POST /api/auth/signout-all` (Admin) reject all tokens issued before and drop stored provider tokens.
- **Emergency access** when the provider is down: `docker exec system_manager python -m users.emergency --minutes 60` prints `{"code": ...}`; open `http://<dashboard>/#oidc_code=<code>` (or exchange it at `/api/auth/oidc/exchange`) within 10 minutes to get a non-renewable Admin session for the local user `emergency-admin` (`oak cluster emergency-login` does this for you).
- The JWT generator port (10011) is no longer published on the host (requires Docker Compose >= 2.24.4). It does not apply with `override-network-host.yml`, where every container listens on the host directly.

Keycloak example (realm `lab`, root at `192.168.1.10`):

1. Clients → Create client `oakestra-lab`: OpenID Connect, *Client authentication* on, *Standard flow* and *OAuth 2.0 Device Authorization Grant* on. Valid redirect URI `http://192.168.1.10:10000/api/auth/oidc/callback`. Copy the secret from *Credentials*.
2. Client `oakestra-lab` → Roles: create `Admin`, `Application_Provider`, ... (or any names plus `OIDC_ROLE_MAP`) and assign them to users or groups.
3. Client scopes → `roles` → Mappers → `client roles`: turn on *Add to ID token* (Keycloak puts client roles only in the access token by default).
4. Export `OIDC_ISSUER=http://<keycloak>:8080/realms/lab`, `OIDC_CLIENT_ID=oakestra-lab`, `OIDC_CLIENT_SECRET=...`, `OIDC_REDIRECT_URIS=...`, `OIDC_ROLES_CLAIM=resource_access.oakestra-lab.roles`, `OIDC_PROVIDER_NAME=Keycloak` and start the root with the override.

With Oakverse, `oak install root --setup <id>` registers the client and exports all of this for you.
