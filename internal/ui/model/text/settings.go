package text

func settingDescriptions() []entry {
	return []entry{
		{SettingDescription("schema"), "Version of the machine settings file, managed by KitBag."},
		{SettingDescription("host"), "Choose this machine's instructions from hosts/*.md, or enter another host name."},
		{SettingDescription("repo.url"), "Git address of your shared configuration. Do not include passwords or tokens."},
		{SettingDescription("repo.path"), "Where the working copy lives under your home directory. Use a path starting with ~/."},
		{SettingDescription("repo.branch"), "Branch to sync. Remove this setting to follow the remote's default branch."},
		{SettingDescription("git.backend"), "Auto uses installed Git when available. Choose system or embedded to require one implementation."},
		{SettingDescription("git.identity.name"), "Commit author name when embedded Git has no name in Git config. Set name and email together using kitbag config edit."},
		{SettingDescription("git.identity.email"), "Commit author email when embedded Git has no email in Git config. Set name and email together using kitbag config edit."},
		{SettingDescription("sync.poll_interval"), "How often the daemon checks Git for changes. Restart the daemon after saving. Custom: 1s to 24h."},
		{SettingDescription("daemon.debounce"), "How long the daemon waits to combine file changes into one sync. Restart after saving. Custom: 100ms to 1m."},
		{SettingDescription("daemon.update_check"), "Check once a day for a new KitBag release. Nothing is downloaded or installed. Restart the daemon after saving."},
		{SettingDescription("daemon.release_url"), "Release metadata address used by update checks and tests."},
		{SettingDescription("sync.mode"), "Link keeps files connected to the working copy. Copy writes separate files. Files marked copy in kitbag.toml always use copies."},
		{SettingDescription("ui.notifications"), "Reserved for future desktop notifications; currently has no effect."},
		{SettingDescription("ui.theme"), "Reserved for a future desktop theme; currently has no effect."},
		{SettingDescription("ui.color"), "Auto follows the terminal. Never disables styling; always enables CLI colour. NO_COLOR takes priority. Reopen the TUI after saving."},
		{SettingDescription("migration.legacy_roots"), "Folders from your old setup that KitBag may adopt. They are never guessed or added automatically."},
	}
}
