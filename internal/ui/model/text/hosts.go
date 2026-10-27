package text

func hostsText() []entry {
	return []entry{
		{WizardStepHosts, "Machines"},
		{WizardExplainHosts, "Which machines use this instance. Other machines ignore it and never warn about its directory."},
		{WizardHostsKeep, "Keep the current machines (%s)"},
		{WizardHostsHere, "This machine only (%s)"},
		{WizardHostsAll, "All machines"},
		{WizardHostsPick, "Choose machines…"},
		{WizardHostsPickHint, "Host names separated by commas, for example mac, runner02."},
		{WizardHostsKnown, "Machines named in kitbag.toml: %s"},
		{AgentsScopeSchema1, "%s; choosing machines needs manifest schema 2"},
		{AgentInvalidHosts, "Enter at least one host name."},
		{AgentInvalidHost, "%s is not a valid host name: use letters, digits, dots, underscores or hyphens."},
		{AgentInvalidHostTwice, "%s is listed twice."},
		{AgentWarnUnknownHost, "%s is neither this machine nor named in kitbag.toml; check its spelling."},
		{UpgradeTitle, "Upgrade kitbag.toml to schema 2"},
		{UpgradeExplain, "Schema 2 lets each agent instance name the machines that use it. Review the changes; nothing is written until you upgrade."},
		{UpgradeLoading, "Preparing the upgrade preview…"},
		{UpgradeApplying, "Upgrading and publishing…"},
		{UpgradeChanges, "What changes:"},
		{UpgradeChecked, "Checked: %s keep the same agent instances."},
		{UpgradeCurrent, "kitbag.toml already uses schema 2; nothing to do."},
		{UpgradeApply, "Upgrade and publish"},
		{UpgradeDone, "kitbag.toml uses schema 2 now; you can choose machines."},
		{ActionUpgradeManifest, "Upgrade the manifest"},
	}
}
