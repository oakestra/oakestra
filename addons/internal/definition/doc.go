// Package definition holds the addon definition format shared by the marketplace, the addons
// manager and addonctl: the types, the index file, name rules, structural validation and the
// permission policy.
//
// Two checks exist and callers almost always want both:
//
//   - Validate checks that a definition is well formed (names, versions, image references,
//     mount syntax, references between services, volumes and networks).
//   - CheckPolicy checks that every risky capability the definition would get at run time is
//     declared in its permissions block, and rejects capabilities that can't be declared at all.
//
// Check runs both and returns a single error. The Mode says where the definition comes from:
// ModeIndex for official and tap catalog entries, ModeLocal for local registrations (tags
// allowed), ModeInstall for anything about to run (any source, digests required).
//
// Host path grants cover exactly the path they name. Docker follows symlinks in bind sources on
// the host, so any path below a writable bind source could be swapped for a symlink to / or the
// docker socket; for that reason CheckPolicy also forbids an addon from mounting a path below
// one of its own read-write bind sources.
//
// The policy only knows about the fields that exist on Service, Volume and Network. Decoding is
// strict (unknown fields, duplicate keys and keys spelled differently from their JSON tag are
// rejected), so a definition can't smuggle in fields like "privileged" or "cap_add" that a
// runtime might pass through, nor show a reviewer one value while decoding to another. Any
// runtime field added to these types needs a matching rule in policy.go, otherwise it bypasses
// the permission model.
//
// # Runtime contract
//
// The policy only holds if the runtime (cmd/addons) does its part:
//
//   - The installer and the reconcile loop check with ModeInstall, not ModeIndex: ModeIndex
//     rejects local registrations, ModeInstall accepts every source but still requires digests.
//   - Re-run Check on the stored definition right before creating containers, so a tampered
//     database record can't escalate.
//   - Refuse to start an addon whose HostBindSources conflict, per OverlapConflict, with those
//     of any other addon that isn't disabled. Otherwise one addon can plant a symlink in a
//     directory another addon mounts, which CheckPolicy can't see because it looks at one
//     definition at a time.
//   - On an update, refuse the new version if UpgradeConflict(old, new) reports a conflict:
//     files the old version wrote stay on the host, so a symlink it planted below its own
//     read-write path would redirect the new version's mount. Permissions.Grows does report the
//     new path, but the admin has no way to tell a planted symlink from a real directory.
//   - Reject images whose own labels (from the image config) match IsReservedLabel. Image
//     labels end up on the container without passing through the definition.
//   - Name containers ContainerName(ID, service_name), never from free-form definition fields.
//   - Create declared volumes and networks as VolumeName/NetworkName(ID, name) and rewrite the
//     service references to match, so addons can't attach core or other addons' resources.
//   - A service with no networks gets an addon-private default network (e.g.
//     NetworkName(ID, "default")), not CoreNetwork. Only services that list CoreNetwork, which
//     requires permissions.core_network, join it.
//   - Attach the ui.service to UINetwork with the alias UIAlias(ID), in addition to its own
//     networks. Definitions can't reference UINetwork themselves; the dashboard joins it via
//     compose.
package definition
