# Addon Engine


## Start

Run the service by running `./start-up.sh` or go to root-orchestrator folder and execute:
```bash
docker compose -f docker-compose.yml -f override-addons.yml up addons_manager addons_monitor addons_marketplace
```

## Addons with a dashboard UI

An addon can add pages to the Oakestra dashboard. The dashboard loads them at runtime as a
Module Federation remote. To ship a UI, add a `ui` block to the marketplace definition:

```json
{
  "name": "insights",
  "description": "Cluster insights for the dashboard.",
  "services": [
    { "service_name": "insights-ui", "image": "ghcr.io/your-org/addon-insights-ui:0.1.0" }
  ],
  "ui": { "service": "insights-ui", "sdk": "^0.1.0" }
}
```

`ui.service` is the `service_name` of the service that serves the UI and has to be one of the
addon's services. `ui.sdk` is the semver range of the dashboard SDK the UI was built against.
Addons with a `ui` block also need a name matching `^[a-z0-9-]+$`, since the dashboard uses it in
URLs and host names. Don't use the name of a built-in dashboard module (`applications`,
`infrastructure`, `users`, `organizations`, `help`): the dashboard ignores such an addon and only
logs a warning in the browser console, even though the addon is `active`.

The UI container has to serve `mf-manifest.json`, `remoteEntry.js` and `assets/` over plain HTTP
on port 80. The monitor attaches it to the `oakestra` network with the alias `addonui-<name>`,
in addition to any networks the service declares. The container name itself doesn't change. The
dashboard's nginx then proxies `/addon-assets/<name>/...` to `http://addonui-<name>:80/...`.
Only the root monitor sets the alias, since the dashboard only talks to the root addons manager.
Two UI addons with the same name would share the alias, so the addons manager answers 409 when
you install one whose name is taken. Disable the old one first.

To find out which UIs to load, the dashboard calls `GET /api/v1/ui-addons` on the addons manager
(without a token). It returns every `active` addon that has a `ui` block:

```json
[{ "name": "insights", "version": "0.1.0", "sdk": "^0.1.0" }]
```

`version` is the tag of the `ui.service` image, or `latest` if the image has no tag.

The dashboard container finds both managers through `ADDONS_MANAGER_ADDRESS` and
`MARKETPLACE_ADDRESS`. Addon UIs don't work with `override-network-host.yml`, because there is
no Docker DNS to resolve `addonui-<name>`.

`override-no-addons.yml` removes the addons engine and sets a dashboard preset (`PRESET_URL`) that
turns off addon discovery and the addon pages. For your own presets, see `docs/addons/presets.md`
in the dashboard repo.
